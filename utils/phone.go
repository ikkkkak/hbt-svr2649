package utils

import (
	"regexp"
	"strings"
)

// FormatPhoneNumber formats a phone number to a standard format
// Removes all non-digit characters and ensures it starts with country code
func FormatPhoneNumber(phoneNumber string) string {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(phoneNumber, "")

	// If it doesn't start with country code, assume Mauritania (+222)
	if len(digits) > 0 && !strings.HasPrefix(digits, "222") {
		// Remove leading zeros
		digits = strings.TrimLeft(digits, "0")
		// Add Mauritania country code
		digits = "222" + digits
	}

	return digits
}

// ValidatePhoneNumber validates if a phone number is in correct format
func ValidatePhoneNumber(phoneNumber string) bool {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	cleaned := re.ReplaceAllString(phoneNumber, "")

	// Check if it's exactly 8 digits for Mauritania
	if len(cleaned) != 8 {
		return false
	}

	// Check if all characters are digits
	matched, _ := regexp.MatchString(`^\d+$`, cleaned)
	if !matched {
		return false
	}

	// Check if it starts with valid Mauritanian prefixes (2, 3, or 4)
	firstDigit := string(cleaned[0])
	validPrefixes := []string{"2", "3", "4"}
	for _, prefix := range validPrefixes {
		if firstDigit == prefix {
			return true
		}
	}

	return false
}

// NormalizePhoneNumber normalizes phone number for database storage
func NormalizePhoneNumber(phoneNumber string) string {
	return FormatPhoneNumber(phoneNumber)
}

// DisplayPhoneNumber formats phone number for display
func DisplayPhoneNumber(phoneNumber string) string {
	formatted := FormatPhoneNumber(phoneNumber)
	if len(formatted) == 12 && strings.HasPrefix(formatted, "222") {
		// Format as +222 XX XX XX XX
		return "+" + formatted[:3] + " " + formatted[3:5] + " " + formatted[5:7] + " " + formatted[7:9] + " " + formatted[9:11]
	}
	return phoneNumber
}
