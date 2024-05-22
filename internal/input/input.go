package input

import (
	"log"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/allanmaral/go-expert-otel-challenge/internal/webserver"
)

func New(
	logger *log.Logger,
	tracer trace.Tracer,
	orchestratorURL string,
) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, logger, tracer, orchestratorURL)

	var handler http.Handler = mux
	handler = webserver.WithLogging(logger, handler)
	handler = webserver.WithRequestID(handler)

	return handler
}
