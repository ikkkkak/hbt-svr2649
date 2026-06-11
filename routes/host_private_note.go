package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"os"
	"strings"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
)

const hostPrivateNoteMaxRunes = 2000

func sanitizeHostPrivateNote(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) > hostPrivateNoteMaxRunes {
		return string(runes[:hostPrivateNoteMaxRunes])
	}
	return s
}

// optionalAuthUserID returns the authenticated user when a Bearer token is present, else 0.
func optionalAuthUserID(ctx iris.Context) uint {
	var userID uint
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
				}
			}
		}
	}
	return userID
}

func canViewSaleOrLandmarkHostNote(viewerID uint, ownerID *uint, orgID *uint) bool {
	if viewerID == 0 {
		return false
	}
	if ownerID != nil && *ownerID == viewerID {
		return true
	}
	if orgID == nil {
		return false
	}
	var org models.Organization
	if err := storage.DB.Select("owner_id").First(&org, *orgID).Error; err == nil && org.OwnerID == viewerID {
		return true
	}
	var n int64
	storage.DB.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND status = ? AND is_active = ?", *orgID, viewerID, "active", true).
		Count(&n)
	return n > 0
}

func redactPropertySaleHostNote(p *models.PropertySale, viewerID uint) {
	if p == nil {
		return
	}
	if canViewSaleOrLandmarkHostNote(viewerID, p.OwnerID, p.OrganizationID) {
		return
	}
	p.HostPrivateNote = ""
}

func redactLandmarkHostNote(l *models.Landmark, viewerID uint) {
	if l == nil {
		return
	}
	if canViewSaleOrLandmarkHostNote(viewerID, l.OwnerID, l.OrganizationID) {
		return
	}
	l.HostPrivateNote = ""
}

func redactRentPropertyHostNote(p *models.Property, viewerID uint) {
	if p == nil {
		return
	}
	if viewerID > 0 && p.HostID == viewerID {
		return
	}
	p.HostPrivateNote = ""
}

func redactPropertySaleSliceForViewer(props []models.PropertySale, viewerID uint) {
	for i := range props {
		redactPropertySaleHostNote(&props[i], viewerID)
		redactPropertySaleBrokerProfile(&props[i])
	}
}

func redactLandmarkSliceForViewer(items []models.Landmark, viewerID uint) {
	for i := range items {
		redactLandmarkHostNote(&items[i], viewerID)
		redactLandmarkBrokerProfile(&items[i])
	}
}

func redactPropertySaleBrokerProfile(p *models.PropertySale) {
	if p == nil {
		return
	}
	if p.Owner != nil {
		services.ApplyBrokerProfilePrivacyToUser(p.Owner)
	}
	if p.Organization != nil && p.Organization.OwnerID != 0 {
		services.ApplyBrokerProfilePrivacyToUser(&p.Organization.Owner)
	}
}

func redactLandmarkBrokerProfile(l *models.Landmark) {
	if l == nil {
		return
	}
	if l.Owner != nil {
		services.ApplyBrokerProfilePrivacyToUser(l.Owner)
	}
	if l.Organization != nil && l.Organization.OwnerID != 0 {
		services.ApplyBrokerProfilePrivacyToUser(&l.Organization.Owner)
	}
}

func redactRentPropertyBrokerProfile(p *models.Property) {
	if p == nil {
		return
	}
	services.ApplyBrokerProfilePrivacyToUser(&p.Host)
}
