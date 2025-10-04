package models

import "gorm.io/gorm"

// Feedback represents user-submitted feedback for the app
type Feedback struct {
	gorm.Model
	UserID     uint   `json:"userID" gorm:"index;not null"`
	User       User   `json:"user" gorm:"foreignKey:UserID;references:ID"`
	Title      string `json:"title" gorm:"size:200"`
	Message    string `json:"message" gorm:"type:text;not null"`
	Rating     *int   `json:"rating" gorm:"index"`     // optional 1-5
	Context    string `json:"context" gorm:"size:200"` // e.g., screen/component
	AppVersion string `json:"appVersion" gorm:"size:50"`
	DeviceInfo string `json:"deviceInfo" gorm:"size:200"`
}
