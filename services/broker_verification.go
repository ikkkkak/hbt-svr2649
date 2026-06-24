package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"strings"
)

// BrokerIDString returns the public broker ID or "" when unset.
func BrokerIDString(u *models.User) string {
	if u == nil || u.BrokerID == nil {
		return ""
	}
	return strings.TrimSpace(*u.BrokerID)
}

// IsVerifiedBroker returns true when the user completed broker identity verification.
func IsVerifiedBroker(u *models.User) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(u.BrokerStatus), "approved") &&
		BrokerIDString(u) != "" {
		return true
	}
	return u.TrueBroker
}

// GenerateBrokerID assigns the next MSK-B-XXXXXX identifier.
func GenerateBrokerID() (string, error) {
	var maxSeq int64
	err := storage.DB.Raw(`
		SELECT COALESCE(MAX(
			CASE
				WHEN broker_id ~ '^MSK-B-[0-9]+$'
				THEN CAST(SUBSTRING(broker_id FROM 7) AS INTEGER)
				ELSE 100000
			END
		), 100000)
		FROM users
		WHERE broker_id IS NOT NULL AND broker_id <> ''
	`).Scan(&maxSeq).Error
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MSK-B-%06d", maxSeq+1), nil
}
