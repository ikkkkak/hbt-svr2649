package models

import "gorm.io/gorm"

type StoryLike struct {
	gorm.Model
	StoryID uint `json:"story_id" gorm:"index;not null"`
	UserID  uint `json:"user_id" gorm:"index;not null"`
}

func (StoryLike) TableName() string {
	return "story_likes"
}


