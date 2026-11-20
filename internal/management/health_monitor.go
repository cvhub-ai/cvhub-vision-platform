package management

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// HealthProbe checks the health of a specific service instance.
// Implementations are responsible for gRPC client and connection reuse.
type HealthProbe interface {
	Check(
		ctx context.Context,
		instance ServiceInstance,
	) (HealthStatus, error)
}

// HealthMonitor periodically checks registered service instances.
type HealthMonitor struct {
	registry     *Registry
	healthStore  *HealthStore
	probe        HealthProbe
	interval     time.Duration
	probeTimeout time.Duration
}

// NewHealthMonitor creates a health monitor.
func NewHealthMonitor(
	registry *Registry,
	healthStore *HealthStore,
	probe HealthProbe,
	interval time.Duration,
	probeTimeout time.Duration,
) (*HealthMonitor, error) {
	if registry == nil {
		return nil, fmt.Errorf("service registry cannot be nil")
	}

	if healthStore == nil {
		return nil, fmt.Errorf("health store cannot be nil")
	}

	if probe == nil {
		return nil, fmt.Errorf("health probe cannot be nil")
	}

	if interval <= 0 {
		return nil, fmt.Errorf("health check interval must be positive")
	}

	if probeTimeout <= 0 {
		return nil, fmt.Errorf("health check timeout must be positive")
	}

	return &HealthMonitor{
		registry:     registry,
		healthStore:  healthStore,
		probe:        probe,
		interval:     interval,
		probeTimeout: probeTimeout,
	}, nil
}

// CheckOnce checks all enabled service instances once.
func (monitor *HealthMonitor) CheckOnce(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	instances := monitor.registry.List()

	for _, instance := range instances {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !instance.Enabled {
			continue
		}

		// Apply an independent timeout to each health check.
		checkContext, cancel := context.WithTimeout(
			ctx,
			monitor.probeTimeout,
		)

		status, probeErr := monitor.probe.Check(
			checkContext,
			instance,
		)
		cancel()

		if err := ctx.Err(); err != nil {
			return err
		}

		// A failed probe must not stop checks for other instances.
		if probeErr != nil {
			status = HealthStatusUnknown

			slog.Warn(
				"Service health check failed",
				"service_id", instance.ID,
				"error", probeErr,
			)
		}

		if err := monitor.healthStore.Update(
			instance.ID,
			status,
		); err != nil {
			return fmt.Errorf(
				"update health status for service %q: %w",
				instance.ID,
				err,
			)
		}
	}

	return nil
}

// Run performs an initial check and continues at fixed intervals
// until the context is canceled.
func (monitor *HealthMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(monitor.interval)
	defer ticker.Stop()

	for {
		if err := monitor.CheckOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf(
				"health monitor cycle failed: %w",
				err,
			)
		}

		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
		}
	}
}
