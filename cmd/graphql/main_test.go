package main

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/graph"
	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"sync"
	"testing"
	"time"
)

type MockService struct {
	mock.Mock
}

func (m *MockService) GetCurrentTide(_ context.Context, _, _ float64, _, _ *string) (*models.ExtendedTideResponse, error) {
	panic("implement me")
}

func (m *MockService) GetCurrentTideForStation(_ context.Context, _ string, _, _ *string) (*models.ExtendedTideResponse, error) {
	panic("implement me")
}

func (m *MockService) GetPredictions(ctx context.Context, stationID string, start time.Time, end time.Time) ([]models.TidePrediction, error) {
	args := m.Called(ctx, stationID, start, end)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TidePrediction), args.Error(1)
}

func (m *MockService) GetExtremes(ctx context.Context, stationID string, start time.Time, end time.Time) ([]models.TideExtreme, error) {
	args := m.Called(ctx, stationID, start, end)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TideExtreme), args.Error(1)
}

type MockFinder struct {
	mock.Mock
}

func (m *MockFinder) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	args := m.Called(ctx, stationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Station), args.Error(1)
}

func (m *MockFinder) FindNearestStations(ctx context.Context, lat float64, lon float64, limit int) ([]models.Station, error) {
	args := m.Called(ctx, lat, lon, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Station), args.Error(1)
}

func (m *MockFinder) GetStations(ctx context.Context) ([]models.Station, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Station), args.Error(1)
}

type mockFinderFactory struct {
	mock.Mock
}

func (m *mockFinderFactory) NewFinder(httpClient *client.Client, memCache *cache.StationCache) (*station.NOAAStationFinder, error) {
	args := m.Called(httpClient, memCache)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*station.NOAAStationFinder), args.Error(1)
}

type mockTideFactory struct {
	mock.Mock
}

func (m *mockTideFactory) NewService(ctx context.Context, httpClient *client.Client, finder models.StationFinder) (*tide.Service, error) {
	args := m.Called(ctx, httpClient, finder)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*tide.Service), args.Error(1)
}

func TestInitHandler(t *testing.T) {
	// Save original handler
	originalHandler := handler
	defer func() {
		handler = originalHandler
	}()

	// Reset handler
	handler = nil

	// Create context
	ctx := context.Background()

	// Initialize handler
	h, err := initHandler(ctx)

	// Assert results
	assert.NoError(t, err)
	assert.NotNil(t, h)
}

func TestInit(t *testing.T) {
	// Save original handler
	originalHandler := handler
	defer func() {
		handler = originalHandler
	}()

	// Reset handler
	handler = nil

	// Test the initialization logic by calling initHandler directly
	h, err := initHandler(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, h)

	// Set the handler like init() would
	handler = h

	// Verify handler was initialized
	assert.NotNil(t, handler)
}

func TestHandleRequest(t *testing.T) {
	// Save original handler
	originalHandler := handler
	defer func() {
		handler = originalHandler
	}()

	// Create a mock resolver
	mockStationFinder := &MockFinder{}
	mockTideService := &MockService{}

	resolver := &graph.Resolver{
		StationFinder: mockStationFinder,
		TideService:   mockTideService,
	}

	// Create test handler
	handler = graph.NewHandler(resolver, nil)

	tests := []struct {
		name         string
		request      events.APIGatewayProxyRequest
		expectedCode int
		expectError  bool
	}{
		{
			name: "valid graphql query",
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "POST",
				Body:       `{"query": "{ stations { id name } }"}`,
			},
			expectedCode: 200,
			expectError:  false,
		},
		{
			name: "invalid method",
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
			},
			expectedCode: 405,
			expectError:  false,
		},
		{
			name: "invalid json",
			request: events.APIGatewayProxyRequest{
				HTTPMethod: "POST",
				Body:       `invalid json`,
			},
			expectedCode: 400,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := handleRequest(context.Background(), tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCode, response.StatusCode)
			}
		})
	}
}

func TestInitHandler_ErrorInitializingStationFinder(t *testing.T) {
	originalFinderFactory := finderFactory
	defer func() { finderFactory = originalFinderFactory }()

	mockFinderFactory := &mockFinderFactory{}
	mockFinderFactory.On("NewFinder", mock.Anything, mock.Anything).
		Return(nil, errors.New("mock error initializing station finder"))
	finderFactory = mockFinderFactory

	h, err := initHandler(context.Background())
	assert.Error(t, err)
	assert.Nil(t, h)
	assert.Contains(t, err.Error(), "initializing station finder")
}

func TestInitHandler_ErrorInitializingTideService(t *testing.T) {
	originalTideFactory := tideFactory
	defer func() { tideFactory = originalTideFactory }()

	mockTideFactory := &mockTideFactory{}
	mockTideFactory.On("NewService", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("mock error initializing tide service"))
	tideFactory = mockTideFactory

	h, err := initHandler(context.Background())
	assert.Error(t, err)
	assert.Nil(t, h)
	assert.Contains(t, err.Error(), "initializing tide service")
}

func TestInitialization(t *testing.T) {
	originalHandler := handler
	defer func() { handler = originalHandler }()

	handler = nil
	setupOnce = sync.Once{}
	initHandler = func(ctx context.Context) (*graph.Handler, error) {
		return nil, errors.New("mock error initializing handler")
	}

	err := InitializeService()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock error initializing handler")
	assert.Nil(t, handler)
}
