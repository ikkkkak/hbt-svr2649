package models

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Float64Slice maps to PostgreSQL double precision[].
type Float64Slice []float64

func (s Float64Slice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	if len(s) == 0 {
		return "{}", nil
	}
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (s *Float64Slice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	switch v := value.(type) {
	case string:
		return s.parsePGArray(v)
	case []byte:
		return s.parsePGArray(string(v))
	default:
		return fmt.Errorf("Float64Slice: unsupported type %T", value)
	}
}

func (s *Float64Slice) parsePGArray(src string) error {
	src = strings.TrimSpace(src)
	if src == "" || src == "{}" {
		*s = Float64Slice{}
		return nil
	}
	src = strings.TrimPrefix(src, "{")
	src = strings.TrimSuffix(src, "}")
	if src == "" {
		*s = Float64Slice{}
		return nil
	}
	parts := strings.Split(src, ",")
	out := make(Float64Slice, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.EqualFold(p, "null") {
			continue
		}
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return err
		}
		out = append(out, f)
	}
	*s = out
	return nil
}

// HabitatPlan is a district/zone in the cadastre (e.g. Tevergh Zeina, Arafat).
type HabitatPlan struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	Code          string         `json:"code" gorm:"size:50;uniqueIndex;not null"`
	Name          string         `json:"name" gorm:"size:100;not null"`
	NameAr        string         `json:"name_ar" gorm:"size:100;not null"`
	Color         string         `json:"color" gorm:"size:20;default:'#2a5298'"`
	BoundsGeoJSON datatypes.JSON `json:"bounds_geojson,omitempty" gorm:"type:jsonb"`
	CentroidLat   *float64       `json:"centroid_lat,omitempty"`
	CentroidLng   *float64       `json:"centroid_lng,omitempty"`
	SectorCount   int            `json:"sector_count" gorm:"default:0"`
	PlotCount     int64          `json:"plot_count" gorm:"default:0"`
	TotalAreaM2   float64        `json:"total_area_m2" gorm:"default:0"`
	Metadata      datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb;default:'{}'"`
	IsActive      bool           `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Sectors []HabitatSector `json:"sectors,omitempty" gorm:"foreignKey:PlanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Plots   []HabitatPlot   `json:"plots,omitempty" gorm:"foreignKey:PlanID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (HabitatPlan) TableName() string { return "habitat_plans" }

// HabitatSector is a sub-zone within a plan.
type HabitatSector struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	PlanID        uint           `json:"plan_id" gorm:"not null;index:idx_habitat_sectors_plan;uniqueIndex:idx_habitat_sector_plan_name,composite:name;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Name          string         `json:"name" gorm:"size:200;not null;uniqueIndex:idx_habitat_sector_plan_name,composite:plan_id"`
	NameAr        string         `json:"name_ar" gorm:"size:200"`
	Code          string         `json:"code" gorm:"size:100"`
	BoundsGeoJSON datatypes.JSON `json:"bounds_geojson,omitempty" gorm:"type:jsonb"`
	CentroidLat   *float64       `json:"centroid_lat,omitempty"`
	CentroidLng   *float64       `json:"centroid_lng,omitempty"`
	PlotCount     int            `json:"plot_count" gorm:"default:0"`
	TotalAreaM2   float64        `json:"total_area_m2" gorm:"default:0"`
	OriginalSize  *int           `json:"original_size,omitempty"`
	OriginalLat   *float64       `json:"original_lat,omitempty"`
	OriginalLng   *float64       `json:"original_lng,omitempty"`
	Metadata      datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb;default:'{}'"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Plan  HabitatPlan   `json:"plan,omitempty" gorm:"foreignKey:PlanID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Plots []HabitatPlot `json:"plots,omitempty" gorm:"foreignKey:SectorID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (HabitatSector) TableName() string { return "habitat_sectors" }

// HabitatPlot is an individual land parcel.
type HabitatPlot struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	PlanID           uint           `json:"plan_id" gorm:"not null;index:idx_habitat_plots_plan;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	SectorID         uint           `json:"sector_id" gorm:"not null;index:idx_habitat_plots_sector;uniqueIndex:unique_plot_per_sector"`
	PlotNumber       string         `json:"plot_number" gorm:"size:100;not null;uniqueIndex:unique_plot_per_sector"`
	LValue           string         `json:"l_value,omitempty" gorm:"size:100"`
	IValue           *int           `json:"i_value,omitempty"`
	AreaM2           *float64       `json:"area_m2,omitempty"`
	AreaRounded      *int           `json:"area_rounded,omitempty"`
	AreaHectares     *float64       `json:"area_hectares,omitempty"`
	PerimeterM       *float64       `json:"perimeter_m,omitempty"`
	SidesM           Float64Slice   `json:"sides_m,omitempty" gorm:"type:double precision[]"`
	DimensionsString string         `json:"dimensions_string,omitempty" gorm:"size:200"`
	LengthM          *float64       `json:"length_m,omitempty"`
	WidthM           *float64       `json:"width_m,omitempty"`
	ILValue          *float64       `json:"il_value,omitempty" gorm:"column:il_value"`
	ELValue          *float64       `json:"el_value,omitempty" gorm:"column:el_value"`
	RESValue         *float64       `json:"res_value,omitempty" gorm:"column:res_value"`
	GeomGeoJSON      datatypes.JSON `json:"geom_geojson,omitempty" gorm:"type:jsonb"`
	CentroidLat      *float64       `json:"centroid_lat,omitempty"`
	CentroidLng      *float64       `json:"centroid_lng,omitempty"`
	Corners          datatypes.JSON `json:"corners,omitempty" gorm:"type:jsonb"`
	RawProperties    datatypes.JSON `json:"raw_properties,omitempty" gorm:"type:jsonb"`
	DataFilePath     string         `json:"data_file_path,omitempty" gorm:"size:500"`
	// Computed at query time via JOIN on verified+published landmarks.
	// Not a physical DB column; defaults to false when not selected.
	IsForSale        bool           `json:"is_for_sale" gorm:"->;column:is_for_sale"`
	Version          int            `json:"version" gorm:"default:1"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Plan   HabitatPlan   `json:"plan,omitempty" gorm:"foreignKey:PlanID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Sector HabitatSector `json:"sector,omitempty" gorm:"foreignKey:SectorID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (HabitatPlot) TableName() string { return "habitat_plots" }
