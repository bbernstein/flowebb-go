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

func TestTideExtreme_Validate(t *testing.T) {
	tests := []struct {
		name      string
		extreme   TideExtreme
		wantErr   bool
		errString string
	}{
		{
			name: "invalid negative timestamp",
			extreme: TideExtreme{
				Type:      TideTypeHigh,
				Timestamp: -1,
				LocalTime: "2024-01-01T00:00:00",
				Height:    1.5,
			},
			wantErr:   true,
			errString: "invalid timestamp: -1",
		},
		{
			name: "invalid local time format",
			extreme: TideExtreme{
				Type:      TideTypeHigh,
				Timestamp: time.Now().UnixMilli(),
				LocalTime: "invalid-time-format",
				Height:    1.5,
			},
			wantErr:   true,
			errString: "invalid local time format",
		},
		{
			name: "local time more than 24 hours from timestamp",
			extreme: TideExtreme{
				Type:      TideTypeHigh,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
				LocalTime: "2024-01-03T00:00:00", // 48 hours difference
				Height:    1.5,
			},
			wantErr:   true,
			errString: "local time does not match timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.extreme.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTidePrediction_GetTimestamp(t *testing.T) {
	timestamp := time.Now().UnixMilli()
	prediction := TidePrediction{
		Timestamp: timestamp,
		LocalTime: time.Now().Format("2006-01-02T15:04:05"),
		Height:    1.5,
	}

	assert.Equal(t, timestamp, prediction.GetTimestamp())
}

func TestExtendedTideResponse_Validate_Additional(t *testing.T) {
	baseTime := time.Now()
	validPrediction := TidePrediction{
		Timestamp: baseTime.UnixMilli(),
		LocalTime: baseTime.Format("2006-01-02T15:04:05"),
		Height:    1.5,
	}
	validExtreme := TideExtreme{
		Type:      TideTypeHigh,
		Timestamp: baseTime.UnixMilli(),
		LocalTime: baseTime.Format("2006-01-02T15:04:05"),
		Height:    1.5,
	}

	tests := []struct {
		name      string
		response  ExtendedTideResponse
		wantErr   bool
		errString string
	}{
		{
			name: "missing nearest station",
			response: ExtendedTideResponse{
				Timestamp:  time.Now().UnixMilli(),
				WaterLevel: &[]float64{1.5}[0],
			},
			wantErr:   true,
			errString: "nearest station is required",
		},
		{
			name: "invalid longitude",
			response: ExtendedTideResponse{
				Timestamp:      time.Now().UnixMilli(),
				NearestStation: "TEST001",
				Longitude:      181.0,
			},
			wantErr:   true,
			errString: "invalid longitude: 181.000000",
		},
		{
			name: "negative station distance",
			response: ExtendedTideResponse{
				Timestamp:       time.Now().UnixMilli(),
				NearestStation:  "TEST001",
				Longitude:       -122.3321,
				StationDistance: -1.0,
			},
			wantErr:   true,
			errString: "invalid station distance: -1.000000",
		},
		{
			name: "invalid tide type",
			response: ExtendedTideResponse{
				Timestamp:      time.Now().UnixMilli(),
				NearestStation: "TEST001",
				Longitude:      -122.3321,
				TideType:       &[]TideType{"INVALID"}[0],
			},
			wantErr:   true,
			errString: "invalid tide type: INVALID",
		},
		{
			name: "invalid timezone offset",
			response: ExtendedTideResponse{
				Timestamp:             time.Now().UnixMilli(),
				NearestStation:        "TEST001",
				Longitude:             -122.3321,
				TimeZoneOffsetSeconds: &[]int{-50000}[0], // Too negative
			},
			wantErr:   true,
			errString: "invalid timezone offset: -50000",
		},
		{
			name: "invalid prediction in array",
			response: ExtendedTideResponse{
				Timestamp:      time.Now().UnixMilli(),
				NearestStation: "TEST001",
				Longitude:      -122.3321,
				Predictions: []TidePrediction{
					validPrediction,
					{Timestamp: -1}, // Invalid prediction
				},
			},
			wantErr:   true,
			errString: "invalid prediction at index 1",
		},
		{
			name: "invalid extreme in array",
			response: ExtendedTideResponse{
				Timestamp:      time.Now().UnixMilli(),
				NearestStation: "TEST001",
				Longitude:      -122.3321,
				Extremes: []TideExtreme{
					validExtreme,
					{Timestamp: -1}, // Invalid extreme
				},
			},
			wantErr:   true,
			errString: "invalid extreme at index 1",
		},
		{
			name: "valid response with all fields",
			response: ExtendedTideResponse{
				ResponseType:          "tide",
				Timestamp:             time.Now().UnixMilli(),
				NearestStation:        "TEST001",
				Longitude:             -122.3321,
				Latitude:              47.6062,
				Predictions:           []TidePrediction{validPrediction},
				Extremes:              []TideExtreme{validExtreme},
				TimeZoneOffsetSeconds: &[]int{-28800}[0], // -8 hours
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				return
			}
			require.NoError(t, err)
		})
	}
}

// Add benchmarks for validation
func BenchmarkTideExtreme_Validate(b *testing.B) {
	extreme := TideExtreme{
		Type:      TideTypeHigh,
		Timestamp: time.Now().UnixMilli(),
		LocalTime: time.Now().Format("2006-01-02T15:04:05"),
		Height:    1.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extreme.Validate()
	}
}
