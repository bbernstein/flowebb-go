package cache

import (
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"sync"
	"time"

	"github.com/bbernstein/flowebb/backend-go/internal/models"
)

type StationCache struct {
	stations    []models.Station
	lastUpdated time.Time
	mu          sync.RWMutex
	ttl         time.Duration
}

func NewStationCache(config *config.CacheConfig) *StationCache {
	ttl := config.GetStationListTTL()
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
	return c.stations
}

func (c *StationCache) SetStations(stations []models.Station) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stations = stations
	c.lastUpdated = time.Now()
}

func (c *StationCache) isExpired() bool {
	return time.Since(c.lastUpdated) > c.ttl
}
