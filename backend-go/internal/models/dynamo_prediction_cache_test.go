package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTidePredictionRecord_Validate(t *testing.T) {
	// Helper function to create a valid base record
	createValidRecord := func() TidePredictionRecord {
		now := time.Now()
		return TidePredictionRecord{
			StationID:   "TEST001",
			Date:        now.Format("2006-01-02"),
			StationType: "R",
			Predictions: []TidePrediction{
				{
					Timestamp: now.UnixMilli(),
					LocalTime: now.Format("2006-01-02T15:04:05"),
					Height:    1.5,
				},
			},
			Extremes: []TideExtreme{
				{
					Type:      TideTypeHigh,
					Timestamp: now.UnixMilli(),
					LocalTime: now.Format("2006-01-02T15:04:05"),
					Height:    2.0,
				},
			},
			LastUpdated: now.Unix(),
			TTL:         now.Add(24 * time.Hour).Unix(),
		}
	}

	tests := []struct {
		name        string
		modifyFunc  func(*TidePredictionRecord)
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid record",
			modifyFunc: func(r *TidePredictionRecord) {},
			wantErr:    false,
		},
		{
			name: "missing station ID",
			modifyFunc: func(r *TidePredictionRecord) {
				r.StationID = ""
			},
			wantErr:     true,
			errContains: "station ID is required",
		},
		{
			name: "missing date",
			modifyFunc: func(r *TidePredictionRecord) {
				r.Date = ""
			},
			wantErr:     true,
			errContains: "date is required",
		},
		{
			name: "invalid date format",
			modifyFunc: func(r *TidePredictionRecord) {
				r.Date = "01-02-2006" // Wrong format
			},
			wantErr:     true,
			errContains: "invalid date format",
		},
		{
			name: "invalid station type",
			modifyFunc: func(r *TidePredictionRecord) {
				r.StationType = "X" // Should be R or S
			},
			wantErr:     true,
			errContains: "invalid station type",
		},
		{
			name: "invalid prediction",
			modifyFunc: func(r *TidePredictionRecord) {
				r.Predictions[0].Timestamp = -1 // Invalid timestamp
			},
			wantErr:     true,
			errContains: "invalid prediction at index 0",
		},
		{
			name: "invalid extreme",
			modifyFunc: func(r *TidePredictionRecord) {
				r.Extremes[0].Type = "INVALID" // Invalid tide type
			},
			wantErr:     true,
			errContains: "invalid extreme at index 0",
		},
		{
			name: "mismatched prediction timestamp and local time",
			modifyFunc: func(r *TidePredictionRecord) {
				r.Predictions[0].LocalTime = time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04:05")
			},
			wantErr:     true,
			errContains: "invalid prediction at index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := createValidRecord()
			tt.modifyFunc(&record)

			err := record.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Add performance benchmark
func BenchmarkTidePredictionRecord_Validate(b *testing.B) {
	record := TidePredictionRecord{
		StationID:   "TEST001",
		Date:        time.Now().Format("2006-01-02"),
		StationType: "R",
		Predictions: make([]TidePrediction, 100), // Test with 100 predictions
		Extremes:    make([]TideExtreme, 10),     // and 10 extremes
		LastUpdated: time.Now().Unix(),
		TTL:         time.Now().Add(24 * time.Hour).Unix(),
	}

	// Fill with valid data
	now := time.Now()
	for i := range record.Predictions {
		ts := now.Add(time.Duration(i) * time.Hour)
		record.Predictions[i] = TidePrediction{
			Timestamp: ts.UnixMilli(),
			LocalTime: ts.Format("2006-01-02T15:04:05"),
			Height:    float64(i) / 10,
		}
	}

	for i := range record.Extremes {
		ts := now.Add(time.Duration(i*6) * time.Hour)
		record.Extremes[i] = TideExtreme{
			Type:      TideTypeHigh,
			Timestamp: ts.UnixMilli(),
			LocalTime: ts.Format("2006-01-02T15:04:05"),
			Height:    float64(i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = record.Validate()
	}
}
