package main

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
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

// Define the error that would normally come from the station package
var ErrStationNotFound = errors.New("station not found")

func newMockTideService() *tide.Service {
	return &tide.Service{
		HttpClient:      &client.Client{},     // Mock or real HTTP client
		StationFinder:   &mockStationFinder{}, // Mock station finder
		PredictionCache: &mockCacheService{},  // Mock cache service
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
