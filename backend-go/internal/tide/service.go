package tide

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/rs/zerolog/log"
	"math"
	"sort"
	"strconv"
	"time"
)

type Service struct {
	httpClient      *client.Client
	stationFinder   *station.NOAAStationFinder
	predictionCache *cache.TidePredictionCache
}

func NewService(httpClient *client.Client, stationFinder *station.NOAAStationFinder) *Service {
	return &Service{
		httpClient:      httpClient,
		stationFinder:   stationFinder,
		predictionCache: cache.NewTidePredictionCache(),
	}
}

func (s *Service) GetCurrentTide(ctx context.Context, lat, lon float64, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error) {
	stations, err := s.stationFinder.FindNearestStations(ctx, lat, lon, 1)
	if err != nil {
		return nil, fmt.Errorf("finding nearest station: %w", err)
	}

	if len(stations) == 0 {
		return nil, fmt.Errorf("no stations found near coordinates")
	}

	return s.GetCurrentTideForStation(ctx, stations[0].ID, startTimeStr, endTimeStr)
}

func (s *Service) GetCurrentTideForStation(ctx context.Context, stationID string, startTimeStr, endTimeStr *string) (*models.ExtendedTideResponse, error) {
	localStation, err := s.stationFinder.FindStation(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("finding station: %w", err)
	}

	// Create timezone location for the station
	location := time.FixedZone("Station", localStation.TimeZoneOffset)
	now := time.Now().In(location)

	// Parse start time if provided, otherwise use start of today in station's timezone
	var startTime time.Time
	if startTimeStr != nil {
		// Parse local datetime string in station's timezone
		var err error
		startTime, err = time.ParseInLocation("2006-01-02T15:04:05", *startTimeStr, location)
		if err != nil {
			return nil, fmt.Errorf("parsing start time: %w", err)
		}
	} else {
		// Use start of today in station's timezone
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	}

	// Parse end time if provided, otherwise use next day
	var endTime time.Time
	if endTimeStr != nil {
		var err error
		endTime, err = time.ParseInLocation("2006-01-02T15:04:05", *endTimeStr, location)
		if err != nil {
			return nil, fmt.Errorf("parsing end time: %w", err)
		}
	} else {
		endTime = startTime.AddDate(0, 0, 1)
	}

	// Validate date range
	if endTime.Sub(startTime) > 5*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 5 days")
	}

	// Calculate query range
	useExtremes := localStation.StationType != nil && *localStation.StationType == "S"
	queryStart := startTime
	if useExtremes {
		// For extremes, go back one day for better interpolation
		queryStart = startTime.Truncate(24*time.Hour).AddDate(0, 0, -1)
	}

	// End time should be the start of the day after the last day
	queryEnd := endTime.Truncate(24*time.Hour).AddDate(0, 0, 1)

	record, err := s.getPredictionsForDateRange(ctx, localStation, queryStart, queryEnd, location)
	if err != nil {
		return nil, fmt.Errorf("getting predictions: %w", err)
	}

	// Calculate current tide level and type
	var currentLevel *float64
	var currentType *models.TideType
	var filteredPredictions []models.TidePrediction
	var filteredExtremes []models.TideExtreme

	// Convert times for filtering while preserving local time meaning
	startLocal := startTime.Unix() * 1000 // milliseconds
	endLocal := endTime.Unix() * 1000     // milliseconds
	nowLocal := now.Unix() * 1000         // milliseconds

	if useExtremes {
		log.Debug().Msg("Using extremes for prediction")
		var interpolatedPredictions []models.TidePrediction
		// Generate predictions at 6-minute intervals
		for t := startLocal; t <= endLocal; t += 6 * 60 * 1000 {
			height := interpolateExtremes(record.Extremes, t)
			interpolatedPredictions = append(interpolatedPredictions, models.TidePrediction{
				Timestamp: t,
				LocalTime: formatLocalTime(t, location),
				Height:    height,
			})
		}
		filteredPredictions = interpolatedPredictions
		level := interpolateExtremes(record.Extremes, nowLocal)
		currentLevel = &level
	} else {
		log.Debug().Msg("Using predictions for prediction")
		level := interpolatePredictions(record.Predictions, nowLocal)
		currentLevel = &level
		filteredPredictions = filterTimestamps(record.Predictions, startLocal, endLocal)
	}

	filteredExtremes = filterExtremes(record.Extremes, startLocal, endLocal)

	// Determine tide type
	if len(filteredPredictions) >= 2 {
		idx := findNearestIndex(filteredPredictions, nowLocal)
		if idx > 0 && idx < len(filteredPredictions) {
			if *currentLevel > filteredPredictions[idx-1].Height {
				rising := models.TideTypeRising
				currentType = &rising
			} else {
				falling := models.TideFalling
				currentType = &falling
			}
		}
	}

	// Format current time in local timezone for response
	nowStr := now.Format("2006-01-02T15:04:05")

	return &models.ExtendedTideResponse{
		ResponseType:          "tide",
		Timestamp:             nowLocal,
		LocalTime:             nowStr, // Add local time string
		WaterLevel:            currentLevel,
		PredictedLevel:        currentLevel,
		NearestStation:        localStation.ID,
		Location:              &localStation.Name,
		Latitude:              localStation.Latitude,
		Longitude:             localStation.Longitude,
		StationDistance:       localStation.Distance,
		TideType:              currentType,
		CalculationMethod:     "NOAA API",
		Extremes:              filteredExtremes,
		Predictions:           filteredPredictions,
		TimeZoneOffsetSeconds: &localStation.TimeZoneOffset,
	}, nil
}

func (s *Service) fetchNoaaPredictions(ctx context.Context, stationID, startDate, endDate string, location *time.Location) ([]models.TidePrediction, error) {
	resp, err := s.httpClient.Get(ctx, fmt.Sprintf("/api/prod/datagetter"+
		"?station=%s&begin_date=%s&end_date=%s&product=predictions&datum=MLLW"+
		"&units=english&time_zone=lst_ldt&format=json&interval=6",
		stationID, startDate, endDate))
	if err != nil {
		return nil, fmt.Errorf("fetching predictions: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing response body: %w", closeErr)
		}
	}()

	log.Debug().Msgf("Fetched predictions from noaa: station=%s begin_date=%s end_date=%s",
		stationID, startDate, endDate)

	var noaaResp models.NoaaResponse
	if err := json.NewDecoder(resp.Body).Decode(&noaaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	predictions := make([]models.TidePrediction, len(noaaResp.Predictions))
	for i, p := range noaaResp.Predictions {
		timestamp, err := parseNoaaTime(p.Time, location)
		if err != nil {
			return nil, err
		}

		height, err := strconv.ParseFloat(p.Height, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing height %s: %w", p.Height, err)
		}

		predictions[i] = models.TidePrediction{
			Timestamp: timestamp,
			LocalTime: formatLocalTime(timestamp, location),
			Height:    height,
		}
	}

	return predictions, nil
}

func (s *Service) fetchNoaaExtremes(ctx context.Context, stationID, startDate, endDate string, location *time.Location) ([]models.TideExtreme, error) {
	resp, err := s.httpClient.Get(ctx, fmt.Sprintf("/api/prod/datagetter"+
		"?station=%s&begin_date=%s&end_date=%s&product=predictions&datum=MLLW"+
		"&units=english&time_zone=lst_ldt&format=json&interval=hilo",
		stationID, startDate, endDate))
	if err != nil {
		return nil, fmt.Errorf("fetching extremes: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing response body: %w", closeErr)
		}
	}()

	log.Debug().Msgf("Fetched extremes from noaa: station=%s begin_date=%s end_date=%s",
		stationID, startDate, endDate)

	var noaaResp models.NoaaResponse
	if err := json.NewDecoder(resp.Body).Decode(&noaaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	extremes := make([]models.TideExtreme, len(noaaResp.Predictions))
	for i, p := range noaaResp.Predictions {
		timestamp, err := parseNoaaTime(p.Time, location)
		if err != nil {
			return nil, err
		}

		height, err := strconv.ParseFloat(p.Height, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing height %s: %w", p.Height, err)
		}

		var tideType models.TideType
		if p.Type != nil {
			if *p.Type == "H" {
				tideType = models.TideTypeHigh
			} else {
				tideType = models.TideTypeLow
			}
		}

		extremes[i] = models.TideExtreme{
			Type:      tideType,
			Timestamp: timestamp,
			LocalTime: formatLocalTime(timestamp, location),
			Height:    height,
		}
	}

	return extremes, nil
}

// Helper functions for interpolation and filtering

func interpolatePredictions(predictions []models.TidePrediction, timestamp int64) float64 {
	if len(predictions) == 0 {
		return 0
	}

	// Find the two predictions that bracket the requested timestamp
	idx := findNearestIndex(predictions, timestamp)
	if idx <= 0 {
		return predictions[0].Height
	}
	if idx >= len(predictions) {
		return predictions[len(predictions)-1].Height
	}

	// Linear interpolation
	p1 := predictions[idx-1]
	p2 := predictions[idx]
	ratio := float64(timestamp-p1.Timestamp) / float64(p2.Timestamp-p1.Timestamp)
	return p1.Height + (p2.Height-p1.Height)*ratio
}

func interpolateExtremes(extremes []models.TideExtreme, timestamp int64) float64 {
	if len(extremes) == 0 {
		return 0
	}

	// Find the two extremes that bracket the requested timestamp
	idx := findNearestExtremeIndex(extremes, timestamp)
	if idx <= 0 {
		return extremes[0].Height
	}
	if idx >= len(extremes) {
		return extremes[len(extremes)-1].Height
	}

	// Cubic spline interpolation for smoother transitions between extremes
	e1 := extremes[idx-1]
	e2 := extremes[idx]
	t := float64(timestamp-e1.Timestamp) / float64(e2.Timestamp-e1.Timestamp)

	// Hermite interpolation
	h00 := 2*math.Pow(t, 3) - 3*math.Pow(t, 2) + 1
	h10 := math.Pow(t, 3) - 2*math.Pow(t, 2) + t
	h01 := -2*math.Pow(t, 3) + 3*math.Pow(t, 2)
	h11 := math.Pow(t, 3) - math.Pow(t, 2)

	// Approximate tangents using neighboring points
	m1 := 0.0
	m2 := 0.0
	if idx > 1 {
		m1 = (e2.Height - extremes[idx-2].Height) / float64(e2.Timestamp-extremes[idx-2].Timestamp)
	}
	if idx < len(extremes)-1 {
		m2 = (extremes[idx+1].Height - e1.Height) / float64(extremes[idx+1].Timestamp-e1.Timestamp)
	}

	return h00*e1.Height + h10*m1*float64(e2.Timestamp-e1.Timestamp) +
		h01*e2.Height + h11*m2*float64(e2.Timestamp-e1.Timestamp)
}

func (s *Service) getPredictionsForDateRange(ctx context.Context, station *models.Station, startDate, endDate time.Time, location *time.Location) (*cache.TidePredictionRecord, error) {
	// Format dates for NOAA API (YYYYMMDD)
	startStr := startDate.Format("20060102")
	endStr := endDate.Format("20060102")

	predictions, err := s.fetchNoaaPredictions(ctx, station.ID, startStr, endStr, location)
	if err != nil {
		return nil, err
	}

	extremes, err := s.fetchNoaaExtremes(ctx, station.ID, startStr, endStr, location)
	if err != nil {
		return nil, err
	}

	record := &cache.TidePredictionRecord{
		StationID:   station.ID,
		Date:        startDate.Format("2006-01-02"), // Use start date for the record
		StationType: *station.StationType,
		Predictions: predictions,
		Extremes:    extremes,
		LastUpdated: time.Now().Unix(),
	}

	// Note: We might want to update the cache storage to handle date ranges
	// For now, just return the record without caching
	return record, nil
}

func findNearestIndex(predictions []models.TidePrediction, timestamp int64) int {
	return sort.Search(len(predictions), func(i int) bool {
		return predictions[i].Timestamp >= timestamp
	})
}

func findNearestExtremeIndex(extremes []models.TideExtreme, timestamp int64) int {
	return sort.Search(len(extremes), func(i int) bool {
		return extremes[i].Timestamp >= timestamp
	})
}

func filterTimestamps(predictions []models.TidePrediction, start, end int64) []models.TidePrediction {
	var filtered []models.TidePrediction
	for _, p := range predictions {
		if p.Timestamp >= start && p.Timestamp <= end {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func filterExtremes(extremes []models.TideExtreme, start, end int64) []models.TideExtreme {
	var filtered []models.TideExtreme
	for _, extreme := range extremes {
		if extreme.Timestamp >= start && extreme.Timestamp <= end {
			filtered = append(filtered, extreme)
		}
	}
	return filtered
}

func parseNoaaTime(timeStr string, location *time.Location) (int64, error) {
	// NOAA time format is "2006-01-02 15:04"
	// Parse in the station's local timezone
	t, err := time.ParseInLocation("2006-01-02 15:04", timeStr, location)
	if err != nil {
		return 0, fmt.Errorf("parsing time %s: %w", timeStr, err)
	}
	return t.UnixMilli(), nil
}

func formatLocalTime(timestamp int64, location *time.Location) string {
	t := time.Unix(timestamp/1000, 0).In(location)
	return t.Format("2006-01-02T15:04:05")
}
