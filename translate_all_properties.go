package main

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/datatypes"
)

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func testLibreTranslate() bool {
	log.Println("🧪 Testing LibreTranslate API connection...")

	libreTranslateURL := os.Getenv("LIBRETRANSLATE_URL")
	if libreTranslateURL == "" {
		libreTranslateURL = "https://librerender.onrender.com/translate"
	}

	log.Printf("📍 Testing URL: %s", libreTranslateURL)

	// First, test a direct API call to French to see what we get
	log.Println("🔍 Making direct test call to French translation...")
	testText := "Hello world"
	frenchResult, err := services.TranslateOnceDirect(testText, "fr")
	if err != nil {
		log.Printf("❌ Direct API test failed: %v", err)
		log.Println("   This suggests the API might not be working correctly")
	} else {
		log.Printf("📝 Direct test result (EN->FR): '%s' -> '%s'", testText, frenchResult)
		if frenchResult == testText {
			log.Println("⚠️  WARNING: French translation returned same text!")
		}
	}

	// Test with a simple English phrase that should translate differently
	testText = "Beautiful apartment in the city center"
	testResult := services.TranslateAllLanguages(testText)

	if len(testResult) == 0 {
		log.Println("❌ LibreTranslate test failed: No translations returned")
		log.Println("   Check if LibreTranslate is running and accessible")
		return false
	}

	log.Println("📊 Test translation results:")
	allSame := true
	uniqueCount := 0
	uniqueTranslations := make(map[string]bool)

	for lang, trans := range testResult {
		uniqueTranslations[trans] = true
		if trans == testText {
			log.Printf("  %s: %s (SAME AS ORIGINAL)", lang, trans)
		} else {
			log.Printf("  %s: %s ✅", lang, trans)
			allSame = false
		}
	}

	uniqueCount = len(uniqueTranslations)
	log.Printf("📈 Unique translations: %d", uniqueCount)

	if allSame {
		log.Println("❌ ERROR: All translations returned the same text!")
		log.Println("   Possible issues:")
		log.Println("   1. LibreTranslate API is not working")
		log.Println("   2. The API URL is incorrect: " + libreTranslateURL)
		log.Println("   3. The API is not accessible from this machine")
		log.Println("   4. Check if LibreTranslate service is running")
		return false
	}

	if uniqueCount < 2 {
		log.Println("⚠️  WARNING: Only 1 unique translation - API might not be working correctly")
		return false
	}

	log.Println("✅ LibreTranslate test passed! API is working correctly.")
	return true
}

func fm() {
	log.Println("🚀 Starting translation script for all properties...")

	// Initialize database connection
	db := storage.InitializeDB()
	if db == nil {
		log.Fatal("❌ Failed to connect to database")
	}

	// Set LibreTranslate URL from environment or use default
	libreTranslateURL := os.Getenv("LIBRETRANSLATE_URL")
	if libreTranslateURL == "" {
		libreTranslateURL = "https://librerender.onrender.com/translate"
		log.Printf("⚠️  LIBRETRANSLATE_URL not set, using default: %s", libreTranslateURL)
	} else {
		log.Printf("✅ Using LibreTranslate URL: %s", libreTranslateURL)
	}

	// Test LibreTranslate before proceeding
	if !testLibreTranslate() {
		log.Println("\n❌ LibreTranslate test failed. Please check:")
		log.Println("   1. Is LibreTranslate running?")
		log.Println("   2. Is LIBRETRANSLATE_URL correct?")
		log.Println("   3. Can you access the API from this machine?")
		log.Fatal("Exiting to prevent storing incorrect translations.")
	}

	log.Println("\n" + strings.Repeat("-", 60))

	// Translate Properties (rent properties)
	log.Println("\n📦 Processing Properties (Rent)...")
	var properties []models.Property
	if err := db.Find(&properties).Error; err != nil {
		log.Fatalf("❌ Failed to fetch properties: %v", err)
	}

	log.Printf("Found %d properties to process", len(properties))
	propertyCount := 0
	propertyErrorCount := 0

	for i, prop := range properties {
		log.Printf("\n[%d/%d] Processing Property ID: %d", i+1, len(properties), prop.ID)

		updated := false

		// Translate Title
		if prop.Title != "" {
			needsTranslation := false
			var titleTranslations map[string]string

			// Check if translations exist and are valid
			if prop.TitleTranslations != nil && len(prop.TitleTranslations) > 0 {
				if err := json.Unmarshal(prop.TitleTranslations, &titleTranslations); err != nil {
					needsTranslation = true
				} else {
					// Check if all languages are present
					if titleTranslations["en"] == "" || titleTranslations["fr"] == "" || titleTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating title: %s", prop.Title)
				titleTranslations = services.TranslateAllLanguages(prop.Title)

				// Verify we got translations
				if len(titleTranslations) == 0 {
					log.Printf("  ❌ No title translations returned! Skipping title...")
				} else {
					log.Printf("  📊 Translation results:")
					uniqueTranslations := make(map[string]bool)
					translatedCount := 0

					for lang, trans := range titleTranslations {
						uniqueTranslations[trans] = true
						if trans == prop.Title {
							log.Printf("    ⚠️  %s: SAME AS ORIGINAL (expected if source is %s)", lang, lang)
						} else {
							log.Printf("    ✅ %s: %s", lang, trans[:min(60, len(trans))])
							translatedCount++
						}
					}

					// We need at least 2 unique translations (e.g., if source is English, we should get French and Arabic)
					if len(uniqueTranslations) < 2 {
						log.Printf("  ❌ ERROR: Only %d unique translation(s) - API might not be working correctly!", len(uniqueTranslations))
						log.Printf("  ⚠️  Skipping title translation to avoid storing incorrect data")
					} else if translatedCount == 0 {
						log.Printf("  ⚠️  WARNING: All translations match original - might be OK if source is multilingual")
						// Still save it - might be legitimate
						titleJSON, _ := json.Marshal(titleTranslations)
						prop.TitleTranslations = datatypes.JSON(titleJSON)
						updated = true
					} else {
						log.Printf("  ✅ Got %d unique translations, %d different from original", len(uniqueTranslations), translatedCount)
						titleJSON, _ := json.Marshal(titleTranslations)
						prop.TitleTranslations = datatypes.JSON(titleJSON)
						updated = true
					}
				}
				time.Sleep(200 * time.Millisecond) // Be gentle with the API
			}
		}

		// Translate Description
		if prop.Description != "" {
			needsTranslation := false
			var descTranslations map[string]string

			if prop.DescriptionTranslations != nil && len(prop.DescriptionTranslations) > 0 {
				if err := json.Unmarshal(prop.DescriptionTranslations, &descTranslations); err != nil {
					needsTranslation = true
				} else {
					if descTranslations["en"] == "" || descTranslations["fr"] == "" || descTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating description (length: %d chars)...", len(prop.Description))
				descTranslations = services.TranslateAllLanguages(prop.Description)

				// Verify translations
				if len(descTranslations) == 0 {
					log.Printf("  ❌ No description translations returned! Skipping description...")
				} else {
					uniqueTranslations := make(map[string]bool)
					translatedCount := 0

					for _, trans := range descTranslations {
						uniqueTranslations[trans] = true
						if trans != prop.Description {
							translatedCount++
						}
					}

					if len(uniqueTranslations) < 2 {
						log.Printf("  ❌ ERROR: Only %d unique translation(s) - API might not be working!", len(uniqueTranslations))
						log.Printf("  ⚠️  Skipping description translation")
					} else {
						log.Printf("  📊 Description: %d unique translations, %d different from original", len(uniqueTranslations), translatedCount)
						descJSON, _ := json.Marshal(descTranslations)
						prop.DescriptionTranslations = datatypes.JSON(descJSON)
						updated = true
					}
				}
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Translate Neighborhood Description
		if prop.NeighborhoodDescription != "" {
			needsTranslation := false
			var neighborhoodTranslations map[string]string

			if prop.NeighborhoodDescriptionTranslations != nil && len(prop.NeighborhoodDescriptionTranslations) > 0 {
				if err := json.Unmarshal(prop.NeighborhoodDescriptionTranslations, &neighborhoodTranslations); err != nil {
					needsTranslation = true
				} else {
					if neighborhoodTranslations["en"] == "" || neighborhoodTranslations["fr"] == "" || neighborhoodTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating neighborhood description...")
				neighborhoodTranslations = services.TranslateAllLanguages(prop.NeighborhoodDescription)

				// Verify translations
				if len(neighborhoodTranslations) == 0 {
					log.Printf("  ❌ No neighborhood translations returned! Skipping...")
				} else {
					uniqueTranslations := make(map[string]bool)
					translatedCount := 0

					for _, trans := range neighborhoodTranslations {
						uniqueTranslations[trans] = true
						if trans != prop.NeighborhoodDescription {
							translatedCount++
						}
					}

					if len(uniqueTranslations) < 2 {
						log.Printf("  ❌ ERROR: Only %d unique translation(s) - API might not be working!", len(uniqueTranslations))
						log.Printf("  ⚠️  Skipping neighborhood translation")
					} else {
						log.Printf("  📊 Neighborhood: %d unique translations, %d different from original", len(uniqueTranslations), translatedCount)
						neighborhoodJSON, _ := json.Marshal(neighborhoodTranslations)
						prop.NeighborhoodDescriptionTranslations = datatypes.JSON(neighborhoodJSON)
						updated = true
					}
				}
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Update database if translations were added
		if updated {
			if err := db.Save(&prop).Error; err != nil {
				log.Printf("  ❌ Failed to update property %d: %v", prop.ID, err)
				propertyErrorCount++
			} else {
				log.Printf("  ✅ Updated property %d with translations", prop.ID)
				propertyCount++
			}
		} else {
			log.Printf("  ⏭️  Property %d already has all translations", prop.ID)
		}
	}

	// Translate Property Sales
	log.Println("\n🏠 Processing Property Sales...")
	var propertySales []models.PropertySale
	if err := db.Find(&propertySales).Error; err != nil {
		log.Fatalf("❌ Failed to fetch property sales: %v", err)
	}

	log.Printf("Found %d property sales to process", len(propertySales))
	saleCount := 0
	saleErrorCount := 0

	for i, sale := range propertySales {
		log.Printf("\n[%d/%d] Processing Property Sale ID: %d", i+1, len(propertySales), sale.ID)

		updated := false

		// Translate Title
		if sale.Title != "" {
			needsTranslation := false
			var titleTranslations map[string]string

			if sale.TitleTranslations != nil && len(sale.TitleTranslations) > 0 {
				if err := json.Unmarshal(sale.TitleTranslations, &titleTranslations); err != nil {
					needsTranslation = true
				} else {
					if titleTranslations["en"] == "" || titleTranslations["fr"] == "" || titleTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating title: %s", sale.Title)
				titleTranslations = services.TranslateAllLanguages(sale.Title)
				titleJSON, _ := json.Marshal(titleTranslations)
				sale.TitleTranslations = datatypes.JSON(titleJSON)
				updated = true
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Translate Description
		if sale.Description != "" {
			needsTranslation := false
			var descTranslations map[string]string

			if sale.DescriptionTranslations != nil && len(sale.DescriptionTranslations) > 0 {
				if err := json.Unmarshal(sale.DescriptionTranslations, &descTranslations); err != nil {
					needsTranslation = true
				} else {
					if descTranslations["en"] == "" || descTranslations["fr"] == "" || descTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating description...")
				descTranslations = services.TranslateAllLanguages(sale.Description)
				descJSON, _ := json.Marshal(descTranslations)
				sale.DescriptionTranslations = datatypes.JSON(descJSON)
				updated = true
				time.Sleep(200 * time.Millisecond)
			}
		}

		if updated {
			if err := db.Save(&sale).Error; err != nil {
				log.Printf("  ❌ Failed to update property sale %d: %v", sale.ID, err)
				saleErrorCount++
			} else {
				log.Printf("  ✅ Updated property sale %d with translations", sale.ID)
				saleCount++
			}
		} else {
			log.Printf("  ⏭️  Property sale %d already has all translations", sale.ID)
		}
	}

	// Translate Landmarks
	log.Println("\n🗺️  Processing Landmarks...")
	var landmarks []models.Landmark
	if err := db.Find(&landmarks).Error; err != nil {
		log.Fatalf("❌ Failed to fetch landmarks: %v", err)
	}

	log.Printf("Found %d landmarks to process", len(landmarks))
	landmarkCount := 0
	landmarkErrorCount := 0

	for i, landmark := range landmarks {
		log.Printf("\n[%d/%d] Processing Landmark ID: %d", i+1, len(landmarks), landmark.ID)

		updated := false

		// Translate Title
		if landmark.Title != "" {
			needsTranslation := false
			var titleTranslations map[string]string

			if landmark.TitleTranslations != nil && len(landmark.TitleTranslations) > 0 {
				if err := json.Unmarshal(landmark.TitleTranslations, &titleTranslations); err != nil {
					needsTranslation = true
				} else {
					if titleTranslations["en"] == "" || titleTranslations["fr"] == "" || titleTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating title: %s", landmark.Title)
				titleTranslations = services.TranslateAllLanguages(landmark.Title)
				titleJSON, _ := json.Marshal(titleTranslations)
				landmark.TitleTranslations = datatypes.JSON(titleJSON)
				updated = true
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Translate Description
		if landmark.Description != "" {
			needsTranslation := false
			var descTranslations map[string]string

			if landmark.DescriptionTranslations != nil && len(landmark.DescriptionTranslations) > 0 {
				if err := json.Unmarshal(landmark.DescriptionTranslations, &descTranslations); err != nil {
					needsTranslation = true
				} else {
					if descTranslations["en"] == "" || descTranslations["fr"] == "" || descTranslations["ar"] == "" {
						needsTranslation = true
					}
				}
			} else {
				needsTranslation = true
			}

			if needsTranslation {
				log.Printf("  📝 Translating description...")
				descTranslations = services.TranslateAllLanguages(landmark.Description)
				descJSON, _ := json.Marshal(descTranslations)
				landmark.DescriptionTranslations = datatypes.JSON(descJSON)
				updated = true
				time.Sleep(200 * time.Millisecond)
			}
		}

		if updated {
			if err := db.Save(&landmark).Error; err != nil {
				log.Printf("  ❌ Failed to update landmark %d: %v", landmark.ID, err)
				landmarkErrorCount++
			} else {
				log.Printf("  ✅ Updated landmark %d with translations", landmark.ID)
				landmarkCount++
			}
		} else {
			log.Printf("  ⏭️  Landmark %d already has all translations", landmark.ID)
		}
	}

	// Summary
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("📊 TRANSLATION SUMMARY")
	log.Println(strings.Repeat("=", 60))
	log.Printf("✅ Properties updated: %d", propertyCount)
	log.Printf("❌ Properties errors: %d", propertyErrorCount)
	log.Printf("✅ Property Sales updated: %d", saleCount)
	log.Printf("❌ Property Sales errors: %d", saleErrorCount)
	log.Printf("✅ Landmarks updated: %d", landmarkCount)
	log.Printf("❌ Landmarks errors: %d", landmarkErrorCount)
	log.Println(strings.Repeat("=", 60))
	log.Println("🎉 Translation script completed!")
}
