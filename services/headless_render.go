package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

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
	if p := os.Getenv("CHROME_BIN"); p != "" {
		return p
	}
	for _, p := range []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
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

// renderDocQuiet renders a URL with headless Chromium and returns the parsed
// rendered DOM. Serialized process-wide. Returns nil if unavailable/failed.
func renderDocQuiet(rawURL string) (doc *goquery.Document) {
	browserMu.Lock()
	defer browserMu.Unlock()
	// A Chromium/chromedp panic must not propagate — return nil (plain fetch
	// already provided a fallback) rather than crash the crawl goroutine.
	defer func() {
		if r := recover(); r != nil {
			browserBad = true // stop trying this process
			doc = nil
		}
	}()

	if !ensureBrowser() {
		return nil
	}
	tabCtx, cancelTab := chromedp.NewContext(sharedBrowse)
	defer cancelTab()
	runCtx, cancelRun := context.WithTimeout(tabCtx, 25*time.Second)
	defer cancelRun()

	var html string
	err := chromedp.Run(runCtx,
		chromedp.Navigate(rawURL),
		chromedp.Sleep(2800*time.Millisecond), // let client-side JS render
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	if err != nil || strings.TrimSpace(html) == "" {
		return nil
	}
	doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	return doc
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
