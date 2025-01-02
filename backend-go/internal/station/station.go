package station

import (
	"context"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
)

// StationFinder defines the interface for finding stations
type StationFinder interface {
	FindStation(ctx context.Context, stationID string) (*models.Station, error)
	FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error)
}
