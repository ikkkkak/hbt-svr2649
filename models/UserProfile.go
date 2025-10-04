package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// UserProfile represents the detailed profile information for a user
// This is separate from the User model which handles authentication
type UserProfile struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"userID" gorm:"not null;uniqueIndex"`
	User        User           `json:"user" gorm:"foreignKey:UserID"`
	
	// Basic Information
	FirstName   string         `json:"firstName" gorm:"size:100"`
	LastName    string         `json:"lastName" gorm:"size:100"`
	AvatarURL   string         `json:"avatarURL" gorm:"size:512"`
	DateOfBirth string         `json:"dateOfBirth" gorm:"size:20"`
	Bio         string         `json:"bio" gorm:"type:text"`
	
	// Languages and Skills
	Languages   datatypes.JSON `json:"languages"` // Array of strings
	Skills      datatypes.JSON `json:"skills"`    // Array of strings
	
	// Location and Preferences
	Location    string         `json:"location" gorm:"size:100"`
	Interests   datatypes.JSON `json:"interests"` // Array of strings
	
	// Professional Information
	Occupation  string         `json:"occupation" gorm:"size:100"`
	Company     string         `json:"company" gorm:"size:100"`
	Website     string         `json:"website" gorm:"size:255"`
	
	// Social Links
	Instagram   string         `json:"instagram" gorm:"size:100"`
	Twitter     string         `json:"twitter" gorm:"size:100"`
	LinkedIn    string         `json:"linkedin" gorm:"size:100"`
	
	// Travel Preferences
	TravelStyle string         `json:"travelStyle" gorm:"size:50"` // budget, luxury, adventure, etc.
	AccommodationType string   `json:"accommodationType" gorm:"size:50"` // hotel, hostel, airbnb, etc.
	
	// Profile Status
	IsPublic    bool           `json:"isPublic" gorm:"default:true"`
	IsComplete  bool           `json:"isComplete" gorm:"default:false"`
	CompletionPercentage int   `json:"completionPercentage" gorm:"default:0"`
	
	// Timestamps
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"deletedAt" gorm:"index"`
}

// Custom JSON marshaling to handle JSON fields properly
func (up *UserProfile) MarshalJSON() ([]byte, error) {
	type Alias UserProfile
	aux := &struct {
		Languages []string `json:"languages,omitempty"`
		Skills    []string `json:"skills,omitempty"`
		Interests []string `json:"interests,omitempty"`
		*Alias
	}{
		Languages: []string{},
		Skills:    []string{},
		Interests: []string{},
		Alias:     (*Alias)(up),
	}

	// Parse Languages JSON
	if up.Languages != nil {
		var languages []string
		if err := json.Unmarshal(up.Languages, &languages); err == nil {
			aux.Languages = languages
		}
	}

	// Parse Skills JSON
	if up.Skills != nil {
		var skills []string
		if err := json.Unmarshal(up.Skills, &skills); err == nil {
			aux.Skills = skills
		}
	}

	// Parse Interests JSON
	if up.Interests != nil {
		var interests []string
		if err := json.Unmarshal(up.Interests, &interests); err == nil {
			aux.Interests = interests
		}
	}

	return json.Marshal(aux)
}

// CalculateCompletionPercentage calculates how complete the profile is
func (up *UserProfile) CalculateCompletionPercentage() int {
	fields := []bool{
		up.FirstName != "",
		up.LastName != "",
		up.AvatarURL != "",
		up.Bio != "",
		up.Languages != nil && len(up.Languages) > 0,
		up.Location != "",
		up.DateOfBirth != "",
		up.Occupation != "",
	}
	
	completed := 0
	for _, field := range fields {
		if field {
			completed++
		}
	}
	
	percentage := (completed * 100) / len(fields)
	up.CompletionPercentage = percentage
	up.IsComplete = percentage >= 80 // Consider complete if 80% or more
	
	return percentage
}
