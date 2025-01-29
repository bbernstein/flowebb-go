package graph

import (
	"context"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/graph/model"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResolver_Stations(t *testing.T) {
	tests := []struct {
		name      string
		lat       float64
		lon       float64
		limit     *int
		setupMock func() *Resolver
		want      []*model.Station
		wantErr   bool
	}{
		{
			name:  "successful station lookup",
			lat:   47.6062,
			lon:   -122.3321,
			limit: func() *int { limit := 2; return &limit }(),
			setupMock: func() *Resolver {
				return &Resolver{
					StationFinder: &mockStationFinder{
						findNearestStationsFn: func(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
							return []models.Station{
								{
									ID:        "TEST001",
									Name:      "Test Station 1",
									Latitude:  lat,
									Longitude: lon,
								},
							}, nil
						},
					},
				}
			},
			want: []*model.Station{
				{
					ID:        "TEST001",
					Name:      "Test Station 1",
					Latitude:  47.6062,
					Longitude: -122.3321,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := tt.setupMock()
			queryResolver := resolver.Query()

			got, err := queryResolver.Stations(context.Background(), &tt.lat, &tt.lon, tt.limit)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, len(tt.want), len(got))

			for i, station := range tt.want {
				assert.Equal(t, station.ID, got[i].ID)
				assert.Equal(t, station.Name, got[i].Name)
				assert.Equal(t, station.Latitude, got[i].Latitude)
				assert.Equal(t, station.Longitude, got[i].Longitude)
			}
		})
	}
}

func TestResolver_Tides(t *testing.T) {
	tests := []struct {
		name      string
		stationID string
		startTime string
		endTime   string
		setupMock func() *Resolver
		want      *model.TideData
		wantErr   bool
	}{
		{
			name:      "successful tide lookup with predictions and extremes",
			stationID: "TEST001",
			startTime: "2024-01-01T00:00:00",
			endTime:   "2024-01-02T00:00:00",
			setupMock: func() *Resolver {
				waterLevel := 1.5
				predictedLevel := 1.6
				tideType := models.TideTypeHigh
				timeZoneOffset := -28800

				return &Resolver{
					TideService: &mockTideService{
						getCurrentTideForStationFn: func(ctx context.Context, stationID string, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error) {
							return &models.ExtendedTideResponse{
								Timestamp:             1704067200000,
								LocalTime:             "2024-01-01T00:00:00",
								WaterLevel:            &waterLevel,
								PredictedLevel:        &predictedLevel,
								NearestStation:        stationID,
								Latitude:              47.6062,
								Longitude:             -122.3321,
								StationDistance:       0,
								TideType:              &tideType,
								CalculationMethod:     "NOAA API",
								TimeZoneOffsetSeconds: &timeZoneOffset,
								Predictions: []models.TidePrediction{
									{Timestamp: 1704067200000, LocalTime: "2024-01-01T00:00:00", Height: 1.5},
								},
								Extremes: []models.TideExtreme{
									{Type: models.TideTypeHigh, Timestamp: 1704067200000, LocalTime: "2024-01-01T00:00:00", Height: 1.5},
								},
							}, nil
						},
					},
				}
			},
			want: &model.TideData{
				Timestamp:             1704067200000,
				LocalTime:             "2024-01-01T00:00:00",
				WaterLevel:            1.5,
				PredictedLevel:        1.6,
				NearestStation:        "TEST001",
				Latitude:              47.6062,
				Longitude:             -122.3321,
				StationDistance:       0,
				TideType:              "HIGH",
				CalculationMethod:     "NOAA API",
				TimeZoneOffsetSeconds: -28800,
				Predictions: []*model.TidePrediction{
					{Timestamp: 1704067200000, LocalTime: "2024-01-01T00:00:00", Height: 1.5},
				},
				Extremes: []*model.TideExtreme{
					{Type: "HIGH", Timestamp: 1704067200000, LocalTime: "2024-01-01T00:00:00", Height: 1.5},
				},
			},
			wantErr: false,
		},
		{
			name:      "nil tide response",
			stationID: "TEST002",
			startTime: "2024-01-01T00:00:00",
			endTime:   "2024-01-02T00:00:00",
			setupMock: func() *Resolver {
				return &Resolver{
					TideService: &mockTideService{
						getCurrentTideForStationFn: func(ctx context.Context, stationID string, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error) {
							return nil, nil
						},
					},
				}
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "error from tide service",
			stationID: "TEST003",
			startTime: "2024-01-01T00:00:00",
			endTime:   "2024-01-02T00:00:00",
			setupMock: func() *Resolver {
				return &Resolver{
					TideService: &mockTideService{
						getCurrentTideForStationFn: func(ctx context.Context, stationID string, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error) {
							return nil, fmt.Errorf("service error")
						},
					},
				}
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := tt.setupMock()
			queryResolver := resolver.Query()

			got, err := queryResolver.Tides(context.Background(), tt.stationID, tt.startTime, tt.endTime)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Timestamp, got.Timestamp)
			assert.Equal(t, tt.want.LocalTime, got.LocalTime)
			assert.Equal(t, tt.want.WaterLevel, got.WaterLevel)
			assert.Equal(t, tt.want.PredictedLevel, got.PredictedLevel)
			assert.Equal(t, tt.want.NearestStation, got.NearestStation)
			assert.Equal(t, tt.want.Latitude, got.Latitude)
			assert.Equal(t, tt.want.Longitude, got.Longitude)
			assert.Equal(t, tt.want.StationDistance, got.StationDistance)
			assert.Equal(t, tt.want.TideType, got.TideType)
			assert.Equal(t, tt.want.CalculationMethod, got.CalculationMethod)
			assert.Equal(t, tt.want.TimeZoneOffsetSeconds, got.TimeZoneOffsetSeconds)
			assert.Equal(t, len(tt.want.Predictions), len(got.Predictions))
			for i, p := range tt.want.Predictions {
				assert.Equal(t, p.Timestamp, got.Predictions[i].Timestamp)
				assert.Equal(t, p.LocalTime, got.Predictions[i].LocalTime)
				assert.Equal(t, p.Height, got.Predictions[i].Height)
			}
			assert.Equal(t, len(tt.want.Extremes), len(got.Extremes))
			for i, e := range tt.want.Extremes {
				assert.Equal(t, e.Type, got.Extremes[i].Type)
				assert.Equal(t, e.Timestamp, got.Extremes[i].Timestamp)
				assert.Equal(t, e.LocalTime, got.Extremes[i].LocalTime)
				assert.Equal(t, e.Height, got.Extremes[i].Height)
			}
		})
	}
}
