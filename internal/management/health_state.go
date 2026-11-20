package management

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// HealthRecord stores the latest health check result for a service instance.
type HealthRecord struct {
	Status    HealthStatus
	CheckedAt time.Time
}

// HealthStore maintains runtime health records for service instances.
type HealthStore struct {
	mu      sync.RWMutex
	records map[string]HealthRecord
}

// NewHealthStore creates an empty health state store.
func NewHealthStore() *HealthStore {
	return &HealthStore{
		records: make(map[string]HealthRecord),
	}
}

// Update stores the latest health status of a service instance.
func (healthStore *HealthStore) Update(
	serviceID string,
	status HealthStatus,
) error {
	serviceID = strings.TrimSpace(serviceID)

	if serviceID == "" {
		return fmt.Errorf("service ID cannot be empty")
	}

	switch status {
	case HealthStatusUnknown,
		HealthStatusHealthy,
		HealthStatusUnhealthy:
		// Valid status.
	default:
		return fmt.Errorf("unsupported health status: %q", status)
	}

	record := HealthRecord{
		Status:    status,
		CheckedAt: time.Now().UTC(),
	}

	healthStore.mu.Lock()
	defer healthStore.mu.Unlock()

	healthStore.records[serviceID] = record

	return nil
}

// Get retrieves the latest health record of a service instance.
func (healthStore *HealthStore) Get(
	serviceID string,
) (HealthRecord, bool) {
	serviceID = strings.TrimSpace(serviceID)

	healthStore.mu.RLock()
	defer healthStore.mu.RUnlock()

	record, exists := healthStore.records[serviceID]

	return record, exists
}
