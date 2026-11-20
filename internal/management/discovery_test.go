package management

import (
	"testing"
	"time"
)

func newDiscoveryTestInstance(
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

func registerDiscoveryTestInstance(
	test *testing.T,
	registry *Registry,
	instance ServiceInstance,
) {
	test.Helper()

	if err := registry.Register(instance); err != nil {
		test.Fatalf("Register(%q) failed: %v", instance.ID, err)
	}
}

func updateDiscoveryTestHealth(
	test *testing.T,
	healthStore *HealthStore,
	serviceID string,
	status HealthStatus,
) {
	test.Helper()

	if err := healthStore.Update(serviceID, status); err != nil {
		test.Fatalf("Update(%q) failed: %v", serviceID, err)
	}
}

func TestNewDiscovery(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	testCases := []struct {
		name         string
		registry     *Registry
		healthStore  *HealthStore
		maxHealthAge time.Duration
		wantError    bool
	}{
		{
			name:         "valid configuration",
			registry:     registry,
			healthStore:  healthStore,
			maxHealthAge: 30 * time.Second,
			wantError:    false,
		},
		{
			name:         "nil registry",
			registry:     nil,
			healthStore:  healthStore,
			maxHealthAge: 30 * time.Second,
			wantError:    true,
		},
		{
			name:         "nil health store",
			registry:     registry,
			healthStore:  nil,
			maxHealthAge: 30 * time.Second,
			wantError:    true,
		},
		{
			name:         "zero max health age",
			registry:     registry,
			healthStore:  healthStore,
			maxHealthAge: 0,
			wantError:    true,
		},
		{
			name:         "negative max health age",
			registry:     registry,
			healthStore:  healthStore,
			maxHealthAge: -time.Second,
			wantError:    true,
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			discovery, err := NewDiscovery(
				testCase.registry,
				testCase.healthStore,
				testCase.maxHealthAge,
			)

			if testCase.wantError {
				if err == nil || discovery != nil {
					test.Fatalf(
						"got discovery=%v, err=%v; want nil and error",
						discovery,
						err,
					)
				}
				return
			}

			if err != nil || discovery == nil {
				test.Fatalf(
					"got discovery=%v, err=%v; want valid discovery",
					discovery,
					err,
				)
			}
		})
	}
}

func TestDiscoveryResolve(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	instances := []ServiceInstance{
		newDiscoveryTestInstance("ocr-01", false),
		newDiscoveryTestInstance("ocr-02", true),
		newDiscoveryTestInstance("ocr-03", true),
		newDiscoveryTestInstance("ocr-04", true),
		newDiscoveryTestInstance("ocr-05", true),
	}

	for _, instance := range instances {
		registerDiscoveryTestInstance(test, registry, instance)
	}

	updateDiscoveryTestHealth(
		test, healthStore, "ocr-01", HealthStatusHealthy,
	)
	updateDiscoveryTestHealth(
		test, healthStore, "ocr-02", HealthStatusUnhealthy,
	)
	updateDiscoveryTestHealth(
		test, healthStore, "ocr-03", HealthStatusHealthy,
	)
	updateDiscoveryTestHealth(
		test, healthStore, "ocr-04", HealthStatusHealthy,
	)

	discovery, err := NewDiscovery(
		registry,
		healthStore,
		30*time.Second,
	)
	if err != nil {
		test.Fatalf("NewDiscovery failed: %v", err)
	}

	selected, err := discovery.Resolve(ServiceTypeOCR)
	if err != nil {
		test.Fatalf("Resolve failed: %v", err)
	}

	// ocr-01 is disabled, ocr-02 is unhealthy.
	// ocr-03 is the first enabled, healthy instance.
	if selected.ID != "ocr-03" {
		test.Errorf(
			"selected instance = %q, want ocr-03",
			selected.ID,
		)
	}
}

func TestDiscoveryResolveNoHealthyInstance(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	registerDiscoveryTestInstance(
		test,
		registry,
		newDiscoveryTestInstance("ocr-01", true),
	)

	discovery, err := NewDiscovery(
		registry,
		healthStore,
		30*time.Second,
	)
	if err != nil {
		test.Fatalf("NewDiscovery failed: %v", err)
	}

	// No health record exists.
	if _, err := discovery.Resolve(ServiceTypeOCR); err == nil {
		test.Error("expected error when health status is unknown")
	}

	updateDiscoveryTestHealth(
		test,
		healthStore,
		"ocr-01",
		HealthStatusUnhealthy,
	)

	if _, err := discovery.Resolve(ServiceTypeOCR); err == nil {
		test.Error("expected error when instance is unhealthy")
	}

	updateDiscoveryTestHealth(
		test,
		healthStore,
		"ocr-01",
		HealthStatusUnknown,
	)

	if _, err := discovery.Resolve(ServiceTypeOCR); err == nil {
		test.Error("expected error when health status is Unknown")
	}
}

func TestDiscoveryRejectsExpiredHealth(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	registerDiscoveryTestInstance(
		test,
		registry,
		newDiscoveryTestInstance("ocr-01", true),
	)

	updateDiscoveryTestHealth(
		test,
		healthStore,
		"ocr-01",
		HealthStatusHealthy,
	)

	// Simulate an old health record without using time.Sleep.
	healthStore.mu.Lock()
	healthStore.records["ocr-01"] = HealthRecord{
		Status:    HealthStatusHealthy,
		CheckedAt: time.Now().Add(-time.Hour),
	}
	healthStore.mu.Unlock()

	discovery, err := NewDiscovery(
		registry,
		healthStore,
		30*time.Second,
	)
	if err != nil {
		test.Fatalf("NewDiscovery failed: %v", err)
	}

	if _, err := discovery.Resolve(ServiceTypeOCR); err == nil {
		test.Fatal("expected expired health record to be rejected")
	}
}

func TestDiscoveryRejectsFutureHealth(test *testing.T) {
	registry := NewRegistry()
	healthStore := NewHealthStore()

	registerDiscoveryTestInstance(
		test,
		registry,
		newDiscoveryTestInstance("ocr-01", true),
	)

	healthStore.mu.Lock()
	healthStore.records["ocr-01"] = HealthRecord{
		Status:    HealthStatusHealthy,
		CheckedAt: time.Now().Add(time.Hour),
	}
	healthStore.mu.Unlock()

	discovery, err := NewDiscovery(
		registry,
		healthStore,
		30*time.Second,
	)
	if err != nil {
		test.Fatalf("NewDiscovery failed: %v", err)
	}

	if _, err := discovery.Resolve(ServiceTypeOCR); err == nil {
		test.Fatal("expected future health record to be rejected")
	}
}

func TestDiscoveryResolveInvalidType(test *testing.T) {
	discovery, err := NewDiscovery(
		NewRegistry(),
		NewHealthStore(),
		30*time.Second,
	)
	if err != nil {
		test.Fatalf("NewDiscovery failed: %v", err)
	}

	if _, err := discovery.Resolve(ServiceType("unknown")); err == nil {
		test.Fatal("expected unsupported service type error")
	}
}
