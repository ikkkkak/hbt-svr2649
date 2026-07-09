package routes

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"apartments-clone-server/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func extractHabitatPlotFromRawProperties(plot *models.HabitatPlot) bool {
	if plot == nil || len(plot.RawProperties) == 0 {
		return false
	}
	var props map[string]any
	if err := json.Unmarshal(plot.RawProperties, &props); err != nil || len(props) == 0 {
		return false
	}
	return applyHabitatPropsToPlot(plot, props)
}

func applyHabitatPropsToPlot(plot *models.HabitatPlot, props map[string]any) bool {
	if plot == nil || len(props) == 0 {
		return false
	}
	changed := false

	if plot.PlotNumber == "" {
		if pn := firstString(props, "plot_number", "plotNumber", "NUMERO", "numero", "Numero", "N", "n", "PARCEL", "parcel"); pn != "" {
			plot.PlotNumber = pn
			changed = true
		}
	}
	if plot.LValue == "" {
		if v := firstString(props, "l_value", "L", "l", "L_value"); v != "" {
			plot.LValue = v
			changed = true
		}
	}
	if plot.IValue == nil {
		if v, ok := firstFloat(props, "i_value", "I", "i", "I_value"); ok {
			iv := int(math.Round(v))
			plot.IValue = &iv
			changed = true
		}
	}
	if plotAreaMissing(plot.AreaM2, plot.AreaRounded) {
		if v, ok := firstFloat(props, "area_m2", "area", "AREA", "Area", "surface", "SURFACE"); ok && v > 0 {
			plot.AreaM2 = &v
			r := int(math.Round(v))
			plot.AreaRounded = &r
			changed = true
		}
	}
	if plot.ILValue == nil {
		if v, ok := firstFloat(props, "il_value", "IL", "il", "IL_value"); ok {
			plot.ILValue = &v
			changed = true
		}
	}
	if plot.ELValue == nil {
		if v, ok := firstFloat(props, "el_value", "EL", "el", "EL_value", "elevation", "ELEVATION", "elev", "ELEV"); ok {
			plot.ELValue = &v
			changed = true
		}
	}
	if plot.RESValue == nil {
		if v, ok := firstFloat(props, "res_value", "RES", "res", "RES_value"); ok {
			plot.RESValue = &v
			changed = true
		}
	}
	if len(plot.SidesM) == 0 || dimensionPartCount(plot.DimensionsString) < 3 {
		if sides := firstFloatSlice(props, "sides_m", "sides", "SIDES", "sidesM", "cotes", "COTES"); len(sides) >= 3 {
			plot.SidesM = models.Float64Slice(sides)
			plot.DimensionsString = formatSideLengths(sides)
			changed = true
		}
	}
	if plot.DimensionsString == "" || dimensionPartCount(plot.DimensionsString) < 3 {
		if s := firstString(props, "dimensions_string", "dimensions", "DIMENSIONS", "Dimensions", "dim", "DIM"); s != "" && dimensionPartCount(s) >= 3 {
			plot.DimensionsString = normalizeDimensionsString(s)
			changed = true
		}
	}
	if plot.DimensionsString == "" && len(plot.SidesM) >= 3 {
		plot.DimensionsString = formatSideLengths([]float64(plot.SidesM))
		changed = true
	}
	if plot.LengthM == nil || plot.WidthM == nil {
		if len, wid, ok := firstLengthWidth(props); ok {
			if plot.LengthM == nil {
				plot.LengthM = &len
			}
			if plot.WidthM == nil {
				plot.WidthM = &wid
			}
			changed = true
		}
	}
	if (plot.LengthM == nil || plot.WidthM == nil) && len(plot.SidesM) >= 2 {
		minSide, maxSide := plot.SidesM[0], plot.SidesM[0]
		for _, s := range plot.SidesM[1:] {
			if s < minSide {
				minSide = s
			}
			if s > maxSide {
				maxSide = s
			}
		}
		if plot.LengthM == nil && maxSide > 0 {
			plot.LengthM = &maxSide
			changed = true
		}
		if plot.WidthM == nil && minSide > 0 {
			plot.WidthM = &minSide
			changed = true
		}
	}
	changed = scanLooseHabitatProps(plot, props) || changed
	return changed
}

func scanLooseHabitatProps(plot *models.HabitatPlot, props map[string]any) bool {
	if plot == nil || len(props) == 0 {
		return false
	}
	changed := false
	for key, val := range props {
		if val == nil {
			continue
		}
		kl := strings.ToLower(strings.TrimSpace(key))
		switch {
		case plot.ELValue == nil && (kl == "el" || kl == "el_value" || kl == "elevation" || kl == "elev" || strings.HasSuffix(kl, "_el")):
			if f, ok := anyToFloat(val); ok {
				plot.ELValue = &f
				changed = true
			}
		case plot.ILValue == nil && (kl == "il" || kl == "il_value" || strings.HasSuffix(kl, "_il")):
			if f, ok := anyToFloat(val); ok {
				plot.ILValue = &f
				changed = true
			}
		case plot.RESValue == nil && (kl == "res" || kl == "res_value" || strings.HasSuffix(kl, "_res")):
			if f, ok := anyToFloat(val); ok {
				plot.RESValue = &f
				changed = true
			}
		case plot.DimensionsString == "" && (strings.Contains(kl, "dimension") || kl == "dim" || kl == "cotes"):
			if s := strings.TrimSpace(fmt.Sprint(val)); s != "" && dimensionPartCount(s) >= 3 {
				plot.DimensionsString = normalizeDimensionsString(s)
				changed = true
			}
		}
	}
	return changed
}

func anyToFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		return parseFloatString(t)
	default:
		return 0, false
	}
}

func plotNeedsRawHydration(plot *models.HabitatPlot) bool {
	if plot == nil {
		return false
	}
	return plot.ELValue == nil ||
		plot.ILValue == nil ||
		plot.RESValue == nil ||
		plot.DimensionsString == "" ||
		len(plot.SidesM) == 0 ||
		plot.LengthM == nil ||
		plot.WidthM == nil ||
		plotAreaMissing(plot.AreaM2, plot.AreaRounded)
}

func hydrateHabitatPlotsFromRawProperties(db *gorm.DB, plots []models.HabitatPlot) {
	if db == nil || len(plots) == 0 {
		return
	}

	needIDs := make([]uint, 0, len(plots))
	for i := range plots {
		if plotNeedsRawHydration(&plots[i]) {
			needIDs = append(needIDs, plots[i].ID)
		}
	}
	if len(needIDs) > 0 {
		var rawRows []models.HabitatPlot
		_ = db.Select("id", "raw_properties").
			Where("id IN ?", needIDs).
			Find(&rawRows).Error
		rawByID := make(map[uint]datatypes.JSON, len(rawRows))
		for _, row := range rawRows {
			rawByID[row.ID] = row.RawProperties
		}
		for i := range plots {
			if raw, ok := rawByID[plots[i].ID]; ok && len(raw) > 0 {
				plots[i].RawProperties = raw
			}
		}
	}

	for i := range plots {
		extractHabitatPlotFromRawProperties(&plots[i])
		extractHabitatPlotFromCorners(&plots[i])
	}
}

func normalizeDimensionsString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Keep cadastre formatting but normalize separators.
	s = strings.ReplaceAll(s, "×", " x ")
	s = strings.ReplaceAll(s, "*", " x ")
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func firstString(props map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := props[key]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func firstFloat(props map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		v, ok := props[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t, true
		case float32:
			return float64(t), true
		case int:
			return float64(t), true
		case int64:
			return float64(t), true
		case json.Number:
			f, err := t.Float64()
			if err == nil {
				return f, true
			}
		case string:
			s := strings.TrimSpace(strings.ReplaceAll(t, ",", "."))
			if s == "" {
				continue
			}
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func firstFloatSlice(props map[string]any, keys ...string) []float64 {
	for _, key := range keys {
		v, ok := props[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case []any:
			out := make([]float64, 0, len(t))
			for _, item := range t {
				switch n := item.(type) {
				case float64:
					out = append(out, n)
				case float32:
					out = append(out, float64(n))
				case int:
					out = append(out, float64(n))
				case int64:
					out = append(out, float64(n))
				case json.Number:
					f, err := n.Float64()
					if err == nil {
						out = append(out, f)
					}
				case string:
					s := strings.TrimSpace(strings.ReplaceAll(n, ",", "."))
					if f, err := strconv.ParseFloat(s, 64); err == nil {
						out = append(out, f)
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case []float64:
			if len(t) > 0 {
				return t
			}
		case string:
			var arr []float64
			if err := json.Unmarshal([]byte(t), &arr); err == nil && len(arr) > 0 {
				return arr
			}
		}
	}
	return nil
}

func firstLengthWidth(props map[string]any) (length, width float64, ok bool) {
	if l, okL := firstFloat(props, "length_m", "length", "longueur", "LONGUEUR", "L_m"); okL {
		if w, okW := firstFloat(props, "width_m", "width", "largeur", "LARGEUR", "l_m"); okW {
			return l, w, true
		}
	}
	return 0, 0, false
}

func backfillHabitatPlotColumnsFromRaw(db *gorm.DB, batchSize int) (updated int, err error) {
	if db == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	var lastID uint
	for {
		var batch []models.HabitatPlot
		q := db.Select("id", "plot_number", "l_value", "i_value", "area_m2", "area_rounded",
			"dimensions_string", "length_m", "width_m", "sides_m", "perimeter_m",
			"il_value", "el_value", "res_value", "raw_properties",
			"geom_geojson", "corners").
			Where(`(
				(raw_properties IS NOT NULL AND raw_properties::text NOT IN ('null', '{}'))
				OR (geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}'))
				OR (corners IS NOT NULL AND corners::text NOT IN ('null', '{}'))
			)`).
			Order("id ASC").
			Limit(batchSize)
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&batch).Error; err != nil {
			return updated, err
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			p := batch[i]
			fillHabitatPlotDerivedFields(&p)
			if p.PlotNumber == "" && p.DimensionsString == "" && len(p.SidesM) == 0 &&
				p.ELValue == nil && p.ILValue == nil && p.RESValue == nil &&
				plotAreaMissing(p.AreaM2, p.AreaRounded) {
				continue
			}
			if err := db.Model(&models.HabitatPlot{}).Where("id = ?", p.ID).Updates(map[string]interface{}{
				"plot_number":        p.PlotNumber,
				"l_value":            p.LValue,
				"i_value":            p.IValue,
				"area_m2":            p.AreaM2,
				"area_rounded":       p.AreaRounded,
				"perimeter_m":        p.PerimeterM,
				"dimensions_string":  p.DimensionsString,
				"length_m":           p.LengthM,
				"width_m":            p.WidthM,
				"sides_m":            p.SidesM,
				"il_value":           p.ILValue,
				"el_value":           p.ELValue,
				"res_value":          p.RESValue,
			}).Error; err != nil {
				return updated, err
			}
			updated++
		}
		lastID = batch[len(batch)-1].ID
	}
	return updated, nil
}
