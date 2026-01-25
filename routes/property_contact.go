package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/kataras/iris/v12"
)

// ContactPropertySaleHost - Creates a 1:1 conversation with property sale host
func ContactPropertySaleHost(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		log.Printf("❌ ContactPropertySaleHost: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var body struct {
		PropertySaleID uint   `json:"property_sale_id"`
		InitialMessage  string `json:"initial_message"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		log.Printf("❌ ContactPropertySaleHost: Invalid JSON - %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid json"})
		return
	}

	// Validate property sale exists and get host info
	var property models.PropertySale
	if err := storage.DB.Preload("Owner").Preload("Organization").First(&property, body.PropertySaleID).Error; err != nil {
		log.Printf("❌ ContactPropertySaleHost: Property sale %d not found", body.PropertySaleID)
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "property not found"})
		return
	}

	// Determine host ID (organization owner or property owner)
	var hostID uint
	var hostName string
	var hostAvatarURL string
	var organizationName string

	if property.OrganizationID != nil && property.Organization != nil {
		// Property belongs to organization - contact organization owner
		hostID = property.Organization.OwnerID
		organizationName = property.Organization.Name
		// Get organization owner details
		var orgOwner models.User
		if err := storage.DB.First(&orgOwner, hostID).Error; err == nil {
			hostName = fmt.Sprintf("%s %s", orgOwner.FirstName, orgOwner.LastName)
			if hostName == " " {
				hostName = organizationName
			}
			hostAvatarURL = orgOwner.AvatarURL
		} else {
			hostName = organizationName
		}
	} else if property.OwnerID != nil {
		// Property belongs to individual owner
		hostID = *property.OwnerID
		var owner models.User
		if err := storage.DB.First(&owner, hostID).Error; err == nil {
			hostName = fmt.Sprintf("%s %s", owner.FirstName, owner.LastName)
			if hostName == " " {
				if owner.PhoneNumber != nil {
					hostName = *owner.PhoneNumber
				} else {
					hostName = "Host"
				}
			}
			hostAvatarURL = owner.AvatarURL
		}
	} else {
		log.Printf("❌ ContactPropertySaleHost: Property %d has no owner or organization", body.PropertySaleID)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "property has no owner"})
		return
	}

	// Prevent self-contact
	if hostID == uid {
		log.Printf("❌ ContactPropertySaleHost: User %d cannot contact themselves", uid)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "cannot contact yourself"})
		return
	}

	// Check if conversation already exists
	var existingConversation models.DirectMessage
	storage.DB.Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)",
		uid, hostID, hostID, uid).
		Order("created_at DESC").
		First(&existingConversation)

	var conversationID uint
	if existingConversation.ID > 0 {
		// Conversation exists, use it
		conversationID = existingConversation.ID
		log.Printf("✅ ContactPropertySaleHost: Reusing existing conversation %d", conversationID)
	} else {
		// Create initial message to establish conversation
		initialMessage := body.InitialMessage
		if initialMessage == "" {
			initialMessage = fmt.Sprintf("Hello! I'm interested in your property: %s", property.Title)
		}

		// Create direct message
		message := models.DirectMessage{
			SenderID:   uid,
			ReceiverID: hostID,
			Content:    initialMessage,
			Type:       "text",
			RefType:    "property_sale",
			RefID:      &body.PropertySaleID,
		}
		if err := storage.DB.Create(&message).Error; err != nil {
			log.Printf("❌ ContactPropertySaleHost: Failed to create message - %v", err)
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "failed to create message"})
			return
		}
		conversationID = message.ID
		log.Printf("✅ ContactPropertySaleHost: Created new conversation with message %d", conversationID)
	}

	// Get sender info for notification
	var sender models.User
	if err := storage.DB.First(&sender, uid).Error; err != nil {
		log.Printf("❌ ContactPropertySaleHost: Sender %d not found", uid)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "sender not found"})
		return
	}

	// Get receiver (host) info
	var receiver models.User
	if err := storage.DB.First(&receiver, hostID).Error; err != nil {
		log.Printf("❌ ContactPropertySaleHost: Receiver %d not found", hostID)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "receiver not found"})
		return
	}

	// Send push notification to host
	senderName := fmt.Sprintf("%s %s", sender.FirstName, sender.LastName)
	if senderName == " " {
		if sender.PhoneNumber != nil {
			senderName = *sender.PhoneNumber
		} else {
			senderName = "User"
		}
	}

	// Get property image for notification
	propertyImage := ""
	if len(property.Images) > 0 {
		propertyImage = property.Images[0]
	}

	// Prepare notification data
	notificationData := map[string]string{
		"type":              "property_inquiry",
		"property_sale_id":  fmt.Sprintf("%d", body.PropertySaleID),
		"sender_id":         fmt.Sprintf("%d", uid),
		"conversation_id":   fmt.Sprintf("%d", conversationID),
		"property_title":    property.Title,
		"property_city":     property.City,
		"property_price":    fmt.Sprintf("%.2f", property.ListingPrice),
	}

	// Send rich notification with property image
	notificationTitle := "New Property Inquiry"
	if organizationName != "" {
		notificationTitle = fmt.Sprintf("Inquiry about %s", organizationName)
	}
	notificationBody := fmt.Sprintf("%s is interested in your property: %s", senderName, property.Title)

	// Get push tokens from receiver
	var pushTokens []string
	if receiver.PushTokens != nil {
		if err := json.Unmarshal(receiver.PushTokens, &pushTokens); err == nil && len(pushTokens) > 0 {
			// Use notification service to send with image
			// Send to first available token
			if receiver.AllowsNotifications == nil || (receiver.AllowsNotifications != nil && *receiver.AllowsNotifications) {
				utils.SendRichNotification(
					pushTokens[0],
					notificationTitle,
					notificationBody,
					propertyImage,
					notificationData,
				)
				log.Printf("✅ ContactPropertySaleHost: Sent notification to host %d", hostID)
			} else {
				log.Printf("⚠️ ContactPropertySaleHost: Host %d has notifications disabled", hostID)
			}
		}
	}

	// Return conversation info
	ctx.JSON(iris.Map{
		"success":         true,
		"conversation_id": conversationID,
		"host_id":         hostID,
		"host_name":       hostName,
		"host_avatar_url": hostAvatarURL,
		"organization_name": organizationName,
		"message":         "Conversation started successfully",
	})
}
