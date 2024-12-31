package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bbernstein/flowebb/backend-go/internal/api"
	"github.com/bbernstein/flowebb/backend-go/internal/cache"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	stationFinder *station.NOAAStationFinder
	setupOnce     sync.Once
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

		// Decide on output format by environment.
		env := os.Getenv("ENV") // e.g. "local", "development", "production"
		if env == "local" || env == "development" {
			// Use console-friendly output in local/dev
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		} else {
			// Use structured JSON logs in production/AWS
			// Add any additional fields or timestamp as desired
			log.Logger = zerolog.New(os.Stdout).
				With().
				Timestamp().
				Logger()
		}

		log.Info().Str("env", env).Msg("Environment")
		log.Debug().Msg("Debug logs enabled")
		log.Info().Msg("Info logs enabled")

		// Initialize dependencies
		httpClient := client.New(client.Options{
			BaseURL: "https://api.tidesandcurrents.noaa.gov",
			Timeout: 30 * time.Second,
		})
		stationCache := cache.NewStationCache()
		stationFinder = station.NewNOAAStationFinder(httpClient, stationCache)
	})
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract parameters
	params := request.QueryStringParameters

	log.Info().Msg("Handling Lambda request")

	// Check if we're looking for a specific station
	if stationID, ok := params["stationId"]; ok {
		localStation, err := stationFinder.FindStation(ctx, stationID)
		if err != nil {
			return api.Error("Station not found", http.StatusNotFound)
		}
		return api.Success(api.NewStationsResponse([]models.Station{*localStation}))
	}

	// Otherwise, look for coordinates
	lat, lon, err := api.ParseCoordinates(params)
	if err != nil {
		switch err.(type) {
		case api.InvalidCoordinatesError:
			return api.Error(err.Error(), http.StatusBadRequest)
		default:
			return api.Error("Invalid parameters", http.StatusBadRequest)
		}
	}

	// Find nearest stations
	stations, err := stationFinder.FindNearestStations(ctx, lat, lon, 5)
	if err != nil {
		return api.Error("Error finding stations", http.StatusInternalServerError)
	}

	return api.Success(api.NewStationsResponse(stations))
}

func main() {
	lambda.Start(handleRequest)
}
