package orchestrator

import (
	"log"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/allanmaral/go-expert-otel-challenge/internal/webserver"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/cep"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/weather"
)

func New(
	logger *log.Logger,
	tracer trace.Tracer,
	cepLoader cep.Loader,
	weatherLoader weather.Loader,
) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, logger, tracer, cepLoader, weatherLoader)

	var handler http.Handler = mux
	handler = webserver.WithLogging(logger, handler)
	handler = webserver.WithRequestID(handler)

	return handler
}
