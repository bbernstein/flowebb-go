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
	lambdaStart = lambda.Start
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
		var err error
		var level zerolog.Level
		level, err = zerolog.ParseLevel(levelStr)
		if err != nil {
			level = zerolog.InfoLevel
		}
		zerolog.SetGlobalLevel(level)

		// Setup console logger for development
		if env := os.Getenv("ENV"); env == "local" || env == "development" {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
		}

		ctx := context.Background()
		httpClient := client.New(client.Options{
			Timeout:    10 * time.Second,
			MaxRetries: 3,
			BaseURL:    "https://api.tidesandcurrents.noaa.gov",
		})
		stationFinder, _ := station.NewNOAAStationFinder(httpClient, nil)

		tideService, err = tide.NewService(ctx, httpClient, stationFinder)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to create tide service: %v", err)
		}
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
	var lat, lon float64

	// Check if we're looking up by station ID or coordinates
	if stationID, ok := params["stationId"]; ok {
		response, err = tideService.GetCurrentTideForStation(ctx, stationID, startTimeStr, endTimeStr)
	} else if lat, lon, err = api.ParseCoordinates(params); err == nil {
		response, err = tideService.GetCurrentTide(ctx, lat, lon, startTimeStr, endTimeStr)
	} else {
		return api.Error("Missing required parameters", http.StatusBadRequest)
	}

	if err != nil {
		var noaaErr *tide.NoaaAPIError
		if errors.As(err, &noaaErr) {
			log.Error().Err(err).Msg("Error from NOAA API")
			return api.Error("Error fetching tide data from upstream service", http.StatusBadGateway)
		}
		log.Error().Err(err).Msg("Error getting tide data")
		return api.Error("Error getting tide data", http.StatusInternalServerError)
	}

	return api.Success(response)
}

func main() {
	lambdaStart(handleRequest)
}
