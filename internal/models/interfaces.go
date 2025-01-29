package models

import "context"

type StationFinder interface {
	FindStation(ctx context.Context, stationID string) (*Station, error)
	FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]Station, error)
}
