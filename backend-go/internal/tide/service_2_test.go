package tide

import (
	"context"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Mock StationFinder for testing
type mockStationFinder2 struct {
	findStationFn         func(ctx context.Context, stationID string) (*models.Station, error)
	findNearestStationsFn func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error)
}

func (m *mockStationFinder2) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	if m.findStationFn != nil {
		return m.findStationFn(ctx, stationID)
	}
	return nil, nil
}

func (m *mockStationFinder2) FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
	if m.findNearestStationsFn != nil {
		return m.findNearestStationsFn(ctx, lat, lon, limit)
	}
	return nil, nil
}

// Mock CacheService for testing
type mockStationService2 struct {
	getPredictionsFn       func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error)
	savePredictionsBatchFn func(ctx context.Context, records []models.TidePredictionRecord) error
}

func (m *mockStationService2) GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	if m.getPredictionsFn != nil {
		return m.getPredictionsFn(ctx, stationID, date)
	}
	return nil, nil
}

func (m *mockStationService2) SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error {
	if m.savePredictionsBatchFn != nil {
		return m.savePredictionsBatchFn(ctx, records)
	}
	return nil
}

func createTestStation(timeZoneOffset int) *models.Station {
	stationType := "R" // Reference station
	return &models.Station{
		ID:             "TEST001",
		Name:           "Test Station",
		Latitude:       47.6062,
		Longitude:      -122.3321,
		TimeZoneOffset: timeZoneOffset,
		StationType:    &stationType,
	}
}

func TestGetCurrentTide_InvalidParameters(t *testing.T) {
	tests := []struct {
		name       string
		lat        float64
		lon        float64
		wantErr    bool
		errMessage string
	}{
		{
			name:       "invalid latitude too high",
			lat:        91.0,
			lon:        0.0,
			wantErr:    true,
			errMessage: "invalid latitude",
		},
		{
			name:       "invalid latitude too low",
			lat:        -91.0,
			lon:        0.0,
			wantErr:    true,
			errMessage: "invalid latitude",
		},
		{
			name:       "invalid longitude too high",
			lat:        0.0,
			lon:        181.0,
			wantErr:    true,
			errMessage: "invalid longitude",
		},
		{
			name:       "invalid longitude too low",
			lat:        0.0,
			lon:        -181.0,
			wantErr:    true,
			errMessage: "invalid longitude",
		},
	}

	service := &Service{
		HttpClient:      &client.Client{},
		StationFinder:   &mockStationFinder2{},
		PredictionCache: &mockStationService2{},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := service.GetCurrentTide(context.Background(), tt.lat, tt.lon, nil, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.NotNil(t, response)
			}
		})
	}
}

func TestGetCurrentTideForStation_DateRangeValidation(t *testing.T) {
	tests := []struct {
		name       string
		startTime  string
		endTime    string
		wantErr    bool
		errMessage string
	}{
		//{
		//	name:      "valid one day range",
		//	startTime: time.Now().Format("2006-01-02T15:04:05"),
		//	endTime:   time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04:05"),
		//	wantErr:   false,
		//},
		{
			name:       "range too large",
			startTime:  time.Now().Format("2006-01-02T15:04:05"),
			endTime:    time.Now().Add(6 * 24 * time.Hour).Format("2006-01-02T15:04:05"),
			wantErr:    true,
			errMessage: "date range cannot exceed 5 days",
		},
		{
			name:       "invalid start time format",
			startTime:  "invalid",
			endTime:    time.Now().Format("2006-01-02T15:04:05"),
			wantErr:    true,
			errMessage: "parsing start time",
		},
		{
			name:       "invalid end time format",
			startTime:  time.Now().Format("2006-01-02T15:04:05"),
			endTime:    "invalid",
			wantErr:    true,
			errMessage: "parsing end time",
		},
	}

	stationFinder := &mockStationFinder2{
		findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
			return createTestStation(-28800), nil
		},
	}

	service := &Service{
		HttpClient:      &client.Client{},
		StationFinder:   stationFinder,
		PredictionCache: &mockStationService2{},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := service.GetCurrentTideForStation(context.Background(), "TEST001", &tt.startTime, &tt.endTime)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.NotNil(t, response)
			}
		})
	}
}

func TestTideTypeDetermination(t *testing.T) {
	// Create test predictions with known rising/falling patterns
	now := time.Now()
	predictions := []models.TidePrediction{
		{Timestamp: now.Add(-1 * time.Hour).UnixMilli(), Height: 1.0},
		{Timestamp: now.UnixMilli(), Height: 2.0}, // Current (rising)
		{Timestamp: now.Add(1 * time.Hour).UnixMilli(), Height: 3.0},
	}

	// Mock NOAA API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprint(w, `{"predictions":[{"t":"2024-01-01 00:00","v":"1.0"},{"t":"2024-01-01 01:00","v":"2.0"}]}`)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	stationFinder := &mockStationFinder2{
		findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
			return createTestStation(-28800), nil
		},
	}

	cache := &mockStationService2{
		getPredictionsFn: func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
			return &models.TidePredictionRecord{
				StationID:   "TEST001",
				Date:        date.Format("2006-01-02"),
				StationType: "R",
				Predictions: predictions,
				LastUpdated: time.Now().Unix(),
				TTL:         time.Now().Add(24 * time.Hour).Unix(),
			}, nil
		},
	}

	httpClient := client.New(client.Options{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	service := &Service{
		HttpClient:      httpClient,
		StationFinder:   stationFinder,
		PredictionCache: cache,
	}

	response, err := service.GetCurrentTideForStation(context.Background(), "TEST001", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify tide type is determined correctly
	require.NotNil(t, response.TideType)
	assert.Equal(t, models.TideTypeRising, *response.TideType)

	// Test falling tide
	predictions[1].Height = 0.5 // Make current height lower than previous
	response, err = service.GetCurrentTideForStation(context.Background(), "TEST001", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.TideType)
	assert.Equal(t, models.TideFalling, *response.TideType)
}

func TestInterpolationAccuracy(t *testing.T) {
	tests := []struct {
		name          string
		predictions   []models.TidePrediction
		timestamp     int64
		expectedLevel float64
		tolerance     float64
	}{
		{
			name: "exact match",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			timestamp:     1000,
			expectedLevel: 2.0,
			tolerance:     0.001,
		},
		{
			name: "midpoint interpolation",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 4.0},
			},
			timestamp:     1500,
			expectedLevel: 3.0,
			tolerance:     0.001,
		},
		{
			name: "before first prediction",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			timestamp:     500,
			expectedLevel: 2.0,
			tolerance:     0.001,
		},
		{
			name: "after last prediction",
			predictions: []models.TidePrediction{
				{Timestamp: 1000, Height: 2.0},
				{Timestamp: 2000, Height: 3.0},
			},
			timestamp:     2500,
			expectedLevel: 3.0,
			tolerance:     0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolatePredictions(tt.predictions, tt.timestamp)
			assert.InDelta(t, tt.expectedLevel, result, tt.tolerance)
		})
	}
}

func TestTimeZoneHandling(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/prod/datagetter" {
			// Mock response for predictions
			if r.URL.Query().Get("interval") == "6" {
				_, _ = fmt.Fprint(w, `{"predictions":[{"t":"2024-01-01 00:00","v":"1.0"},{"t":"2024-01-01 01:00","v":"2.0"}]}`)
			}
			// Mock response for extremes
			if r.URL.Query().Get("interval") == "hilo" {
				_, _ = fmt.Fprint(w, `{"predictions":[{"t":"2024-01-01 00:00","v":"1.0","type":"H"},{"t":"2024-01-01 06:00","v":"0.5","type":"L"}]}`)
			}
		}
	}))
	defer mockServer.Close()

	// Create a mock CacheService
	cache := &mockStationService2{
		getPredictionsFn: func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
			return nil, nil // Simulate cache miss
		},
		savePredictionsBatchFn: func(ctx context.Context, records []models.TidePredictionRecord) error {
			return nil
		},
	}

	// Create a Client configured to use the mock server
	httpClient := client.New(client.Options{
		BaseURL: mockServer.URL,
		Timeout: 5 * time.Second,
	})

	tests := []struct {
		name           string
		timeZoneOffset int
		wantErr        bool
	}{
		{
			name:           "Pacific Time",
			timeZoneOffset: -28800, // -8 hours
			wantErr:        false,
		},
		{
			name:           "UTC",
			timeZoneOffset: 0,
			wantErr:        false,
		},
		{
			name:           "India Time",
			timeZoneOffset: 19800, // +5:30 hours
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			station := createTestStation(tt.timeZoneOffset)
			station.TimeZoneOffset = tt.timeZoneOffset

			// Create a mock StationFinder
			stationFinder := &mockStationFinder2{
				findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
					return station, nil
				},
			}

			// Create the Service
			service := &Service{
				HttpClient:      httpClient,
				StationFinder:   stationFinder,
				PredictionCache: cache,
			}

			response, err := service.GetCurrentTideForStation(context.Background(), station.ID, nil, nil)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, response)
			require.NotNil(t, response.TimeZoneOffsetSeconds)
			assert.Equal(t, tt.timeZoneOffset, *response.TimeZoneOffsetSeconds)
		})
	}
}

func TestCacheIntegration(t *testing.T) {
	// Set up a fixed time for the test in UTC
	now := time.Now().UTC()
	// Convert to Pacific time for the test (UTC-8)
	location := time.FixedZone("PST", -8*60*60)
	nowPacific := now.In(location)
	today := nowPacific.Format("20060102") // Format date as YYYYMMDD for NOAA API

	// Create a WaitGroup to synchronize cache operations
	var wg sync.WaitGroup
	var savedBatchMu sync.Mutex
	var savedBatch []models.TidePredictionRecord

	// Mock server for NOAA API that uses the actual requested dates
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/prod/datagetter" {
			beginDate := r.URL.Query().Get("begin_date")
			require.Equal(t, today, beginDate, "Begin date should match today")

			// Mock response for predictions
			if r.URL.Query().Get("interval") == "6" {
				response := fmt.Sprintf(`{"predictions":[
                    {"t":"%s 00:00","v":"1.0"},
                    {"t":"%s 06:00","v":"2.0"},
                    {"t":"%s 12:00","v":"1.5"},
                    {"t":"%s 18:00","v":"2.5"}
                ]}`,
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"))
				_, _ = fmt.Fprint(w, response)
			}

			// Mock response for extremes
			if r.URL.Query().Get("interval") == "hilo" {
				response := fmt.Sprintf(`{"predictions":[
                    {"t":"%s 00:00","v":"1.0","type":"H"},
                    {"t":"%s 06:00","v":"0.5","type":"L"},
                    {"t":"%s 12:00","v":"2.0","type":"H"},
                    {"t":"%s 18:00","v":"0.8","type":"L"}
                ]}`,
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"),
					nowPacific.Format("2006-01-02"))
				_, _ = fmt.Fprint(w, response)
			}
		}
	}))
	defer server.Close()

	// Track cache operations
	var getCalled bool
	var getCalledMu sync.Mutex

	cache := &mockStationService2{
		getPredictionsFn: func(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
			getCalledMu.Lock()
			getCalled = true
			getCalledMu.Unlock()

			// Verify the date being requested matches our expected date
			expectedDate := nowPacific.Format("2006-01-02")
			actualDate := date.Format("2006-01-02")
			nextDate := date.Add(24 * time.Hour).Format("2006-01-02")
			require.GreaterOrEqual(t, actualDate, expectedDate, "Cache request date should be after start date")
			require.LessOrEqual(t, expectedDate, nextDate, "Cache request date should be before next date")
			return nil, nil // Simulate cache miss
		},
		savePredictionsBatchFn: func(ctx context.Context, records []models.TidePredictionRecord) error {
			wg.Add(1)
			defer wg.Done()

			savedBatchMu.Lock()
			savedBatch = records
			savedBatchMu.Unlock()
			return nil
		},
	}

	// Create test station with Pacific timezone
	station := createTestStation(-8 * 3600) // -8 hours for Pacific Time

	stationFinder := &mockStationFinder2{
		findStationFn: func(ctx context.Context, stationID string) (*models.Station, error) {
			return station, nil
		},
	}

	httpClient := client.New(client.Options{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	service := &Service{
		HttpClient:      httpClient,
		StationFinder:   stationFinder,
		PredictionCache: cache,
	}

	// Test getting predictions
	response, err := service.GetCurrentTideForStation(context.Background(), "TEST001", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Wait for cache operations to complete
	wg.Wait()

	// Verify cache was checked first
	getCalledMu.Lock()
	assert.True(t, getCalled, "Cache should have been checked")
	getCalledMu.Unlock()

	// Verify predictions were fetched from API and cached
	savedBatchMu.Lock()
	assert.NotEmpty(t, savedBatch, "Expected predictions to be cached after API fetch")
	if len(savedBatch) > 0 {
		assert.Equal(t, "TEST001", savedBatch[0].StationID)
		// Verify the cached date matches our test date
		assert.Equal(t, nowPacific.Format("2006-01-02"), savedBatch[0].Date)
		assert.NotEmpty(t, savedBatch[0].Predictions, "Cached record should contain predictions from API")
		assert.NotEmpty(t, savedBatch[0].Extremes, "Cached record should contain extremes from API")
	}
	savedBatchMu.Unlock()

	// Verify the response contains the expected data
	require.NotNil(t, response.PredictedLevel)
	require.NotNil(t, response.TideType)
	assert.NotEmpty(t, response.Predictions)
	assert.NotEmpty(t, response.Extremes)

	// Verify timestamps in predictions and extremes are within expected range
	expectedDate := nowPacific.Format("2006-01-02")
	for _, p := range response.Predictions {
		predTime := time.Unix(p.Timestamp/1000, 0).In(location)
		assert.Equal(t, expectedDate, predTime.Format("2006-01-02"),
			"Prediction timestamp should be on test date")
	}

	for _, e := range response.Extremes {
		extremeTime := time.Unix(e.Timestamp/1000, 0).In(location)
		assert.Equal(t, expectedDate, extremeTime.Format("2006-01-02"),
			"Extreme timestamp should be on test date")
	}
}

func TestInterpolateExtremes(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		extremes  []models.TideExtreme
		timestamp int64
		expected  float64
		tolerance float64
	}{
		{
			name: "basic high-low interpolation",
			extremes: []models.TideExtreme{
				{
					Type:      models.TideTypeHigh,
					Timestamp: now.UnixMilli(),
					Height:    10.0,
				},
				{
					Type:      models.TideTypeLow,
					Timestamp: now.Add(6 * time.Hour).UnixMilli(),
					Height:    2.0,
				},
			},
			timestamp: now.Add(3 * time.Hour).UnixMilli(),
			expected:  6.0, // Should be roughly halfway between high and low
			tolerance: 0.5,
		},
		{
			name: "before first extreme",
			extremes: []models.TideExtreme{
				{
					Type:      models.TideTypeHigh,
					Timestamp: now.UnixMilli(),
					Height:    10.0,
				},
			},
			timestamp: now.Add(-1 * time.Hour).UnixMilli(),
			expected:  10.0, // Should use first extreme
			tolerance: 0.001,
		},
		{
			name: "after last extreme",
			extremes: []models.TideExtreme{
				{
					Type:      models.TideTypeLow,
					Timestamp: now.UnixMilli(),
					Height:    2.0,
				},
			},
			timestamp: now.Add(1 * time.Hour).UnixMilli(),
			expected:  2.0, // Should use last extreme
			tolerance: 0.001,
		},
		{
			name: "exact match with extreme",
			extremes: []models.TideExtreme{
				{
					Type:      models.TideTypeHigh,
					Timestamp: now.UnixMilli(),
					Height:    10.0,
				},
			},
			timestamp: now.UnixMilli(),
			expected:  10.0,
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolateExtremes(tt.extremes, tt.timestamp)
			assert.InDelta(t, tt.expected, result, tt.tolerance)
		})
	}
}

func TestNewService(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func() (*client.Client, StationFinder)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful creation",
			setupMocks: func() (*client.Client, StationFinder) {
				return &client.Client{}, &mockStationFinder2{}
			},
			wantErr: false,
		},
		{
			name: "nil http client",
			setupMocks: func() (*client.Client, StationFinder) {
				return nil, &mockStationFinder2{}
			},
			wantErr:     true,
			errContains: "http client is required",
		},
		{
			name: "nil station finder",
			setupMocks: func() (*client.Client, StationFinder) {
				return &client.Client{}, nil
			},
			wantErr:     true,
			errContains: "station finder is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient, stationFinder := tt.setupMocks()
			service, err := NewService(context.Background(), httpClient, stationFinder)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				require.NotNil(t, service)
				require.NotNil(t, service.HttpClient)
				require.NotNil(t, service.StationFinder)
				require.NotNil(t, service.PredictionCache)
			}
		})
	}
}
