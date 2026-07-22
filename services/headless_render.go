package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// CapturedAPI is one XHR/fetch/GraphQL response intercepted while a page
// rendered — the clean structured data behind JS-driven sites.
type CapturedAPI struct {
	URL          string
	Method       string
	ResourceType string
	Status       int
	ContentType  string
	Body         string
}

// Headless rendering (chromedp + headless Chromium) for JavaScript-rendered
// sites (Next.js/SPA gov portals, etc.) where the server HTML is an empty
// shell. The crawler uses this AUTOMATICALLY: it fetches plainly first, and
// only renders a page when the plain HTML has almost no text — so
// server-rendered sites stay fast and JS sites still get fully captured.
//
// One browser is shared process-wide (launching Chromium is expensive), and
// renders are serialized with a mutex so we never run multiple Chromium
// instances at once — bounded memory, safe on a small VPS.

var (
	browserMu    sync.Mutex
	sharedAlloc  context.Context
	sharedBrowse context.Context
	browserInit  bool
	browserBad   bool // Chromium unavailable — stop trying this process
)

// chromiumExecPath returns the Chromium binary path (env override → common
// Alpine/Debian locations).
func chromiumExecPath() string {
	// Honor CHROME_BIN only if it actually exists (a stale/wrong env value
	// otherwise makes chromedp fail to launch a nonexistent binary).
	if p := os.Getenv("CHROME_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range []string{
		"/usr/bin/chromium",         // modern Alpine
		"/usr/bin/chromium-browser", // older Alpine/Debian
		"/usr/lib/chromium/chrome",
		"/usr/lib/chromium/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chrome",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ensureBrowser lazily launches the shared headless browser. Returns false if
// Chromium isn't available (caller falls back to plain HTML).
func ensureBrowser() bool {
	if browserBad {
		return false
	}
	if browserInit && sharedBrowse != nil {
		return true
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		// Reduce headless detection (navigator.webdriver etc.) — some sites
		// serve an empty shell to obvious bots.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("lang", "fr-FR"),
		chromedp.UserAgent("Mozilla/5.0 (compatible; MeskenyBot/1.0; +https://meskeny.com/bot)"),
	)
	if p := chromiumExecPath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	sharedAlloc, _ = chromedp.NewExecAllocator(context.Background(), opts...)
	sharedBrowse, _ = chromedp.NewContext(sharedAlloc)

	warm, cancel := context.WithTimeout(sharedBrowse, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(warm); err != nil {
		browserBad = true
		sharedBrowse = nil
		return false
	}
	browserInit = true
	return true
}

// renderDocQuiet renders a URL and returns just the parsed rendered DOM.
func renderDocQuiet(rawURL string) *goquery.Document {
	doc, _ := renderCaptured(rawURL)
	return doc
}

// bodyReadyJS returns the visible text length — used to wait for client-side
// content to actually render before we snapshot.
const bodyReadyJS = `(document.body && document.body.innerText) ? document.body.innerText.replace(/\s+/g,' ').trim().length : 0`

// renderCaptured renders a URL with headless Chromium AND intercepts every
// XHR/fetch response the page makes (the JSON APIs behind JS-driven sites),
// returning both the rendered DOM and the captured API responses. Serialized
// process-wide. Returns (nil, nil) if unavailable/failed.
func renderCaptured(rawURL string) (doc *goquery.Document, apis []CapturedAPI) {
	browserMu.Lock()
	defer browserMu.Unlock()
	// A Chromium/chromedp panic must not propagate — return empty (plain fetch
	// already provided a fallback) rather than crash the crawl goroutine.
	defer func() {
		if r := recover(); r != nil {
			browserBad = true // stop trying this process
			doc = nil
		}
	}()

	if !ensureBrowser() {
		return nil, nil
	}
	tabCtx, cancelTab := chromedp.NewContext(sharedBrowse)
	defer cancelTab()
	runCtx, cancelRun := context.WithTimeout(tabCtx, 40*time.Second)
	defer cancelRun()

	type reqMeta struct {
		url, method, rtype, ctype string
		status                    int
	}
	var mu sync.Mutex
	meta := map[network.RequestID]*reqMeta{}
	captured := map[network.RequestID]*CapturedAPI{}

	chromedp.ListenTarget(runCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			mu.Lock()
			m := meta[e.RequestID]
			if m == nil {
				m = &reqMeta{}
				meta[e.RequestID] = m
			}
			if e.Request != nil {
				m.method = e.Request.Method
				if m.url == "" {
					m.url = e.Request.URL
				}
			}
			mu.Unlock()
		case *network.EventResponseReceived:
			if e.Type != network.ResourceTypeXHR && e.Type != network.ResourceTypeFetch {
				return
			}
			mu.Lock()
			m := meta[e.RequestID]
			if m == nil {
				m = &reqMeta{}
				meta[e.RequestID] = m
			}
			m.rtype = string(e.Type)
			if e.Response != nil {
				m.url = e.Response.URL
				m.status = int(e.Response.Status)
				m.ctype = e.Response.MimeType
			}
			mu.Unlock()
		case *network.EventLoadingFinished:
			mu.Lock()
			m := meta[e.RequestID]
			mu.Unlock()
			if m == nil || (m.rtype != string(network.ResourceTypeXHR) && m.rtype != string(network.ResourceTypeFetch)) {
				return
			}
			id := e.RequestID
			snap := *m
			// GetResponseBody must run off the event-loop goroutine, with a
			// cdp executor. Best-effort: the body may already be evicted.
			go func() {
				defer func() { _ = recover() }()
				c := chromedp.FromContext(runCtx)
				if c == nil || c.Target == nil {
					return
				}
				ex := cdp.WithExecutor(runCtx, c.Target)
				body, err := network.GetResponseBody(id).Do(ex)
				if err != nil || len(body) == 0 {
					return
				}
				mu.Lock()
				captured[id] = &CapturedAPI{
					URL: snap.url, Method: snap.method, ResourceType: snap.rtype,
					Status: snap.status, ContentType: snap.ctype, Body: string(body),
				}
				mu.Unlock()
			}()
		}
	})

	var html string
	err := chromedp.Run(runCtx,
		network.Enable(),
		chromedp.Navigate(rawURL),
		// Wait until the client-side content actually renders (poll the body
		// text length) instead of a fixed sleep — SPA data fetches vary.
		chromedp.ActionFunc(func(c context.Context) error {
			deadline := time.Now().Add(16 * time.Second)
			for time.Now().Before(deadline) {
				var n int
				if e := chromedp.Evaluate(bodyReadyJS, &n).Do(c); e == nil && n > 300 {
					return nil
				}
				time.Sleep(700 * time.Millisecond)
			}
			return nil
		}),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	// Let late XHR bodies finish arriving before we tear the tab down.
	time.Sleep(900 * time.Millisecond)

	mu.Lock()
	for _, c := range captured {
		apis = append(apis, *c)
	}
	mu.Unlock()

	if err == nil && strings.TrimSpace(html) != "" {
		if d, e := goquery.NewDocumentFromReader(strings.NewReader(html)); e == nil {
			doc = d
		}
	}
	return doc, apis
}

// headlessAvailable reports whether Chromium can be used (for status messages).
func headlessAvailable() bool {
	browserMu.Lock()
	defer browserMu.Unlock()
	return ensureBrowser()
}

// HeadlessSelfCheck launches Chromium and reports the outcome + the resolved
// binary path — an admin diagnostic. Panic-safe.
func HeadlessSelfCheck() (ok bool, detail string) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			detail = fmt.Sprintf("panic: %v", r)
		}
	}()
	path := chromiumExecPath()
	if path == "" {
		return false, "no Chromium binary found (CHROME_BIN unset and no known path)"
	}
	if headlessAvailable() {
		return true, "Chromium OK at " + path
	}
	return false, "Chromium present at " + path + " but failed to launch"
}
