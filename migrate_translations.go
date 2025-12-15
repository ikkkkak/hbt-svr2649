package main

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// NOTE: This is a one-off migration tool that can be run with:
//   go run migrate_translations.go
// It fills missing translation JSON fields for existing properties, property sales, and landmarks.

func migrateProperties() error {
	var props []models.Property
	if err := storage.DB.Find(&props).Error; err != nil {
		return err
	}
	for _, p := range props {
		// Skip if we already have translations
		if len(p.TitleTranslations) > 0 && len(p.DescriptionTranslations) > 0 {
			continue
		}

		titleMap := services.TranslateAllLanguages(p.Title)
		descMap := services.TranslateAllLanguages(p.Description)
		neighMap := services.TranslateAllLanguages(p.NeighborhoodDescription)

		titleJSON, _ := json.Marshal(titleMap)
		descJSON, _ := json.Marshal(descMap)
		neighJSON, _ := json.Marshal(neighMap)

		if err := storage.DB.Model(&models.Property{}).
			Where("id = ?", p.ID).
			Updates(map[string]interface{}{
				"title_translations":                       titleJSON,
				"description_translations":                 descJSON,
				"neighborhood_description_translations":    neighJSON,
			}).Error; err != nil {
			log.Printf("property %d migration error: %v\n", p.ID, err)
		}

		time.Sleep(150 * time.Millisecond)
	}
	return nil
}

func migratePropertySales() error {
	var sales []models.PropertySale
	if err := storage.DB.Find(&sales).Error; err != nil {
		return err
	}
	for _, s := range sales {
		if len(s.TitleTranslations) > 0 && len(s.DescriptionTranslations) > 0 {
			continue
		}
		titleMap := services.TranslateAllLanguages(s.Title)
		descMap := services.TranslateAllLanguages(s.Description)

		titleJSON, _ := json.Marshal(titleMap)
		descJSON, _ := json.Marshal(descMap)

		if err := storage.DB.Model(&models.PropertySale{}).
			Where("id = ?", s.ID).
			Updates(map[string]interface{}{
				"title_translations":       titleJSON,
				"description_translations": descJSON,
			}).Error; err != nil {
			log.Printf("property_sale %d migration error: %v\n", s.ID, err)
		}

		time.Sleep(150 * time.Millisecond)
	}
	return nil
}

func migrateLandmarks() error {
	var lms []models.Landmark
	if err := storage.DB.Find(&lms).Error; err != nil {
		return err
	}
	for _, l := range lms {
		if len(l.TitleTranslations) > 0 && len(l.DescriptionTranslations) > 0 {
			continue
		}
		titleMap := services.TranslateAllLanguages(l.Title)
		descMap := services.TranslateAllLanguages(l.Description)

		titleJSON, _ := json.Marshal(titleMap)
		descJSON, _ := json.Marshal(descMap)

		if err := storage.DB.Model(&models.Landmark{}).
			Where("id = ?", l.ID).
			Updates(map[string]interface{}{
				"title_translations":       titleJSON,
				"description_translations": descJSON,
			}).Error; err != nil {
			log.Printf("landmark %d migration error: %v\n", l.ID, err)
		}

		time.Sleep(150 * time.Millisecond)
	}
	return nil
}

func main() {
	fmt.Println("🚀 Starting translations migration...")

	if err := migrateProperties(); err != nil {
		log.Fatalf("migrateProperties failed: %v", err)
	}
	if err := migratePropertySales(); err != nil {
		log.Fatalf("migratePropertySales failed: %v", err)
	}
	if err := migrateLandmarks(); err != nil {
		log.Fatalf("migrateLandmarks failed: %v", err)
	}

	fmt.Println("✅ Translation migration completed successfully.")
}


