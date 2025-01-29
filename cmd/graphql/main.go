package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bbernstein/flowebb/backend-go/graph"
	"github.com/bbernstein/flowebb/backend-go/internal/station"
	"github.com/bbernstein/flowebb/backend-go/internal/tide"
	"github.com/bbernstein/flowebb/backend-go/pkg/http/client"
	"github.com/rs/zerolog/log"
	"net/http"
	"sync"
	"time"
)

var (
	handler       *graph.Handler
	setupOnce     sync.Once
	tideFactory   tide.ServiceFactory   = &tide.DefaultServiceFactory{}
	finderFactory station.FinderFactory = &station.DefaultFinderFactory{}
	initHandler                         = defaultInitHandler
)

func defaultInitHandler(ctx context.Context) (*graph.Handler, error) {
	httpClient := client.New(client.Options{
		BaseURL: "https://api.tidesandcurrents.noaa.gov",
		Timeout: 30 * time.Second,
	})

	stationFinder, err := finderFactory.NewFinder(httpClient, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing station finder: %w", err)
	}

	tideService, err := tideFactory.NewService(ctx, httpClient, stationFinder)
	if err != nil {
		return nil, fmt.Errorf("initializing tide service: %w", err)
	}

	resolver := &graph.Resolver{
		TideService:   tideService,
		StationFinder: stationFinder,
	}

	return graph.NewHandler(resolver, nil), nil
}

func handleRequest(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if handler == nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"errors": ["Handler not initialized"]}`,
		}, fmt.Errorf("handler not initialized")
	}
	return handler.HandleRequest(ctx, event)
}

func InitializeService() error { // Return error instead of fatal
	var initError error
	setupOnce.Do(func() {
		if tideFactory == nil {
			tideFactory = &tide.DefaultServiceFactory{}
		}
		if finderFactory == nil {
			finderFactory = &station.DefaultFinderFactory{}
		}
		ctx := context.Background()
		log.Debug().Msg("Initializing GraphQL service...")
		var err error
		handler, err = initHandler(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to initialize handler: %v", err)
			log.Error().Err(err).Msg("Failed to initialize handler")
			return
		}
		log.Debug().Msg("GraphQL service initialized successfully")
	})
	return initError
}

func init() {
	if err := InitializeService(); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize service")
	}
}

func main() {
	lambda.Start(handleRequest)
}
