package main

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bbernstein/flowebb/backend-go/internal/api"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	tideService *tide.Service
	setupOnce   sync.Once
)

func init() {
	setupOnce.Do(func() {
		// Initialize logger
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		levelStr := os.Getenv("LOG_LEVEL")
		if levelStr == "" {
			levelStr = "info"
		}
		level, err := zerolog.ParseLevel(levelStr)
		if err != nil {
			level = zerolog.InfoLevel
		}
		zerolog.SetGlobalLevel(level)

		// Setup console logger for development
		if env := os.Getenv("ENV"); env == "local" || env == "development" {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		}

		// Initialize HTTP client
		httpClient := client.New(client.Options{
			BaseURL: "https://api.tidesandcurrents.noaa.gov",
			Timeout: 30 * time.Second,
		})

		// Initialize station finder
		stationFinder := station.NewNOAAStationFinder(httpClient, nil) // We can pass nil for station cache as it's maintained in the stations lambda

		// Initialize tide service
		tideService = tide.NewService(httpClient, stationFinder)
	})
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	params := request.QueryStringParameters
	log.Info().Msg("Handling tides request")

	var startTimeStr, endTimeStr *string
	if str, ok := params["startDateTime"]; ok {
		startTimeStr = &str
	}
	if str, ok := params["endDateTime"]; ok {
		endTimeStr = &str
	}

	var response *models.ExtendedTideResponse
	var err error

	// Check if we're looking up by station ID or coordinates
	if stationID, ok := params["stationId"]; ok {
		response, err = tideService.GetCurrentTideForStation(ctx, stationID, startTimeStr, endTimeStr)
	} else {
		// Otherwise, look for coordinates
		lat, lon, err := api.ParseCoordinates(params)
		if err != nil {
			var invalidCoordErr api.InvalidCoordinatesError
			if errors.As(err, &invalidCoordErr) {
				return api.Error(err.Error(), http.StatusBadRequest)
			}
			return api.Error("Invalid parameters", http.StatusBadRequest)
		}

		response, err = tideService.GetCurrentTide(ctx, lat, lon, startTimeStr, endTimeStr)
		if err != nil {
			return api.Error("Error getting tide data", http.StatusInternalServerError)
		}
	}

	if err != nil {
		log.Error().Err(err).Msg("Error getting tide data")
		return api.Error("Error getting tide data", http.StatusInternalServerError)
	}

	return api.Success(response)
}

func main() {
	lambda.Start(handleRequest)
}
