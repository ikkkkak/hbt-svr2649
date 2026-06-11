package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	GuideListingSale = "sale"

	GuideTriggerViewsDrop      = "views_drop"
	GuideTriggerViewsSpike     = "views_spike"
	GuideTriggerRankingShift   = "ranking_shift"
	GuideTriggerFirstInquiry   = "first_inquiry"
	GuideTriggerInquiryDrop    = "inquiry_rate_drop"
	GuideTriggerCompetitive    = "competitive_pressure"
	GuideTriggerSeasonal       = "seasonal_momentum"
	GuideTriggerActionImpact   = "action_impact"
	GuideTriggerStale          = "stale_listing"
	GuideTriggerSocial         = "social_signal"

	GuideSeverityInfo    = "info"
	GuideSeverityAction  = "action"
	GuideSeverityUrgent  = "urgent"

	GuideStatusUnread       = "unread"
	GuideStatusRead         = "read"
	GuideStatusImplemented  = "implemented"
	GuideStatusDismissed    = "dismissed"
	GuideStatusResolved     = "resolved"

	GuideChannelInApp    = "in_app"
	GuideChannelPush     = "push"
	GuideChannelWhatsApp = "whatsapp"
)

// GuideComment is a structured AI analyst note on a host listing.
type GuideComment struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	ListingKind    string `json:"listingKind" gorm:"type:varchar(16);not null;default:'sale'"`
	PropertySaleID *uint  `json:"propertySaleId" gorm:"index"`
	PropertyID     *uint  `json:"propertyId" gorm:"index"`
	LandmarkID     *uint  `json:"landmarkId" gorm:"index"`
	HostID         uint   `json:"hostId" gorm:"not null;index"`
	Locale         string `json:"locale" gorm:"type:varchar(8);not null;default:'en'"`
	ParentID       *uint  `json:"parentId" gorm:"index"`

	TriggerEvent string `json:"triggerEvent" gorm:"type:varchar(32);not null;index"`
	Severity     string `json:"severity" gorm:"type:varchar(16);not null"`
	Category     string `json:"category" gorm:"type:varchar(24);not null"`
	Tone         string `json:"tone" gorm:"type:varchar(16);not null;default:'clinical'"`

	Diagnosis       string `json:"diagnosis" gorm:"type:text"`
	RootCause       string `json:"rootCause" gorm:"type:text"`
	Prescription    string `json:"prescription" gorm:"type:text"`
	ImpactForecast  string `json:"impactForecast" gorm:"type:text"`
	AlgorithmSignals JSONMap `json:"algorithmSignals" gorm:"type:jsonb;serializer:json"`

	Status     string  `json:"status" gorm:"type:varchar(16);not null;default:'unread';index"`
	HostAction *string `json:"hostAction" gorm:"type:varchar(32)"`
	Body       string  `json:"body,omitempty" gorm:"type:text"` // host/support replies

	FollowUpScheduledAt *time.Time `json:"followUpScheduledAt" gorm:"index"`

	PropertySale *PropertySale `json:"propertySale,omitempty" gorm:"foreignKey:PropertySaleID"`
	Replies      []GuideComment `json:"replies,omitempty" gorm:"foreignKey:ParentID"`
}

func (GuideComment) TableName() string { return "guide_comments" }

// GuideNotification tracks delivery per channel.
type GuideNotification struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`

	CommentID uint   `json:"commentId" gorm:"not null;index"`
	HostID    uint   `json:"hostId" gorm:"not null;index"`
	Channel   string `json:"channel" gorm:"type:varchar(16);not null"`
	Status    string `json:"status" gorm:"type:varchar(16);not null;default:'pending'"`
	DeepLink  string `json:"deepLink" gorm:"type:text"`
	SentAt    *time.Time `json:"sentAt"`
	ReadAt    *time.Time `json:"readAt"`

	Comment *GuideComment `json:"comment,omitempty" gorm:"foreignKey:CommentID"`
}

func (GuideNotification) TableName() string { return "guide_notifications" }

// GuideHostPreference stores per-listing dismiss cooldowns and pause state.
type GuideHostPreference struct {
	HostID           uint      `json:"hostId" gorm:"primaryKey"`
	PropertySaleID   uint      `json:"propertySaleId" gorm:"primaryKey"`
	ConsecutiveDismissals int   `json:"consecutiveDismissals" gorm:"default:0"`
	PausedUntil      *time.Time `json:"pausedUntil"`
	SuppressedCategories JSONMap `json:"suppressedCategories" gorm:"type:jsonb;serializer:json"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (GuideHostPreference) TableName() string { return "guide_host_preferences" }

// JSONMap for jsonb columns.
type JSONMap map[string]interface{}
