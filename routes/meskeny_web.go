package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/kataras/iris/v12"
)

// meskeny.com public web endpoints: the Universal/App-Link association files and
// a per-listing Open Graph page. The OG page lets WhatsApp/social show the
// listing photo + title (their crawlers ignore JavaScript, so the tags must be
// server-rendered), and on a device with the app installed the Universal Link
// opens the app directly before this page is even fetched.

const meskenyOrigin = "https://meskeny.com"

// Keep in sync with app.json (bundle id) + the AASA/assetlinks in the app repo.
const (
	appleAppID          = "GFH6U59F83.com.jeremypersing.apartmentsclone"
	androidPackage      = "com.jeremypersing.apartmentsclone"
	androidSHA256Cert   = "22:69:AA:3A:CA:B1:0C:3E:5B:E1:A1:4C:1C:11:8D:50:C1:B0:1F:B5:46:D9:F1:66:80:3C:33:01:D4:9F:42:B1"
	appCustomSchemePfx  = "com.jeremypersing.apartmentsclone://"
	ogFallbackImage     = meskenyOrigin + "/og-default.jpg"
	ogFallbackTitleText = "Meskeny"
)

// GET /.well-known/apple-app-site-association
func ServeAppleAppSiteAssociation(ctx iris.Context) {
	ctx.ContentType("application/json")
	ctx.Header("Cache-Control", "public, max-age=3600")
	_ = ctx.JSON(iris.Map{
		"applinks": iris.Map{
			"details": []iris.Map{
				{
					"appIDs": []string{appleAppID},
					"components": []iris.Map{
						{"/": "/property-sale/*"},
						{"/": "/property/*"},
						{"/": "/land/*"},
					},
				},
			},
		},
		"webcredentials": iris.Map{"apps": []string{appleAppID}},
	})
}

// GET /.well-known/assetlinks.json
func ServeAssetLinks(ctx iris.Context) {
	ctx.ContentType("application/json")
	ctx.Header("Cache-Control", "public, max-age=3600")
	_ = ctx.JSON([]iris.Map{
		{
			"relation": []string{"delegate_permission/common.handle_all_urls"},
			"target": iris.Map{
				"namespace":                "android_app",
				"package_name":             androidPackage,
				"sha256_cert_fingerprints": []string{androidSHA256Cert},
			},
		},
	})
}

// leadingUint pulls the numeric id out of a slugged segment ("56-villa" -> 56).
func leadingUint(s string) uint {
	var n uint
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint(c-'0')
	}
	return n
}

func firstNonEmpty(imgs []string) string {
	for _, u := range imgs {
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

func imagesFromJSONString(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// renderOG writes the Open Graph landing page. path is the meskeny.com path so
// the Universal Link matches; the custom scheme is the in-app fallback link.
func renderOG(ctx iris.Context, title, desc, image, path string) {
	if strings.TrimSpace(title) == "" {
		title = ogFallbackTitleText
	}
	if strings.TrimSpace(image) == "" {
		image = ogFallbackImage
	}
	esc := html.EscapeString
	httpsURL := meskenyOrigin + path
	appURL := appCustomSchemePfx + strings.TrimPrefix(path, "/")

	ctx.ContentType("text/html; charset=utf-8")
	ctx.Header("Cache-Control", "public, max-age=300")
	_, _ = ctx.WriteString(fmt.Sprintf(`<!doctype html>
<html lang="ar" dir="rtl"><head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>%s · Meskeny</title>
<meta property="og:site_name" content="Meskeny"/>
<meta property="og:type" content="website"/>
<meta property="og:title" content="%s"/>
<meta property="og:description" content="%s"/>
<meta property="og:image" content="%s"/>
<meta property="og:url" content="%s"/>
<meta name="twitter:card" content="summary_large_image"/>
<style>
  body{margin:0;font-family:-apple-system,Segoe UI,Roboto,sans-serif;background:#FAEFE9;color:#1A1A1A;display:flex;min-height:100vh;align-items:center;justify-content:center}
  .card{max-width:420px;width:92%%;background:#fff;border-radius:20px;overflow:hidden;box-shadow:0 12px 40px rgba(0,0,0,.12)}
  .photo{width:100%%;height:230px;object-fit:cover;background:#eee;display:block}
  .body{padding:20px}
  h1{font-size:19px;margin:0 0 6px}
  p{color:#8a8a8a;margin:0 0 18px;font-size:14px}
  a.btn{display:block;text-align:center;background:#D16024;color:#fff;text-decoration:none;padding:14px;border-radius:12px;font-weight:700}
</style>
</head><body>
  <div class="card">
    <img class="photo" src="%s" alt=""/>
    <div class="body">
      <h1>%s</h1>
      <p>%s</p>
      <a class="btn" href="%s">Open in the Meskeny app</a>
    </div>
  </div>
  <script>
    // Try to hand off to the installed app; the Universal Link usually does this
    // before the page even loads, so this is just a fallback.
    setTimeout(function(){ window.location.href = %q; }, 350);
  </script>
</body></html>`,
		esc(title), esc(title), esc(desc), esc(image), esc(httpsURL),
		esc(image), esc(title), esc(desc), esc(httpsURL), appURL))
}

// GET /property-sale/{slug}
func ServePropertySaleOG(ctx iris.Context) {
	id := leadingUint(ctx.Params().Get("slug"))
	var p models.PropertySale
	desc := ""
	title := ""
	image := ""
	if id > 0 && storage.DB.Select("id", "title", "city", "images").First(&p, id).Error == nil {
		title = p.Title
		image = firstNonEmpty(p.Images)
		if p.City != "" {
			desc = "For sale · " + p.City
		}
	}
	renderOG(ctx, title, desc, image, "/property-sale/"+ctx.Params().Get("slug"))
}

// GET /property/{slug}  (rental)
func ServeRentalOG(ctx iris.Context) {
	id := leadingUint(ctx.Params().Get("slug"))
	var p models.Property
	title := ""
	image := ""
	desc := ""
	if id > 0 && storage.DB.Select("id", "title", "city", "images").First(&p, id).Error == nil {
		title = p.Title
		image = firstNonEmpty(imagesFromJSONString(p.Images))
		if p.City != "" {
			desc = "For rent · " + p.City
		}
	}
	renderOG(ctx, title, desc, image, "/property/"+ctx.Params().Get("slug"))
}

// GET /land/{slug}
func ServeLandmarkOG(ctx iris.Context) {
	id := leadingUint(ctx.Params().Get("slug"))
	var l models.Landmark
	title := ""
	image := ""
	desc := ""
	if id > 0 && storage.DB.Select("id", "title", "district", "region", "images").First(&l, id).Error == nil {
		title = l.Title
		image = firstLandmarkImageURL(l)
		loc := strings.TrimSpace(strings.TrimSpace(l.District + " " + l.Region))
		if loc != "" {
			desc = "Land · " + loc
		}
	}
	renderOG(ctx, title, desc, image, "/land/"+ctx.Params().Get("slug"))
}
