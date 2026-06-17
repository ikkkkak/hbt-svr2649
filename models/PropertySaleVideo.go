package models

import (
	"gorm.io/gorm"
)

// PropertySaleVideo represents a video for a property sale
type PropertySaleVideo struct {
	gorm.Model
	PropertySaleID uint         `json:"propertySaleID" gorm:"not null;index"`
	PropertySale   PropertySale `json:"propertySale" gorm:"foreignKey:PropertySaleID;references:ID"`

	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID;references:ID"`

	VideoURL     string  `json:"videoURL" gorm:"not null"`
	ThumbnailURL string  `json:"thumbnailURL"`
	DurationSec  float64 `json:"durationSec"`
	Caption      string  `json:"caption" gorm:"type:text"`

	// Adaptive streaming (HLS) — populated by async transcoding worker
	HlsURL             string `json:"hlsURL" gorm:"column:hls_url"`
	MobileVideoURL     string `json:"mobileVideoURL" gorm:"column:mobile_video_url"`
	ProcessingStatus   string `json:"processingStatus" gorm:"column:processing_status;default:pending;index"`
	ProcessingError    string `json:"processingError,omitempty" gorm:"column:processing_error;type:text"`
	SourceWidth        int    `json:"sourceWidth,omitempty" gorm:"column:source_width"`
	SourceHeight       int    `json:"sourceHeight,omitempty" gorm:"column:source_height"`
	RenditionsJSON     []byte `json:"renditions,omitempty" gorm:"column:renditions_json;type:jsonb"`
	ProcessingProgress int    `json:"processingProgress" gorm:"column:processing_progress;default:0"`
	SpriteSheetURL     string `json:"spriteSheetURL" gorm:"column:sprite_sheet_url"`
	SpriteVttURL       string `json:"spriteVttURL,omitempty" gorm:"column:sprite_vtt_url"`
	PreviewBlurURL     string `json:"preview_blur_url" gorm:"column:preview_blur_url"`

	LikesCount    int64 `json:"likesCount" gorm:"default:0"`
	CommentsCount int64 `json:"commentsCount" gorm:"default:0"`
	SavesCount    int64 `json:"savesCount" gorm:"default:0"`

	// Admin moderation fields
	ViewCount int64  `json:"viewCount" gorm:"default:0;index"`
	IsFlagged bool   `json:"isFlagged" gorm:"default:false;index"`
	Status    string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, approved, rejected

	// Frontend helper fields (not stored in DB)
	Liked bool `json:"liked" gorm:"-"`
	Saved bool `json:"saved" gorm:"-"`
}

// PropertySaleVideoLike represents a like on a property sale video
type PropertySaleVideoLike struct {
	gorm.Model
	PropertySaleVideoID uint `json:"propertySaleVideoID" gorm:"index;not null"`
	UserID              uint `json:"userID" gorm:"index;not null"`
}

// PropertySaleVideoSave represents a save on a property sale video
type PropertySaleVideoSave struct {
	gorm.Model
	PropertySaleVideoID uint `json:"propertySaleVideoID" gorm:"index;not null"`
	UserID              uint `json:"userID" gorm:"index;not null"`
}

// PropertySaleVideoComment represents a comment on a property sale video
// Note: PropertySaleVideoID stores the property sale ID (not a PropertySaleVideo record ID)
// because property sale videos are synthetic (stored in PropertySale.Videos array, not as separate records)
type PropertySaleVideoComment struct {
	gorm.Model
	PropertySaleVideoID uint                       `json:"propertySaleVideoID" gorm:"index;not null"` // Stores property sale ID
	UserID              uint                       `json:"userID" gorm:"index;not null"`
	User                User                       `json:"user" gorm:"foreignKey:UserID"`
	Content             string                     `json:"content" gorm:"type:text;not null"`
	Edited              bool                       `json:"edited" gorm:"default:false"`
	ParentID            *uint                      `json:"parentID" gorm:"index"` // For replies
	Parent              *PropertySaleVideoComment  `json:"parent" gorm:"foreignKey:ParentID"`
	Replies             []PropertySaleVideoComment `json:"replies" gorm:"foreignKey:ParentID"`
	LikesCount          int64                      `json:"likesCount" gorm:"default:0"`
}

// PropertySaleVideoCommentLike represents a like on a property sale video comment
type PropertySaleVideoCommentLike struct {
	gorm.Model
	CommentID uint `json:"commentID" gorm:"index;not null"`
	UserID    uint `json:"userID" gorm:"index;not null"`
}

// TableName specifies the table name for PropertySaleVideoCommentLike
func (PropertySaleVideoCommentLike) TableName() string {
	return "property_sale_video_comment_likes"
}

// PropertySaleVideoReport represents a report made against a property sale video
type PropertySaleVideoReport struct {
	gorm.Model
	PropertySaleVideoID uint              `json:"propertySaleVideoID" gorm:"not null;index"`
	PropertySaleVideo   PropertySaleVideo `json:"propertySaleVideo" gorm:"foreignKey:PropertySaleVideoID"`
	ReporterID          *uint             `json:"reporterID" gorm:"index"` // Nullable for anonymous reports
	Reporter            *User             `json:"reporter" gorm:"foreignKey:ReporterID"`
	Reason              string            `json:"reason" gorm:"not null"` // inappropriate, spam, harassment, violence, fake, other
	Description         string            `json:"description" gorm:"type:text"`
	Status              string            `json:"status" gorm:"default:'pending'"` // pending, reviewed, resolved, dismissed
	AdminNotes          string            `json:"adminNotes" gorm:"type:text"`
}

// HiddenPropertySaleVideo represents a property sale video hidden from a user's feed
type HiddenPropertySaleVideo struct {
	gorm.Model
	PropertySaleVideoID uint              `json:"propertySaleVideoID" gorm:"not null;index;uniqueIndex:idx_hidden_user_property_sale_video"`
	PropertySaleVideo   PropertySaleVideo `json:"propertySaleVideo" gorm:"foreignKey:PropertySaleVideoID"`
	UserID              *uint             `json:"userID" gorm:"index;uniqueIndex:idx_hidden_user_property_sale_video"` // Nullable for anonymous hides
	User                *User             `json:"user" gorm:"foreignKey:UserID"`
	Reason              string            `json:"reason" gorm:"not null"`
}
