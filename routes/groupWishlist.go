package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"net/http"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type addWishlistInput struct {
	ExperienceID *uint `json:"experienceID"`
	PropertyID   *uint `json:"propertyID"`
}

// List items in a group's wishlist with liker avatars
func ListGroupWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// membership check
	var m models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&m).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var items []models.GroupWishlistItem
	storage.DB.Where("group_id = ?", groupID).Preload("AddedBy").Order("id DESC").Find(&items)

	// fetch likes per item
	type itemDTO struct {
		models.GroupWishlistItem
		Likes      []models.GroupWishlistLike `json:"likes"`
		Property   *models.Property           `json:"property"`
		Experience *models.Experience         `json:"experience"`
	}
	out := []itemDTO{}
	for _, it := range items {
		var likes []models.GroupWishlistLike
		storage.DB.Where("wishlist_id = ?", it.ID).Preload("User").Find(&likes)

		var prop *models.Property
		var exp *models.Experience
		if it.PropertyID != nil {
			var p models.Property
			if err := storage.DB.First(&p, *it.PropertyID).Error; err == nil {
				prop = &p
			}
		}
		if it.ExperienceID != nil {
			var e models.Experience
			if err := storage.DB.Preload("Photos").First(&e, *it.ExperienceID).Error; err == nil {
				exp = &e
			}
		}

		out = append(out, itemDTO{GroupWishlistItem: it, Likes: likes, Property: prop, Experience: exp})
	}

	ctx.JSON(iris.Map{"success": true, "items": out})
}

// Add item to wishlist + emit a chat system message
func AddGroupWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// membership check
	var m models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&m).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	var input addWishlistInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	if (input.ExperienceID == nil && input.PropertyID == nil) || (input.ExperienceID != nil && input.PropertyID != nil) {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	item := models.GroupWishlistItem{GroupID: groupID, ExperienceID: input.ExperienceID, PropertyID: input.PropertyID, AddedByID: user.ID}
	q := storage.DB.Where("group_id = ?", groupID)
	if input.ExperienceID != nil {
		q = q.Where("experience_id = ?", *input.ExperienceID)
	}
	if input.PropertyID != nil {
		q = q.Where("property_id = ?", *input.PropertyID)
	}
	if err := q.FirstOrCreate(&item).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	// auto-like by adder
	var like models.GroupWishlistLike
	storage.DB.Where("wishlist_id = ? AND user_id = ?", item.ID, user.ID).FirstOrCreate(&like, models.GroupWishlistLike{WishlistID: item.ID, UserID: user.ID})

	// Create a system chat message with preview
	var content string
	var refType string
	var refID *uint
	var previewTitle string
	var previewSubtitle string
	var previewImage string
	var previewDesc string

	if item.PropertyID != nil {
		refType = "property"
		refID = item.PropertyID
		var p models.Property
		if err := storage.DB.First(&p, *item.PropertyID).Error; err == nil {
			previewTitle = p.Title
			previewSubtitle = p.City
			// Try to parse first image from JSON string field
			if p.Images != "" {
				var imgs []string
				if jsonErr := json.Unmarshal([]byte(p.Images), &imgs); jsonErr == nil && len(imgs) > 0 {
					previewImage = imgs[0]
				}
			}
			if p.Description != "" {
				previewDesc = p.Description
			}
		}
		content = "added a property to wishlist"
	} else if item.ExperienceID != nil {
		refType = "experience"
		refID = item.ExperienceID
		var e models.Experience
		if err := storage.DB.First(&e, *item.ExperienceID).Error; err == nil {
			previewTitle = e.Title
			previewSubtitle = e.City
			// Photos is JSON; attempt to parse first url
			if len(e.Photos) > 0 {
				// Try array of objects with url
				var arrObj []map[string]interface{}
				if err1 := json.Unmarshal([]byte(e.Photos), &arrObj); err1 == nil && len(arrObj) > 0 {
					if u, ok := arrObj[0]["url"].(string); ok {
						previewImage = u
					}
				}
				if previewImage == "" {
					// Try array of strings
					var arrStr []string
					if err2 := json.Unmarshal([]byte(e.Photos), &arrStr); err2 == nil && len(arrStr) > 0 {
						previewImage = arrStr[0]
					}
				}
			}
			if e.Description != "" {
				previewDesc = e.Description
			}
		}
		content = "added an experience to wishlist"
	} else {
		content = "added to wishlist"
	}
	msg := models.ChatMessage{
		GroupID:            groupID,
		SenderID:           user.ID,
		Content:            content,
		Type:               "wishlist",
		RefType:            refType,
		RefID:              refID,
		PreviewTitle:       previewTitle,
		PreviewSubtitle:    previewSubtitle,
		PreviewImageURL:    previewImage,
		PreviewDescription: previewDesc,
	}
	storage.DB.Create(&msg)

	ctx.JSON(iris.Map{"success": true, "item": item})
}

// Like an item
func LikeGroupWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	wishlistID, err := ctx.Params().GetUint("wishlistID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// membership check
	var m models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&m).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	var item models.GroupWishlistItem
	if err := storage.DB.Where("id = ? AND group_id = ?", wishlistID, groupID).First(&item).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	storage.DB.Where("wishlist_id = ? AND user_id = ?", wishlistID, user.ID).FirstOrCreate(&models.GroupWishlistLike{WishlistID: wishlistID, UserID: user.ID})
	ctx.JSON(iris.Map{"success": true})
}
