package services

import "apartments-clone-server/models"

// BrokerProfileVisibleOnListings reports whether the broker chose to show name/photo on listings.
func BrokerProfileVisibleOnListings(u *models.User) bool {
	if u == nil || u.ID == 0 {
		return true
	}
	if !IsVerifiedBroker(u) {
		return true
	}
	return u.BrokerShowProfileOnListings
}

// ApplyBrokerProfilePrivacyToUser clears public name/photo when the broker opted out.
func ApplyBrokerProfilePrivacyToUser(u *models.User) {
	if u == nil || u.ID == 0 {
		return
	}
	if IsVerifiedBroker(u) && !u.BrokerShowProfileOnListings {
		u.FirstName = ""
		u.LastName = ""
		u.AvatarURL = ""
	}
}
