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
	"reflect"
	"sync"
	"testing"
	"time"
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

var (
	mu sync.Mutex // Protect lambdaStart in tests
)

func TestLambdaInit(t *testing.T) {
	// Set required Lambda environment variables
	originalServerPort := os.Getenv("_LAMBDA_SERVER_PORT")
	originalRuntimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")

	err := os.Setenv("_LAMBDA_SERVER_PORT", "8080")
	require.NoError(t, err)
	err = os.Setenv("AWS_LAMBDA_RUNTIME_API", "localhost")
	require.NoError(t, err)

	// Cleanup environment after test
	defer func() {
		err := os.Setenv("_LAMBDA_SERVER_PORT", originalServerPort)
		if err != nil {
			t.Errorf("Failed to restore _LAMBDA_SERVER_PORT: %v", err)
		}
		err = os.Setenv("AWS_LAMBDA_RUNTIME_API", originalRuntimeAPI)
		if err != nil {
			t.Errorf("Failed to restore AWS_LAMBDA_RUNTIME_API: %v", err)
		}
	}()

	// Save original lambda.Start function
	mu.Lock()
	originalStartFn := lambdaStart
	var startCalled bool
	lambdaStart = func(handler interface{}) {
		mu.Lock()
		startCalled = true
		mu.Unlock()

		// Verify the handler is a function with the correct signature
		handlerType := reflect.TypeOf(handler)
		if handlerType.Kind() != reflect.Func {
			t.Error("Handler is not a function")
		}

		// Verify the handler has the correct signature
		contextInterface := reflect.TypeOf((*context.Context)(nil)).Elem()
		proxyRequest := reflect.TypeOf(events.APIGatewayProxyRequest{})
		proxyResponse := reflect.TypeOf(events.APIGatewayProxyResponse{})
		errorInterface := reflect.TypeOf((*error)(nil)).Elem()

		if handlerType.NumIn() != 2 || handlerType.NumOut() != 2 ||
			!handlerType.In(0).Implements(contextInterface) ||
			handlerType.In(1) != proxyRequest ||
			handlerType.Out(0) != proxyResponse ||
			!handlerType.Out(1).Implements(errorInterface) {
			t.Error("Handler does not match expected signature")
		}
	}
	mu.Unlock()

	defer func() {
		mu.Lock()
		lambdaStart = originalStartFn
		mu.Unlock()
	}()

	// Call main() which should trigger our mock lambda.Start
	go main()

	// Give main() a moment to run
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	wasStartCalled := startCalled
	mu.Unlock()

	if !wasStartCalled {
		t.Error("Lambda start was not called")
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
			// Create handler with empty mock
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
			// Create handler with mock
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
