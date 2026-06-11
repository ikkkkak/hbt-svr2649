package utils

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

func gmailUser() string {
	if u := strings.TrimSpace(os.Getenv("GMAIL_USER")); u != "" {
		return u
	}
	return strings.TrimSpace(os.Getenv("EMAIL_FROM"))
}

func gmailAppPassword() string {
	return strings.TrimSpace(os.Getenv("GMAIL_APP_PASSWORD"))
}

// EmailConfigured reports whether Gmail SMTP credentials are set.
func EmailConfigured() bool {
	return gmailUser() != "" && gmailAppPassword() != ""
}

func SendMail(userEmail string, subject string, html string) (bool, error) {
	from := gmailUser()
	password := gmailAppPassword()
	if from == "" || password == "" {
		return false, fmt.Errorf("set EMAIL_FROM (or GMAIL_USER) and GMAIL_APP_PASSWORD")
	}

	to := strings.TrimSpace(userEmail)
	if to == "" {
		return false, fmt.Errorf("recipient email is required")
	}

	fromName := strings.TrimSpace(os.Getenv("EMAIL_FROM_NAME"))
	if fromName == "" {
		fromName = "Meskeny"
	}
	fromHeader := fmt.Sprintf("%s <%s>", fromName, from)

	msg := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		html,
	}, "\r\n")

	auth := smtp.PlainAuth("", from, password, "smtp.gmail.com")
	if err := smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, []byte(msg)); err != nil {
		return false, fmt.Errorf("gmail smtp: %w", err)
	}

	return true, nil
}
