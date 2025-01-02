package cache

import (
	"context"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"time"
)

type MockCacheService struct{}

func NewMockCacheService() *MockCacheService {
	return &MockCacheService{}
}

func (m *MockCacheService) GetPredictions(_ context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	return &models.TidePredictionRecord{
		StationID:   stationID,
		Date:        date.Format("2006-01-02"),
		Predictions: []models.TidePrediction{},
		Extremes:    []models.TideExtreme{},
	}, nil
}

func (m *MockCacheService) SavePredictionsBatch(_ context.Context, _ []models.TidePredictionRecord) error {
	return nil
}
