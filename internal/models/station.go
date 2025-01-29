package models

import "fmt"

type Source string

const (
	SourceNOAA Source = "NOAA"
	SourceUKHO Source = "UKHO"
	SourceCHS  Source = "CHS"
)

type Station struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	State          *string  `json:"state,omitempty"`
	Region         *string  `json:"region,omitempty"`
	Distance       float64  `json:"distance"`
	Latitude       float64  `json:"latitude"`
	Longitude      float64  `json:"longitude"`
	Source         Source   `json:"source"`
	Capabilities   []string `json:"capabilities"`
	TimeZoneOffset int      `json:"timeZoneOffset"`
	Level          *string  `json:"level,omitempty"`
	StationType    *string  `json:"stationType,omitempty"`
}

// Validate checks if a Station's fields are valid
func (s *Station) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("station ID is required")
	}

	// Validate latitude range (-90 to 90)
	if s.Latitude < -90 || s.Latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", s.Latitude)
	}

	// Validate longitude range (-180 to 180)
	if s.Longitude < -180 || s.Longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", s.Longitude)
	}

	// Validate Source is one of the allowed values
	switch s.Source {
	case SourceNOAA, SourceUKHO, SourceCHS:
		// Valid source
	default:
		return fmt.Errorf("invalid source: %s", s.Source)
	}

	// Validate TimeZoneOffset is within reasonable range (-12 to +14 hours in seconds)
	if s.TimeZoneOffset < -43200 || s.TimeZoneOffset > 50400 {
		return fmt.Errorf("invalid timezone offset: %d", s.TimeZoneOffset)
	}

	return nil
}
