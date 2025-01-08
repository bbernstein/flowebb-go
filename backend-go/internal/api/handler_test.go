package api

import (
	"encoding/json"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestSuccess(t *testing.T) {
	tests := []struct {
		name     string
		response interface{}
		want     int
	}{
		{
			name: "station response",
			response: StationsResponse{
				APIResponse: APIResponse{ResponseType: "stations"},
				Stations:    []models.Station{},
			},
			want: http.StatusOK,
		},
		{
			name: "nil response",
			response: ErrorResponse{
				APIResponse: APIResponse{ResponseType: "error"},
				Error:       "test error",
			},
			want: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Success(tt.response)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.StatusCode)

			// Verify response body can be unmarshaled back to the correct type
			var resp APIResponse
			err = json.Unmarshal([]byte(got.Body), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.response.(interface{ GetResponseType() string }).GetResponseType(), resp.ResponseType)

			// Verify CORS headers
			assert.Equal(t, "application/json", got.Headers["Content-Type"])
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		statusCode int
		want       string
	}{
		{
			name:       "basic error",
			message:    "test error",
			statusCode: http.StatusBadRequest,
			want:       "test error",
		},
		{
			name:       "server error",
			message:    "internal server error",
			statusCode: http.StatusInternalServerError,
			want:       "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Error(tt.message, tt.statusCode)
			require.NoError(t, err)
			assert.Equal(t, tt.statusCode, got.StatusCode)

			// Verify error response structure
			var errorResp ErrorResponse
			err = json.Unmarshal([]byte(got.Body), &errorResp)
			require.NoError(t, err)
			assert.Equal(t, "error", errorResp.ResponseType)
			assert.Equal(t, tt.want, errorResp.Error)

			// Verify CORS headers
			assert.Equal(t, "application/json", got.Headers["Content-Type"])
		})
	}
}

func TestParseCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		{
			name: "valid coordinates",
			params: map[string]string{
				"lat": "47.6062",
				"lon": "-122.3321",
			},
			wantLat: 47.6062,
			wantLon: -122.3321,
			wantErr: false,
		},
		{
			name: "invalid latitude",
			params: map[string]string{
				"lat": "91.0", // Beyond valid range
				"lon": "-122.3321",
			},
			wantErr: true,
		},
		{
			name: "invalid longitude",
			params: map[string]string{
				"lat": "47.6062",
				"lon": "-181.0", // Beyond valid range
			},
			wantErr: true,
		},
		{
			name:    "missing coordinates",
			params:  map[string]string{},
			wantLat: 0,
			wantLon: 0,
			wantErr: true,
		},
		{
			name: "non-numeric latitude",
			params: map[string]string{
				"lat": "invalid",
				"lon": "-122.3321",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, err := ParseCoordinates(tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantLat, lat)
			assert.Equal(t, tt.wantLon, lon)
		})
	}
}

func TestNewStationsResponse(t *testing.T) {
	stations := []models.Station{
		{ID: "station1", Name: "Station 1"},
		{ID: "station2", Name: "Station 2"},
	}

	response := NewStationsResponse(stations)

	assert.Equal(t, "stations", response.ResponseType)
	assert.Equal(t, stations, response.Stations)
}
