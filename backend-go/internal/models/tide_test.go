package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTideResponseSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response ExtendedTideResponse
		wantErr  bool
	}{
		{
			name: "complete valid response",
			response: ExtendedTideResponse{
				ResponseType:          "tide",
				Timestamp:             1672531200000, // 2023-01-01 00:00:00
				LocalTime:             "2023-01-01T00:00:00",
				NearestStation:        "TEST001",
				Location:              stringPtr("Test Location"),
				Latitude:              47.6062,
				Longitude:             -122.3321,
				StationDistance:       0.5,
				TideType:              tideTypePtr(TideTypeHigh),
				CalculationMethod:     "NOAA API",
				TimeZoneOffsetSeconds: intPtr(-28800), // -8 hours
				WaterLevel:            float64Ptr(4.5),
				PredictedLevel:        float64Ptr(4.6),
				Predictions: []TidePrediction{
					{
						Timestamp: 1672531200000,
						LocalTime: "2023-01-01T00:00:00",
						Height:    4.5,
					},
				},
				Extremes: []TideExtreme{
					{
						Type:      TideTypeHigh,
						Timestamp: 1672531200000,
						LocalTime: "2023-01-01T00:00:00",
						Height:    4.5,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing required fields",
			response: ExtendedTideResponse{
				ResponseType: "tide",
				// Missing Timestamp
				NearestStation: "TEST001",
			},
			wantErr: true,
		},
		{
			name: "invalid coordinates",
			response: ExtendedTideResponse{
				ResponseType:   "tide",
				Timestamp:      1672531200000,
				NearestStation: "TEST001",
				Latitude:       91.0, // Invalid latitude
				Longitude:      -122.3321,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test marshaling
			data, err := json.Marshal(tt.response)
			require.NoError(t, err, "marshaling should not error")

			// Test unmarshaling
			var unmarshaled ExtendedTideResponse
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err, "unmarshaling should not error")

			// Validate the unmarshaled data
			err = unmarshaled.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Compare specific fields
			assert.Equal(t, tt.response.ResponseType, unmarshaled.ResponseType)
			assert.Equal(t, tt.response.Timestamp, unmarshaled.Timestamp)
			assert.Equal(t, tt.response.NearestStation, unmarshaled.NearestStation)
		})
	}
}

func TestTideTypeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tideType  TideType
		wantValid bool
	}{
		{
			name:      "valid rising",
			tideType:  TideTypeRising,
			wantValid: true,
		},
		{
			name:      "valid falling",
			tideType:  TideFalling,
			wantValid: true,
		},
		{
			name:      "valid high",
			tideType:  TideTypeHigh,
			wantValid: true,
		},
		{
			name:      "valid low",
			tideType:  TideTypeLow,
			wantValid: true,
		},
		{
			name:      "invalid type",
			tideType:  TideType("INVALID"),
			wantValid: false,
		},
		{
			name:      "empty type",
			tideType:  TideType(""),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			extreme := TideExtreme{
				Type:      tt.tideType,
				Timestamp: time.Now().UnixMilli(),
				LocalTime: time.Now().Format("2006-01-02T15:04:05"),
				Height:    4.5,
			}

			err := extreme.Validate()
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid tide type")
			}
		})
	}
}

func TestTimeFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		timestamp int64
		localTime string
		wantErr   bool
	}{
		{
			name:      "valid time format",
			timestamp: 1672531200000, // 2023-01-01 00:00:00
			localTime: "2023-01-01T00:00:00",
			wantErr:   false,
		},
		{
			name:      "invalid time format",
			timestamp: 1672531200000,
			localTime: "2023-01-01", // Missing time component
			wantErr:   true,
		},
		{
			name:      "mismatched timestamp and local time",
			timestamp: 1672531200000,         // 2023-01-01 00:00:00
			localTime: "2024-01-01T00:00:00", // Different year
			wantErr:   true,
		},
		{
			name:      "invalid timestamp",
			timestamp: -1, // Invalid negative timestamp
			localTime: "2023-01-01T00:00:00",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prediction := TidePrediction{
				Timestamp: tt.timestamp,
				LocalTime: tt.localTime,
				Height:    4.5,
			}

			err := prediction.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// Helper functions for creating pointers to primitives
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func tideTypePtr(t TideType) *TideType {
	return &t
}

// Benchmark tide response validation
func BenchmarkTideResponseValidation(b *testing.B) {
	response := ExtendedTideResponse{
		ResponseType:   "tide",
		Timestamp:      time.Now().UnixMilli(),
		LocalTime:      time.Now().Format(time.RFC3339),
		NearestStation: "TEST001",
		Location:       stringPtr("Test Location"),
		Latitude:       47.6062,
		Longitude:      -122.3321,
		TideType:       tideTypePtr(TideTypeHigh),
		Predictions:    make([]TidePrediction, 100),
		Extremes:       make([]TideExtreme, 10),
	}

	// Fill predictions and extremes with test data
	for i := 0; i < 100; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Hour).UnixMilli()
		response.Predictions[i] = TidePrediction{
			Timestamp: ts,
			LocalTime: time.Unix(ts/1000, 0).Format(time.RFC3339),
			Height:    float64(i) / 10,
		}
	}

	for i := 0; i < 10; i++ {
		ts := time.Now().Add(time.Duration(i*6) * time.Hour).UnixMilli()
		response.Extremes[i] = TideExtreme{
			Type:      TideTypeHigh,
			Timestamp: ts,
			LocalTime: time.Unix(ts/1000, 0).Format(time.RFC3339),
			Height:    float64(i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = response.Validate()
	}
}
