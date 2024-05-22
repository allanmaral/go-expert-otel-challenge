package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/allanmaral/go-expert-otel-challenge/internal/webserver"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/cep"
)

func addRoutes(
	mux *http.ServeMux,
	logger *log.Logger,
	tracer trace.Tracer,
	orchestratorURL string,
) {
	mux.Handle("POST /api/weather", handleGetTemperature(logger, tracer, orchestratorURL))
	mux.Handle("GET /ready", handleReady())
}

func handleGetTemperature(
	logger *log.Logger,
	tracer trace.Tracer,
	orchestratorURL string,
) http.Handler {
	type request struct {
		CEP string `json:"cep"`
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		ctx, span := tracer.Start(ctx, "/api/weather")
		defer span.End()

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			_ = webserver.Encode(w, r, http.StatusBadRequest, webserver.ErrorResponse{Message: "invalid input format"})
			return
		}

		var input request
		if err := json.NewDecoder(bytes.NewBuffer(reqBody)).Decode(&input); err != nil {
			_ = webserver.Encode(w, r, http.StatusBadRequest, webserver.ErrorResponse{Message: "invalid input format"})
			logger.Printf("%s\n", err)
			return
		}

		if valid := cep.Valid(input.CEP); !valid {
			_ = webserver.Encode(w, r, http.StatusUnprocessableEntity, webserver.ErrorResponse{Message: "invalid zipcode"})
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/weather", orchestratorURL), bytes.NewBuffer(reqBody))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			logger.Printf("could not create request to the orchestrator service %s\n", err)
			return
		}

		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			logger.Printf("could not reach the orchestrator service %s\n", err)
			return
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			logger.Printf("could not read the orchestrator service response %s\n", err)
			return
		}

		w.WriteHeader(resp.StatusCode)
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.Write(bodyBytes)
	})
}

func handleReady() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	)
}
