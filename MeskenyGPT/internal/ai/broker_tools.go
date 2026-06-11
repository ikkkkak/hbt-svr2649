package ai

import (
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// HostPortfolioSummary is a compact broker dashboard for the agent.
type HostPortfolioSummary struct {
	ActiveListings int
	TotalViews     int64
	TotalSaves     int64
	TotalLikes     int64
	TopListings    []string
}

func loadHostPortfolioSummary(gdb *gorm.DB, userID uint) (*HostPortfolioSummary, error) {
	if gdb == nil || userID == 0 {
		return nil, fmt.Errorf("no database or user")
	}
	out := &HostPortfolioSummary{}

	var rentCount int64
	_ = gdb.Model(&models.Property{}).
		Where("host_id = ? AND deleted_at IS NULL", userID).
		Where("LOWER(status) IN ?", []string{"approved", "live", "published", "pending"}).
		Count(&rentCount).Error

	var saleProps []models.PropertySale
	_ = gdb.Select("id", "title", "view_count").
		Where("owner_id = ?", userID).
		Where("COALESCE(is_deactivated, false) = ?", false).
		Order("view_count DESC").
		Limit(8).
		Find(&saleProps).Error

	out.ActiveListings = int(rentCount) + len(saleProps)
	for _, ps := range saleProps {
		out.TotalViews += int64(ps.ViewCount)
		title := strings.TrimSpace(ps.Title)
		if title != "" {
			out.TopListings = append(out.TopListings, fmt.Sprintf("%s (%d views)", title, ps.ViewCount))
		}
	}
	if len(out.TopListings) > 5 {
		out.TopListings = out.TopListings[:5]
	}
	return out, nil
}

func isMarketingPackRequest(text string) bool {
	lower := strings.ToLower(text)
	if containsHan(text) {
		if strings.Contains(text, "营销") || strings.Contains(text, "中文") ||
			strings.Contains(text, "推广") || strings.Contains(text, "房源") {
			return true
		}
	}
	return strings.Contains(lower, "marketing pack") ||
		strings.Contains(lower, "marketing post") ||
		strings.Contains(lower, "chinese brochure") ||
		strings.Contains(lower, "chinese version") ||
		strings.Contains(lower, "wechat") ||
		strings.Contains(lower, "brochure") ||
		strings.Contains(lower, "social media post") ||
		strings.Contains(lower, "pack chinois") ||
		strings.Contains(lower, "version chinoise")
}

func isPortfolioRequest(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "portfolio") ||
		strings.Contains(lower, "my listings") ||
		strings.Contains(lower, "mes annonces") ||
		strings.Contains(lower, "performance") && strings.Contains(lower, "listing") ||
		strings.Contains(text, "أداء") && strings.Contains(text, "إعلان") ||
		strings.Contains(text, "房源") && strings.Contains(text, "数据")
}

func isBrokerProTier(tier string) bool {
	switch strings.TrimSpace(strings.ToLower(tier)) {
	case "pro", "broker", "enterprise":
		return true
	default:
		return false
	}
}

func brokerProLockedMessage(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "أدوات الوسيط المتقدمة (محفظة العقارات، حزمة تسويق صينية/عربية) متاحة في Meskeny Pro. يمكنك التواصل مع الدعم للترقية."
	case lang.LangZH:
		return "经纪人专业工具（房源组合分析、中英营销文案）需要 Meskeny Pro。请联系支持升级。"
	case lang.LangEN:
		return "Broker Pro tools (portfolio insights, Chinese/Arabic marketing packs) require Meskeny Pro. Contact support to upgrade."
	default:
		return "Les outils Broker Pro (portefeuille, pack marketing chinois) nécessitent Meskeny Pro. Contactez le support pour passer à la version Pro."
	}
}
