package tide

import (
	"context"
	"errors"
	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// Define the error that would normally come from the station package
var ErrStationNotFound = errors.New("station not found")

type mockStationFinder struct{}

func (m *mockStationFinder) FindNearestStations(_ context.Context, lat, lon float64, _ int) ([]models.Station, error) {
	return []models.Station{
		{
			ID:             "1234567",
			Name:           "Test Station",
			Latitude:       lat,
			Longitude:      lon,
			TimeZoneOffset: 0,
		},
	}, nil
}

func (m *mockStationFinder) FindStation(_ context.Context, stationID string) (*models.Station, error) {
	if stationID == "1234567" {
		return &models.Station{
			ID:             "1234567",
			Name:           "Test Station",
			Latitude:       42.0,
			Longitude:      -70.0,
			TimeZoneOffset: 0,
		}, nil
	}
	return nil, ErrStationNotFound
}

func TestGetCurrentTide(t *testing.T) {
	mockService := &Service{
		HttpClient:      &client.Client{},            // Mock HTTP client
		StationFinder:   &mockStationFinder{},        // Mock station finder
		PredictionCache: cache.NewMockCacheService(), // Mock cache service
	}

	tests := []struct {
		name       string
		lat        float64
		lon        float64
		wantErr    bool
		errMessage string
	}{
		{
			name:    "valid coordinates",
			lat:     42.0,
			lon:     -70.0,
			wantErr: false,
		},
		{
			name:       "invalid coordinates",
			lat:        181.0, // Invalid latitude
			lon:        -70.0,
			wantErr:    true,
			errMessage: "invalid latitude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			response, err := mockService.GetCurrentTide(ctx, tt.lat, tt.lon, nil, nil)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, "tide", response.ResponseType)
			assert.NotEmpty(t, response.NearestStation)
		})
	}
}

func TestGetCurrentTideForStation(t *testing.T) {
	mockService := &Service{
		HttpClient:      &client.Client{},            // Mock HTTP client
		StationFinder:   &mockStationFinder{},        // Mock station finder
		PredictionCache: cache.NewMockCacheService(), // Mock cache service
	}

	tests := []struct {
		name       string
		stationID  string
		wantErr    bool
		errMessage string
	}{
		{
			name:      "valid station",
			stationID: "1234567",
			wantErr:   false,
		},
		{
			name:       "invalid station",
			stationID:  "invalid",
			wantErr:    true,
			errMessage: "station not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			response, err := mockService.GetCurrentTideForStation(ctx, tt.stationID, nil, nil)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, "tide", response.ResponseType)
			assert.Equal(t, tt.stationID, response.NearestStation)
		})
	}
}

func TestInterpolation(t *testing.T) {
	tests := []struct {
		name           string
		predictions    []models.TidePrediction
		targetTime     int64
		expectedHeight float64
		tolerance      float64
	}{
		{
			name: "exact match",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			targetTime:     1000,
			expectedHeight: 2.0,
			tolerance:      0.001,
		},
		{
			name: "midpoint interpolation",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 4.0},
			},
			targetTime:     1500,
			expectedHeight: 3.0,
			tolerance:      0.001,
		},
		{
			name: "before first prediction",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			targetTime:     500,
			expectedHeight: 2.0, // Should use first prediction
			tolerance:      0.001,
		},
		{
			name: "after last prediction",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			targetTime:     2500,
			expectedHeight: 3.0, // Should use last prediction
			tolerance:      0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolatePredictions(tt.predictions, tt.targetTime)
			assert.InDelta(t, tt.expectedHeight, result, tt.tolerance)
		})
	}
}

func TestDateRangeValidation(t *testing.T) {
	mockService := &Service{
		HttpClient:      &client.Client{},            // Mock HTTP client
		StationFinder:   &mockStationFinder{},        // Mock station finder
		PredictionCache: cache.NewMockCacheService(), // Mock cache service
	}
	ctx := context.Background()

	tests := []struct {
		name        string
		startTime   *string
		endTime     *string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid one day range",
			startTime: stringPtr("2024-01-01T00:00:00"),
			endTime:   stringPtr("2024-01-02T00:00:00"),
			wantErr:   false,
		},
		{
			name:      "valid partial day",
			startTime: stringPtr("2024-01-01T12:00:00"),
			endTime:   stringPtr("2024-01-02T12:00:00"),
			wantErr:   false,
		},
		{
			name:        "range too large",
			startTime:   stringPtr("2024-01-01T00:00:00"),
			endTime:     stringPtr("2024-01-07T00:00:00"), // 6 days
			wantErr:     true,
			errContains: "date range cannot exceed 5 days",
		},
		{
			name:        "invalid date format",
			startTime:   stringPtr("2024-01-01"), // Missing time component
			endTime:     stringPtr("2024-01-02T00:00:00"),
			wantErr:     true,
			errContains: "parsing start time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mockService.GetCurrentTideForStation(ctx, "1234567", tt.startTime, tt.endTime)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

// Benchmarks
func BenchmarkInterpolation(b *testing.B) {
	predictions := []models.TidePrediction{
		{Timestamp: 1000, Height: 2.0},
		{Timestamp: 2000, Height: 3.0},
		{Timestamp: 3000, Height: 4.0},
		{Timestamp: 4000, Height: 3.5},
		{Timestamp: 5000, Height: 2.5},
	}
	targetTime := int64(2500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = interpolatePredictions(predictions, targetTime)
	}
}
