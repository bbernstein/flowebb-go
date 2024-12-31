package cache

import (
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"sync"
	"time"
)

type TidePredictionRecord struct {
	StationID   string
	Date        string // Format: YYYY-MM-DD
	StationType string // "R" for reference, "S" for subordinate
	Predictions []models.TidePrediction
	Extremes    []models.TideExtreme
	LastUpdated int64
	TTL         int64
}

type TidePredictionCache struct {
	records     map[string]map[string]TidePredictionRecord // map[stationID]map[date]record
	mu          sync.RWMutex
	validityTTL time.Duration
}

func NewTidePredictionCache() *TidePredictionCache {
	return &TidePredictionCache{
		records:     make(map[string]map[string]TidePredictionRecord),
		validityTTL: 7 * 24 * time.Hour, // 7 days
	}
}

func (c *TidePredictionCache) GetPredictions(stationID string, date time.Time) (*TidePredictionRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dateStr := date.Format("2006-01-02")

	if stationRecords, exists := c.records[stationID]; exists {
		if record, exists := stationRecords[dateStr]; exists {
			if c.isValid(record) {
				return &record, nil
			}
			// Record exists but is expired, remove it
			delete(stationRecords, dateStr)
			if len(stationRecords) == 0 {
				delete(c.records, stationID)
			}
		}
	}

	return nil, nil
}

func (c *TidePredictionCache) SavePredictions(record TidePredictionRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.records[record.StationID] == nil {
		c.records[record.StationID] = make(map[string]TidePredictionRecord)
	}

	record.LastUpdated = time.Now().Unix()
	record.TTL = time.Now().Add(c.validityTTL).Unix()

	c.records[record.StationID][record.Date] = record
	return nil
}

func (c *TidePredictionCache) SavePredictionsBatch(records []TidePredictionRecord) error {
	for _, record := range records {
		if err := c.SavePredictions(record); err != nil {
			return fmt.Errorf("saving record for station %s on %s: %w", record.StationID, record.Date, err)
		}
	}
	return nil
}

func (c *TidePredictionCache) isValid(record TidePredictionRecord) bool {
	return time.Now().Unix() < record.TTL
}
