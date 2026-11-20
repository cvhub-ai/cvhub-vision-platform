package management

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// mockHealthProbe implements HealthProbe for unit testing.
type mockHealthProbe struct {
	checkFunc func(
		ctx context.Context,
		instance ServiceInstance,
	) (HealthStatus, error)
}

func (probe *mockHealthProbe) Check(
	ctx context.Context,
	instance ServiceInstance,
) (HealthStatus, error) {
	return probe.checkFunc(ctx, instance)
}

// createMonitorTestInstance creates a test service instance.
func createMonitorTestInstance(
	serviceID string,
	enabled bool,
) ServiceInstance {
	return ServiceInstance{
		ID:      serviceID,
		Type:    ServiceTypeOCR,
		Address: "localhost:50051",
		Enabled: enabled,
		Timeout: 30 * time.Second,
	}
}

// createMonitorTestRegistry registers the given service instances.
func createMonitorTestRegistry(
	test *testing.T,
	instances ...ServiceInstance,
) *Registry {
	test.Helper()

	registry := NewRegistry()

	for _, instance := range instances {
		if err := registry.Register(instance); err != nil {
			test.Fatalf(
				"failed to register service %q: %v",
				instance.ID,
				err,
			)
		}
	}

	return registry
}

// TestNewHealthMonitor verifies initialization and validation.
func TestNewHealthMonitor(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	probe := &mockHealthProbe{
		checkFunc: func(
			context.Context,
			ServiceInstance,
		) (HealthStatus, error) {
			return HealthStatusHealthy, nil
		},
	}

	test.Run("valid configuration", func(test *testing.T) {
		monitor, err := NewHealthMonitor(
			registry,
			healthStore,
			probe,
			10*time.Second,
			3*time.Second,
		)

		if err != nil {
			test.Fatalf("NewHealthMonitor failed: %v", err)
		}

		if monitor == nil {
			test.Fatal("expected initialized health monitor")
		}
	})

	testCases := []struct {
		name         string
		registry     *Registry
		healthStore  *HealthStore
		probe        HealthProbe
		interval     time.Duration
		probeTimeout time.Duration
	}{
		{
			name:         "nil registry",
			registry:     nil,
			healthStore:  healthStore,
			probe:        probe,
			interval:     time.Second,
			probeTimeout: time.Second,
		},
		{
			name:         "nil health store",
			registry:     registry,
			healthStore:  nil,
			probe:        probe,
			interval:     time.Second,
			probeTimeout: time.Second,
		},
		{
			name:         "nil probe",
			registry:     registry,
			healthStore:  healthStore,
			probe:        nil,
			interval:     time.Second,
			probeTimeout: time.Second,
		},
		{
			name:         "zero interval",
			registry:     registry,
			healthStore:  healthStore,
			probe:        probe,
			interval:     0,
			probeTimeout: time.Second,
		},
		{
			name:         "negative interval",
			registry:     registry,
			healthStore:  healthStore,
			probe:        probe,
			interval:     -time.Second,
			probeTimeout: time.Second,
		},
		{
			name:         "zero probe timeout",
			registry:     registry,
			healthStore:  healthStore,
			probe:        probe,
			interval:     time.Second,
			probeTimeout: 0,
		},
		{
			name:         "negative probe timeout",
			registry:     registry,
			healthStore:  healthStore,
			probe:        probe,
			interval:     time.Second,
			probeTimeout: -time.Second,
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			monitor, err := NewHealthMonitor(
				testCase.registry,
				testCase.healthStore,
				testCase.probe,
				testCase.interval,
				testCase.probeTimeout,
			)

			if err == nil {
				test.Fatal("expected configuration validation error")
			}

			if monitor != nil {
				test.Fatal("expected nil monitor on initialization failure")
			}
		})
	}
}

// TestHealthMonitorCheckOnce verifies health checking and state updates.
func TestHealthMonitorCheckOnce(test *testing.T) {
	registry := createMonitorTestRegistry(
		test,
		createMonitorTestInstance("ocr-03", true),
		createMonitorTestInstance("ocr-01", true),
		createMonitorTestInstance("ocr-02", false),
		createMonitorTestInstance("ocr-04", true),
	)

	healthStore := NewHealthStore()
	calledServices := make([]string, 0)

	probe := &mockHealthProbe{
		checkFunc: func(
			ctx context.Context,
			instance ServiceInstance,
		) (HealthStatus, error) {
			calledServices = append(calledServices, instance.ID)

			if _, exists := ctx.Deadline(); !exists {
				test.Error("expected health check context to have a deadline")
			}

			switch instance.ID {
			case "ocr-01":
				return HealthStatusHealthy, nil

			case "ocr-03":
				return HealthStatusUnhealthy, nil

			case "ocr-04":
				return HealthStatusUnknown, fmt.Errorf("connection failed")

			default:
				test.Errorf("unexpected service: %s", instance.ID)
				return HealthStatusUnknown, nil
			}
		},
	}

	monitor, err := NewHealthMonitor(
		registry,
		healthStore,
		probe,
		10*time.Second,
		3*time.Second,
	)
	if err != nil {
		test.Fatalf("NewHealthMonitor failed: %v", err)
	}

	if err := monitor.CheckOnce(context.Background()); err != nil {
		test.Fatalf("CheckOnce failed: %v", err)
	}

	// Verify enabled instances were checked in sorted order.
	expectedCalls := []string{
		"ocr-01",
		"ocr-03",
		"ocr-04",
	}

	if !slices.Equal(calledServices, expectedCalls) {
		test.Errorf(
			"probe calls = %v, want %v",
			calledServices,
			expectedCalls,
		)
	}

	// Verify stored health statuses.
	expectedStatuses := map[string]HealthStatus{
		"ocr-01": HealthStatusHealthy,
		"ocr-03": HealthStatusUnhealthy,
		"ocr-04": HealthStatusUnknown,
	}

	for serviceID, expectedStatus := range expectedStatuses {
		record, exists := healthStore.Get(serviceID)

		if !exists {
			test.Errorf("missing health record for %s", serviceID)
			continue
		}

		if record.Status != expectedStatus {
			test.Errorf(
				"service %s: got %q, want %q",
				serviceID,
				record.Status,
				expectedStatus,
			)
		}

		if record.CheckedAt.IsZero() {
			test.Errorf(
				"missing health check timestamp for %s",
				serviceID,
			)
		}
	}

	// Disabled services must not be checked.
	if _, exists := healthStore.Get("ocr-02"); exists {
		test.Error("disabled service must not have a health record")
	}
}

// TestHealthMonitorProbeTimeout verifies per-instance timeout handling.
func TestHealthMonitorProbeTimeout(test *testing.T) {
	registry := createMonitorTestRegistry(
		test,
		createMonitorTestInstance("ocr-01", true),
		createMonitorTestInstance("ocr-02", true),
	)

	healthStore := NewHealthStore()
	calledServices := make([]string, 0)

	probe := &mockHealthProbe{
		checkFunc: func(
			ctx context.Context,
			instance ServiceInstance,
		) (HealthStatus, error) {
			calledServices = append(calledServices, instance.ID)

			if instance.ID == "ocr-01" {
				// Simulate a probe that waits until its timeout.
				<-ctx.Done()
				return HealthStatusUnknown, ctx.Err()
			}

			return HealthStatusHealthy, nil
		},
	}

	monitor, err := NewHealthMonitor(
		registry,
		healthStore,
		probe,
		time.Second,
		10*time.Millisecond,
	)
	if err != nil {
		test.Fatalf("NewHealthMonitor failed: %v", err)
	}

	if err := monitor.CheckOnce(context.Background()); err != nil {
		test.Fatalf("CheckOnce failed: %v", err)
	}

	firstRecord, exists := healthStore.Get("ocr-01")
	if !exists || firstRecord.Status != HealthStatusUnknown {
		test.Errorf(
			"expected timed-out service to be Unknown, got %+v",
			firstRecord,
		)
	}

	secondRecord, exists := healthStore.Get("ocr-02")
	if !exists || secondRecord.Status != HealthStatusHealthy {
		test.Errorf(
			"expected second service to be Healthy, got %+v",
			secondRecord,
		)
	}

	if !slices.Equal(calledServices, []string{"ocr-01", "ocr-02"}) {
		test.Errorf(
			"unexpected probe calls: %v",
			calledServices,
		)
	}
}

// TestHealthMonitorInvalidProbeStatus verifies invalid status rejection.
func TestHealthMonitorInvalidProbeStatus(test *testing.T) {
	registry := createMonitorTestRegistry(
		test,
		createMonitorTestInstance("ocr-01", true),
	)

	healthStore := NewHealthStore()

	probe := &mockHealthProbe{
		checkFunc: func(
			context.Context,
			ServiceInstance,
		) (HealthStatus, error) {
			return HealthStatus("invalid"), nil
		},
	}

	monitor, err := NewHealthMonitor(
		registry,
		healthStore,
		probe,
		time.Second,
		time.Second,
	)
	if err != nil {
		test.Fatalf("NewHealthMonitor failed: %v", err)
	}

	if err := monitor.CheckOnce(context.Background()); err == nil {
		test.Fatal("expected invalid health status to return an error")
	}

	if _, exists := healthStore.Get("ocr-01"); exists {
		test.Error("invalid health status must not be stored")
	}
}

// TestHealthMonitorCanceledContext verifies cancellation handling.
func TestHealthMonitorCanceledContext(test *testing.T) {
	registry := createMonitorTestRegistry(
		test,
		createMonitorTestInstance("ocr-01", true),
	)

	healthStore := NewHealthStore()

	probe := &mockHealthProbe{
		checkFunc: func(
			context.Context,
			ServiceInstance,
		) (HealthStatus, error) {
			test.Error("probe must not execute for a canceled context")
			return HealthStatusUnknown, nil
		},
	}

	monitor, err := NewHealthMonitor(
		registry,
		healthStore,
		probe,
		time.Second,
		time.Second,
	)
	if err != nil {
		test.Fatalf("NewHealthMonitor failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// CheckOnce should return the context cancellation error.
	if err := monitor.CheckOnce(ctx); !errors.Is(err, context.Canceled) {
		test.Fatalf(
			"CheckOnce error = %v, want context.Canceled",
			err,
		)
	}

	// Run should exit gracefully on cancellation.
	if err := monitor.Run(ctx); err != nil {
		test.Fatalf("Run should exit gracefully: %v", err)
	}
}

// TestHealthMonitorRun verifies periodic execution and graceful shutdown.
func TestHealthMonitorRun(test *testing.T) {
	registry := createMonitorTestRegistry(
		test,
		createMonitorTestInstance("ocr-01", true),
	)

	healthStore := NewHealthStore()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	callCount := 0

	probe := &mockHealthProbe{
		checkFunc: func(
			context.Context,
			ServiceInstance,
		) (HealthStatus, error) {
			callCount++

			// Stop after the second scheduled check.
			if callCount == 2 {
				cancel()
			}

			return HealthStatusHealthy, nil
		},
	}

	monitor, err := NewHealthMonitor(
		registry,
		healthStore,
		probe,
		10*time.Millisecond,
		time.Second,
	)
	if err != nil {
		test.Fatalf("NewHealthMonitor failed: %v", err)
	}

	if err := monitor.Run(ctx); err != nil {
		test.Fatalf("HealthMonitor.Run failed: %v", err)
	}

	if callCount != 2 {
		test.Errorf(
			"probe called %d times, want 2",
			callCount,
		)
	}

	// A completed check should have produced a record.
	// Note: cancellation during a check may prevent its
	// result from being stored, so only the first record
	// is required here.
	record, exists := healthStore.Get("ocr-01")
	if !exists || record.Status != HealthStatusHealthy {
		test.Errorf(
			"expected a Healthy record from the first check, got %+v",
			record,
		)
	}
}
