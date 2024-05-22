package orchestrator

import (
	"errors"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/allanmaral/go-expert-otel-challenge/internal/webserver"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/cep"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/weather"
)

func addRoutes(
	mux *http.ServeMux,
	logger *log.Logger,
	tracer trace.Tracer,
	cepLoader cep.Loader,
	weatherLoader weather.Loader,
) {
	mux.Handle("POST /api/weather", handleGetTemperature(logger, tracer, cepLoader, weatherLoader))
	mux.Handle("GET /ready", handleReady())
}

func handleGetTemperature(
	logger *log.Logger,
	tracer trace.Tracer,
	cepLoader cep.Loader,
	weatherLoader weather.Loader,
) http.Handler {
	type request struct {
		CEP string `json:"cep"`
	}

	type response struct {
		City  string  `json:"city"`
		TempC float64 `json:"temp_C"`
		TempF float64 `json:"temp_F"`
		TempK float64 `json:"temp_K"`
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), carrier)
		ctx, span := tracer.Start(ctx, "/api/weather")
		defer span.End()

		input, err := webserver.Decode[request](r)
		if err != nil {
			_ = webserver.Encode(w, r, http.StatusBadRequest, webserver.ErrorResponse{Message: "invalid input format"})
			return
		}

		cepCtx, cepSpan := tracer.Start(ctx, "cep-loader")
		cepRes, err := cepLoader.Load(cepCtx, input.CEP)
		if err != nil {
			if errors.Is(err, cep.ErrInvalidCEP) {
				_ = webserver.Encode(w, r, http.StatusUnprocessableEntity, webserver.ErrorResponse{Message: "invalid zipcode"})
			} else if errors.Is(err, cep.ErrCEPNotFound) {
				_ = webserver.Encode(w, r, http.StatusNotFound, webserver.ErrorResponse{Message: "can not find zipcode"})
			} else if errors.Is(err, cep.ErrServiceUnavailable) {
				_ = webserver.Encode(w, r, http.StatusBadGateway, webserver.ErrorResponse{Message: "cep service is unavailable, try again later"})
				logger.Printf("cep service is unavailable %s\n", err)
			} else {
				_ = webserver.Encode(w, r, http.StatusInternalServerError, webserver.ErrorResponse{Message: "internal server error"})
				logger.Printf("unhandled error while loading cep %s\n", err)
			}
			cepSpan.SetStatus(codes.Error, "cep loader failed")
			cepSpan.RecordError(err)
			cepSpan.End()
			return
		}
		cepSpan.End()

		weatherCtx, weatherSpan := tracer.Start(ctx, "weather-loader")
		weatherRes, err := weatherLoader.Load(weatherCtx, cepRes.Latitude, cepRes.Longitude)
		if err != nil {
			if errors.Is(err, weather.ErrServiceUnavailable) {
				_ = webserver.Encode(w, r, http.StatusBadGateway, webserver.ErrorResponse{Message: "weather service is unavailable, try again later"})
				logger.Printf("weather service in unavailable %s\n", err)
			} else {
				_ = webserver.Encode(w, r, http.StatusInternalServerError, webserver.ErrorResponse{Message: "internal server error"})
				logger.Printf("unhandled error while loading weather %s\n", err)
			}
			cepSpan.SetStatus(codes.Error, "weather loader failed")
			weatherSpan.RecordError(err)
			weatherSpan.End()
			return
		}
		weatherSpan.End()

		resp := response{
			City:  cepRes.City,
			TempC: weatherRes.TempC,
			TempF: weatherRes.TempF,
			TempK: weatherRes.TempK,
		}

		_ = webserver.Encode(w, r, http.StatusOK, resp)
	})
}

func handleReady() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	)
}
