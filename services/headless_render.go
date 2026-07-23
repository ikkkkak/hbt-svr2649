package services

import "github.com/PuerkitoBio/goquery"

// Headless Chromium was REMOVED (it exhausted the VM's RAM/disk and blew up
// Docker build time). The crawler now relies on plain HTTP fetch, and
// JavaScript-rendered sites are handled by pasting their JSON via the admin
// "Paste external JSON" tool instead of in-server browser rendering.
//
// These stubs keep the existing call sites compiling while disabling all
// browser rendering. No chromedp/Chromium dependency remains.

// CapturedAPI is one intercepted XHR/fetch response. Retained as a type so
// callers still compile; nothing is captured now that rendering is disabled.
type CapturedAPI struct {
	URL          string
	Method       string
	ResourceType string
	Status       int
	ContentType  string
	Body         string
}

// renderDocQuiet previously rendered a page in headless Chromium; now a no-op.
func renderDocQuiet(_ string) *goquery.Document { return nil }

// renderCaptured previously rendered + intercepted APIs; now a no-op.
func renderCaptured(_ string) (*goquery.Document, []CapturedAPI) { return nil, nil }

// headlessAvailable now always reports false — no browser is bundled.
func headlessAvailable() bool { return false }

// HeadlessSelfCheck reports that headless rendering is intentionally disabled.
func HeadlessSelfCheck() (ok bool, detail string) {
	return false, "headless rendering is disabled (Chromium removed to keep the server lean; use Paste external JSON for JS-rendered sites)"
}
