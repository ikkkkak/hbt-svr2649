package routes

import (
	"apartments-clone-server/models"
	orgmember "apartments-clone-server/services"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

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

	// Check if userID is 0 (invalid)
	if userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user ID: user ID cannot be 0"})
		return
	}

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

	// Create the owner as the first organization member with admin role
	adminPermissions := models.GetDefaultPermissions("admin")
	permissionsJSON, err := json.Marshal(adminPermissions)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create owner member"})
		return
	}

	member := models.OrganizationMember{
		UserID:         userID,
		OrganizationID: organization.ID,
		Role:           "admin", // Owner is admin by default
		Permissions:    permissionsJSON,
		Status:         "active",
		IsActive:       true,
		JoinedAt:       time.Now(),
	}

	if err := storage.DB.Create(&member).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create owner member"})
		return
	}

	// Also create as Agent for backward compatibility
	agent := models.Agent{
		UserID:         userID,
		OrganizationID: organization.ID,
		Status:         "approved", // Owner is automatically approved
		IsActive:       true,
	}

	if err := storage.DB.Create(&agent).Error; err != nil {
		// Log but don't fail - member creation is more important
		log.Printf("⚠️ Failed to create owner agent (non-critical): %v", err)
	}

	// Log audit event
	orgmember.LogOrganizationAudit(organization.ID, userID, models.ActionOrgSettingsUpdated, models.ActionTypeOrganizationSettings, "organization", &organization.ID, "Organization created", "", "")

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message":      "Organization created successfully",
		"organization": organization,
	})
}

// GetUserOrganization gets the user's organization (as owner or member)
func GetUserOrganization(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// First check if user is owner
	var organization models.Organization
	if err := storage.DB.Preload("Owner").Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		// If not owner, check if user is a member using helper function
		org, _, _ := orgmember.GetUserOrganization(userID)
		if org == nil {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Organization not found"})
			return
		}
		organization = *org
		// Owner preload for consistent payload shape
		storage.DB.Preload("Owner").First(&organization, organization.ID)
	}

	// Helper function to get days left until next edit
	daysLeftUntilEdit := func(lastEdit *time.Time) int {
		if lastEdit == nil {
			return 0 // Can edit now
		}
		daysSinceEdit := time.Since(*lastEdit).Hours() / 24
		daysLeft := 30 - int(daysSinceEdit)
		if daysLeft < 0 {
			return 0
		}
		return daysLeft
	}

	// Calculate days left for each restricted field
	daysLeftMap := map[string]int{
		"name":          daysLeftUntilEdit(organization.LastNameEdit),
		"description":   daysLeftUntilEdit(organization.LastDescriptionEdit),
		"business_type": daysLeftUntilEdit(organization.LastBusinessTypeEdit),
		"banner_image":  daysLeftUntilEdit(organization.LastBannerEdit),
		"logo":          daysLeftUntilEdit(organization.LastLogoEdit),
	}

	ctx.JSON(iris.Map{
		"organization": organization,
		"days_left":    daysLeftMap,
	})
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
		BannerImage   string `json:"banner_image"`
		Logo          string `json:"logo"`
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

	// Helper function to check if field can be edited (30 day cooldown)
	canEditField := func(lastEdit *time.Time) bool {
		if lastEdit == nil {
			return true // Never edited, can edit
		}
		daysSinceEdit := time.Since(*lastEdit).Hours() / 24
		return daysSinceEdit >= 30
	}

	// Helper function to get days left until next edit
	daysLeftUntilEdit := func(lastEdit *time.Time) int {
		if lastEdit == nil {
			return 0 // Can edit now
		}
		daysSinceEdit := time.Since(*lastEdit).Hours() / 24
		daysLeft := 30 - int(daysSinceEdit)
		if daysLeft < 0 {
			return 0
		}
		return daysLeft
	}

	now := time.Now()
	editRestrictions := make(map[string]interface{})

	// Update Name with 30-day cooldown check
	if input.Name != "" && input.Name != organization.Name {
		if !canEditField(organization.LastNameEdit) {
			daysLeft := daysLeftUntilEdit(organization.LastNameEdit)
			editRestrictions["name"] = map[string]interface{}{
				"can_edit": false,
				"days_left": daysLeft,
				"message": fmt.Sprintf("Name can only be changed once every 30 days. %d days remaining.", daysLeft),
			}
		} else {
			organization.Name = input.Name
			organization.LastNameEdit = &now
		}
	}

	// Update Description with 30-day cooldown check
	if input.Description != "" && input.Description != organization.Description {
		if !canEditField(organization.LastDescriptionEdit) {
			daysLeft := daysLeftUntilEdit(organization.LastDescriptionEdit)
			editRestrictions["description"] = map[string]interface{}{
				"can_edit": false,
				"days_left": daysLeft,
				"message": fmt.Sprintf("Description can only be changed once every 30 days. %d days remaining.", daysLeft),
			}
		} else {
			organization.Description = input.Description
			organization.LastDescriptionEdit = &now
		}
	}

	// Update BusinessType with 30-day cooldown check
	if input.BusinessType != "" && input.BusinessType != organization.BusinessType {
		if !canEditField(organization.LastBusinessTypeEdit) {
			daysLeft := daysLeftUntilEdit(organization.LastBusinessTypeEdit)
			editRestrictions["business_type"] = map[string]interface{}{
				"can_edit": false,
				"days_left": daysLeft,
				"message": fmt.Sprintf("Business type can only be changed once every 30 days. %d days remaining.", daysLeft),
			}
		} else {
			organization.BusinessType = input.BusinessType
			organization.LastBusinessTypeEdit = &now
		}
	}

	// Update BannerImage with 30-day cooldown check
	if input.BannerImage != "" && input.BannerImage != organization.BannerImage {
		if !canEditField(organization.LastBannerEdit) {
			daysLeft := daysLeftUntilEdit(organization.LastBannerEdit)
			editRestrictions["banner_image"] = map[string]interface{}{
				"can_edit": false,
				"days_left": daysLeft,
				"message": fmt.Sprintf("Banner can only be changed once every 30 days. %d days remaining.", daysLeft),
			}
		} else {
			organization.BannerImage = input.BannerImage
			organization.LastBannerEdit = &now
		}
	}

	// Update Logo with 30-day cooldown check
	if input.Logo != "" && input.Logo != organization.Logo {
		if !canEditField(organization.LastLogoEdit) {
			daysLeft := daysLeftUntilEdit(organization.LastLogoEdit)
			editRestrictions["logo"] = map[string]interface{}{
				"can_edit": false,
				"days_left": daysLeft,
				"message": fmt.Sprintf("Logo can only be changed once every 30 days. %d days remaining.", daysLeft),
			}
		} else {
			organization.Logo = input.Logo
			organization.LastLogoEdit = &now
		}
	}

	// Update other fields (no restrictions)
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

	if err := storage.DB.Save(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update organization"})
		return
	}

	// Calculate days left for each restricted field
	daysLeftMap := map[string]int{
		"name":         daysLeftUntilEdit(organization.LastNameEdit),
		"description":  daysLeftUntilEdit(organization.LastDescriptionEdit),
		"business_type": daysLeftUntilEdit(organization.LastBusinessTypeEdit),
		"banner_image": daysLeftUntilEdit(organization.LastBannerEdit),
		"logo":         daysLeftUntilEdit(organization.LastLogoEdit),
	}

	response := iris.Map{
		"message":      "Organization updated successfully",
		"organization": organization,
		"edit_restrictions": editRestrictions,
		"days_left":    daysLeftMap,
	}

	// If there are restrictions, return 200 but include warnings
	if len(editRestrictions) > 0 {
		response["warnings"] = editRestrictions
	}

	ctx.JSON(response)
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

var mrNationalDigitsRe = regexp.MustCompile(`^\d{8}$`)

// canonicalMauritanianPhone normalizes a Mauritania (+222) mobile to "+222" + 8 digits.
func canonicalMauritanianPhone(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimPrefix(s, "+")
	switch {
	case len(s) == 8 && mrNationalDigitsRe.MatchString(s):
		return "+222" + s, true
	case len(s) == 11 && strings.HasPrefix(s, "222") && mrNationalDigitsRe.MatchString(s[3:]):
		return "+" + s, true
	default:
		return "", false
	}
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

// AdminCreateOrganization creates an organization (admin only). Bypasses the one-org-per-user
// rule used by the public CreateOrganization flow. Multiple orgs may share the same owner_user_id
// (e.g. admin as technical owner for agency records).
func AdminCreateOrganization(ctx iris.Context) {
	adminUID, ok := ctx.Values().Get("userID").(uint)
	if !ok || adminUID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User ID not found in token"})
		return
	}

	var input struct {
		Name          string `json:"name" validate:"required"`
		Description   string `json:"description"`
		Logo          string `json:"logo"`
		BannerImage   string `json:"banner_image"`
		Website       string `json:"website"`
		Phone         string `json:"phone" validate:"required"`
		Email         string `json:"email" validate:"omitempty,email"`
		Address       string `json:"address"`
		City          string `json:"city"`
		State         string `json:"state"`
		Country       string `json:"country"`
		PostalCode    string `json:"postal_code"`
		LicenseNumber string `json:"license_number"`
		TaxID         string `json:"tax_id"`
		BusinessType  string `json:"business_type"`
		OwnerUserID   *uint  `json:"owner_user_id"` // optional; defaults to admin
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	phoneCanon, phoneOK := canonicalMauritanianPhone(input.Phone)
	if !phoneOK {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{
			"error": "Invalid Mauritanian phone: use 8 digits (e.g. 45123456) or +22245123456",
		})
		return
	}

	ownerID := adminUID
	if input.OwnerUserID != nil && *input.OwnerUserID != 0 {
		ownerID = *input.OwnerUserID
	}

	var ownerUser models.User
	if err := storage.DB.First(&ownerUser, ownerID).Error; err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "owner_user_id does not match an existing user"})
		return
	}

	organization := models.Organization{
		Name:          input.Name,
		Description:   input.Description,
		Logo:          input.Logo,
		BannerImage:   input.BannerImage,
		Website:       input.Website,
		Phone:         phoneCanon,
		Email:         input.Email,
		Address:       input.Address,
		City:          input.City,
		State:         input.State,
		Country:       input.Country,
		PostalCode:    input.PostalCode,
		LicenseNumber: input.LicenseNumber,
		TaxID:         input.TaxID,
		BusinessType:  input.BusinessType,
		OwnerID:       ownerID,
		Status:        "approved",
		IsActive:      true,
	}

	if err := storage.DB.Create(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create organization"})
		return
	}

	adminPermissions := models.GetDefaultPermissions("admin")
	permissionsJSON, err := json.Marshal(adminPermissions)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to prepare owner member"})
		return
	}

	member := models.OrganizationMember{
		UserID:         ownerID,
		OrganizationID: organization.ID,
		Role:           "admin",
		Permissions:    permissionsJSON,
		Status:         "active",
		IsActive:       true,
		JoinedAt:       time.Now(),
	}
	if err := storage.DB.Create(&member).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create organization member"})
		return
	}

	agent := models.Agent{
		UserID:         ownerID,
		OrganizationID: organization.ID,
		Status:         "approved",
		IsActive:       true,
	}
	if err := storage.DB.Create(&agent).Error; err != nil {
		log.Printf("⚠️ AdminCreateOrganization: could not create agent (user may already be agent elsewhere): %v", err)
	}

	orgmember.LogOrganizationAudit(organization.ID, adminUID, models.ActionOrgSettingsUpdated, models.ActionTypeOrganizationSettings, "organization", &organization.ID, "Organization created by admin", "", "")

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message":      "Organization created successfully",
		"organization": organization,
	})
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

// ==================== Organization Member Management Routes ====================

// GenerateOrganizationInviteCode generates a secure invite code for an organization
// Only owners and admins can generate invite codes
func GenerateOrganizationInviteCode(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Get user's organization (as owner or member)
	var organization models.Organization
	var member *models.OrganizationMember
	
	// First check if user is owner
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// Owner can always generate invite codes
	} else {
		// If not owner, check if user is a member with admin role
		org, mem, _ := orgmember.GetUserOrganization(userID)
		if org == nil {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Organization not found. Only organization owners and admins can generate invite codes."})
			return
		}
		organization = *org
		member = mem
		// Check if member has admin role
		if member == nil || member.Role != "admin" {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Only organization owners and admins can generate invite codes."})
			return
		}
	}

	// Parse request body for expiry and usage settings
	var reqBody struct {
		ExpiryDays int `json:"expiry_days"` // 0 = never expires, 7 = 7 days, 30 = 30 days
		MaxUses    int `json:"max_uses"`    // 0 = unlimited, 1 = single use, 10 = 10 uses
	}
	
	// Default values: 30 days expiry, unlimited uses
	reqBody.ExpiryDays = 30
	reqBody.MaxUses = 0
	
	// Try to read request body (ignore errors, use defaults)
	if err := ctx.ReadJSON(&reqBody); err == nil {
		// Validate values
		if reqBody.ExpiryDays < 0 {
			reqBody.ExpiryDays = 30 // Default to 30 days
		}
		if reqBody.MaxUses < 0 {
			reqBody.MaxUses = 0 // Default to unlimited
		}
	}

	// Generate invite code
	code, inviteCode, err := orgmember.GenerateInviteCode(organization.ID, userID, reqBody.ExpiryDays, reqBody.MaxUses)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": fmt.Sprintf("Failed to generate invite code: %v", err)})
		return
	}

	// Format expiry message
	expiryMsg := "Never expires"
	if inviteCode.ExpiresAt != nil {
		days := int(inviteCode.ExpiresAt.Sub(time.Now()).Hours() / 24)
		if days == 7 {
			expiryMsg = "Expires in 7 days"
		} else if days == 30 {
			expiryMsg = "Expires in 30 days"
		} else {
			expiryMsg = fmt.Sprintf("Expires in %d days", days)
		}
	}

	// Format usage limit message
	usageMsg := "Unlimited uses"
	if inviteCode.MaxUses != nil {
		if *inviteCode.MaxUses == 1 {
			usageMsg = "Single use"
		} else {
			usageMsg = fmt.Sprintf("%d uses", *inviteCode.MaxUses)
		}
	}

	// Log audit event
	orgmember.LogOrganizationAudit(organization.ID, userID, models.ActionInviteCodeGenerated, models.ActionTypeOrganizationSettings, "invite_code", &inviteCode.ID, fmt.Sprintf("Invite code generated (%s, %s)", expiryMsg, usageMsg), "", "")

	ctx.JSON(iris.Map{
		"message":      "Invite code generated successfully",
		"code":         code,
		"expires_at":   inviteCode.ExpiresAt,
		"expires_in":   expiryMsg,
		"max_uses":     inviteCode.MaxUses,
		"current_uses": inviteCode.CurrentUses,
		"usage_limit":  usageMsg,
	})
}

// ValidateOrganizationInviteCode validates an invite code and returns organization preview
// This allows the frontend to show a preview before the user confirms joining
// Note: This endpoint does NOT require authentication to allow preview before login
func ValidateOrganizationInviteCode(ctx iris.Context) {
	code := ctx.Params().GetStringDefault("code", "")
	if code == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invite code required"})
		return
	}

	// Validate invite code
	inviteCode, err := orgmember.ValidateInviteCode(code)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	// Get organization details
	var organization models.Organization
	if err := storage.DB.Preload("Owner").First(&organization, inviteCode.OrganizationID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	// Count properties in organization
	var propertyCount int64
	storage.DB.Model(&models.PropertySale{}).
		Where("organization_id = ? AND is_published = ?", organization.ID, true).
		Count(&propertyCount)

	ctx.JSON(iris.Map{
		"organization": iris.Map{
			"id":            organization.ID,
			"name":          organization.Name,
			"logo":          organization.Logo,
			"owner": iris.Map{
				"id":        organization.OwnerID,
				"firstName": organization.Owner.FirstName,
				"lastName":  organization.Owner.LastName,
				"fullName":  fmt.Sprintf("%s %s", organization.Owner.FirstName, organization.Owner.LastName),
			},
			"property_count": propertyCount,
		},
		"code": code,
	})
}

// JoinOrganization allows a user to join an organization using an invite code
func JoinOrganization(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var input struct {
		Code string `json:"code" validate:"required"`
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

	// Check if user is already a member of an organization
	canCreatePersonal, err := orgmember.CheckUserCanCreatePersonalContent(userID)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to check user status"})
		return
	}

	if !canCreatePersonal {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "You are already a member of an organization. You must leave your current organization before joining another."})
		return
	}

	// Validate invite code
	inviteCode, err := orgmember.ValidateInviteCode(input.Code)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	// Join organization
	member, err := orgmember.JoinOrganization(userID, inviteCode.OrganizationID, inviteCode.ID)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	// Get organization details
	var organization models.Organization
	if err := storage.DB.First(&organization, inviteCode.OrganizationID).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch organization details"})
		return
	}

	// Send notification to organization owner
	var owner models.User
	if err := storage.DB.First(&owner, organization.OwnerID).Error; err == nil {
		ownerTokens := pushsvc.GetUserPushTokens(organization.OwnerID)
		if len(ownerTokens) > 0 {
			var newMember models.User
			storage.DB.First(&newMember, userID)
			memberName := fmt.Sprintf("%s %s", newMember.FirstName, newMember.LastName)
			title := "New Member Joined"
			body := fmt.Sprintf("%s joined your organization", memberName)
			go func() {
				if err := pushsvc.SendPush(ownerTokens, title, body); err != nil {
					log.Printf("⚠️ Failed to send notification to organization owner: %v", err)
				}
			}()
		}
	}

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message":      "Successfully joined organization",
		"organization": organization,
		"member":       member,
		"role":         member.Role,
	})
}

// GetOrganizationMembers returns all members of an organization
// Accessible by both owners and members
// Includes the owner in the response (owner is not in members table)
func GetOrganizationMembers(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Get user's organization (as owner or member)
	var organization models.Organization
	
	// First check if user is owner
	if err := storage.DB.Preload("Owner").Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// User is owner
	} else {
		// If not owner, check if user is a member
		org, _, _ := orgmember.GetUserOrganization(userID)
		if org == nil {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Organization not found. You must be an owner or member to view members."})
			return
		}
		organization = *org
		// Preload owner for response
		storage.DB.Preload("Owner").First(&organization, organization.ID)
	}

	// Get all members
	members, err := orgmember.GetOrganizationMembers(organization.ID)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	// Include owner in the members list (owner is not in members table)
	// Create a pseudo-member entry for the owner
	ownerMember := map[string]interface{}{
		"id":       0, // Special ID for owner
		"user_id":  organization.OwnerID,
		"user": map[string]interface{}{
			"id":        organization.Owner.ID,
			"firstName": organization.Owner.FirstName,
			"lastName":  organization.Owner.LastName,
			"email":     organization.Owner.Email,
			"avatarURL": organization.Owner.AvatarURL,
		},
		"role":     "admin", // Owner is always admin
		"status":   "active",
		"is_owner": true,
		"joined_at": organization.CreatedAt,
	}

	// Convert members to map format for consistent response
	membersList := make([]map[string]interface{}, 0, len(members)+1)
	
	// Add owner first
	membersList = append(membersList, ownerMember)
	
	// Add other members
	for _, m := range members {
		membersList = append(membersList, map[string]interface{}{
			"id":       m.ID,
			"user_id":  m.UserID,
			"user": map[string]interface{}{
				"id":        m.User.ID,
				"firstName": m.User.FirstName,
				"lastName":  m.User.LastName,
				"email":     m.User.Email,
				"avatarURL": m.User.AvatarURL,
			},
			"role":      m.Role,
			"status":    m.Status,
			"is_owner":  false,
			"joined_at": m.JoinedAt,
		})
	}

	ctx.JSON(iris.Map{
		"members": membersList,
		"total":   len(membersList),
	})
}

// UpdateOrganizationMemberRole updates a member's role and permissions
func UpdateOrganizationMemberRole(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	memberID, err := strconv.ParseUint(ctx.Params().Get("memberId"), 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid member ID"})
		return
	}

	// Get user's organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found. Only organization owners can update member roles."})
		return
	}

	var input struct {
		Role            string   `json:"role" validate:"required,oneof=admin manager editor viewer"`
		CustomPermissions []string `json:"custom_permissions"` // Optional: override default permissions
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

	// Update member role
	if err := orgmember.UpdateMemberRole(organization.ID, uint(memberID), input.Role, input.CustomPermissions, userID); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Member role updated successfully",
	})
}

// RemoveOrganizationMember removes a member from an organization
func RemoveOrganizationMember(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	memberID, err := strconv.ParseUint(ctx.Params().Get("memberId"), 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid member ID"})
		return
	}

	// Get user's organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found. Only organization owners can remove members."})
		return
	}

	// Remove member
	if err := orgmember.RemoveMember(organization.ID, uint(memberID), userID); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Member removed successfully",
	})
}

// LeaveOrganization allows a member to leave an organization
func LeaveOrganization(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Leave organization
	if err := orgmember.LeaveOrganization(userID); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Successfully left the organization",
	})
}

// CheckUserCanCreatePersonalContent checks if user can create personal properties/lands
func CheckUserCanCreatePersonalContent(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	canCreate, err := orgmember.CheckUserCanCreatePersonalContent(userID)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to check user status"})
		return
	}

	// Get user's organization if they're a member
	var org *models.Organization
	var member *models.OrganizationMember
	if !canCreate {
		org, member, _ = orgmember.GetUserOrganization(userID)
	}

	ctx.JSON(iris.Map{
		"can_create_personal": canCreate,
		"organization":        org,
		"member":             member,
	})
}
