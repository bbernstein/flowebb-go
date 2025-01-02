package main

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"net/http"
	"testing"
)

// Define the error that would normally come from the station package
var ErrStationNotFound = errors.New("station not found")

func newMockTideService() *tide.Service {
	return &tide.Service{
		HttpClient:      &client.Client{},            // Mock or real HTTP client
		StationFinder:   &mockStationFinder{},        // Mock station finder
		PredictionCache: cache.NewMockCacheService(), // Mock cache service
	}
}

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

func TestHandleRequest(t *testing.T) {
	// Replace the real tide service with our mock
	originalTideService := tideService
	mockService := newMockTideService()
	tideService = mockService
	defer func() { tideService = originalTideService }()

	testCases := []struct {
		name         string
		request      events.APIGatewayProxyRequest
		expectedCode int
	}{
		{
			name: "valid station ID request",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "1234567",
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "valid coordinates request",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "42.0",
					"lon": "-70.0",
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "invalid coordinates",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "invalid",
					"lon": "-70.0",
				},
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "missing parameters",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{},
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "with date range",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId":     "1234567",
					"startDateTime": "2024-01-01T00:00:00",
					"endDateTime":   "2024-01-02T00:00:00",
				},
			},
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			response, err := handleRequest(ctx, tc.request)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if response.StatusCode != tc.expectedCode {
				t.Errorf("expected status code %d but got %d", tc.expectedCode, response.StatusCode)
			}
		})
	}
}
