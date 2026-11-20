package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cvhub-vision-platform/internal/clients/health"
	"cvhub-vision-platform/internal/config"
	"cvhub-vision-platform/internal/management"
)

const (
	defaultHealthCheckInterval = 10 * time.Second
	defaultHealthCheckTimeout  = 3 * time.Second
	defaultMaxHealthAge        = 30 * time.Second
)

// application holds the shared components of the Vision Platform.
type application struct {
	registry      *management.Registry
	healthStore   *management.HealthStore
	discovery     *management.Discovery
	healthProbe   *health.GRPCHealthProbe
	healthMonitor *management.HealthMonitor
}

// newApplication initializes the Vision Platform components.
func newApplication(configPath string) (*application, error) {
	// Load configured service instances.
	instances, err := config.LoadServices(configPath)
	if err != nil {
		return nil, fmt.Errorf("load service configuration: %w", err)
	}

	// Initialize the service registry.
	registry := management.NewRegistry()

	for _, instance := range instances {
		if err := registry.Register(instance); err != nil {
			return nil, fmt.Errorf(
				"register service %q: %w",
				instance.ID,
				err,
			)
		}
	}

	// Initialize shared runtime health state.
	healthStore := management.NewHealthStore()

	// Initialize service discovery.
	discovery, err := management.NewDiscovery(
		registry,
		healthStore,
		defaultMaxHealthAge,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize service discovery: %w", err)
	}

	// Initialize the grpc health probe.
	healthProbe, err := health.NewGRPCHealthProbe(
		defaultHealthCheckTimeout,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize gRPC health probe: %w", err)
	}

	// Initialize the health monitor.
	healthMonitor, err := management.NewHealthMonitor(
		registry,
		healthStore,
		healthProbe,
		defaultHealthCheckInterval,
		defaultHealthCheckTimeout,
	)
	if err != nil {
		_ = healthProbe.Close()
		return nil, fmt.Errorf("initialize health monitor: %w", err)
	}

	return &application{
		registry:      registry,
		healthStore:   healthStore,
		discovery:     discovery,
		healthProbe:   healthProbe,
		healthMonitor: healthMonitor,
	}, nil
}

func (app *application) Run(ctx context.Context) error {
	slog.Info("Starting Vision Platform health monitor")

	if err := app.healthMonitor.Run(ctx); err != nil {
		return fmt.Errorf("health monitor stopped unexpectedly: %w", err)
	}

	return nil
}

func (app *application) Close() error {
	if err := app.healthProbe.Close(); err != nil {
		return fmt.Errorf("close gRPC health probe: %w", err)
	}

	return nil
}

func run(ctx context.Context, configPath string) (runError error) {
	app, err := newApplication(configPath)
	if err != nil {
		return fmt.Errorf("initialize Vision Platform: %w", err)
	}

	// Always release application resources before returning.
	defer func() {
		if err := app.Close(); err != nil {
			runError = errors.Join(runError, err)
		}
	}()

	slog.Info("Vision Platform initialized")

	if err := app.Run(ctx); err != nil {
		return err
	}

	slog.Info("Vision Platform stopped")

	return nil
}

func main() {
	configPath := flag.String(
		"config",
		"configs/services.yaml",
		"Path to the service configuration file",
	)
	flag.Parse()

	// Cancel the application context when a shutdown signal arrives.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := run(ctx, *configPath); err != nil {
		slog.Error(
			"Vision Platform failed",
			"error", err,
		)
		os.Exit(1)
	}
}
