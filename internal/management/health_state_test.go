package management

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHealthStoreGetUnknown(test *testing.T) {
	healthStore := NewHealthStore()

	record, exists := healthStore.Get("ocr-01")

	if exists {
		test.Fatalf(
			"expected no health record, got %+v",
			record,
		)
	}
}

func TestHealthStoreUpdate(test *testing.T) {
	healthStore := NewHealthStore()

	if err := healthStore.Update(
		"ocr-01",
		HealthStatusHealthy,
	); err != nil {
		test.Fatalf("failed to update health status: %v", err)
	}

	record, exists := healthStore.Get("ocr-01")

	if !exists {
		test.Fatal("expected health record to exist")
	}

	if record.Status != HealthStatusHealthy {
		test.Errorf(
			"got health status %q, want %q",
			record.Status,
			HealthStatusHealthy,
		)
	}

	if record.CheckedAt.IsZero() {
		test.Error("expected a valid health check timestamp")
	}
}

func TestHealthStoreStatusTransition(test *testing.T) {
	healthStore := NewHealthStore()

	statuses := []HealthStatus{
		HealthStatusUnknown,
		HealthStatusHealthy,
		HealthStatusUnhealthy,
	}

	for _, status := range statuses {
		if err := healthStore.Update("ocr-01", status); err != nil {
			test.Fatalf("failed to update status: %v", err)
		}

		record, exists := healthStore.Get("ocr-01")

		if !exists || record.Status != status {
			test.Fatalf(
				"got record %+v, exists=%t; want status %q",
				record,
				exists,
				status,
			)
		}
	}
}

func TestHealthStoreInvalidInput(test *testing.T) {
	testCases := []struct {
		name      string
		serviceID string
		status    HealthStatus
	}{
		{
			name:      "empty service ID",
			serviceID: "  ",
			status:    HealthStatusHealthy,
		},
		{
			name:      "invalid status",
			serviceID: "ocr-01",
			status:    HealthStatus("invalid"),
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			healthStore := NewHealthStore()

			err := healthStore.Update(
				testCase.serviceID,
				testCase.status,
			)

			if err == nil {
				test.Fatal("expected validation error")
			}
		})
	}
}

func TestHealthStoreConcurrentAccess(test *testing.T) {
	healthStore := NewHealthStore()

	if err := healthStore.Update(
		"ocr-base",
		HealthStatusHealthy,
	); err != nil {
		test.Fatalf("failed to initialize health record: %v", err)
	}

	const workerCount = 50

	var workers sync.WaitGroup

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workers.Add(2)

		go func(index int) {
			defer workers.Done()

			serviceID := fmt.Sprintf("ocr-%03d", index)

			if err := healthStore.Update(
				serviceID,
				HealthStatusHealthy,
			); err != nil {
				test.Errorf("concurrent Update failed: %v", err)
			}
		}(workerIndex)

		go func() {
			defer workers.Done()

			record, exists := healthStore.Get("ocr-base")

			if !exists || record.Status != HealthStatusHealthy {
				test.Error("unexpected health record during concurrent access")
			}
		}()
	}

	workers.Wait()

	for instanceIndex := 0; instanceIndex < workerCount; instanceIndex++ {
		serviceID := fmt.Sprintf("ocr-%03d", instanceIndex)

		record, exists := healthStore.Get(serviceID)

		if !exists || record.Status != HealthStatusHealthy {
			test.Errorf("missing or invalid record for %s", serviceID)
		}
	}
}

func TestHealthStoreUpdatesTimestamp(test *testing.T) {
	healthStore := NewHealthStore()

	if err := healthStore.Update(
		"ocr-01",
		HealthStatusHealthy,
	); err != nil {
		test.Fatal(err)
	}

	firstRecord, _ := healthStore.Get("ocr-01")

	time.Sleep(time.Millisecond)

	if err := healthStore.Update(
		"ocr-01",
		HealthStatusUnhealthy,
	); err != nil {
		test.Fatal(err)
	}

	secondRecord, _ := healthStore.Get("ocr-01")

	if !secondRecord.CheckedAt.After(firstRecord.CheckedAt) {
		test.Error("expected health check timestamp to be updated")
	}
}
