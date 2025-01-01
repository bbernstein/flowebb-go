package station

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/rs/zerolog/log"
	"math"
	"sort"
	"strconv"
	"sync"

	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
)

type NOAAStationFinder struct {
	httpClient *client.Client
	cache      *cache.StationCache
	cacheMutex sync.RWMutex
}

func NewNOAAStationFinder(httpClient *client.Client, stationCache *cache.StationCache) *NOAAStationFinder {
	if stationCache == nil {
		cacheConfig := config.GetCacheConfig()
		stationCache = cache.NewStationCache(cacheConfig)
	}
	return &NOAAStationFinder{
		httpClient: httpClient,
		cache:      stationCache,
	}
}

func (f *NOAAStationFinder) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	stations, err := f.getStationList(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting station list: %w", err)
	}

	for _, station := range stations {
		if station.ID == stationID {
			log.Trace().Str("station_id", stationID).Str("stationType", *station.StationType).Msg("FindStation: Found station")
			return &station, nil
		}
	}

	return nil, fmt.Errorf("station not found: %s", stationID)
}

func (f *NOAAStationFinder) FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
	stations, err := f.getStationList(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting station list: %w", err)
	}

	// Calculate distances in parallel using worker pool
	const workerCount = 4
	work := make(chan models.Station, len(stations))
	results := make(chan models.Station, len(stations))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for station := range work {
				station.Distance = calculateDistance(lat, lon, station.Latitude, station.Longitude)
				results <- station
			}
		}()
	}

	// Send work
	for _, station := range stations {
		work <- station
	}
	close(work)

	// Wait for workers and close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and sort results
	var stationsWithDistance []models.Station
	for station := range results {
		log.Trace().Str("station_id", station.ID).Str("stationType", *station.StationType).Msg("FindNearestStations: Collecting")
		stationsWithDistance = append(stationsWithDistance, station)
	}

	// Sort by distance and limit results
	sort.Slice(stationsWithDistance, func(i, j int) bool {
		return stationsWithDistance[i].Distance < stationsWithDistance[j].Distance
	})

	if len(stationsWithDistance) > limit {
		stationsWithDistance = stationsWithDistance[:limit]
	}

	return stationsWithDistance, nil
}

func (f *NOAAStationFinder) getStationList(ctx context.Context) ([]models.Station, error) {
	// Check cache first
	f.cacheMutex.RLock()
	cachedStations := f.cache.GetStations()
	f.cacheMutex.RUnlock()

	if cachedStations != nil {
		log.Debug().Msg("Cache HIT for station list")
		return cachedStations, nil
	}
	log.Debug().Msg("Cache MISS for station list, calling noaa API")

	// Fetch from NOAA API
	resp, err := f.httpClient.Get(ctx, "/mdapi/prod/webapi/tidepredstations.json")
	if err != nil {
		return nil, fmt.Errorf("fetching stations: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing response body: %w", closeErr)
		}
	}()

	var noaaResp struct {
		Stations []struct {
			ID           string  `json:"stationId"`
			Name         string  `json:"name"`
			State        string  `json:"state"`
			Region       string  `json:"region"`
			Lat          float64 `json:"lat"`
			Lon          float64 `json:"lon"`
			TimeZoneCorr string  `json:"timeZoneCorr"`
			Level        string  `json:"level"`
			StationType  string  `json:"stationType"`
		} `json:"stationList"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&noaaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to Station objects
	stations := make([]models.Station, len(noaaResp.Stations))
	for i, s := range noaaResp.Stations {
		var level, stationType *string
		if s.Level != "" {
			levelValue := s.Level
			level = &levelValue
		}
		if s.StationType != "" {
			stationTypeValue := s.StationType
			stationType = &stationTypeValue
		}

		stations[i] = models.Station{
			ID:             s.ID,
			Name:           s.Name,
			State:          &s.State,
			Region:         &s.Region,
			Latitude:       s.Lat,
			Longitude:      s.Lon,
			Source:         models.SourceNOAA,
			Capabilities:   []string{"WATER_LEVEL"},
			TimeZoneOffset: parseTimeZoneOffset(s.TimeZoneCorr),
			Level:          level,
			StationType:    stationType,
		}
	}

	log.Debug().Int("station_count", len(stations)).Msgf("Caching list of %d stations", len(stations))

	// Update cache
	f.cacheMutex.Lock()
	f.cache.SetStations(stations)
	f.cacheMutex.Unlock()

	return stations, nil
}

func parseTimeZoneOffset(tzCorr string) int {
	offset, err := strconv.Atoi(tzCorr)
	if err != nil {
		return 0
	}
	return offset * 3600 // Convert hours to seconds
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371.0 // km

	dLat := toRadians(lat2 - lat1)
	dLon := toRadians(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

func toRadians(deg float64) float64 {
	return deg * math.Pi / 180
}
