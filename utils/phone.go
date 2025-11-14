package utils

import (
	"regexp"
	"strings"
)

// FormatPhoneNumber formats a phone number to a standard format with country code
// Removes all non-digit characters and ensures it starts with country code
// Used for display purposes only
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
// Accepts both 8-digit local numbers and numbers with country code prefix
func ValidatePhoneNumber(phoneNumber string) bool {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	cleaned := re.ReplaceAllString(phoneNumber, "")

	// Remove country code prefix if present (222)
	if strings.HasPrefix(cleaned, "222") {
		cleaned = cleaned[3:] // Remove "222" prefix
	}

	// Remove leading zeros
	cleaned = strings.TrimLeft(cleaned, "0")

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
// Returns only the 8-digit local number WITHOUT country code prefix
func NormalizePhoneNumber(phoneNumber string) string {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(phoneNumber, "")

	// Remove country code prefix if present (222)
	if strings.HasPrefix(digits, "222") {
		digits = digits[3:] // Remove "222" prefix
	}

	// Remove leading zeros
	digits = strings.TrimLeft(digits, "0")

	// Ensure we have exactly 8 digits (local Mauritanian number)
	// If we have more than 8, take the last 8 digits
	if len(digits) > 8 {
		digits = digits[len(digits)-8:]
	}

	// If we have less than 8, pad with zeros at the beginning (shouldn't happen with validation)
	if len(digits) < 8 {
		digits = strings.Repeat("0", 8-len(digits)) + digits
	}

	return digits
}

// DisplayPhoneNumber formats phone number for display
// Handles both 8-digit local numbers and numbers with country code
func DisplayPhoneNumber(phoneNumber string) string {
	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(phoneNumber, "")

	// If it's 8 digits (local number), add country code for display
	if len(digits) == 8 {
		// Format as +222 XX XX XX XX
		return "+222 " + digits[0:2] + " " + digits[2:4] + " " + digits[4:6] + " " + digits[6:8]
	}

	// If it already has country code (12 digits starting with 222)
	if len(digits) == 12 && strings.HasPrefix(digits, "222") {
		// Format as +222 XX XX XX XX
		return "+" + digits[:3] + " " + digits[3:5] + " " + digits[5:7] + " " + digits[7:9] + " " + digits[9:11]
	}

	// Fallback: return as is
	return phoneNumber
}
