package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"net/http"

	"github.com/kataras/iris/v12"
)

// DeactivatePropertySale allows host to deactivate their property
func DeactivatePropertySale(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check ownership
	isOwner := false
	if property.OrganizationID != nil && *property.OrganizationID > 0 {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err == nil && org.OwnerID > 0 && org.OwnerID == userID {
			isOwner = true
		}
	} else if property.OwnerID != nil && *property.OwnerID == userID {
		isOwner = true
	}

	if !isOwner {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You don't own this property"})
		return
	}

	// Deactivate property
	storage.DB.Model(&property).Update("is_deactivated", true)
	log.Printf("✅ Property %d deactivated by user %d", propertyID, userID)

	ctx.JSON(iris.Map{"success": true, "message": "Property deactivated successfully"})
}

// ReactivatePropertySale allows host to reactivate their property
func ReactivatePropertySale(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check ownership
	isOwner := false
	if property.OrganizationID != nil && *property.OrganizationID > 0 {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err == nil && org.OwnerID > 0 && org.OwnerID == userID {
			isOwner = true
		}
	} else if property.OwnerID != nil && *property.OwnerID == userID {
		isOwner = true
	}

	if !isOwner {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You don't own this property"})
		return
	}

	// Reactivate property
	storage.DB.Model(&property).Update("is_deactivated", false)
	log.Printf("✅ Property %d reactivated by user %d", propertyID, userID)

	ctx.JSON(iris.Map{"success": true, "message": "Property reactivated successfully"})
}

// DeletePropertySale marks property for deletion (soft delete, permanent after 15 days)
func DeletePropertySale(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check ownership
	isOwner := false
	if property.OrganizationID != nil && *property.OrganizationID > 0 {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err == nil && org.OwnerID > 0 && org.OwnerID == userID {
			isOwner = true
		}
	} else if property.OwnerID != nil && *property.OwnerID == userID {
		isOwner = true
	}

	if !isOwner {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You don't own this property"})
		return
	}

	// Soft delete (GORM handles this with DeletedAt)
	storage.DB.Delete(&property)
	log.Printf("✅ Property %d marked for deletion by user %d (will be permanently deleted in 15 days)", propertyID, userID)

	ctx.JSON(iris.Map{"success": true, "message": "Property marked for deletion. It will be permanently deleted in 15 days."})
}

// MarkPropertySaleAsSold allows host to mark property as sold
func MarkPropertySaleAsSold(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check ownership
	isOwner := false
	if property.OrganizationID != nil && *property.OrganizationID > 0 {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err == nil && org.OwnerID > 0 && org.OwnerID == userID {
			isOwner = true
		}
	} else if property.OwnerID != nil && *property.OwnerID == userID {
		isOwner = true
	}

	if !isOwner {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You don't own this property"})
		return
	}

	// Mark as sold and deactivate
	storage.DB.Model(&property).Updates(map[string]interface{}{
		"is_sold":        true,
		"is_deactivated": true,
		"status":         "sold",
	})
	log.Printf("✅ Property %d marked as sold by user %d", propertyID, userID)

	ctx.JSON(iris.Map{"success": true, "message": "Property marked as sold successfully"})
}

// MarkPropertySaleAsUnsold allows host to unmark property as sold
func MarkPropertySaleAsUnsold(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check ownership
	isOwner := false
	if property.OrganizationID != nil && *property.OrganizationID > 0 {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err == nil && org.OwnerID > 0 && org.OwnerID == userID {
			isOwner = true
		}
	} else if property.OwnerID != nil && *property.OwnerID == userID {
		isOwner = true
	}

	if !isOwner {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You don't own this property"})
		return
	}

	// Unmark as sold and reactivate
	storage.DB.Model(&property).Updates(map[string]interface{}{
		"is_sold":        false,
		"is_deactivated": false,
		"status":         "published", // Reset to published status
	})
	log.Printf("✅ Property %d unmarked as sold by user %d", propertyID, userID)

	ctx.JSON(iris.Map{"success": true, "message": "Property unmarked as sold successfully"})
}
