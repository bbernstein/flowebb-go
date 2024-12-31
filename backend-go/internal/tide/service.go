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

func (s *Service) GetCurrentTide(ctx context.Context, lat, lon float64, startTime, endTime *time.Time) (*models.ExtendedTideResponse, error) {
	stations, err := s.stationFinder.FindNearestStations(ctx, lat, lon, 1)
	if err != nil {
		return nil, fmt.Errorf("finding nearest station: %w", err)
	}

	if len(stations) == 0 {
		return nil, fmt.Errorf("no stations found near coordinates")
	}

	return s.GetCurrentTideForStation(ctx, stations[0].ID, startTime, endTime)
}

func (s *Service) GetCurrentTideForStation(ctx context.Context, stationID string, startTime, endTime *time.Time) (*models.ExtendedTideResponse, error) {
	localStation, err := s.stationFinder.FindStation(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("finding station: %w", err)
	}

	// Create timezone location for the localStation
	location := time.FixedZone("Station", localStation.TimeZoneOffset)

	now := time.Now().In(location)
	if startTime == nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		startTime = &midnight
	} else {
		// Convert input time to station's timezone
		adjustedTime := startTime.In(location)
		startTime = &adjustedTime
	}
	if endTime == nil {
		nextDay := startTime.AddDate(0, 0, 1)
		endTime = &nextDay
	} else {
		// Convert input time to station's timezone
		adjustedTime := endTime.In(location)
		endTime = &adjustedTime
	}

	// Validate date range
	if endTime.Sub(*startTime) > 5*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 5 days")
	}

	// Get predictions for the date range
	var records []cache.TidePredictionRecord
	currentDate := *startTime

	useExtremes := localStation.StationType != nil && *localStation.StationType == "S"
	startQuery := *startTime
	if useExtremes {
		startQuery = startQuery.AddDate(0, 0, -1) // Go back one day for extremes
	}
	endQuery := endTime.AddDate(0, 0, 1) // Add one day for interpolation

	for currentDate.Before(endQuery) {
		record, err := s.getPredictionsForDate(ctx, localStation, currentDate, location)
		if err != nil {
			return nil, fmt.Errorf("getting predictions for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		records = append(records, *record)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Combine predictions and extremes
	var allPredictions []models.TidePrediction
	var allExtremes []models.TideExtreme
	for _, record := range records {
		allPredictions = append(allPredictions, record.Predictions...)
		allExtremes = append(allExtremes, record.Extremes...)
	}

	// Sort predictions and extremes by timestamp
	sort.Slice(allPredictions, func(i, j int) bool {
		return allPredictions[i].Timestamp < allPredictions[j].Timestamp
	})
	sort.Slice(allExtremes, func(i, j int) bool {
		return allExtremes[i].Timestamp < allExtremes[j].Timestamp
	})

	// Calculate current tide level and type
	var currentLevel *float64
	var currentType *models.TideType
	var filteredPredictions []models.TidePrediction
	var filteredExtremes []models.TideExtreme

	nowMillis := now.UnixMilli()
	startMillis := startTime.UnixMilli()
	endMillis := endTime.UnixMilli()

	// For subordinate stations (type "S" or "C"), generate predictions by interpolating between extremes
	if useExtremes {
		// Generate predictions at 6-minute intervals using extremes
		log.Debug().Msg("Using extremes for prediction")
		var interpolatedPredictions []models.TidePrediction
		for t := startMillis; t <= endMillis; t += 6 * 60 * 1000 { // 6-minute intervals
			height := interpolateExtremes(allExtremes, t)
			interpolatedPredictions = append(interpolatedPredictions, models.TidePrediction{
				Timestamp: t,
				Height:    height,
			})
		}
		filteredPredictions = interpolatedPredictions
		level := interpolateExtremes(allExtremes, nowMillis)
		currentLevel = &level
	} else {
		log.Debug().Msg("Using predictions for prediction")
		level := interpolatePredictions(allPredictions, nowMillis)
		currentLevel = &level
		filteredPredictions = filterTimestamps(allPredictions, startMillis, endMillis)
	}

	filteredExtremes = filterExtremes(allExtremes, startMillis, endMillis)

	// Determine tide type
	if len(filteredPredictions) >= 2 {
		if currentLevel != nil {
			idx := findNearestIndex(filteredPredictions, nowMillis)
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
	}

	return &models.ExtendedTideResponse{
		ResponseType:          "tide",
		Timestamp:             nowMillis,
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

func (s *Service) getPredictionsForDate(ctx context.Context, station *models.Station, date time.Time, location *time.Location) (*cache.TidePredictionRecord, error) {
	// Check cache first
	if record, err := s.predictionCache.GetPredictions(station.ID, date); err == nil && record != nil {
		return record, nil
	}

	// Not in cache, fetch from NOAA
	dateStr := date.Format("20060102") // YYYYMMDD format for NOAA API
	predictions, err := s.fetchNoaaPredictions(ctx, station.ID, dateStr, location)
	if err != nil {
		return nil, err
	}

	extremes, err := s.fetchNoaaExtremes(ctx, station.ID, dateStr, location)
	if err != nil {
		return nil, err
	}

	record := &cache.TidePredictionRecord{
		StationID:   station.ID,
		Date:        date.Format("2006-01-02"),
		StationType: *station.StationType,
		Predictions: predictions,
		Extremes:    extremes,
		LastUpdated: time.Now().Unix(),
	}

	if err := s.predictionCache.SavePredictions(*record); err != nil {
		log.Warn().Err(err).Msg("Failed to cache predictions")
	}

	return record, nil
}

func (s *Service) fetchNoaaPredictions(ctx context.Context, stationID string, date string, location *time.Location) ([]models.TidePrediction, error) {
	resp, err := s.httpClient.Get(ctx, fmt.Sprintf("/api/prod/datagetter"+
		"?station=%s&begin_date=%s&end_date=%s&product=predictions&datum=MLLW"+
		"&units=english&time_zone=lst&format=json&interval=6",
		stationID, date, date))
	if err != nil {
		return nil, fmt.Errorf("fetching predictions: %w", err)
	}
	defer resp.Body.Close()

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
			Height:    height,
		}
	}

	return predictions, nil
}

func (s *Service) fetchNoaaExtremes(ctx context.Context, stationID string, date string, location *time.Location) ([]models.TideExtreme, error) {
	resp, err := s.httpClient.Get(ctx, fmt.Sprintf("/api/prod/datagetter"+
		"?station=%s&begin_date=%s&end_date=%s&product=predictions&datum=MLLW"+
		"&units=english&time_zone=lst&format=json&interval=hilo",
		stationID, date, date))
	if err != nil {
		return nil, fmt.Errorf("fetching extremes: %w", err)
	}
	defer resp.Body.Close()

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
