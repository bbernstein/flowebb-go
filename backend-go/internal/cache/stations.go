package cache

import (
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"sync"
	"time"
)

type StationCache struct {
	stations    []models.Station
	lastUpdated time.Time
	mu          sync.RWMutex
	ttl         time.Duration
}

func NewStationCache(cacheConfig *config.CacheConfig) *StationCache {
	// If no config provided, use default config
	if cacheConfig == nil {
		cacheConfig = config.GetCacheConfig()
	}

	ttl := cacheConfig.GetStationListTTL()
	return &StationCache{
		stations:    make([]models.Station, 0),
		lastUpdated: time.Time{}, // Zero time to ensure first fetch
		ttl:         ttl,
	}
}

func (c *StationCache) GetStations() []models.Station {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.isExpired() {
		return nil
	}

	// Create a copy of the stations slice to prevent race conditions
	stations := make([]models.Station, len(c.stations))
	copy(stations, c.stations)
	return stations
}

func (c *StationCache) SetStations(stations []models.Station) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a copy of the input slice
	newStations := make([]models.Station, len(stations))
	copy(newStations, stations)

	c.stations = newStations
	c.lastUpdated = time.Now()
}

func (c *StationCache) isExpired() bool {
	return time.Since(c.lastUpdated) > c.ttl
}
