package handler

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/internal/api"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"net/http"
	"strconv"
)

type StationsHandler struct {
	stationFinder station.StationFinder
}

func NewStationsHandler(finder station.StationFinder) *StationsHandler {
	return &StationsHandler{
		stationFinder: finder,
	}
}

func (h *StationsHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	params := request.QueryStringParameters

	// Check if we're looking up by station ID or coordinates
	if stationID, ok := params["stationId"]; ok {
		stationLocal, err := h.stationFinder.FindStation(ctx, stationID)
		if err != nil {
			return api.Error("Error finding station", http.StatusInternalServerError)
		}
		if stationLocal == nil {
			return api.Error("Station not found", http.StatusNotFound)
		}
		return api.Success(api.NewStationsResponse([]models.Station{*stationLocal}))
	}

	// Parse coordinates
	lat, lon, err := api.ParseCoordinates(params)
	if err != nil {
		var invalidCoordErr api.InvalidCoordinatesError
		if errors.As(err, &invalidCoordErr) {
			return api.Error(err.Error(), http.StatusBadRequest)
		}
		return api.Error("Invalid parameters", http.StatusBadRequest)
	}

	// Default limit to 5 if not specified
	limit := 5
	if limitStr, ok := params["limit"]; ok {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	stations, err := h.stationFinder.FindNearestStations(ctx, lat, lon, limit)
	if err != nil {
		return api.Error("Error finding stations", http.StatusInternalServerError)
	}

	return api.Success(api.NewStationsResponse(stations))
}
