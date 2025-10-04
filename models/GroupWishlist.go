package models

import "time"

type GroupWishlistItem struct {
	ID      uint `json:"id" gorm:"primaryKey"`
	GroupID uint `json:"groupID" gorm:"not null;index"`
	Group   ExperienceGroup

	// Either PropertyID or ExperienceID should be set
	PropertyID   *uint `json:"propertyID" gorm:"index"`
	ExperienceID *uint `json:"experienceID" gorm:"index"`

	AddedByID uint `json:"addedByID" gorm:"not null;index"`
	AddedBy   User `json:"addedBy" gorm:"foreignKey:AddedByID"`

	CreatedAt time.Time `json:"createdAt"`
}

type GroupWishlistLike struct {
	ID         uint `json:"id" gorm:"primaryKey"`
	WishlistID uint `json:"wishlistID" gorm:"not null;index"`
	Wishlist   GroupWishlistItem

	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID"`

	CreatedAt time.Time `json:"createdAt"`
}
