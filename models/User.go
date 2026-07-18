package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	FirstName           string         `json:"firstName"`
	LastName            string         `json:"lastName"`
	Email               string         `json:"email"`
	PhoneNumber         *string        `json:"phoneNumber" gorm:"uniqueIndex"`
	Password            string         `json:"password"`
	SocialLogin         bool           `json:"socialLogin"`
	SocialProvider      string         `json:"socialProvider"`
	AvatarURL           string         `json:"avatarURL"`
	DateOfBirth         string         `json:"dateOfBirth"`
	Bio                 string         `json:"bio"`
	Languages           datatypes.JSON `json:"languages"`
	Skills              datatypes.JSON `json:"skills"`
	Properties          []Property     `json:"properties" gorm:"foreignKey:HostID;references:ID"`
	SavedProperties     datatypes.JSON `json:"savedProperties"`
	SavedExperiences    datatypes.JSON `json:"savedExperiences"`
	SavedPropertySales  datatypes.JSON `json:"savedPropertySales"`
	PushTokens          datatypes.JSON `json:"pushTokens"`
	AllowsNotifications *bool          `json:"allowsNotifications"`
	IsVerified          *bool          `json:"isVerified"`
	VerificationStatus  string         `json:"verificationStatus"` // pending, approved, rejected
	IDType              string         `json:"idType"`
	IDNumber            string         `json:"idNumber"`
	IDFrontImage        string         `json:"idFrontImage"`
	IDBackImage         string         `json:"idBackImage"`
	SelfieImage         string         `json:"selfieImage"`
	Role                string         `json:"role" gorm:"type:varchar(20);default:user;index"` // user, host, admin, super_admin
	TrueBroker          bool           `json:"true_broker" gorm:"default:false"`                // admin-verified broker; all their properties show TrueBroker
	BrokerID            *string        `json:"broker_id" gorm:"size:32"` // assigned on broker approval; NULL until then
	BrokerStatus        string         `json:"broker_status" gorm:"default:'none';index"`       // none, pending, approved, rejected
	BrokerVerifiedAt    *time.Time     `json:"broker_verified_at"`
	BrokerVerifiedBy    *uint          `json:"broker_verified_by"`
	BrokerSubmittedAt   *time.Time     `json:"broker_submitted_at"`
	BrokerLicenseURL    string         `json:"broker_license_url"`
	BrokerSpokenLanguages datatypes.JSON `json:"broker_spoken_languages" gorm:"type:jsonb"`
	BrokerRejectionNotes string        `json:"broker_rejection_notes,omitempty" gorm:"type:text"`
	// When false, public listings still show verified badge + broker ID but hide name/avatar.
	BrokerShowProfileOnListings bool `json:"broker_show_profile_on_listings" gorm:"default:true"`
	FavoriteCityID      *uint        `json:"favoriteCityId" gorm:"index"`
	FavoriteCityName    string         `json:"favoriteCityName" gorm:"type:varchar(255)"`
	FavoriteZoneID      *uint          `json:"favoriteZoneId" gorm:"index"`
	FavoriteZoneName    string         `json:"favoriteZoneName" gorm:"type:varchar(255)"`
	// Host buyer-matching: nil = never asked; false = declined; true = opted in.
	ShareProfileWithHosts *bool `json:"shareProfileWithHosts" gorm:"index"`
	// When set, buyer profile is only shared with this host (exclusive).
	HostShareLockedHostID *uint `json:"hostShareLockedHostId" gorm:"index"`
}

// Custom JSON marshaling to handle JSON fields properly
func (u *User) MarshalJSON() ([]byte, error) {
	type Alias User
	aux := &struct {
		Languages         []string `json:"languages,omitempty"`
		Skills            []string `json:"skills,omitempty"`
		SavedProperties   []int    `json:"savedProperties,omitempty"`
		SavedExperiences  []int    `json:"savedExperiences,omitempty"`
		SavedPropertySales []int    `json:"savedPropertySales,omitempty"`
		PushTokens        []string `json:"pushTokens,omitempty"`
		// Shadows the model's Password field with an always-empty value so
		// the bcrypt hash can never serialize into ANY API response (it was
		// shipping in every payload that embeds a user — property host,
		// login, profile). Empty + omitempty = key absent entirely.
		Password string `json:"password,omitempty"`
		*Alias
	}{
		Languages:          []string{},
		Skills:             []string{},
		SavedProperties:    []int{},
		SavedExperiences:   []int{},
		SavedPropertySales: []int{},
		PushTokens:         []string{},
		Alias:            (*Alias)(u),
	}

	// Parse Languages JSON
	if u.Languages != nil {
		var languages []string
		if err := json.Unmarshal(u.Languages, &languages); err == nil {
			aux.Languages = languages
		}
	}

	// Parse Skills JSON
	if u.Skills != nil {
		var skills []string
		if err := json.Unmarshal(u.Skills, &skills); err == nil {
			aux.Skills = skills
		}
	}

	// Parse SavedProperties JSON
	if u.SavedProperties != nil {
		var savedProperties []int
		if err := json.Unmarshal(u.SavedProperties, &savedProperties); err == nil {
			aux.SavedProperties = savedProperties
		}
	}

	// Parse SavedExperiences JSON
	if u.SavedExperiences != nil {
		var savedExperiences []int
		if err := json.Unmarshal(u.SavedExperiences, &savedExperiences); err == nil {
			aux.SavedExperiences = savedExperiences
		}
	}

	// Parse SavedPropertySales JSON
	if u.SavedPropertySales != nil {
		var savedPropertySales []int
		if err := json.Unmarshal(u.SavedPropertySales, &savedPropertySales); err == nil {
			aux.SavedPropertySales = savedPropertySales
		}
	}

	// Parse PushTokens JSON
	if u.PushTokens != nil {
		var pushTokens []string
		if err := json.Unmarshal(u.PushTokens, &pushTokens); err == nil {
			aux.PushTokens = pushTokens
		}
	}

	// Note: Properties field is excluded to prevent circular reference

	return json.Marshal(aux)
}
