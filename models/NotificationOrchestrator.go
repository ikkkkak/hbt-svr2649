package models

import (
	"time"

	"gorm.io/datatypes"
)

// NotificationCandidate is the central queue row for the AI notification orchestrator.
// user_id references users.id (uint) — Meskeny uses numeric user PKs, not UUIDs.
type NotificationCandidate struct {
	ID        string    `json:"id" gorm:"primaryKey;type:char(36)"`
	CreatedAt time.Time `json:"createdAt"`

	UserID           uint           `json:"userId" gorm:"not null;index:idx_nc_user_decision_sched"`
	NotificationType string         `json:"notificationType" gorm:"size:64;index:idx_nc_user_type_created"`
	Title            string         `json:"title" gorm:"size:255"`
	Body             string         `json:"body" gorm:"type:text"`
	Payload          datatypes.JSON `json:"payload" gorm:"type:jsonb"`
	ImageURL         string         `json:"imageUrl" gorm:"size:500"`

	RelevanceScore int    `json:"relevanceScore"`
	UrgencyLevel   string `json:"urgencyLevel" gorm:"size:20"` // critical, high, normal, low
	PropertySaleID *uint  `json:"propertySaleId" gorm:"index"`
	MatchScore     *int   `json:"matchScore"`

	AIScore    int    `json:"aiScore"`
	// IMPORTANT: DB column name must be exactly `ai_decision`.
	// Without an explicit column tag, GORM may generate a wrong name like `a_idecision`
	// (this breaks the worker queries that use `ai_decision`).
	AIDecision string `json:"aiDecision" gorm:"column:ai_decision;size:20;index:idx_nc_user_decision_sched"` // pending, send, delay, batch, drop, batched, digest_sent
	AIReason   string `json:"aiReason" gorm:"type:text"`

	RequestedAt  time.Time  `json:"requestedAt"`
	ScheduledFor *time.Time `json:"scheduledFor" gorm:"index"`
	SentAt       *time.Time `json:"sentAt" gorm:"index"`

	Delivered   bool       `json:"delivered"`
	Opened      bool       `json:"opened"`
	OpenedAt    *time.Time `json:"openedAt"`
	Dismissed   bool       `json:"dismissed"`
	DismissedAt *time.Time `json:"dismissedAt"`

	// BatchID links rows merged into one digest send
	BatchID *string `json:"batchId" gorm:"size:36;index"`
}

func (NotificationCandidate) TableName() string {
	return "notification_candidates"
}

// UserNotificationQuota tracks consumption inside a rolling window (see orchestrator service).
type UserNotificationQuota struct {
	ID        string    `json:"id" gorm:"primaryKey;type:char(36)"`
	CreatedAt time.Time `json:"createdAt"`

	UserID uint `json:"userId" gorm:"not null;index:idx_unq_user_window"`

	WindowStart time.Time `json:"windowStart" gorm:"not null;index:idx_unq_user_window"`
	WindowEnd   time.Time `json:"windowEnd" gorm:"not null"`

	SentCount      int `json:"sentCount"`
	OpenedCount    int `json:"openedCount"`
	DismissedCount int `json:"dismissedCount"`

	DailyLimit int `json:"dailyLimit" gorm:"default:4"`

	LastSentAt   *time.Time `json:"lastSentAt"`
	LastOpenedAt *time.Time `json:"lastOpenedAt"`
}

func (UserNotificationQuota) TableName() string {
	return "user_notification_quota"
}

// UserNotificationLearned stores AI/learned preferences (spec: user_notification_preferences).
type UserNotificationLearned struct {
	ID        string    `json:"id" gorm:"primaryKey;type:char(36)"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	UserID uint `json:"userId" gorm:"not null;uniqueIndex"`

	PreferredHourStart *int `json:"preferredHourStart"`
	PreferredHourEnd   *int `json:"preferredHourEnd"`
	PeakOpenHour       *int `json:"peakOpenHour"`
	PeakOpenDay        *int `json:"peakOpenDay"`

	OpenRate7d      float64 `json:"openRate7d"`
	DismissRate7d   float64 `json:"dismissRate7d"`
	AvgOpenDelaySec int     `json:"avgOpenDelaySeconds"`

	MatchOpenRate    float64 `json:"matchOpenRate"`
	MessageOpenRate  float64 `json:"messageOpenRate"`
	DigestPreference bool    `json:"digestPreference"`

	DoNotDisturbEnabled bool `json:"doNotDisturbEnabled"`
	QuietHoursStart     int  `json:"quietHoursStart" gorm:"default:23"`
	QuietHoursEnd       int  `json:"quietHoursEnd" gorm:"default:7"`

	DailyLimitOverride int `json:"dailyLimitOverride" gorm:"default:0"` // 0 = use default 4
}

func (UserNotificationLearned) TableName() string {
	return "user_notification_preferences"
}
