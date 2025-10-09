package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
)

// CreateOrganization creates a new organization
func CreateOrganization(ctx iris.Context) {
	var input struct {
		Name          string `json:"name" validate:"required"`
		Description   string `json:"description"`
		BannerImage   string `json:"banner_image"`
		Website       string `json:"website"`
		Phone         string `json:"phone"`
		Email         string `json:"email"`
		Address       string `json:"address"`
		City          string `json:"city"`
		State         string `json:"state"`
		Country       string `json:"country"`
		PostalCode    string `json:"postal_code"`
		LicenseNumber string `json:"license_number"`
		TaxID         string `json:"tax_id"`
		BusinessType  string `json:"business_type"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	// Get user ID from token
	userIDInterface := ctx.Values().Get("userID")
	fmt.Printf("🔍 DEBUG: userIDInterface = %v (type: %T)\n", userIDInterface, userIDInterface)

	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User ID not found in token"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user ID format"})
		return
	}

	fmt.Printf("🔍 DEBUG: userID = %d\n", userID)

	// Check if user already has an organization
	var existingOrg models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&existingOrg).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "User already has an organization"})
		return
	}

	// Create organization
	organization := models.Organization{
		Name:          input.Name,
		Description:   input.Description,
		BannerImage:   input.BannerImage,
		Website:       input.Website,
		Phone:         input.Phone,
		Email:         input.Email,
		Address:       input.Address,
		City:          input.City,
		State:         input.State,
		Country:       input.Country,
		PostalCode:    input.PostalCode,
		LicenseNumber: input.LicenseNumber,
		TaxID:         input.TaxID,
		BusinessType:  input.BusinessType,
		OwnerID:       userID,
		Status:        "pending",
		IsActive:      true,
	}

	if err := storage.DB.Create(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create organization"})
		return
	}

	// Create the owner as the first agent
	agent := models.Agent{
		UserID:         userID,
		OrganizationID: organization.ID,
		Status:         "approved", // Owner is automatically approved
		IsActive:       true,
	}

	if err := storage.DB.Create(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create owner agent"})
		return
	}

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message":      "Organization created successfully",
		"organization": organization,
	})
}

// GetUserOrganization gets the user's organization
func GetUserOrganization(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var organization models.Organization
	if err := storage.DB.Preload("Owner").Preload("Agents.User").Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	ctx.JSON(iris.Map{"organization": organization})
}

// UpdateOrganization updates an organization
func UpdateOrganization(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	var input struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Website       string `json:"website"`
		Phone         string `json:"phone"`
		Email         string `json:"email"`
		Address       string `json:"address"`
		City          string `json:"city"`
		State         string `json:"state"`
		Country       string `json:"country"`
		PostalCode    string `json:"postal_code"`
		LicenseNumber string `json:"license_number"`
		TaxID         string `json:"tax_id"`
		BusinessType  string `json:"business_type"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Update fields
	if input.Name != "" {
		organization.Name = input.Name
	}
	if input.Description != "" {
		organization.Description = input.Description
	}
	if input.Website != "" {
		organization.Website = input.Website
	}
	if input.Phone != "" {
		organization.Phone = input.Phone
	}
	if input.Email != "" {
		organization.Email = input.Email
	}
	if input.Address != "" {
		organization.Address = input.Address
	}
	if input.City != "" {
		organization.City = input.City
	}
	if input.State != "" {
		organization.State = input.State
	}
	if input.Country != "" {
		organization.Country = input.Country
	}
	if input.PostalCode != "" {
		organization.PostalCode = input.PostalCode
	}
	if input.LicenseNumber != "" {
		organization.LicenseNumber = input.LicenseNumber
	}
	if input.TaxID != "" {
		organization.TaxID = input.TaxID
	}
	if input.BusinessType != "" {
		organization.BusinessType = input.BusinessType
	}

	if err := storage.DB.Save(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update organization"})
		return
	}

	ctx.JSON(iris.Map{
		"message":      "Organization updated successfully",
		"organization": organization,
	})
}

// GetOrganizationAgents gets all agents for an organization
func GetOrganizationAgents(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	var agents []models.Agent
	if err := storage.DB.Preload("User").Where("organization_id = ?", organization.ID).Find(&agents).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch agents"})
		return
	}

	ctx.JSON(iris.Map{"agents": agents})
}

// AddAgent adds a new agent to the organization
func AddAgent(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	var input struct {
		UserID         uint     `json:"user_id" validate:"required"`
		LicenseNumber  string   `json:"license_number"`
		Specialization string   `json:"specialization"`
		Experience     int      `json:"experience"`
		Bio            string   `json:"bio"`
		Languages      []string `json:"languages"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	// Check if user is already an agent
	var existingAgent models.Agent
	if err := storage.DB.Where("user_id = ?", input.UserID).First(&existingAgent).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "User is already an agent"})
		return
	}

	// Create agent
	agent := models.Agent{
		UserID:         input.UserID,
		OrganizationID: organization.ID,
		LicenseNumber:  input.LicenseNumber,
		Specialization: input.Specialization,
		Experience:     input.Experience,
		Bio:            input.Bio,
		Languages:      input.Languages,
		Status:         "pending",
		IsActive:       true,
	}

	if err := storage.DB.Create(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to add agent"})
		return
	}

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message": "Agent added successfully",
		"agent":   agent,
	})
}

// UpdateAgentStatus updates an agent's status
func UpdateAgentStatus(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	agentID, _ := strconv.ParseUint(ctx.Params().Get("agentID"), 10, 32)

	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	var agent models.Agent
	if err := storage.DB.Where("id = ? AND organization_id = ?", agentID, organization.ID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Agent not found"})
		return
	}

	var input struct {
		Status string `json:"status" validate:"required,oneof=pending approved rejected suspended"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	agent.Status = input.Status
	if err := storage.DB.Save(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update agent status"})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Agent status updated successfully",
		"agent":   agent,
	})
}

// AdminGetOrganizations gets all organizations (admin only)
func AdminGetOrganizations(ctx iris.Context) {
	var organizations []models.Organization
	if err := storage.DB.Preload("Owner").Preload("Agents.User").Find(&organizations).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch organizations"})
		return
	}

	ctx.JSON(iris.Map{"organizations": organizations})
}

// AdminUpdateOrganizationStatus updates organization status (admin only)
func AdminUpdateOrganizationStatus(ctx iris.Context) {
	orgID, _ := strconv.ParseUint(ctx.Params().Get("orgID"), 10, 32)

	var organization models.Organization
	if err := storage.DB.First(&organization, orgID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	var input struct {
		Status string `json:"status" validate:"required,oneof=pending approved rejected suspended"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	organization.Status = input.Status
	if err := storage.DB.Save(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update organization status"})
		return
	}

	ctx.JSON(iris.Map{
		"message":      "Organization status updated successfully",
		"organization": organization,
	})
}
