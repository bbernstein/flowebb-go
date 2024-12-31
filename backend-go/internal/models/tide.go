package models

type TideType string

const (
	TideTypeRising TideType = "RISING"
	TideFalling    TideType = "FALLING"
	TideTypeHigh   TideType = "HIGH"
	TideTypeLow    TideType = "LOW"
)

// TideExtreme represents a high or low tide
type TideExtreme struct {
	Type      TideType `json:"type"`
	Timestamp int64    `json:"timestamp"`
	LocalTime string   `json:"localTime"` // Add this field
	Height    float64  `json:"height"`
}

// TidePrediction represents a tide prediction at a specific time
type TidePrediction struct {
	Timestamp int64   `json:"timestamp"`
	LocalTime string  `json:"localTime"` // Add this field
	Height    float64 `json:"height"`
}

func (tp TidePrediction) GetTimestamp() int64 {
	return tp.Timestamp
}

// ExtendedTideResponse represents the full tide response including predictions and extremes
type ExtendedTideResponse struct {
	ResponseType          string           `json:"responseType"`
	Timestamp             int64            `json:"timestamp"`
	LocalTime             string           `json:"localTime"` // Add this field
	WaterLevel            *float64         `json:"waterLevel"`
	PredictedLevel        *float64         `json:"predictedLevel"`
	NearestStation        string           `json:"nearestStation"`
	Location              *string          `json:"location"`
	Latitude              float64          `json:"latitude"`
	Longitude             float64          `json:"longitude"`
	StationDistance       float64          `json:"stationDistance"`
	TideType              *TideType        `json:"tideType"`
	CalculationMethod     string           `json:"calculationMethod"`
	Extremes              []TideExtreme    `json:"extremes"`
	Predictions           []TidePrediction `json:"predictions"`
	TimeZoneOffsetSeconds *int             `json:"timeZoneOffsetSeconds"`
}

// NoaaPrediction represents the raw NOAA API prediction response
type NoaaPrediction struct {
	Time   string  `json:"t"`              // Time of prediction
	Height string  `json:"v"`              // Predicted water level
	Type   *string `json:"type,omitempty"` // Type of prediction (H for high, L for low)
}

type NoaaResponse struct {
	Predictions []NoaaPrediction `json:"predictions"`
}
