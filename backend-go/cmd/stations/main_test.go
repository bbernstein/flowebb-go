package main

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/handler"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"testing"
)

// mockStationFinder implements station.StationFinder interface for testing
type mockStationFinder struct {
	findStationFn         func(ctx context.Context, stationID string) (*models.Station, error)
	findNearestStationsFn func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error)
}

func (m *mockStationFinder) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	if m.findStationFn != nil {
		return m.findStationFn(ctx, stationID)
	}
	return nil, nil
}

func (m *mockStationFinder) FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
	if m.findNearestStationsFn != nil {
		return m.findNearestStationsFn(ctx, lat, lon, limit)
	}
	return nil, nil
}

// Helper function to create test stations
func createTestStation(id string) models.Station {
	state := "WA"
	region := "Puget Sound"
	level := "R"
	stationType := "R"
	return models.Station{
		ID:             id,
		Name:           "Test Station " + id,
		State:          &state,
		Region:         &region,
		Distance:       0,
		Latitude:       47.6062,
		Longitude:      -122.3321,
		Source:         models.SourceNOAA,
		Capabilities:   []string{"WATER_LEVEL"},
		TimeZoneOffset: -8 * 3600,
		Level:          &level,
		StationType:    &stationType,
	}
}

func TestMain(m *testing.M) {
	// Set up test environment
	err := os.Setenv("LOG_LEVEL", "debug")
	if err != nil {
		return
	}
	err = os.Setenv("ENV", "test")
	if err != nil {
		return
	}

	// Run tests
	code := m.Run()

	// Exit
	os.Exit(code)
}

func TestHandleRequest(t *testing.T) {
	tests := []struct {
		name           string
		request        events.APIGatewayProxyRequest
		setupMock      func() station.StationFinder
		expectedStatus int
		wantErr        bool
	}{
		{
			name: "successful station lookup by ID",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "TEST001",
				},
			},
			setupMock: func() station.StationFinder {
				return &mockStationFinder{
					findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
						testStation := createTestStation(stationID)
						return &testStation, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantErr:        false,
		},
		{
			name: "successful nearest stations lookup",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat":   "47.6062",
					"lon":   "-122.3321",
					"limit": "2",
				},
			},
			setupMock: func() station.StationFinder {
				return &mockStationFinder{
					findNearestStationsFn: func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
						return []models.Station{
							createTestStation("TEST001"),
							createTestStation("TEST002"),
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock and handler
			stationsHandler = handler.NewStationsHandler(tt.setupMock())

			// Call handler
			response, err := handleRequest(context.Background(), tt.request)

			// Verify response
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, response.StatusCode)

			// Verify response body structure
			var responseBody map[string]interface{}
			err = json.Unmarshal([]byte(response.Body), &responseBody)
			require.NoError(t, err)

			// Verify response contains expected fields
			assert.Contains(t, responseBody, "responseType")
			assert.Contains(t, responseBody, "stations")
		})
	}
}

func TestParameterValidation(t *testing.T) {
	tests := []struct {
		name           string
		request        events.APIGatewayProxyRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "invalid latitude",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "91", // Invalid: latitude > 90
					"lon": "0",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid coordinates",
		},
		{
			name: "invalid longitude",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "0",
					"lon": "181", // Invalid: longitude > 180
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid coordinates",
		},
		{
			name: "non-numeric coordinates",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "invalid",
					"lon": "-122.3321",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new handler with empty mock
			stationsHandler = handler.NewStationsHandler(&mockStationFinder{})

			// Call handler
			response, err := handleRequest(context.Background(), tt.request)

			// We don't expect errors from the handler itself
			require.NoError(t, err)

			// Verify response status and error message
			assert.Equal(t, tt.expectedStatus, response.StatusCode)

			var responseBody map[string]interface{}
			err = json.Unmarshal([]byte(response.Body), &responseBody)
			require.NoError(t, err)

			assert.Equal(t, "error", responseBody["responseType"])
			assert.Equal(t, tt.expectedError, responseBody["error"])
		})
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		request        events.APIGatewayProxyRequest
		setupMock      func() station.StationFinder
		expectedStatus int
		expectedError  string
	}{
		{
			name: "station not found",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "NONEXISTENT",
				},
			},
			setupMock: func() station.StationFinder {
				return &mockStationFinder{
					findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
						return nil, nil // Simulate station not found
					},
				}
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "Station not found",
		},
		{
			name: "internal server error during lookup",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"lat": "47.6062",
					"lon": "-122.3321",
				},
			},
			setupMock: func() station.StationFinder {
				return &mockStationFinder{
					findNearestStationsFn: func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
						return nil, assert.AnError // Simulate internal error
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Error finding stations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock and handler
			stationsHandler = handler.NewStationsHandler(tt.setupMock())

			// Call handler
			response, err := handleRequest(context.Background(), tt.request)

			// We don't expect errors from the handler itself
			require.NoError(t, err)

			// Verify response status and error message
			assert.Equal(t, tt.expectedStatus, response.StatusCode)

			var responseBody map[string]interface{}
			err = json.Unmarshal([]byte(response.Body), &responseBody)
			require.NoError(t, err)

			assert.Equal(t, "error", responseBody["responseType"])
			assert.Equal(t, tt.expectedError, responseBody["error"])
		})
	}
}
