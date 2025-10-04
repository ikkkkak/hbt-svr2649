package models

import (
	"time"
	"gorm.io/gorm"
)

type PropertyAvailability struct {
	gorm.Model
	PropertyID    uint      `json:"propertyID" gorm:"not null;index"`
	Date          time.Time `json:"date" gorm:"not null;index"`
	IsAvailable   bool      `json:"isAvailable" gorm:"default:true"`
	Price         float64   `json:"price" gorm:"not null"`
	MinStay       int       `json:"minStay" gorm:"default:1"`
	MaxStay       int       `json:"maxStay" gorm:"default:0"`
	CheckInTime   string    `json:"checkInTime" gorm:"default:'15:00'"`
	CheckOutTime  string    `json:"checkOutTime" gorm:"default:'11:00'"`
	Notes         string    `json:"notes"`
	Property      Property  `json:"property" gorm:"foreignKey:PropertyID"`
}

type PropertyPricing struct {
	gorm.Model
	PropertyID    uint    `json:"propertyID" gorm:"not null;index"`
	BasePrice     float64 `json:"basePrice" gorm:"not null"`
	WeekendPrice  float64 `json:"weekendPrice"`
	WeeklyPrice   float64 `json:"weeklyPrice"`
	MonthlyPrice  float64 `json:"monthlyPrice"`
	CleaningFee   float64 `json:"cleaningFee"`
	ServiceFee    float64 `json:"serviceFee"`
	SecurityDeposit float64 `json:"securityDeposit"`
	Currency      string  `json:"currency" gorm:"default:'MRO'"`
	Property      Property `json:"property" gorm:"foreignKey:PropertyID"`
}

type PropertyDiscount struct {
	gorm.Model
	PropertyID    uint      `json:"propertyID" gorm:"not null;index"`
	Name          string    `json:"name" gorm:"not null"`
	Type          string    `json:"type" gorm:"not null"` // "percentage", "fixed", "early_bird", "last_minute"
	Value         float64   `json:"value" gorm:"not null"`
	MinStay       int       `json:"minStay"`
	MaxStay       int       `json:"maxStay"`
	StartDate     time.Time `json:"startDate"`
	EndDate       time.Time `json:"endDate"`
	IsActive      bool      `json:"isActive" gorm:"default:true"`
	Property      Property  `json:"property" gorm:"foreignKey:PropertyID"`
}

type PropertyBlock struct {
	gorm.Model
	PropertyID    uint      `json:"propertyID" gorm:"not null;index"`
	StartDate     time.Time `json:"startDate" gorm:"not null"`
	EndDate       time.Time `json:"endDate" gorm:"not null"`
	Reason        string    `json:"reason"`
	IsMaintenance bool      `json:"isMaintenance" gorm:"default:false"`
	Property      Property  `json:"property" gorm:"foreignKey:PropertyID"`
}
