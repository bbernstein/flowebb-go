package station

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"math"
	"sort"
	"strconv"
	"sync"

	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
)

type FinderFactory interface {
	NewFinder(httpClient *client.Client, memCache *cache.StationCache) (*NOAAStationFinder, error)
}

type DefaultFinderFactory struct{}

func (f *DefaultFinderFactory) NewFinder(httpClient *client.Client, memCache *cache.StationCache) (*NOAAStationFinder, error) {
	return NewNOAAStationFinder(httpClient, memCache)
}

type NOAAStationFinder struct {
	httpClient *client.Client
	memCache   *cache.StationCache
	s3Cache    cache.StationListCacheProvider
	cacheMutex sync.RWMutex
}

var _ models.StationFinder = (*NOAAStationFinder)(nil)

func NewNOAAStationFinder(httpClient *client.Client, memCache *cache.StationCache) (*NOAAStationFinder, error) {
	if memCache == nil {
		memCache = cache.NewStationCache(nil) // Use default config
	}

	return &NOAAStationFinder{
		httpClient: httpClient,
		memCache:   memCache,
	}, nil
}

func (f *NOAAStationFinder) FindNearestStations(ctx context.Context, lat, lon float64, limit int) ([]models.Station, error) {
	// Validate coordinates
	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("invalid latitude: %f", lat)
	}
	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("invalid longitude: %f", lon)
	}

	// Get all stations
	stations, err := f.getStationList(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting station list: %w", err)
	}

	// Calculate distances and sort
	type stationDistance struct {
		station  models.Station
		distance float64
	}

	stationDistances := make([]stationDistance, len(stations))
	for i, station := range stations {
		distance := calculateDistance(lat, lon, station.Latitude, station.Longitude)
		stationDistances[i] = stationDistance{
			station:  station,
			distance: distance,
		}
	}

	// Sort by distance
	sort.Slice(stationDistances, func(i, j int) bool {
		return stationDistances[i].distance < stationDistances[j].distance
	})

	// Limit results and convert back to Station slice
	if limit <= 0 {
		limit = 5 // Default limit if not specified
	}
	if limit > len(stationDistances) {
		limit = len(stationDistances)
	}

	result := make([]models.Station, limit)
	for i := 0; i < limit; i++ {
		station := stationDistances[i].station
		station.Distance = stationDistances[i].distance // Add distance to result
		result[i] = station
	}

	return result, nil
}

func (f *NOAAStationFinder) FindStation(ctx context.Context, stationID string) (*models.Station, error) {
	stations, err := f.getStationList(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting station list: %w", err)
	}

	for _, station := range stations {
		if station.ID == stationID {
			return &station, nil
		}
	}

	return nil, fmt.Errorf("station not found: %s", stationID)
}

func (f *NOAAStationFinder) getStationList(ctx context.Context) ([]models.Station, error) {
	// Check memory cache first
	f.cacheMutex.RLock()
	stations := f.memCache.GetStations()
	f.cacheMutex.RUnlock()

	if stations != nil {
		log.Debug().Msg("Memory cache HIT for station list")
		return stations, nil
	}

	// Check S3 cache if available
	if f.s3Cache != nil {
		stations, err := f.s3Cache.GetStations(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Error getting stations from S3 cache")
		} else if stations != nil {
			log.Debug().Msg("S3 cache HIT for station list")
			// Update memory cache
			f.cacheMutex.Lock()
			f.memCache.SetStations(stations)
			f.cacheMutex.Unlock()
			return stations, nil
		}
	}

	log.Debug().Msg("Cache MISS for station list, fetching from NOAA API")

	// Fetch from NOAA API
	resp, err := f.httpClient.Get(ctx, "/mdapi/prod/webapi/tidepredstations.json")
	if err != nil {
		return nil, fmt.Errorf("fetching stations: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("no response from NOAA API")
	}

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

	if err := json.Unmarshal(resp.Body, &noaaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to Station objects
	stations = make([]models.Station, len(noaaResp.Stations))
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

	// Save to both caches asynchronously
	if f.s3Cache != nil {
		go func() {
			if err := f.s3Cache.SaveStations(context.Background(), stations); err != nil {
				log.Error().Err(err).Msg("Failed to save stations to S3 cache")
			}
		}()
	}

	f.cacheMutex.Lock()
	f.memCache.SetStations(stations)
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
