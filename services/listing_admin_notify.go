package services

import (
	"fmt"
	"log"
	"os"
	"strings"

	"apartments-clone-server/utils"
)

// ListingKind identifies which listing pipeline created a row.
type ListingKind string

const (
	ListingKindPropertySale ListingKind = "property_sale"
	ListingKindRent         ListingKind = "rent"
	ListingKindLand         ListingKind = "land"
)

// ListingAdminNotifyInput is the payload emailed to the Meskeny admin inbox.
type ListingAdminNotifyInput struct {
	Kind        ListingKind
	ID          uint
	Title       string
	City        string
	Zone        string
	Price       float64
	Currency    string
	PropertyType string
	HostUserID  uint
	HostEmail   string
	Status      string
}

// EmailConfigured reports whether Gmail SMTP is ready to send mail.
func EmailConfigured() bool {
	return utils.EmailConfigured()
}

// SendAdminListingEmail sends a listing notification email synchronously.
func SendAdminListingEmail(to string, in ListingAdminNotifyInput, test bool) (bool, error) {
	if !EmailConfigured() {
		return false, fmt.Errorf("Gmail SMTP is not configured — set EMAIL_FROM and GMAIL_APP_PASSWORD")
	}
	subject := fmt.Sprintf("Meskeny · New %s listing #%d", kindLabel(in.Kind), in.ID)
	if test {
		subject = "[TEST] " + subject
	}
	html := BuildListingAdminEmailHTML(in)
	return utils.SendMail(to, subject, html)
}

// NotifyAdminNewListing emails the admin when a new listing is created (async).
func NotifyAdminNewListing(in ListingAdminNotifyInput) {
	go func() {
		adminEmail := strings.TrimSpace(os.Getenv("ADMIN_NOTIFY_EMAIL"))
		if adminEmail == "" {
			return
		}
		if !EmailConfigured() {
			log.Printf("ℹ️ listing notify: Gmail SMTP not configured — set EMAIL_FROM and GMAIL_APP_PASSWORD")
			return
		}

		if ok, err := SendAdminListingEmail(adminEmail, in, false); err != nil {
			log.Printf("⚠️ listing notify email failed (%s #%d): %v", in.Kind, in.ID, err)
		} else if ok {
			log.Printf("📧 listing notify sent to admin (%s #%d)", in.Kind, in.ID)
		}
	}()
}

func kindLabel(k ListingKind) string {
	switch k {
	case ListingKindRent:
		return "rent"
	case ListingKindLand:
		return "land for sale"
	default:
		return "property for sale"
	}
}

// BuildListingAdminEmailHTML renders the admin inbox HTML for a listing notification.
func BuildListingAdminEmailHTML(in ListingAdminNotifyInput) string {
	priceLine := "—"
	if in.Price > 0 {
		cur := strings.TrimSpace(in.Currency)
		if cur == "" {
			cur = "MRU"
		}
		priceLine = fmt.Sprintf("%.0f %s", in.Price, cur)
	}
	loc := strings.TrimSpace(in.City)
	if z := strings.TrimSpace(in.Zone); z != "" {
		if loc != "" {
			loc = loc + ", " + z
		} else {
			loc = z
		}
	}
	if loc == "" {
		loc = "—"
	}

	return fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:560px;color:#111">
<h2 style="margin:0 0 12px">New listing on Meskeny</h2>
<p style="color:#444;margin:0 0 16px">A new <strong>%s</strong> was submitted and is awaiting your review in the admin panel.</p>
<table style="width:100%%;border-collapse:collapse;font-size:14px">
<tr><td style="padding:8px 0;color:#666">Listing ID</td><td style="padding:8px 0"><strong>#%d</strong></td></tr>
<tr><td style="padding:8px 0;color:#666">Title</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Type</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Location</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Price</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Status</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Host user</td><td style="padding:8px 0">#%d %s</td></tr>
</table>
<p style="margin:20px 0 0;color:#666;font-size:13px">Open the Meskeny admin moderation queue to approve or reject this listing.</p>
</div>`,
		kindLabel(in.Kind),
		in.ID,
		escapeHTML(in.Title),
		escapeHTML(strings.TrimSpace(in.PropertyType)),
		escapeHTML(loc),
		escapeHTML(priceLine),
		escapeHTML(strings.TrimSpace(in.Status)),
		in.HostUserID,
		escapeHTML(strings.TrimSpace(in.HostEmail)),
	)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
