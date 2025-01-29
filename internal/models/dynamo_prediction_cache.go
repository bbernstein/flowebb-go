package models

import (
	"fmt"
	"time"
)

// models.TidePredictionRecord represents a cached set of predictions for a station and date
type TidePredictionRecord struct {
	StationID   string           `dynamodbav:"stationId"`
	Date        string           `dynamodbav:"date"`
	StationType string           `dynamodbav:"stationType"` // R for reference, S for subordinate
	Predictions []TidePrediction `dynamodbav:"predictions"`
	Extremes    []TideExtreme    `dynamodbav:"extremes"`
	LastUpdated int64            `dynamodbav:"lastUpdated"`
	TTL         int64            `dynamodbav:"ttl"`
}

// Validate checks if a TidePredictionRecord's fields are valid
func (r *TidePredictionRecord) Validate() error {
	if r.StationID == "" {
		return fmt.Errorf("station ID is required")
	}

	if r.Date == "" {
		return fmt.Errorf("date is required")
	}

	// Validate date format
	_, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return fmt.Errorf("invalid date format: %s", r.Date)
	}

	// Validate StationType (R for reference, S for subordinate)
	switch r.StationType {
	case "R", "S":
		// Valid type
	default:
		return fmt.Errorf("invalid station type: %s", r.StationType)
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
