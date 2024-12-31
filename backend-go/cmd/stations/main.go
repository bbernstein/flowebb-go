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
	"strconv"
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

		// Setup console logger for development
		if env := os.Getenv("ENV"); env == "local" || env == "development" {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		}

		// Initialize HTTP client
		httpClient := client.New(client.Options{
			BaseURL: "https://api.tidesandcurrents.noaa.gov",
			Timeout: 30 * time.Second,
		})

		// Initialize station finder with cache
		stationCache := cache.NewStationCache()
		stationFinder = station.NewNOAAStationFinder(httpClient, stationCache)
	})
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	params := request.QueryStringParameters
	log.Info().Msg("Handling stations request")

	// Check if we're looking up by station ID or coordinates
	if stationID, ok := params["stationId"]; ok {
		stationLocal, err := stationFinder.FindStation(ctx, stationID)
		if err != nil {
			log.Error().Err(err).Msg("Error finding station")
			return api.Error("Station not found", http.StatusNotFound)
		}
		return api.Success(api.NewStationsResponse([]models.Station{*stationLocal}))
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

	// Default limit to 5 if not specified
	limit := 5
	if limitStr, ok := params["limit"]; ok {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	stations, err := stationFinder.FindNearestStations(ctx, lat, lon, limit)
	if err != nil {
		log.Error().Err(err).Msg("Error finding stations")
		return api.Error("Error finding stations", http.StatusInternalServerError)
	}

	return api.Success(api.NewStationsResponse(stations))
}

func main() {
	lambda.Start(handleRequest)
}
