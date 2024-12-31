package station

import (
	"context"
	"encoding/json"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNOAAStationFinder_FindStation(t *testing.T) {
	// Create mock HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := struct {
			StationList []struct {
				StationId    string  `json:"stationId"`
				Name         string  `json:"name"`
				State        string  `json:"state"`
				Region       string  `json:"region"`
				Lat          float64 `json:"lat"`
				Lon          float64 `json:"lon"`
				TimeZoneCorr string  `json:"timeZoneCorr"`
				// Level and StationType are missing from this struct
			} `json:"stationList"`
		}{
			StationList: []struct {
				StationId    string  `json:"stationId"`
				Name         string  `json:"name"`
				State        string  `json:"state"`
				Region       string  `json:"region"`
				Lat          float64 `json:"lat"`
				Lon          float64 `json:"lon"`
				TimeZoneCorr string  `json:"timeZoneCorr"`
			}{
				{
					StationId:    "9447130",
					Name:         "Seattle",
					State:        "WA",
					Region:       "Puget Sound",
					Lat:          47.602638889,
					Lon:          -122.339167,
					TimeZoneCorr: "-8",
					// Level and StationType should be omitted completely
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	// Create test cache
	testCache := cache.NewStationCache()

	// Create HTTP client with the test server URL
	httpClient := client.New(client.Options{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	finder := NewNOAAStationFinder(httpClient, testCache)

	tests := []struct {
		name      string
		stationID string
		want      *models.Station // Change this type
		wantErr   bool
	}{
		{
			name:      "existing station",
			stationID: "9447130",
			want: &models.Station{ // Change this type
				ID:             "9447130",
				Name:           "Seattle",
				State:          stringPtr("WA"),
				Region:         stringPtr("Puget Sound"),
				Latitude:       47.602638889,
				Longitude:      -122.339167,
				Source:         models.SourceNOAA, // Use models.Source
				Capabilities:   []string{"WATER_LEVEL"},
				TimeZoneOffset: -8 * 3600,
			},
			wantErr: false,
		},
		{
			name:      "non-existent station",
			stationID: "invalid",
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := finder.FindStation(context.Background(), tt.stationID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNOAAStationFinder_FindNearestStations(t *testing.T) {
	// Create mock HTTP server with multiple stations
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := struct {
			StationList []struct {
				StationId    string  `json:"stationId"`
				Name         string  `json:"name"`
				State        string  `json:"state"`
				Region       string  `json:"region"`
				Lat          float64 `json:"lat"`
				Lon          float64 `json:"lon"`
				TimeZoneCorr string  `json:"timeZoneCorr"`
			} `json:"stationList"`
		}{
			StationList: []struct {
				StationId    string  `json:"stationId"`
				Name         string  `json:"name"`
				State        string  `json:"state"`
				Region       string  `json:"region"`
				Lat          float64 `json:"lat"`
				Lon          float64 `json:"lon"`
				TimeZoneCorr string  `json:"timeZoneCorr"`
			}{
				{
					StationId:    "9447130",
					Name:         "Seattle",
					State:        "WA",
					Region:       "Puget Sound",
					Lat:          47.602638889,
					Lon:          -122.339167,
					TimeZoneCorr: "-8",
				},
				{
					StationId:    "9447819",
					Name:         "Tacoma",
					State:        "WA",
					Region:       "Puget Sound",
					Lat:          47.269,
					Lon:          -122.4138,
					TimeZoneCorr: "-8",
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	testCache := cache.NewStationCache()
	httpClient := client.New(client.Options{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	finder := NewNOAAStationFinder(httpClient, testCache)

	tests := []struct {
		name      string
		lat       float64
		lon       float64
		limit     int
		wantCount int
		wantFirst string // ID of the station that should be first
		wantErr   bool
	}{
		{
			name:      "find nearest to Seattle",
			lat:       47.6062,
			lon:       -122.3321,
			limit:     2,
			wantCount: 2,
			wantFirst: "9447130", // Seattle should be closest
			wantErr:   false,
		},
		{
			name:      "find nearest to Tacoma",
			lat:       47.2690,
			lon:       -122.4138,
			limit:     1,
			wantCount: 1,
			wantFirst: "9447819", // Tacoma should be closest
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := finder.FindNearestStations(context.Background(), tt.lat, tt.lon, tt.limit)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.Len(t, got, tt.wantCount)
			if len(got) > 0 {
				assert.Equal(t, tt.wantFirst, got[0].ID)
			}

			// Verify stations are sorted by distance
			if len(got) > 1 {
				for i := 1; i < len(got); i++ {
					assert.Less(t, got[i-1].Distance, got[i].Distance,
						"Stations should be sorted by distance")
				}
			}
		})
	}
}

func TestCalculateDistance(t *testing.T) {
	tests := []struct {
		name      string
		lat1      float64
		lon1      float64
		lat2      float64
		lon2      float64
		want      float64
		tolerance float64
	}{
		{
			name:      "Seattle to Tacoma",
			lat1:      47.6062,
			lon1:      -122.3321,
			lat2:      47.690,
			lon2:      -122.4138,
			want:      11.1, // km
			tolerance: 0.1,  // Allow 100m difference due to floating point
		},
		{
			name:      "Same point",
			lat1:      47.6062,
			lon1:      -122.3321,
			lat2:      47.6062,
			lon2:      -122.3321,
			want:      0,
			tolerance: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			assert.InDelta(t, tt.want, got, tt.tolerance)
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
