package services

import (
	"fmt"
	"log"
	"os"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/utils"
)

// NotifyAdminNewEscalation emails the Meskeny admin inbox when a specialist is requested.
func NotifyAdminNewEscalation(row *models.AIEscalation, user *models.User) {
	if row == nil {
		return
	}
	go func() {
		adminEmail := strings.TrimSpace(os.Getenv("ADMIN_NOTIFY_EMAIL"))
		if adminEmail == "" {
			return
		}
		if !EmailConfigured() {
			log.Printf("ℹ️ escalation notify: Gmail SMTP not configured")
			return
		}
		subject := fmt.Sprintf("Meskeny · Specialist request #%d (%s)", row.ID, strings.ToUpper(row.Urgency))
		html := BuildEscalationAdminEmailHTML(row, user)
		if ok, err := utils.SendMail(adminEmail, subject, html); err != nil {
			log.Printf("⚠️ escalation notify email failed (#%d): %v", row.ID, err)
		} else if ok {
			log.Printf("📧 escalation notify sent to admin (#%d)", row.ID)
		}
	}()
}

func BuildEscalationAdminEmailHTML(row *models.AIEscalation, user *models.User) string {
	name := strings.TrimSpace(row.GuestName)
	email := strings.TrimSpace(row.GuestEmail)
	phone := strings.TrimSpace(row.GuestPhone)
	if user != nil && user.ID > 0 {
		if name == "" {
			name = strings.TrimSpace(user.FirstName + " " + user.LastName)
		}
		if email == "" {
			email = strings.TrimSpace(user.Email)
		}
		if phone == "" && user.PhoneNumber != nil {
			phone = strings.TrimSpace(*user.PhoneNumber)
		}
	}
	if name == "" {
		name = "Guest (not signed in)"
	}
	if email == "" {
		email = "—"
	}
	if phone == "" {
		phone = "—"
	}
	ctx := strings.TrimSpace(row.ContextSummary)
	if ctx == "" {
		ctx = "—"
	}
	reason := strings.TrimSpace(row.Reason)
	if reason == "" {
		reason = "User requested a specialist"
	}

	return fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:620px;color:#111">
<h2 style="margin:0 0 12px">New specialist request</h2>
<p style="color:#444;margin:0 0 16px">A user asked to speak with a Meskeny specialist from Model X46 chat.</p>
<table style="width:100%%;border-collapse:collapse;font-size:14px">
<tr><td style="padding:8px 0;color:#666">Request ID</td><td style="padding:8px 0"><strong>#%d</strong></td></tr>
<tr><td style="padding:8px 0;color:#666">Urgency</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Status</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Session</td><td style="padding:8px 0;font-family:monospace;font-size:12px">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Contact name</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Email</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666">Phone</td><td style="padding:8px 0">%s</td></tr>
<tr><td style="padding:8px 0;color:#666;vertical-align:top">Reason</td><td style="padding:8px 0">%s</td></tr>
</table>
<h3 style="margin:20px 0 8px;font-size:15px">Conversation context</h3>
<pre style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:12px;font-size:12px;white-space:pre-wrap;line-height:1.5">%s</pre>
<p style="margin:20px 0 0;color:#666;font-size:13px">Open NovaDashboard → Specialist requests to follow up.</p>
</div>`,
		row.ID,
		escapeHTML(strings.ToUpper(row.Urgency)),
		escapeHTML(row.Status),
		escapeHTML(row.SessionID),
		escapeHTML(name),
		escapeHTML(email),
		escapeHTML(phone),
		escapeHTML(reason),
		escapeHTML(ctx),
	)
}
