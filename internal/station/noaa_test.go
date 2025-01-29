package station

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
)

type mockS3Cache struct {
	getStationsFunc  func(context.Context) ([]models.Station, error)
	saveStationsFunc func(context.Context, []models.Station) error
}

func (m *mockS3Cache) GetStations(ctx context.Context) ([]models.Station, error) {
	if m.getStationsFunc != nil {
		return m.getStationsFunc(ctx)
	}
	return nil, nil
}

func (m *mockS3Cache) SaveStations(ctx context.Context, stations []models.Station) error {
	if m.saveStationsFunc != nil {
		return m.saveStationsFunc(ctx, stations)
	}
	return nil
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

// Helper function to create NOAA API response
func createNOAAResponse(stations []models.Station) string {
	// Convert stations to NOAA format
	type noaaStation struct {
		ID           string  `json:"stationId"`
		Name         string  `json:"name"`
		State        string  `json:"state,omitempty"`
		Region       string  `json:"region,omitempty"`
		Lat          float64 `json:"lat"`
		Lon          float64 `json:"lon"`
		TimeZoneCorr string  `json:"timeZoneCorr"`
		Level        string  `json:"level,omitempty"`
		StationType  string  `json:"stationType,omitempty"`
	}

	noaaStations := make([]noaaStation, len(stations))
	for i, s := range stations {
		noaaStations[i] = noaaStation{
			ID:           s.ID,
			Name:         s.Name,
			State:        *s.State,
			Region:       *s.Region,
			Lat:          s.Latitude,
			Lon:          s.Longitude,
			TimeZoneCorr: "-8",
			Level:        *s.Level,
			StationType:  *s.StationType,
		}
	}

	response := struct {
		StationList []noaaStation `json:"stationList"`
	}{
		StationList: noaaStations,
	}

	responseBytes, _ := json.Marshal(response)
	return string(responseBytes)
}

func TestNewNOAAStationFinder(t *testing.T) {
	tests := []struct {
		name      string
		client    *client.Client
		memCache  *cache.StationCache
		wantError bool
	}{
		{
			name:      "valid configuration",
			client:    &client.Client{},
			memCache:  cache.NewStationCache(nil),
			wantError: false,
		},
		{
			name:      "nil cache creates default",
			client:    &client.Client{},
			memCache:  nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finder, err := NewNOAAStationFinder(tt.client, tt.memCache)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, finder)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, finder)
				assert.NotNil(t, finder.memCache)
				assert.NotNil(t, finder.httpClient)
			}
		})
	}
}

func TestFindStation(t *testing.T) {
	// Create test stations
	station1 := createTestStation("9447130")
	testStations := []models.Station{station1}

	// Create mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := createNOAAResponse(testStations)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	tests := []struct {
		name      string
		stationID string
		want      *models.Station
		wantErr   bool
	}{
		{
			name:      "existing station",
			stationID: "9447130",
			want:      &station1,
			wantErr:   false,
		},
		{
			name:      "non-existent station",
			stationID: "invalid",
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := client.New(client.Options{
				BaseURL: srv.URL,
				Timeout: 5 * time.Second,
			})

			finder, err := NewNOAAStationFinder(httpClient, nil)
			require.NoError(t, err)

			got, err := finder.FindStation(context.Background(), tt.stationID)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.Name, got.Name)
			assert.Equal(t, tt.want.State, got.State)
			assert.Equal(t, tt.want.Latitude, got.Latitude)
			assert.Equal(t, tt.want.Longitude, got.Longitude)
			assert.Equal(t, tt.want.TimeZoneOffset, got.TimeZoneOffset)
		})
	}
}

func TestFindNearestStations(t *testing.T) {
	// Create test stations at different distances
	stations := []models.Station{
		createTestStation("NEAR"),   // Base station
		createTestStation("MEDIUM"), // Slightly further
		createTestStation("FAR"),    // Farthest
	}

	// Modify coordinates to create distance differences
	stations[1].Latitude += 0.1 // Medium distance
	stations[2].Latitude += 0.2 // Further away

	// Create mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := createNOAAResponse(stations)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	tests := []struct {
		name        string
		lat         float64
		lon         float64
		limit       int
		wantCount   int
		wantOrder   []string // Expected station IDs in order
		wantErr     bool
		errContains string
	}{
		{
			name:      "find nearest 2 stations",
			lat:       47.6062,
			lon:       -122.3321,
			limit:     2,
			wantCount: 2,
			wantOrder: []string{"NEAR", "MEDIUM"},
			wantErr:   false,
		},
		{
			name:      "find all stations",
			lat:       47.6062,
			lon:       -122.3321,
			limit:     5,
			wantCount: 3, // Should return all 3 stations
			wantOrder: []string{"NEAR", "MEDIUM", "FAR"},
			wantErr:   false,
		},
		{
			name:        "invalid latitude",
			lat:         91.0,
			lon:         -122.3321,
			limit:       2,
			wantErr:     true,
			errContains: "invalid latitude",
		},
		{
			name:        "invalid longitude",
			lat:         47.6062,
			lon:         -181.0,
			limit:       2,
			wantErr:     true,
			errContains: "invalid longitude",
		},
		{
			name:      "zero limit uses default",
			lat:       47.6062,
			lon:       -122.3321,
			limit:     0,
			wantCount: 3, // Should return all stations
			wantOrder: []string{"NEAR", "MEDIUM", "FAR"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := client.New(client.Options{
				BaseURL: srv.URL,
				Timeout: 5 * time.Second,
			})

			finder, err := NewNOAAStationFinder(httpClient, nil)
			require.NoError(t, err)

			got, err := finder.FindNearestStations(context.Background(), tt.lat, tt.lon, tt.limit)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.Len(t, got, tt.wantCount)

			// Verify order of stations
			for i, wantID := range tt.wantOrder {
				assert.Equal(t, wantID, got[i].ID, fmt.Sprintf("Station at position %d", i))
			}

			// Verify distances are in ascending order
			for i := 1; i < len(got); i++ {
				assert.GreaterOrEqual(t, got[i].Distance, got[i-1].Distance,
					"Distances should be in ascending order")
			}
		})
	}
}

func TestParseTimeZoneOffset(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "positive offset",
			input:    "8",
			expected: 28800, // 8 hours in seconds
		},
		{
			name:     "negative offset",
			input:    "-8",
			expected: -28800,
		},
		{
			name:     "zero offset",
			input:    "0",
			expected: 0,
		},
		{
			name:     "invalid string",
			input:    "invalid",
			expected: 0,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimeZoneOffset(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateDistance(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		expected float64
		delta    float64
	}{
		{
			name:     "same point",
			lat1:     47.6062,
			lon1:     -122.3321,
			lat2:     47.6062,
			lon2:     -122.3321,
			expected: 0,
			delta:    0.0001,
		},
		{
			name:     "known distance - Seattle to Portland",
			lat1:     47.6062,
			lon1:     -122.3321, // Seattle
			lat2:     45.5155,
			lon2:     -122.6789, // Portland
			expected: 234.0,     // ~234 km
			delta:    1.0,       // Allow 1km variance
		},
		{
			name:     "antipodal points",
			lat1:     90,
			lon1:     0,
			lat2:     -90,
			lon2:     0,
			expected: 20015.1, // Maximum Earth distance
			delta:    0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestCacheInteraction(t *testing.T) {
	// Create test station
	testStation := createTestStation("TEST001")

	// Create mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := createNOAAResponse([]models.Station{testStation})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	// Create test cache with short TTL
	testCache := cache.NewStationCache(&config.CacheConfig{
		StationListTTLDays: 1,
	})

	// Create finder with test configuration
	httpClient := client.New(client.Options{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	finder, err := NewNOAAStationFinder(httpClient, testCache)
	require.NoError(t, err)

	// Initial cache should be empty
	cachedStations := testCache.GetStations()
	assert.Nil(t, cachedStations)

	// First call should populate cache
	stations, err := finder.getStationList(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stations)
	assert.Len(t, stations, 1)
	assert.Equal(t, testStation.ID, stations[0].ID)

	// Verify cache was populated
	cachedStations = testCache.GetStations()
	require.NotNil(t, cachedStations)
	assert.Len(t, cachedStations, 1)
	assert.Equal(t, testStation.ID, cachedStations[0].ID)

	// Second call should use cache
	stations2, err := finder.getStationList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, stations, stations2)
}

// Benchmarks for key operations
func BenchmarkCalculateDistance(b *testing.B) {
	lat1, lon1 := 47.6062, -122.3321 // Seattle
	lat2, lon2 := 45.5155, -122.6789 // Portland

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculateDistance(lat1, lon1, lat2, lon2)
	}
}

func BenchmarkFindNearestStations(b *testing.B) {
	// Create test stations
	stations := []models.Station{
		createTestStation("NEAR"),
		createTestStation("MEDIUM"),
		createTestStation("FAR"),
	}

	// Setup test server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := createNOAAResponse(stations)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	// Create finder
	httpClient := client.New(client.Options{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})
	finder, _ := NewNOAAStationFinder(httpClient, nil)

	// Benchmark finding nearest stations
	lat, lon := 47.6062, -122.3321
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := finder.FindNearestStations(context.Background(), lat, lon, 2)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseTimeZoneOffset(b *testing.B) {
	offset := "-8"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseTimeZoneOffset(offset)
	}
}

func TestS3CacheScenarios(t *testing.T) {
	// Create base test station
	testStation := createTestStation("TEST001")

	// Create mock server as fallback
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := createNOAAResponse([]models.Station{testStation})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	tests := []struct {
		name         string
		setupS3Cache func() *mockS3Cache
		wantStations []models.Station
		wantErr      bool
	}{
		{
			name: "s3 cache hit",
			setupS3Cache: func() *mockS3Cache {
				return &mockS3Cache{
					getStationsFunc: func(ctx context.Context) ([]models.Station, error) {
						return []models.Station{testStation}, nil
					},
				}
			},
			wantStations: []models.Station{testStation},
			wantErr:      false,
		},
		{
			name: "s3 cache error",
			setupS3Cache: func() *mockS3Cache {
				return &mockS3Cache{
					getStationsFunc: func(ctx context.Context) ([]models.Station, error) {
						return nil, fmt.Errorf("s3 error")
					},
				}
			},
			wantStations: []models.Station{testStation}, // Should fall back to API
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := client.New(client.Options{
				BaseURL: srv.URL,
				Timeout: 5 * time.Second,
			})

			finder, err := NewNOAAStationFinder(httpClient, nil)
			require.NoError(t, err)

			// Set the S3 cache
			finder.s3Cache = tt.setupS3Cache()

			// Test getStationList
			stations, err := finder.getStationList(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, stations)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, stations)
			assert.Equal(t, tt.wantStations, stations)

			// For cache hit case, verify memory cache was updated
			if tt.name == "s3 cache hit" {
				memCacheStations := finder.memCache.GetStations()
				assert.Equal(t, tt.wantStations, memCacheStations)
			}
		})
	}
}
