package management

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Registry manages registered service instances.
type Registry struct {
	mu        sync.RWMutex
	instances map[string]ServiceInstance
}

// NewRegistry creates an empty service registry.
func NewRegistry() *Registry {
	return &Registry{
		instances: make(map[string]ServiceInstance),
	}
}

// Register validates and registers a service instance.
func (registry *Registry) Register(instance ServiceInstance) error {
	instance.ID = strings.TrimSpace(instance.ID)
	instance.Address = strings.TrimSpace(instance.Address)

	if instance.ID == "" {
		return fmt.Errorf("service ID cannot be empty")
	}

	if !instance.Type.IsValid() {
		return fmt.Errorf("unsupported service type: %q", instance.Type)
	}

	if instance.Address == "" {
		return fmt.Errorf("service address cannot be empty")
	}

	if instance.Timeout <= 0 {
		return fmt.Errorf("service timeout must be positive")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.instances[instance.ID]; exists {
		return fmt.Errorf("service %q is already registered", instance.ID)
	}

	registry.instances[instance.ID] = instance
	return nil
}

// Get retrieves a service instance by its ID.
func (registry *Registry) Get(serviceID string) (ServiceInstance, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	instance, exists := registry.instances[serviceID]
	return instance, exists
}

// ListByType returns all instances of the specified service type.
func (registry *Registry) ListByType(serviceType ServiceType) ([]ServiceInstance, error) {
	if !serviceType.IsValid() {
		return nil, fmt.Errorf("unsupported service type: %q", serviceType)
	}

	registry.mu.RLock()

	instances := make([]ServiceInstance, 0)
	for _, instance := range registry.instances {
		if instance.Type == serviceType {
			instances = append(instances, instance)
		}
	}

	registry.mu.RUnlock()

	slices.SortFunc(instances, func(left, right ServiceInstance) int {
		return strings.Compare(left.ID, right.ID)
	})

	return instances, nil
}

// List returns all registered service instances, sorted by ID.
func (registry *Registry) List() []ServiceInstance {
	registry.mu.RLock()

	instances := make(
		[]ServiceInstance,
		0,
		len(registry.instances),
	)

	for _, instance := range registry.instances {
		instances = append(instances, instance)
	}

	registry.mu.RUnlock()

	slices.SortFunc(instances, func(left, right ServiceInstance) int {
		return strings.Compare(left.ID, right.ID)
	})

	return instances
}
