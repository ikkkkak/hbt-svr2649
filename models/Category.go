package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// CategoryNames represents multilingual category names
type CategoryNames struct {
	En string `json:"en"`
	Fr string `json:"fr"`
	Ar string `json:"ar"`
}

// Value implements the driver.Valuer interface for database storage
func (cn CategoryNames) Value() (driver.Value, error) {
	return json.Marshal(cn)
}

// Scan implements the sql.Scanner interface for database retrieval
func (cn *CategoryNames) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, cn)
}

// Category represents a property or experience category
type Category struct {
	ID          int           `json:"id" db:"id"`
	Type        string        `json:"type" db:"type"` // "property" or "experience"
	Name        CategoryNames `json:"name" db:"name"`
	Icon        string        `json:"icon" db:"icon"` // Phosphor icon name
	Description CategoryNames `json:"description" db:"description"`
	IsActive    bool          `json:"is_active" db:"is_active"`
	SortOrder   int           `json:"sort_order" db:"sort_order"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
}

// AmenityNames represents multilingual amenity names
type AmenityNames struct {
	En string `json:"en"`
	Fr string `json:"fr"`
	Ar string `json:"ar"`
}

// Value implements the driver.Valuer interface for database storage
func (an AmenityNames) Value() (driver.Value, error) {
	return json.Marshal(an)
}

// Scan implements the sql.Scanner interface for database retrieval
func (an *AmenityNames) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, an)
}

// Amenity represents a property amenity
type Amenity struct {
	ID          int          `json:"id" db:"id"`
	Name        AmenityNames `json:"name" db:"name"`
	Icon        string       `json:"icon" db:"icon"` // Phosphor icon name
	Category    string       `json:"category" db:"category"`
	Description AmenityNames `json:"description" db:"description"`
	IsActive    bool         `json:"is_active" db:"is_active"`
	SortOrder   int          `json:"sort_order" db:"sort_order"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
}
