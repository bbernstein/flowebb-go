package cache

import (
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestStationCacheGetSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stations []models.Station
		wantLen  int
	}{
		{
			name:     "empty cache",
			stations: []models.Station{},
			wantLen:  0,
		},
		{
			name: "single station",
			stations: []models.Station{
				{
					ID:             "TEST001",
					Name:           "Test Station",
					Latitude:       47.6062,
					Longitude:      -122.3321,
					Source:         models.SourceNOAA,
					TimeZoneOffset: -8 * 3600,
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple stations",
			stations: []models.Station{
				{
					ID:             "TEST001",
					Name:           "Test Station 1",
					Latitude:       47.6062,
					Longitude:      -122.3321,
					Source:         models.SourceNOAA,
					TimeZoneOffset: -8 * 3600,
				},
				{
					ID:             "TEST002",
					Name:           "Test Station 2",
					Latitude:       47.6062,
					Longitude:      -122.3321,
					Source:         models.SourceNOAA,
					TimeZoneOffset: -8 * 3600,
				},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.CacheConfig{
				StationListTTLDays: 1,
			}
			cache := NewStationCache(cfg)

			cache.SetStations(tt.stations)
			got := cache.GetStations()

			assert.Equal(t, tt.wantLen, len(got))
			if tt.wantLen > 0 {
				assert.Equal(t, tt.stations, got)
			}
		})
	}
}

func TestStationCacheExpiration(t *testing.T) {
	t.Parallel()

	cfg := &config.CacheConfig{
		StationListTTLDays: 1,
	}
	cache := NewStationCache(cfg)

	testStations := []models.Station{
		{
			ID:             "TEST001",
			Name:           "Test Station",
			Latitude:       47.6062,
			Longitude:      -122.3321,
			Source:         models.SourceNOAA,
			TimeZoneOffset: -8 * 3600,
		},
	}

	// Set stations and verify initial state
	cache.SetStations(testStations)
	got := cache.GetStations()
	require.NotNil(t, got)
	assert.Equal(t, testStations, got)

	// Manipulate last updated time to simulate expiration
	cache.lastUpdated = time.Now().Add(-25 * time.Hour)

	// Verify expired cache returns nil
	got = cache.GetStations()
	assert.Nil(t, got)
}

func TestConcurrentStationAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}
	t.Parallel()

	cfg := &config.CacheConfig{
		StationListTTLDays: 1,
	}
	cache := NewStationCache(cfg)

	const goroutines = 10
	const iterations = 100

	testStations := []models.Station{
		{
			ID:             "TEST001",
			Name:           "Test Station",
			Latitude:       47.6062,
			Longitude:      -122.3321,
			Source:         models.SourceNOAA,
			TimeZoneOffset: -8 * 3600,
		},
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%2 == 0 {
					cache.SetStations(testStations)
				} else {
					got := cache.GetStations()
					if got != nil {
						assert.Equal(t, testStations, got)
					}
				}
			}
		}()
	}

	wg.Wait()
}

func BenchmarkStationCache(b *testing.B) {
	cfg := &config.CacheConfig{
		StationListTTLDays: 1,
	}
	cache := NewStationCache(cfg)

	testStations := []models.Station{
		{
			ID:             "TEST001",
			Name:           "Test Station",
			Latitude:       47.6062,
			Longitude:      -122.3321,
			Source:         models.SourceNOAA,
			TimeZoneOffset: -8 * 3600,
		},
	}

	b.Run("SetStations", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.SetStations(testStations)
		}
	})

	b.Run("GetStations", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cache.GetStations()
		}
	})
}
