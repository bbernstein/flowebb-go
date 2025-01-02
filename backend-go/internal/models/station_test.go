package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStationSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		station   Station
		wantJSON  string
		wantError bool
	}{
		{
			name: "complete station",
			station: Station{
				ID:             "TEST001",
				Name:           "Test Station",
				State:          stringPtr("WA"),
				Region:         stringPtr("Pacific Northwest"),
				Distance:       10.5,
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				Capabilities:   []string{"WATER_LEVEL"},
				TimeZoneOffset: -28800, // -8 hours in seconds
				Level:          stringPtr("R"),
				StationType:    stringPtr("R"),
			},
			wantError: false,
		},
		{
			name: "minimal station",
			station: Station{
				ID:             "TEST002",
				Name:           "Minimal Station",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				Capabilities:   []string{"WATER_LEVEL"},
				TimeZoneOffset: 0,
			},
			wantError: false,
		},
		{
			name: "invalid source",
			station: Station{
				ID:             "TEST003",
				Name:           "Invalid Source",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         Source("INVALID"),
				Capabilities:   []string{"WATER_LEVEL"},
				TimeZoneOffset: 0,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test marshaling
			data, err := json.Marshal(tt.station)
			require.NoError(t, err, "JSON marshaling should not fail")

			// Test unmarshaling
			var decoded Station
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "JSON unmarshaling should not fail")

			// Validate the station
			err = decoded.Validate()
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Compare specific fields
			assert.Equal(t, tt.station.ID, decoded.ID)
			assert.Equal(t, tt.station.Name, decoded.Name)
			assert.Equal(t, tt.station.Latitude, decoded.Latitude)
			assert.Equal(t, tt.station.Longitude, decoded.Longitude)
			assert.Equal(t, tt.station.Source, decoded.Source)
			assert.Equal(t, tt.station.TimeZoneOffset, decoded.TimeZoneOffset)
		})
	}
}

func TestStationValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		station   Station
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid station",
			station: Station{
				ID:             "TEST001",
				Name:           "Test Station",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				TimeZoneOffset: -28800,
			},
			wantError: false,
		},
		{
			name: "missing ID",
			station: Station{
				Name:           "Missing ID",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				TimeZoneOffset: -28800,
			},
			wantError: true,
			errorMsg:  "station ID is required",
		},
		{
			name: "invalid latitude",
			station: Station{
				ID:             "TEST002",
				Name:           "Invalid Latitude",
				Latitude:       91.0, // Invalid: >90
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				TimeZoneOffset: -28800,
			},
			wantError: true,
			errorMsg:  "invalid latitude",
		},
		{
			name: "invalid longitude",
			station: Station{
				ID:             "TEST003",
				Name:           "Invalid Longitude",
				Latitude:       47.6062,
				Longitude:      -181.0, // Invalid: <-180
				Source:         SourceNOAA,
				TimeZoneOffset: -28800,
			},
			wantError: true,
			errorMsg:  "invalid longitude",
		},
		{
			name: "invalid timezone offset",
			station: Station{
				ID:             "TEST004",
				Name:           "Invalid Timezone",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         SourceNOAA,
				TimeZoneOffset: -60000, // Invalid: too large negative offset
			},
			wantError: true,
			errorMsg:  "invalid timezone offset",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.station.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestStationSourceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    Source
		wantValid bool
	}{
		{
			name:      "NOAA source",
			source:    SourceNOAA,
			wantValid: true,
		},
		{
			name:      "UKHO source",
			source:    SourceUKHO,
			wantValid: true,
		},
		{
			name:      "CHS source",
			source:    SourceCHS,
			wantValid: true,
		},
		{
			name:      "invalid source",
			source:    Source("INVALID"),
			wantValid: false,
		},
		{
			name:      "empty source",
			source:    Source(""),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			station := Station{
				ID:             "TEST001",
				Name:           "Test Station",
				Latitude:       47.6062,
				Longitude:      -122.3321,
				Source:         tt.source,
				TimeZoneOffset: -28800,
			}

			err := station.Validate()
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid source")
			}
		})
	}
}

// Benchmark station validation
func BenchmarkStationValidation(b *testing.B) {
	station := Station{
		ID:             "TEST001",
		Name:           "Test Station",
		State:          stringPtr("WA"),
		Region:         stringPtr("Pacific Northwest"),
		Distance:       10.5,
		Latitude:       47.6062,
		Longitude:      -122.3321,
		Source:         SourceNOAA,
		Capabilities:   []string{"WATER_LEVEL"},
		TimeZoneOffset: -28800,
		Level:          stringPtr("R"),
		StationType:    stringPtr("R"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = station.Validate()
	}
}

// Benchmark station serialization
func BenchmarkStationSerialization(b *testing.B) {
	station := Station{
		ID:             "TEST001",
		Name:           "Test Station",
		State:          stringPtr("WA"),
		Region:         stringPtr("Pacific Northwest"),
		Distance:       10.5,
		Latitude:       47.6062,
		Longitude:      -122.3321,
		Source:         SourceNOAA,
		Capabilities:   []string{"WATER_LEVEL"},
		TimeZoneOffset: -28800,
		Level:          stringPtr("R"),
		StationType:    stringPtr("R"),
	}

	b.Run("Marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := json.Marshal(station)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	data, err := json.Marshal(station)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("Unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var decoded Station
			err := json.Unmarshal(data, &decoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
