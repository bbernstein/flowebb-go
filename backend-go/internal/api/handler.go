package api

import (
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"net/http"
	"strconv"
)

type APIResponse struct {
	ResponseType string `json:"responseType"`
}

type StationsResponse struct {
	APIResponse
	Stations []models.Station `json:"stations"`
}

type ErrorResponse struct {
	APIResponse
	Error string `json:"error"`
}

func NewStationsResponse(stations []models.Station) *StationsResponse {
	return &StationsResponse{
		APIResponse: APIResponse{ResponseType: "stations"},
		Stations:    stations,
	}
}

func NewErrorResponse(message string) *ErrorResponse {
	return &ErrorResponse{
		APIResponse: APIResponse{ResponseType: "error"},
		Error:       message,
	}
}

// Response helpers
func Success(body interface{}) (events.APIGatewayProxyResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return Error("Internal Server Error", http.StatusInternalServerError)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(jsonBody),
	}, nil
}

func Error(message string, statusCode int) (events.APIGatewayProxyResponse, error) {
	body, _ := json.Marshal(NewErrorResponse(message))

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}, nil
}

// Parameter parsing helpers
func ParseCoordinates(params map[string]string) (float64, float64, error) {
	latStr, hasLat := params["lat"]
	lonStr, hasLon := params["lon"]

	if !hasLat || !hasLon {
		return 0, 0, nil
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return 0, 0, err
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return 0, 0, err
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, InvalidCoordinatesError{}
	}

	return lat, lon, nil
}

type InvalidCoordinatesError struct{}

func (e InvalidCoordinatesError) Error() string {
	return "Invalid coordinates"
}
