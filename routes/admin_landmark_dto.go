package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func parseLandmarkJSONStrings(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	clean := make([]string, 0, len(out))
	for _, s := range out {
		if t := strings.TrimSpace(s); t != "" {
			clean = append(clean, t)
		}
	}
	return clean
}

func locationLabel(name, nameAr string) string {
	name = strings.TrimSpace(name)
	nameAr = strings.TrimSpace(nameAr)
	if name != "" && nameAr != "" && !strings.EqualFold(name, nameAr) {
		return name + " / " + nameAr
	}
	if name != "" {
		return name
	}
	return nameAr
}

func buildAdminLandmarkHost(l models.Landmark) iris.Map {
	host := iris.Map{
		"type":    "unknown",
		"name":    "",
		"phone":   "",
		"email":   "",
		"website": "",
	}
	if l.Organization != nil {
		host["type"] = "organization"
		host["name"] = l.Organization.Name
		host["phone"] = l.Organization.Phone
		host["email"] = l.Organization.Email
		host["website"] = l.Organization.Website
		return host
	}
	if l.Owner != nil {
		host["type"] = "individual"
		full := strings.TrimSpace(fmt.Sprintf("%s %s", l.Owner.FirstName, l.Owner.LastName))
		if full == "" {
			full = l.Owner.Email
		}
		host["name"] = full
		host["email"] = l.Owner.Email
		if l.Owner.PhoneNumber != nil {
			host["phone"] = *l.Owner.PhoneNumber
		}
	}
	return host
}

func enrichAdminLandmarks(landmarks []models.Landmark) []iris.Map {
	if len(landmarks) == 0 {
		return []iris.Map{}
	}

	cityIDs := map[uint]struct{}{}
	zoneIDs := map[uint]struct{}{}
	quartierIDs := map[uint]struct{}{}
	plotIDs := map[uint]struct{}{}
	for _, l := range landmarks {
		if l.CityID != nil && *l.CityID > 0 {
			cityIDs[*l.CityID] = struct{}{}
		}
		if l.ZoneID != nil && *l.ZoneID > 0 {
			zoneIDs[*l.ZoneID] = struct{}{}
		}
		if l.QuartierID != nil && *l.QuartierID > 0 {
			quartierIDs[*l.QuartierID] = struct{}{}
		}
		if l.HabitatPlotID != nil && *l.HabitatPlotID > 0 {
			plotIDs[*l.HabitatPlotID] = struct{}{}
		}
	}

	cityNames := map[uint]string{}
	if len(cityIDs) > 0 {
		ids := make([]uint, 0, len(cityIDs))
		for id := range cityIDs {
			ids = append(ids, id)
		}
		var rows []models.City
		_ = storage.DB.Where("id IN ?", ids).Find(&rows).Error
		for _, c := range rows {
			cityNames[c.ID] = locationLabel(c.Name, c.NameAr)
		}
	}

	zoneNames := map[uint]string{}
	if len(zoneIDs) > 0 {
		ids := make([]uint, 0, len(zoneIDs))
		for id := range zoneIDs {
			ids = append(ids, id)
		}
		var rows []models.Zone
		_ = storage.DB.Where("id IN ?", ids).Find(&rows).Error
		for _, z := range rows {
			zoneNames[z.ID] = locationLabel(z.Name, z.NameAr)
		}
	}

	quartierNames := map[uint]string{}
	if len(quartierIDs) > 0 {
		ids := make([]uint, 0, len(quartierIDs))
		for id := range quartierIDs {
			ids = append(ids, id)
		}
		var rows []models.Quartier
		_ = storage.DB.Where("id IN ?", ids).Find(&rows).Error
		for _, q := range rows {
			quartierNames[q.ID] = locationLabel(q.Name, q.NameAr)
		}
	}

	cadastrePlots := map[uint]models.HabitatPlot{}
	if len(plotIDs) > 0 {
		ids := make([]uint, 0, len(plotIDs))
		for id := range plotIDs {
			ids = append(ids, id)
		}
		var rows []models.HabitatPlot
		_ = storage.DB.Preload("Sector").Preload("Plan").Where("id IN ?", ids).Find(&rows).Error
		for _, p := range rows {
			cadastrePlots[p.ID] = p
		}
	}

	out := make([]iris.Map, 0, len(landmarks))
	for _, l := range landmarks {
		dto := iris.Map{
			"id":                        l.ID,
			"title":                     l.Title,
			"description":               l.Description,
			"area":                      l.Area,
			"area_unit":                 l.AreaUnit,
			"land_type":                 l.LandType,
			"zoning":                    l.Zoning,
			"status":                    l.Status,
			"is_verified":               l.IsVerified,
			"is_published":              l.IsPublished,
			"is_investment_opportunity": l.IsInvestmentOpportunity,
			"is_good_deal":              l.IsGoodDeal,
			"is_gold":                   l.IsGold,
			"verified_at":               l.VerifiedAt,
			"verified_by":               l.VerifiedBy,
			"verification_notes":        l.VerificationNotes,
			"images":                    parseLandmarkJSONStrings(l.Images),
			"property_papers":           parseLandmarkJSONStrings(l.PropertyPapers),
			"paper_types":               l.PaperTypes,
			"price":                     l.Price,
			"currency":                  l.Currency,
			"plot_number":               strings.TrimSpace(l.PlotNumber),
			"plot_confirmed":            l.PlotConfirmed,
			"habitat_plot_id":           l.HabitatPlotID,
			"city_id":                   l.CityID,
			"zone_id":                   l.ZoneID,
			"quartier_id":               l.QuartierID,
			"district":                  l.District,
			"region":                    l.Region,
			"elevation_m":               l.ElevationMeters,
			"sides":                     parseLandmarkJSONStrings(l.Sides),
			"lots":                      l.Lots,
			"media_type":                l.MediaType,
			"host_private_note":         strings.TrimSpace(l.HostPrivateNote),
			"organization_id":           l.OrganizationID,
			"owner_id":                  l.OwnerID,
			"created_at":                l.CreatedAt,
			"updated_at":                l.UpdatedAt,
			"point1_lat":                l.Point1Lat,
			"point1_lng":                l.Point1Lng,
			"point2_lat":                l.Point2Lat,
			"point2_lng":                l.Point2Lng,
			"point3_lat":                l.Point3Lat,
			"point3_lng":                l.Point3Lng,
			"point4_lat":                l.Point4Lat,
			"point4_lng":                l.Point4Lng,
			"host":                      buildAdminLandmarkHost(l),
		}

		if l.VideoURL != nil && strings.TrimSpace(*l.VideoURL) != "" {
			dto["video_url"] = strings.TrimSpace(*l.VideoURL)
		} else {
			dto["video_url"] = ""
		}

		if l.Organization != nil {
			dto["organization"] = iris.Map{
				"id":    l.Organization.ID,
				"name":  l.Organization.Name,
				"phone": l.Organization.Phone,
				"email": l.Organization.Email,
			}
		}

		if l.CityID != nil {
			dto["city_name"] = cityNames[*l.CityID]
		}
		if l.ZoneID != nil {
			dto["zone_name"] = zoneNames[*l.ZoneID]
		}
		if l.QuartierID != nil {
			dto["quartier_name"] = quartierNames[*l.QuartierID]
		}

		cadastreLinked := l.HabitatPlotID != nil && *l.HabitatPlotID > 0
		dto["cadastre_linked"] = cadastreLinked
		if cadastreLinked {
			if plot, ok := cadastrePlots[*l.HabitatPlotID]; ok {
				dto["cadastre_plot"] = iris.Map{
					"id":          plot.ID,
					"plot_number": plot.PlotNumber,
					"area_m2":     plot.AreaM2,
					"sector_name": locationLabel(plot.Sector.Name, plot.Sector.NameAr),
					"plan_name":   locationLabel(plot.Plan.Name, plot.Plan.NameAr),
					"plan_code":   plot.Plan.Code,
				}
				listedPN := strings.TrimSpace(l.PlotNumber)
				cadPN := strings.TrimSpace(plot.PlotNumber)
				dto["plot_number_matches_cadastre"] =
					listedPN != "" && cadPN != "" &&
						strings.EqualFold(strings.ReplaceAll(listedPN, " ", ""), strings.ReplaceAll(cadPN, " ", ""))
			}
		} else {
			dto["plot_number_matches_cadastre"] = false
		}

		out = append(out, dto)
	}
	return out
}

func loadLandmarksForAdmin(query *gorm.DB) ([]models.Landmark, error) {
	var landmarks []models.Landmark
	err := query.
		Preload("Organization").
		Preload("Owner").
		Order("updated_at DESC").
		Find(&landmarks).Error
	return landmarks, err
}
