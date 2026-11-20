package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cvhub-vision-platform/internal/management"
)

func writeApplicationConfig(test *testing.T, contents string) string {
	test.Helper()

	configPath := filepath.Join(test.TempDir(), "services.yaml")

	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		test.Fatalf("write application config: %v", err)
	}

	return configPath
}

func TestNewApplication(test *testing.T) {
	configPath := writeApplicationConfig(test, `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: false
    timeout: 30s
  - id: ocr-02
    type: ocr
    address: localhost:50052
    enabled: true
    timeout: 45s
`)

	app, err := newApplication(configPath)
	if err != nil {
		test.Fatalf("newApplication failed: %v", err)
	}

	if app == nil || app.registry == nil || app.discovery == nil {
		test.Fatal(
			"expected initialized application, registry and discovery",
		)
	}

	first, exists := app.registry.Get("ocr-01")
	if !exists || first.Enabled {
		test.Fatalf(
			"unexpected first service: %+v, exists=%t",
			first,
			exists,
		)
	}

	// Before the first health check, no service is available.
	if _, err := app.discovery.Resolve(management.ServiceTypeOCR); err == nil {
		test.Fatal("expected discovery to reject unchecked instances")
	}

	// Simulate a successful health check for the enabled instance.
	if err := app.healthStore.Update(
		"ocr-02",
		management.HealthStatusHealthy,
	); err != nil {
		test.Fatalf("failed to update health status: %v", err)
	}

	// Discovery should now select the healthy instance.
	selected, err := app.discovery.Resolve(management.ServiceTypeOCR)
	if err != nil {
		test.Fatalf("Resolve failed: %v", err)
	}

	if selected.ID != "ocr-02" ||
		selected.Address != "localhost:50052" ||
		selected.Timeout != 45*time.Second {
		test.Errorf(
			"unexpected selected OCR instance: %+v",
			selected,
		)
	}
}

func TestNewApplicationRejectsInvalidConfiguration(test *testing.T) {
	testCases := []struct {
		name     string
		contents string
	}{
		{
			name: "duplicate service IDs",
			contents: `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: true
    timeout: 30s
  - id: ocr-01
    type: ocr
    address: localhost:50052
    enabled: true
    timeout: 30s
`,
		},
		{
			name: "unsupported service type",
			contents: `
services:
  - id: invalid-01
    type: unknown
    address: localhost:50051
    enabled: true
    timeout: 30s
`,
		},
		{
			name: "nonpositive timeout",
			contents: `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: true
    timeout: 0s
`,
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			configPath := writeApplicationConfig(
				test,
				testCase.contents,
			)

			app, err := newApplication(configPath)
			if err == nil || app != nil {
				test.Fatalf(
					"newApplication() = %v, %v; want nil and error",
					app,
					err,
				)
			}
		})
	}
}

func TestNewApplicationAllowsDisabledServices(test *testing.T) {
	configPath := writeApplicationConfig(test, `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: false
    timeout: 30s
`)

	app, err := newApplication(configPath)
	if err != nil {
		test.Fatalf(
			"disabled services should not prevent initialization: %v",
			err,
		)
	}

	if _, err := app.discovery.Resolve(management.ServiceTypeOCR); err == nil {
		test.Fatal(
			"expected discovery to reject a disabled-only service type",
		)
	}
}

func TestNewApplicationMissingFile(test *testing.T) {
	configPath := filepath.Join(test.TempDir(), "missing.yaml")

	app, err := newApplication(configPath)
	if err == nil || app != nil {
		test.Fatalf(
			"newApplication() = %v, %v; want nil and error",
			app,
			err,
		)
	}
}

// TestApplicationLifecycle verifies startup and graceful shutdown.
func TestApplicationLifecycle(test *testing.T) {
	configPath := writeApplicationConfig(test, `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: false
    timeout: 30s
`)

	app, err := newApplication(configPath)
	if err != nil {
		test.Fatalf("newApplication failed: %v", err)
	}

	if app.healthMonitor == nil || app.healthProbe == nil {
		test.Fatal("application health components were not initialized")
	}

	// Simulate an application shutdown request.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := app.Run(ctx); err != nil {
		test.Fatalf("application shutdown failed: %v", err)
	}

	if err := app.Close(); err != nil {
		test.Fatalf("application cleanup failed: %v", err)
	}

	// Closing an already closed probe should be safe.
	if err := app.Close(); err != nil {
		test.Fatalf("repeated cleanup failed: %v", err)
	}
}
