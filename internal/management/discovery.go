package management

import (
	"fmt"
	"time"
)

// Discovery resolves healthy, enabled service instances.
type Discovery struct {
	registry     *Registry
	healthStore  *HealthStore
	maxHealthAge time.Duration
}

// NewDiscovery creates a service discovery component.
func NewDiscovery(
	registry *Registry,
	healthStore *HealthStore,
	maxHealthAge time.Duration,
) (*Discovery, error) {
	if registry == nil {
		return nil, fmt.Errorf("service registry cannot be nil")
	}

	if healthStore == nil {
		return nil, fmt.Errorf("health store cannot be nil")
	}

	if maxHealthAge <= 0 {
		return nil, fmt.Errorf("maximum health age must be positive")
	}

	return &Discovery{
		registry:     registry,
		healthStore:  healthStore,
		maxHealthAge: maxHealthAge,
	}, nil
}

// Resolve returns the first enabled, healthy service instance
// with a fresh health check result.
func (discovery *Discovery) Resolve(
	serviceType ServiceType,
) (ServiceInstance, error) {
	instances, err := discovery.registry.ListByType(serviceType)
	if err != nil {
		return ServiceInstance{}, fmt.Errorf(
			"discover service type %q: %w",
			serviceType,
			err,
		)
	}

	now := time.Now()

	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}

		record, exists := discovery.healthStore.Get(instance.ID)
		if !exists {
			continue
		}

		if record.Status != HealthStatusHealthy {
			continue
		}

		// Reject missing, future or expired timestamps.
		if record.CheckedAt.IsZero() ||
			record.CheckedAt.After(now) ||
			now.Sub(record.CheckedAt) > discovery.maxHealthAge {
			continue
		}

		return instance, nil
	}

	return ServiceInstance{}, fmt.Errorf(
		"no healthy instance available for service type %q",
		serviceType,
	)
}
