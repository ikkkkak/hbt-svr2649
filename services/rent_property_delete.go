package services

import (
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// PurgeRentPropertyDependencies deletes rows that reference a rent property.
// Call before hard-deleting from properties (FK constraints without ON DELETE CASCADE).
func PurgeRentPropertyDependencies(tx *gorm.DB, propertyID uint) error {
	if err := purgeRentPropertyVideos(tx, propertyID); err != nil {
		return err
	}

	var conversationIDs []uint
	if err := tx.Model(&models.Conversation{}).Unscoped().
		Where("property_id = ?", propertyID).
		Pluck("id", &conversationIDs).Error; err != nil {
		return err
	}
	if len(conversationIDs) > 0 {
		if err := tx.Unscoped().Where("conversation_id IN ?", conversationIDs).Delete(&models.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("id IN ?", conversationIDs).Delete(&models.Conversation{}).Error; err != nil {
			return err
		}
	}

	var wishlistIDs []uint
	if err := tx.Model(&models.GroupWishlistItem{}).
		Where("property_id = ?", propertyID).
		Pluck("id", &wishlistIDs).Error; err != nil {
		return err
	}
	if len(wishlistIDs) > 0 {
		if err := tx.Unscoped().Where("wishlist_id IN ?", wishlistIDs).Delete(&models.GroupWishlistLike{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_id = ?", propertyID).Delete(&models.GroupWishlistItem{}).Error; err != nil {
			return err
		}
	}

	for _, target := range []interface{}{
		&models.Review{},
		&models.Reservation{},
		&models.PropertyAvailability{},
		&models.PropertyBlock{},
		&models.PropertyDiscount{},
		&models.PropertyPricing{},
		&models.LocationCriteriaProperty{},
		&models.CollectionProperty{},
		&models.HiddenProperty{},
		&models.PropertyReport{},
		&models.UserBehavior{},
		&models.Interaction{},
		&models.NotificationEvent{},
		&models.NotificationDeliveryLog{},
	} {
		if err := tx.Unscoped().Where("property_id = ?", propertyID).Delete(target).Error; err != nil {
			return err
		}
	}

	// Legacy table (pre-Airbnb migration); ignore if already dropped.
	_ = tx.Exec("DELETE FROM apartments WHERE property_id = ?", propertyID).Error

	return nil
}

func purgeRentPropertyVideos(tx *gorm.DB, propertyID uint) error {
	var videoIDs []uint
	if err := tx.Model(&models.Video{}).Unscoped().
		Where("property_id = ?", propertyID).
		Pluck("id", &videoIDs).Error; err != nil {
		return err
	}
	if len(videoIDs) == 0 {
		return nil
	}

	if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.HiddenVideo{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.VideoReport{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.VideoView{}).Error; err != nil {
		return err
	}

	var commentIDs []uint
	if err := tx.Model(&models.VideoComment{}).Unscoped().
		Where("video_id IN ?", videoIDs).
		Pluck("id", &commentIDs).Error; err != nil {
		return err
	}
	if len(commentIDs) > 0 {
		if err := tx.Unscoped().Where("comment_id IN ?", commentIDs).Delete(&models.VideoCommentLike{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.VideoComment{}).Error; err != nil {
			return err
		}
	}

	if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.VideoLike{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("video_id IN ?", videoIDs).Delete(&models.VideoSave{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("property_id = ?", propertyID).Delete(&models.Video{}).Error
}

// DetachRentPropertyFromDiscovery removes location-criteria links after soft-delete.
func DetachRentPropertyFromDiscovery(tx *gorm.DB, propertyID uint) error {
	return tx.Where("property_id = ?", propertyID).Delete(&models.LocationCriteriaProperty{}).Error
}
