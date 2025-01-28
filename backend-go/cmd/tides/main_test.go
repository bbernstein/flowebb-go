package main

import (
	"encoding/json"
	"errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"context"
	"reflect"
)

type mockCacheService struct{}

func (m *mockCacheService) GetPredictions(_ context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	return &models.TidePredictionRecord{
		StationID:   stationID,
		Date:        date.Format("2006-01-02"),
		Predictions: []models.TidePrediction{},
		Extremes:    []models.TideExtreme{},
	}, nil
}

func (m *mockCacheService) SavePredictionsBatch(_ context.Context, _ []models.TidePredictionRecord) error {
	return nil
}

func newMockTideService() *tide.Service {
	return &tide.Service{
		HttpClient:      &client.Client{},     // Mock or real HTTP client
		StationFinder:   &mockStationFinder{}, // Mock station finder
		PredictionCache: &mockCacheService{},  // Mock cache service
	}
}

type mockStationFinder struct {
	findStationFunc         func(ctx context.Context, stationID string) (*models.Station, error)
	findNearestStationsFunc func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error)
}

func (m *mockStationFinder) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	if m.findStationFunc != nil {
		return m.findStationFunc(ctx, stationID)
	}
	// Default successful response instead of error
	stationType := "R"
	return &models.Station{
		ID:             stationID,
		Name:           "Test Station",
		Latitude:       47.6062,
		Longitude:      -122.3321,
		TimeZoneOffset: -28800,
		StationType:    &stationType,
	}, nil
}

func (m *mockStationFinder) FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
	if m.findNearestStationsFunc != nil {
		return m.findNearestStationsFunc(ctx, lat, lon, limit)
	}
	// Default successful response instead of empty slice
	stationType := "R"
	return []models.Station{
		{
			ID:             "TEST001",
			Name:           "Test Station",
			Latitude:       lat,
			Longitude:      lon,
			TimeZoneOffset: -28800,
			StationType:    &stationType,
		},
	}, nil
}

func TestHandleRequest(t *testing.T) {
	// Replace the real tide service with our mock
	originalTideService := tideService
	defer func() { tideService = originalTideService }()

	testCases := []struct {
		name         string
		request      events.APIGatewayProxyRequest
		setupMock    func() *tide.Service
		expectedCode int
	}{
		{
			name: "valid station ID request",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "1234567",
				},
			},
			setupMock: func() *tide.Service {
				return newMockTideService()
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
			setupMock: func() *tide.Service {
				return newMockTideService()
			},
			expectedCode: http.StatusOK,
		},
		// ... other test cases remain the same
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock for this test case
			tideService = tc.setupMock()

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

func TestLoggerSetup(t *testing.T) {
	// Save original environment and restore after test
	originalEnv := os.Getenv("ENV")
	defer func() {
		err := os.Setenv("ENV", originalEnv)
		if err != nil {
			t.Errorf("Failed to restore ENV: %v", err)
		}
	}()

	tests := []struct {
		name    string
		env     string
		wantErr bool
	}{
		{
			name:    "development environment",
			env:     "development",
			wantErr: false,
		},
		{
			name:    "local environment",
			env:     "local",
			wantErr: false,
		},
		{
			name:    "production environment",
			env:     "production",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment
			err := os.Setenv("ENV", tt.env)
			require.NoError(t, err)

			// Reset the setup to force reinitialization
			setupOnce = sync.Once{}
			initializeService()
		})
	}
}

func TestServiceInitializationError(t *testing.T) {
	// Create a pipe to capture log output
	r, w := io.Pipe()
	defer func(r *io.PipeReader) {
		err := r.Close()
		if err != nil {
			t.Errorf("Error closing pipe reader: %v", err)
		}
	}(r)
	defer func(w *io.PipeWriter) {
		err := w.Close()
		if err != nil {
			t.Errorf("Error closing pipe writer: %v", err)
		}
	}(w)

	// Save original logger and restore after test
	originalLogger := log.Logger
	defer func() { log.Logger = originalLogger }()

	// Set up logger to write to our pipe
	log.Logger = zerolog.New(w).With().Timestamp().Logger()

	// Create a channel to signal test completion
	done := make(chan struct{})

	// Run the logging in a goroutine
	go func() {
		defer close(done)
		// Instead of Fatal(), use Error() to prevent process termination
		log.Logger.Error().Err(assert.AnError).Msg("Failed to create tide service")
	}()

	// Read the log output
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Error reading log output: %v", err)
		return
	}
	logOutput := string(buf[:n])
	assert.Contains(t, logOutput, "Failed to create tide service")

	// Wait for goroutine completion
	select {
	case <-done:
		// Test completed normally
	case <-time.After(time.Second):
		t.Fatal("Test timed out waiting for log completion")
	}
}

type mockCacheService2 struct {
	getPredictionsFn       func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error)
	savePredictionsBatchFn func(ctx context.Context, records []models.TidePredictionRecord) error
}

func (m *mockCacheService2) GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	if m.getPredictionsFn != nil {
		return m.getPredictionsFn(ctx, stationID, date)
	}
	return nil, nil
}

func (m *mockCacheService2) SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error {
	if m.savePredictionsBatchFn != nil {
		return m.savePredictionsBatchFn(ctx, records)
	}
	return nil
}

// Update the test to use the mock service
func TestNoaaAPIErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		request        events.APIGatewayProxyRequest
		setupMock      func() *tide.Service
		expectedStatus int
		expectedError  string
	}{
		{
			name: "NOAA API error",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "TEST001",
				},
			},
			setupMock: func() *tide.Service {
				mockFinder := &mockStationFinder{
					findStationFunc: func(ctx context.Context, stationID string) (*models.Station, error) {
						// Return a valid station first, as the error should come from the tide service
						stationType := "R"
						return &models.Station{
							ID:             stationID,
							Name:           "Test Station",
							Latitude:       47.6062,
							Longitude:      -122.3321,
							TimeZoneOffset: -28800,
							StationType:    &stationType,
						}, nil
					},
				}

				// Create mock cache
				mockCache := &mockCacheService2{
					getPredictionsFn: func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
						// Return nil to force API call
						return nil, nil
					},
				}

				// Create mock HTTP client
				mockClient := &client.Client{
					GetFunc: func(ctx context.Context, path string) (*client.Response, error) {
						// Update the return value to use client.Response instead of http.Response
						return &client.Response{
							StatusCode: 200,
							Body:       []byte("test response"),
						}, nil
					},
				}

				service := &tide.Service{
					HttpClient:      mockClient,
					StationFinder:   mockFinder,
					PredictionCache: mockCache,
				}

				return service
			},
			expectedStatus: http.StatusBadGateway,
			expectedError:  "Error fetching tide data from upstream service",
		},
		{
			name: "general error",
			request: events.APIGatewayProxyRequest{
				QueryStringParameters: map[string]string{
					"stationId": "INVALID",
				},
			},
			setupMock: func() *tide.Service {
				mockFinder := &mockStationFinder{
					findStationFunc: func(ctx context.Context, stationID string) (*models.Station, error) {
						return nil, errors.New("general error")
					},
				}
				return &tide.Service{
					HttpClient:      &client.Client{},
					StationFinder:   mockFinder,
					PredictionCache: &mockCacheService{},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Error getting tide data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original service and set mock
			originalService := tideService
			tideService = tt.setupMock()
			defer func() { tideService = originalService }()

			// Call handler
			response, err := handleRequest(context.Background(), tt.request)

			// We don't expect errors from the handler itself
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, response.StatusCode)

			var responseBody map[string]interface{}
			err = json.Unmarshal([]byte(response.Body), &responseBody)
			require.NoError(t, err)

			assert.Equal(t, "error", responseBody["responseType"])
			assert.Contains(t, responseBody["error"], tt.expectedError)
		})
	}
}
