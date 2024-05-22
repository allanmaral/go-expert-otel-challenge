package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/allanmaral/go-expert-otel-challenge/internal/opentelemetry"
	"github.com/allanmaral/go-expert-otel-challenge/internal/orchestrator"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/cep"
	"github.com/allanmaral/go-expert-otel-challenge/pkg/weather"
)

func run(
	ctx context.Context,
	getEnv func(key string) string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	providerShutdown, err := opentelemetry.InitProvider(ctx, "orchestrator-service", getEnv("OTEL_EXPORTER_URL"))
	if err != nil {
		return fmt.Errorf("failed to initialize the OTEL provider: %w", err)
	}

	logger := log.New(stdout, "ORCHESTRATOR: ", log.LstdFlags)
	tracer := otel.Tracer("orchestrator-service")
	cepLoader := cep.NewAwesomeAPILoader()
	weatherLoader := weather.NewWeatherAPILoader(getEnv("WEATHER_APIKEY"))

	srv := orchestrator.New(logger, tracer, cepLoader, weatherLoader)
	httpServer := &http.Server{
		Addr:    net.JoinHostPort("0.0.0.0", "8181"),
		Handler: srv,
	}

	go func() {
		logger.Printf("listening on %s\n", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			_, _ = fmt.Fprintf(stderr, "error listening and serving: %s\n", err)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logger.Printf("shutting http server down...\n")
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_, _ = fmt.Fprintf(stderr, "error shutting http server down: %s\n", err)
		}
		if err := providerShutdown(shutdownCtx); err != nil {
			_, _ = fmt.Fprintf(stderr, "error shutting OTEL provider down: %s\n", err)
		}
	}()

	wg.Wait()
	return nil
}

func main() {
	ctx := context.Background()

	if err := run(ctx, os.Getenv, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
