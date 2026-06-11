package models

import (
	"gorm.io/gorm"
)

// LandmarkVideoLike represents a like on a landmark's video
type LandmarkVideoLike struct {
	gorm.Model
	LandmarkID uint `json:"landmark_id" gorm:"index;not null"`
	UserID     uint `json:"user_id" gorm:"index;not null"`
}

// TableName for LandmarkVideoLike
func (LandmarkVideoLike) TableName() string {
	return "landmark_video_likes"
}

// LandmarkVideoSave represents a save of a landmark's video
type LandmarkVideoSave struct {
	gorm.Model
	LandmarkID uint `json:"landmark_id" gorm:"index;not null"`
	UserID     uint `json:"user_id" gorm:"index;not null"`
}

// TableName for LandmarkVideoSave
func (LandmarkVideoSave) TableName() string {
	return "landmark_video_saves"
}
