// internal/tide/interface.go
package tide

import (
	"context"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"time"
)

type TideService interface {
	GetCurrentTide(ctx context.Context, lat, lon float64, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error)
	GetCurrentTideForStation(ctx context.Context, stationID string, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error)
}

type StationFinder interface {
	FindStation(ctx context.Context, stationID string) (*models.Station, error)
	FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error)
}

type CacheProvider interface {
	GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error)
	SavePredictions(ctx context.Context, record models.TidePredictionRecord) error
	SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error
	GetCacheStats() map[string]uint64
	Clear()
}
