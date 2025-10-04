package models

import (
	"encoding/json"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	FirstName           string         `json:"firstName"`
	LastName            string         `json:"lastName"`
	Email               string         `json:"email"`
	PhoneNumber         string         `json:"phoneNumber" gorm:"uniqueIndex"`
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
}

// Custom JSON marshaling to handle JSON fields properly
func (u *User) MarshalJSON() ([]byte, error) {
	type Alias User
	aux := &struct {
		Languages        []string `json:"languages,omitempty"`
		Skills           []string `json:"skills,omitempty"`
		SavedProperties  []int    `json:"savedProperties,omitempty"`
		SavedExperiences []int    `json:"savedExperiences,omitempty"`
		PushTokens       []string `json:"pushTokens,omitempty"`
		*Alias
	}{
		Languages:        []string{},
		Skills:           []string{},
		SavedProperties:  []int{},
		SavedExperiences: []int{},
		PushTokens:       []string{},
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
