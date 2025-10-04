package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"math"
	"strconv"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// GetLocationCriteria returns all active location criteria
func GetLocationCriteria(ctx iris.Context) {
	var criteria []models.LocationCriteria

	// Get all active criteria ordered by priority
	if err := storage.DB.Where("is_active = ?", true).
		Order("priority DESC, name ASC").
		Find(&criteria).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch location criteria"})
		return
	}

	// Convert to response format with property counts
	var response []models.GetLocationCriteriaResponse
	for _, criterion := range criteria {
		// Count properties for this criteria
		var propertyCount int64
		storage.DB.Model(&models.LocationCriteriaProperty{}).
			Where("location_criteria_id = ? AND is_active = ?", criterion.ID, true).
			Count(&propertyCount)

		response = append(response, models.GetLocationCriteriaResponse{
			ID:            criterion.ID,
			Name:          criterion.Name,
			DisplayName:   criterion.DisplayName,
			Description:   criterion.Description,
			CenterLat:     criterion.CenterLat,
			CenterLng:     criterion.CenterLng,
			Radius:        criterion.Radius,
			Priority:      criterion.Priority,
			IsActive:      criterion.IsActive,
			Icon:          criterion.Icon,
			Color:         criterion.Color,
			PropertyCount: int(propertyCount),
		})
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}

// GetLocationProperties returns properties for a specific location criteria
func GetLocationProperties(ctx iris.Context) {
	criteriaIDStr := ctx.Params().Get("criteriaId")
	criteriaID, err := strconv.ParseUint(criteriaIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid criteria ID"})
		return
	}

	// Get limit from query params
	limitStr := ctx.URLParamDefault("limit", "8")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 8
	}

	// Get the location criteria
	var criteria models.LocationCriteria
	if err := storage.DB.Where("id = ? AND is_active = ?", criteriaID, true).First(&criteria).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Location criteria not found"})
		return
	}

	// Get properties assigned to this criteria (only active + approved/live)
	var criteriaProperties []models.LocationCriteriaProperty
	if err := storage.DB.Where("location_criteria_id = ? AND is_active = ?", criteriaID, true).
		Preload("Property", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active = ? AND status IN (?)", true, []string{"approved", "live"})
		}).
		Preload("Property.Host").
		Order("distance ASC").
		Limit(limit).
		Find(&criteriaProperties).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch properties"})
		return
	}

	// Extract properties
	var properties []models.Property
	for _, cp := range criteriaProperties {
		if cp.Property.ID != 0 { // Ensure property exists
			properties = append(properties, cp.Property)
		}
	}

	// Convert criteria to response format
	criteriaResponse := models.GetLocationCriteriaResponse{
		ID:            criteria.ID,
		Name:          criteria.Name,
		DisplayName:   criteria.DisplayName,
		Description:   criteria.Description,
		CenterLat:     criteria.CenterLat,
		CenterLng:     criteria.CenterLng,
		Radius:        criteria.Radius,
		Priority:      criteria.Priority,
		IsActive:      criteria.IsActive,
		Icon:          criteria.Icon,
		Color:         criteria.Color,
		PropertyCount: len(properties),
	}

	response := models.GetLocationPropertiesResponse{
		LocationCriteria: criteriaResponse,
		Properties:       properties,
		TotalCount:       len(properties),
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}

// InitializeLocationCriteriaEndpoint initializes location criteria via API
func InitializeLocationCriteriaEndpoint(ctx iris.Context) {
	// Initialize location criteria
	if err := InitializeLocationCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to initialize location criteria", "error": err.Error()})
		return
	}

	// Assign properties to criteria
	if err := AssignPropertiesToCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to assign properties to criteria", "error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Location criteria initialized successfully",
	})
}

// AssignPropertiesToCriteriaEndpoint assigns properties to criteria via API
func AssignPropertiesToCriteriaEndpoint(ctx iris.Context) {
	// Assign properties to criteria
	if err := AssignPropertiesToCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to assign properties to criteria", "error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Properties assigned to criteria successfully",
	})
}

// AssignSinglePropertyToLocationCriteria assigns a single property to appropriate location criteria
func AssignSinglePropertyToLocationCriteria(propertyID uint) error {
	// Get the property
	var property models.Property
	if err := storage.DB.Where("id = ? AND is_active = ?", propertyID, true).First(&property).Error; err != nil {
		return fmt.Errorf("property not found: %v", err)
	}

	// Get all active criteria ordered by priority (highest first)
	var criteria []models.LocationCriteria
	if err := storage.DB.Where("is_active = ?", true).Order("priority DESC").Find(&criteria).Error; err != nil {
		return fmt.Errorf("failed to fetch criteria: %v", err)
	}

	// Remove existing assignments for this property
	if err := storage.DB.Where("property_id = ?", propertyID).Delete(&models.LocationCriteriaProperty{}).Error; err != nil {
		return fmt.Errorf("failed to remove existing assignments: %v", err)
	}

	// Calculate distance and assign to appropriate criteria
	propertyAssigned := false
	for _, criterion := range criteria {
		distance := CalculateDistance(
			float64(property.Lat),
			float64(property.Lng),
			criterion.CenterLat,
			criterion.CenterLng,
		)

		if distance <= criterion.Radius {
			// Create assignment
			assignment := models.LocationCriteriaProperty{
				LocationCriteriaID: criterion.ID,
				PropertyID:         property.ID,
				Distance:           distance,
				IsActive:           true,
			}

			if err := storage.DB.Create(&assignment).Error; err != nil {
				return fmt.Errorf("failed to create assignment: %v", err)
			}

			propertyAssigned = true
			fmt.Printf("✅ Property %d assigned to criteria '%s' (distance: %.2fkm)\n",
				property.ID, criterion.Name, distance)
		}
	}

	if !propertyAssigned {
		fmt.Printf("⚠️ Property %d not assigned to any criteria (outside all radii)\n", property.ID)
	}

	return nil
}

// CalculateDistance calculates the distance between two points in kilometers
func CalculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// IsPointInCircle checks if a point is within a circle
func IsPointInCircle(pointLat, pointLng, centerLat, centerLng, radiusKm float64) bool {
	distance := CalculateDistance(pointLat, pointLng, centerLat, centerLng)
	return distance <= radiusKm
}

// AssignPropertiesToCriteria assigns properties to location criteria based on geographic boundaries
func AssignPropertiesToCriteria() error {
	// Get all active criteria ordered by priority (highest first)
	var criteria []models.LocationCriteria
	if err := storage.DB.Where("is_active = ?", true).Order("priority DESC").Find(&criteria).Error; err != nil {
		return err
	}

	// Get all active properties
	var properties []models.Property
	if err := storage.DB.Where("is_active = ?", true).Find(&properties).Error; err != nil {
		return err
	}

	fmt.Printf("Found %d active properties to assign\n", len(properties))
	fmt.Printf("Processing criteria in priority order:\n")
	for i, criterion := range criteria {
		fmt.Printf("  %d. %s (priority: %d, radius: %.1fkm)\n", i+1, criterion.Name, criterion.Priority, criterion.Radius)
	}

	// Clear existing assignments
	storage.DB.Where("1 = 1").Delete(&models.LocationCriteriaProperty{})

	// Track which properties have been assigned to avoid duplicates
	assignedProperties := make(map[uint]bool)

	// Process each criterion
	for _, criterion := range criteria {
		var assignedCount int

		for _, property := range properties {
			// Skip if property already assigned to another criteria
			if assignedProperties[property.ID] {
				continue
			}

			// Debug: Log property coordinates
			fmt.Printf("Checking property %d: lat=%.6f, lng=%.6f against criteria %s (center: %.6f, %.6f, radius: %.1fkm)\n",
				property.ID, property.Lat, property.Lng, criterion.Name, criterion.CenterLat, criterion.CenterLng, criterion.Radius)

			// Check if property is within the circle
			if IsPointInCircle(float64(property.Lat), float64(property.Lng), criterion.CenterLat, criterion.CenterLng, criterion.Radius) {
				// Calculate distance from center
				distance := CalculateDistance(float64(property.Lat), float64(property.Lng), criterion.CenterLat, criterion.CenterLng)

				// Create assignment
				assignment := models.LocationCriteriaProperty{
					LocationCriteriaID: criterion.ID,
					PropertyID:         property.ID,
					Distance:           distance,
					IsActive:           true,
				}

				if err := storage.DB.Create(&assignment).Error; err != nil {
					fmt.Printf("Error assigning property %d to criteria %d: %v\n", property.ID, criterion.ID, err)
					continue
				}

				// Mark property as assigned
				assignedProperties[property.ID] = true
				assignedCount++

				// Limit properties per criteria to avoid overcrowding
				if assignedCount >= 20 {
					break
				}
			}
		}

		fmt.Printf("Assigned %d properties to criteria '%s'\n", assignedCount, criterion.Name)
	}

	return nil
}

// InitializeLocationCriteria creates default location criteria for Nouakchott
func InitializeLocationCriteria() error {
	// Check if criteria already exist
	var count int64
	storage.DB.Model(&models.LocationCriteria{}).Count(&count)
	if count > 0 {
		return nil // Already initialized
	}

	// Define Nouakchott location criteria
	criteria := []models.LocationCriteria{
		{
			Name:        "tevragh_zeina",
			DisplayName: "Properties in Tevragh Zeina",
			Description: "Luxury stays in the diplomatic quarter",
			CenterLat:   18.0861,
			CenterLng:   -15.9753,
			Radius:      2.0, // 2km radius
			Priority:    10,
			IsActive:    true,
			Icon:        "business",
			Color:       "#00A699",
		},
		{
			Name:        "palais_congres",
			DisplayName: "Near Palais des Congrès",
			Description: "Properties near the convention center",
			CenterLat:   18.0844,
			CenterLng:   -15.9789,
			Radius:      1.5, // 1.5km radius
			Priority:    9,
			IsActive:    true,
			Icon:        "place",
			Color:       "#FF5A5F",
		},
		{
			Name:        "presidential_palace",
			DisplayName: "Near Presidential Palace",
			Description: "Properties near the presidential palace",
			CenterLat:   18.0922,
			CenterLng:   -15.9711,
			Radius:      1.8, // 1.8km radius
			Priority:    8,
			IsActive:    true,
			Icon:        "account_balance",
			Color:       "#C0C0C0",
		},
		{
			Name:        "port_nouakchott",
			DisplayName: "Port de Nouakchott",
			Description: "Properties near the port area",
			CenterLat:   18.0956,
			CenterLng:   -15.9889,
			Radius:      4.0, // 4.0km radius (expanded to include Property 8)
			Priority:    7,
			IsActive:    true,
			Icon:        "local_shipping",
			Color:       "#2196F3",
		},
		{
			Name:        "airport_area",
			DisplayName: "Near Airport",
			Description: "Properties near the international airport",
			CenterLat:   18.0978,
			CenterLng:   -15.9567,
			Radius:      3.0, // 3km radius
			Priority:    6,
			IsActive:    true,
			Icon:        "flight",
			Color:       "#4CAF50",
		},
		{
			Name:        "embassy_quarter",
			DisplayName: "Embassy Quarter",
			Description: "Luxury properties in the embassy district",
			CenterLat:   18.0889,
			CenterLng:   -15.9722,
			Radius:      1.2, // 1.2km radius
			Priority:    5,
			IsActive:    true,
			Icon:        "account_balance",
			Color:       "#9C27B0",
		},
		{
			Name:        "city_center",
			DisplayName: "City Center",
			Description: "Properties in the heart of Nouakchott",
			CenterLat:   18.0733,
			CenterLng:   -15.9589,
			Radius:      2.0, // 2km radius
			Priority:    4,
			IsActive:    true,
			Icon:        "location_city",
			Color:       "#FF9800",
		},
		{
			Name:        "beach_area",
			DisplayName: "Beach Area",
			Description: "Properties near the beach",
			CenterLat:   18.0667,
			CenterLng:   -15.9444,
			Radius:      2.2, // 2.2km radius
			Priority:    3,
			IsActive:    true,
			Icon:        "beach_access",
			Color:       "#00BCD4",
		},
	}

	// Create criteria
	for _, criterion := range criteria {
		if err := storage.DB.Create(&criterion).Error; err != nil {
			return fmt.Errorf("error creating criteria %s: %v", criterion.Name, err)
		}
	}

	fmt.Println("Location criteria initialized successfully")
	return nil
}

func GetPropertyLocationCriteria(ctx iris.Context) {
	propertyID := ctx.Params().GetUintDefault("propertyId", 0)
	if propertyID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	// Get the property
	var property models.Property
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not found"})
		return
	}

	// Find which criteria this property belongs to
	var criteriaProperty models.LocationCriteriaProperty
	if err := storage.DB.Where("property_id = ?", propertyID).
		Preload("LocationCriteria").
		First(&criteriaProperty).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not assigned to any location criteria"})
		return
	}

	// Calculate distance from property to criteria center
	distance := CalculateDistance(
		float64(property.Lat),
		float64(property.Lng),
		criteriaProperty.LocationCriteria.CenterLat,
		criteriaProperty.LocationCriteria.CenterLng,
	)

	response := iris.Map{
		"id":          criteriaProperty.LocationCriteria.ID,
		"name":        criteriaProperty.LocationCriteria.Name,
		"displayName": criteriaProperty.LocationCriteria.DisplayName,
		"centerLat":   criteriaProperty.LocationCriteria.CenterLat,
		"centerLng":   criteriaProperty.LocationCriteria.CenterLng,
		"radius":      criteriaProperty.LocationCriteria.Radius,
		"distance":    distance,
		"icon":        criteriaProperty.LocationCriteria.Icon,
		"color":       criteriaProperty.LocationCriteria.Color,
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}
