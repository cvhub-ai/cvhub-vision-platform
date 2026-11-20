package management

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func validTestInstance(id string) ServiceInstance {
	return ServiceInstance{
		ID:      id,
		Type:    ServiceTypeOCR,
		Address: "localhost:50051",
		Enabled: true,
		Timeout: 30 * time.Second,
	}
}

func registerTestInstance(
	test *testing.T,
	registry *Registry,
	instance ServiceInstance,
) {
	test.Helper()

	if err := registry.Register(instance); err != nil {
		test.Fatalf("Register(%q) failed: %v", instance.ID, err)
	}
}

func TestRegistryRegister(test *testing.T) {
	registry := NewRegistry()
	instance := validTestInstance("ocr-01")

	registerTestInstance(test, registry, instance)

	if err := registry.Register(instance); err == nil {
		test.Fatal("expected duplicate service ID to be rejected")
	}
}

func TestRegistryRegisterValidation(test *testing.T) {
	testCases := []struct {
		name   string
		change func(*ServiceInstance)
	}{
		{
			name: "empty ID",
			change: func(instance *ServiceInstance) {
				instance.ID = "   "
			},
		},
		{
			name: "invalid type",
			change: func(instance *ServiceInstance) {
				instance.Type = "unknown"
			},
		},
		{
			name: "empty address",
			change: func(instance *ServiceInstance) {
				instance.Address = "  "
			},
		},
		{
			name: "zero timeout",
			change: func(instance *ServiceInstance) {
				instance.Timeout = 0
			},
		},
		{
			name: "negative timeout",
			change: func(instance *ServiceInstance) {
				instance.Timeout = -time.Second
			},
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			registry := NewRegistry()
			instance := validTestInstance("ocr-01")
			testCase.change(&instance)

			if err := registry.Register(instance); err == nil {
				test.Fatal("expected invalid instance to be rejected")
			}

			if len(registry.instances) != 0 {
				test.Fatal("invalid instance must not be stored")
			}
		})
	}
}

func TestRegistryRegisterNormalizesFields(test *testing.T) {
	registry := NewRegistry()

	instance := validTestInstance("  ocr-01  ")
	instance.Address = "  localhost:50051  "

	registerTestInstance(test, registry, instance)

	stored, exists := registry.Get("ocr-01")
	if !exists ||
		stored.ID != "ocr-01" ||
		stored.Address != "localhost:50051" {
		test.Fatalf(
			"unexpected normalized service: %+v, exists=%t",
			stored,
			exists,
		)
	}
}

func TestRegistryGet(test *testing.T) {
	registry := NewRegistry()
	expected := validTestInstance("ocr-01")

	registerTestInstance(test, registry, expected)

	actual, exists := registry.Get(expected.ID)
	if !exists {
		test.Fatal("registered service not found")
	}

	if actual != expected {
		test.Errorf("Get() = %+v, want %+v", actual, expected)
	}

	if _, exists := registry.Get("missing"); exists {
		test.Error("unregistered service should not be found")
	}
}

func TestRegistryListByType(test *testing.T) {
	registry := NewRegistry()

	first := validTestInstance("ocr-02")
	first.Enabled = false

	registerTestInstance(test, registry, first)
	registerTestInstance(test, registry, validTestInstance("ocr-01"))

	instances, err := registry.ListByType(ServiceTypeOCR)
	if err != nil {
		test.Fatalf("ListByType failed: %v", err)
	}

	if len(instances) != 2 {
		test.Fatalf(
			"ListByType returned %d instances, want 2",
			len(instances),
		)
	}

	if instances[0].ID != "ocr-01" ||
		instances[1].ID != "ocr-02" {
		test.Errorf("instances not sorted by ID: %+v", instances)
	}

	if instances[1].Enabled {
		test.Error(
			"ListByType must also return disabled registered instances",
		)
	}

	if _, err := registry.ListByType(ServiceType("unknown")); err == nil {
		test.Error("expected invalid service type to return an error")
	}
}

func TestRegistryConcurrentAccess(test *testing.T) {
	registry := NewRegistry()
	registerTestInstance(test, registry, validTestInstance("ocr-base"))

	const workerCount = 50

	var workers sync.WaitGroup

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workers.Add(2)

		go func(index int) {
			defer workers.Done()

			instance := validTestInstance(
				fmt.Sprintf("ocr-%03d", index),
			)

			if err := registry.Register(instance); err != nil {
				test.Errorf("concurrent Register failed: %v", err)
			}
		}(workerIndex)

		go func() {
			defer workers.Done()

			if _, exists := registry.Get("ocr-base"); !exists {
				test.Error(
					"pre-registered service disappeared during concurrent access",
				)
			}

			if _, err := registry.ListByType(ServiceTypeOCR); err != nil {
				test.Errorf("concurrent ListByType failed: %v", err)
			}
		}()
	}

	workers.Wait()

	instances, err := registry.ListByType(ServiceTypeOCR)
	if err != nil {
		test.Fatalf("ListByType failed: %v", err)
	}

	if len(instances) != workerCount+1 {
		test.Fatalf(
			"got %d services after concurrent registration, want %d",
			len(instances),
			workerCount+1,
		)
	}

	for instanceIndex := 1; instanceIndex < len(instances); instanceIndex++ {
		if strings.Compare(
			instances[instanceIndex-1].ID,
			instances[instanceIndex].ID,
		) >= 0 {
			test.Fatalf("instances not strictly sorted: %+v", instances)
		}
	}
}
