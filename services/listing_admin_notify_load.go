package services

import (
	"fmt"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"gorm.io/gorm"
)

// LoadListingAdminNotifyInput loads listing details for admin notification emails.
func LoadListingAdminNotifyInput(kind ListingKind, id uint) (ListingAdminNotifyInput, error) {
	switch kind {
	case ListingKindPropertySale:
		return loadPropertySaleNotifyInput(id)
	case ListingKindRent:
		return loadRentNotifyInput(id)
	case ListingKindLand:
		return loadLandNotifyInput(id)
	default:
		return ListingAdminNotifyInput{}, fmt.Errorf("unsupported listing kind %q", kind)
	}
}

func loadPropertySaleNotifyInput(id uint) (ListingAdminNotifyInput, error) {
	var sale models.PropertySale
	if err := storage.DB.Preload("Owner").First(&sale, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ListingAdminNotifyInput{}, fmt.Errorf("property sale #%d not found", id)
		}
		return ListingAdminNotifyInput{}, err
	}

	hostID := uint(0)
	hostEmail := ""
	if sale.OwnerID != nil && *sale.OwnerID > 0 {
		hostID = *sale.OwnerID
	}
	if sale.Owner != nil {
		hostEmail = strings.TrimSpace(sale.Owner.Email)
	}

	return ListingAdminNotifyInput{
		Kind:         ListingKindPropertySale,
		ID:           sale.ID,
		Title:        sale.Title,
		City:         sale.City,
		Price:        sale.ListingPrice,
		Currency:     sale.Currency,
		PropertyType: sale.PropertyType,
		HostUserID:   hostID,
		HostEmail:    hostEmail,
		Status:       sale.Status,
	}, nil
}

func loadRentNotifyInput(id uint) (ListingAdminNotifyInput, error) {
	var prop models.Property
	if err := storage.DB.Preload("Host").First(&prop, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ListingAdminNotifyInput{}, fmt.Errorf("rent listing #%d not found", id)
		}
		return ListingAdminNotifyInput{}, err
	}

	status := strings.TrimSpace(prop.Status)
	if status == "" {
		status = "pending"
	}

	return ListingAdminNotifyInput{
		Kind:         ListingKindRent,
		ID:           prop.ID,
		Title:        prop.Title,
		City:         prop.City,
		Price:        float64(prop.NightlyPrice),
		Currency:     prop.Currency,
		PropertyType: prop.PropertyType,
		HostUserID:   prop.HostID,
		HostEmail:    strings.TrimSpace(prop.Host.Email),
		Status:       status,
	}, nil
}

func loadLandNotifyInput(id uint) (ListingAdminNotifyInput, error) {
	var landmark models.Landmark
	if err := storage.DB.Preload("Owner").First(&landmark, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ListingAdminNotifyInput{}, fmt.Errorf("land listing #%d not found", id)
		}
		return ListingAdminNotifyInput{}, err
	}

	hostID := uint(0)
	hostEmail := ""
	if landmark.OwnerID != nil && *landmark.OwnerID > 0 {
		hostID = *landmark.OwnerID
	}
	if landmark.Owner != nil {
		hostEmail = strings.TrimSpace(landmark.Owner.Email)
	}

	landCity := strings.TrimSpace(landmark.District)
	if landCity == "" {
		landCity = strings.TrimSpace(landmark.Region)
	}

	return ListingAdminNotifyInput{
		Kind:         ListingKindLand,
		ID:           landmark.ID,
		Title:        landmark.Title,
		City:         landCity,
		Price:        landmark.Price,
		Currency:     landmark.Currency,
		PropertyType: landmark.LandType,
		HostUserID:   hostID,
		HostEmail:    hostEmail,
		Status:       landmark.Status,
	}, nil
}
