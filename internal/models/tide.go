package models

import (
	"fmt"
	"time"
)

type TideType string

const (
	TideTypeRising TideType = "RISING"
	TideFalling    TideType = "FALLING"
	TideTypeHigh   TideType = "HIGH"
	TideTypeLow    TideType = "LOW"
)

// TideExtreme represents a high or low tide
type TideExtreme struct {
	Type      TideType `json:"type"`
	Timestamp int64    `json:"timestamp"`
	LocalTime string   `json:"localTime"`
	Height    float64  `json:"height"`
}

// TidePrediction represents a tide prediction at a specific time
type TidePrediction struct {
	Timestamp int64   `json:"timestamp"`
	LocalTime string  `json:"localTime"`
	Height    float64 `json:"height"`
}

func (tp TidePrediction) GetTimestamp() int64 {
	return tp.Timestamp
}

// ExtendedTideResponse represents the full tide response including predictions and extremes
type ExtendedTideResponse struct {
	ResponseType          string           `json:"responseType"`
	Timestamp             int64            `json:"timestamp"`
	LocalTime             string           `json:"localTime"` // Add this field
	WaterLevel            *float64         `json:"waterLevel"`
	PredictedLevel        *float64         `json:"predictedLevel"`
	NearestStation        string           `json:"nearestStation"`
	Location              *string          `json:"location"`
	Latitude              float64          `json:"latitude"`
	Longitude             float64          `json:"longitude"`
	StationDistance       float64          `json:"stationDistance"`
	TideType              *TideType        `json:"tideType"`
	CalculationMethod     string           `json:"calculationMethod"`
	Extremes              []TideExtreme    `json:"extremes"`
	Predictions           []TidePrediction `json:"predictions"`
	TimeZoneOffsetSeconds *int             `json:"timeZoneOffsetSeconds"`
}

// NoaaPrediction represents the raw NOAA API prediction response
type NoaaPrediction struct {
	Time   string  `json:"t"`              // Time of prediction
	Height string  `json:"v"`              // Predicted water level
	Type   *string `json:"type,omitempty"` // Type of prediction (H for high, L for low)
}

type NoaaResponse struct {
	Predictions []NoaaPrediction `json:"predictions"`
	Error       *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Validate checks if a TidePrediction's fields are valid
func (tp *TidePrediction) Validate() error {
	if tp.Timestamp <= 0 {
		return fmt.Errorf("invalid timestamp: %d", tp.Timestamp)
	}

	// Validate LocalTime is parseable and matches timestamp
	if tp.LocalTime != "" {
		t, err := time.Parse("2006-01-02T15:04:05", tp.LocalTime)
		if err != nil {
			return fmt.Errorf("invalid local time format: %s", tp.LocalTime)
		}
		timeDiff := func(a int64) int64 {
			if a < 0 {
				a = -a
			}
			return a
		}(t.UnixMilli() - tp.Timestamp)

		// must be within 24 hours of localtime
		if timeDiff > 1000*60*60*24 {
			return fmt.Errorf("local time does not match timestamp")
		}
	}

	return nil
}

// Validate checks if a TideExtreme's fields are valid
func (te *TideExtreme) Validate() error {
	if te.Timestamp <= 0 {
		return fmt.Errorf("invalid timestamp: %d", te.Timestamp)
	}

	// Validate TideType is one of the allowed values
	switch te.Type {
	case TideTypeRising, TideFalling, TideTypeHigh, TideTypeLow:
		// Valid type
	default:
		return fmt.Errorf("invalid tide type: %s", te.Type)
	}

	// Validate LocalTime
	if te.LocalTime != "" {
		t, err := time.Parse("2006-01-02T15:04:05", te.LocalTime)
		if err != nil {
			return fmt.Errorf("invalid local time format: %s", te.LocalTime)
		}

		timeDiff := func(a int64) int64 {
			if a < 0 {
				a = -a
			}
			return a
		}(t.UnixMilli() - te.Timestamp)

		// must be within 24 hours of localtime
		if timeDiff > 1000*60*60*24 {
			return fmt.Errorf("local time does not match timestamp")
		}
	}

	return nil
}

// Validate checks if an ExtendedTideResponse's fields are valid
func (r *ExtendedTideResponse) Validate() error {
	if r.Timestamp <= 0 {
		return fmt.Errorf("invalid timestamp: %d", r.Timestamp)
	}

	if r.NearestStation == "" {
		return fmt.Errorf("nearest station is required")
	}

	// Validate latitude range (-90 to 90)
	if r.Latitude < -90 || r.Latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", r.Latitude)
	}

	// Validate longitude range (-180 to 180)
	if r.Longitude < -180 || r.Longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", r.Longitude)
	}

	// Validate StationDistance is non-negative
	if r.StationDistance < 0 {
		return fmt.Errorf("invalid station distance: %f", r.StationDistance)
	}

	// Validate TideType if present
	if r.TideType != nil {
		switch *r.TideType {
		case TideTypeRising, TideFalling, TideTypeHigh, TideTypeLow:
			// Valid type
		default:
			return fmt.Errorf("invalid tide type: %s", *r.TideType)
		}
	}

	// Validate TimeZoneOffsetSeconds if present
	if r.TimeZoneOffsetSeconds != nil {
		if *r.TimeZoneOffsetSeconds < -43200 || *r.TimeZoneOffsetSeconds > 50400 {
			return fmt.Errorf("invalid timezone offset: %d", *r.TimeZoneOffsetSeconds)
		}
	}

	// Validate all predictions
	for i, pred := range r.Predictions {
		if err := pred.Validate(); err != nil {
			return fmt.Errorf("invalid prediction at index %d: %w", i, err)
		}
	}

	// Validate all extremes
	for i, extreme := range r.Extremes {
		if err := extreme.Validate(); err != nil {
			return fmt.Errorf("invalid extreme at index %d: %w", i, err)
		}
	}

	return nil
}
