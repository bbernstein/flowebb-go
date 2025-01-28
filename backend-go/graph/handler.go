package graph

import (
	"bytes"
	"context"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/aws/aws-lambda-go/events"
	"github.com/bbernstein/flowebb/backend-go/graph/generated"
	"github.com/rs/zerolog/log"
	"net/http"
)

type RequestCreator func(ctx context.Context, method, url string, body *bytes.Buffer) (*http.Request, error)

type Handler struct {
	srv            *handler.Server
	requestCreator RequestCreator
}

func defaultRequestCreator(ctx context.Context, method, url string, body *bytes.Buffer) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func NewHandler(resolver *Resolver, requestCreator RequestCreator) *Handler {
	if requestCreator == nil {
		requestCreator = defaultRequestCreator
	}

	config := generated.Config{Resolvers: resolver}
	schema := generated.NewExecutableSchema(config)

	// Create a new server with explicit configuration
	srv := handler.New(schema)

	// Configure the server with HTTP-only settings
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// Add standard middleware
	srv.Use(extension.Introspection{})
	srv.SetErrorPresenter(graphql.DefaultErrorPresenter)
	srv.SetRecoverFunc(graphql.DefaultRecover)

	return &Handler{
		srv:            srv,
		requestCreator: requestCreator,
	}
}

func (h *Handler) HandleRequest(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if event.HTTPMethod == "" {
		event.HTTPMethod = "POST"
	}
	if event.HTTPMethod != "POST" {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       "Only POST method is allowed",
		}, nil
	}

	// Create a new request with the proper URL
	req, err := http.NewRequestWithContext(ctx, event.HTTPMethod, "http://localhost/graphql", bytes.NewBufferString(event.Body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"errors": ["Failed to create request"]}`,
		}, err
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")

	// Add any headers from the event
	for key, value := range event.Headers {
		req.Header.Set(key, value)
	}

	// Create response writer to capture output
	w := &responseWriter{
		headers: make(http.Header),
		body:    &bytes.Buffer{},
		code:    http.StatusOK,
	}

	// Handle the request
	h.srv.ServeHTTP(w, req)

	return events.APIGatewayProxyResponse{
		StatusCode: w.code,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: w.body.String(),
	}, nil
}

// responseWriter implements http.ResponseWriter
type responseWriter struct {
	headers http.Header
	body    *bytes.Buffer
	code    int
}

func (w *responseWriter) Header() http.Header {
	return w.headers
}

func (w *responseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}
