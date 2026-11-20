package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cvhub-vision-platform/internal/management"
)

// writeServiceConfig creates an isolated YAML file for each test.
func writeServiceConfig(test *testing.T, contents string) string {
	test.Helper()

	configPath := filepath.Join(test.TempDir(), "services.yaml")

	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		test.Fatalf("write service config: %v", err)
	}

	return configPath
}

func TestLoadServices(test *testing.T) {
	configPath := writeServiceConfig(test, `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: true
    timeout: 30s
  - id: ocr-02
    type: ocr
    address: localhost:50052
    enabled: false
    timeout: 1m
`)

	instances, err := LoadServices(configPath)
	if err != nil {
		test.Fatalf("LoadServices failed: %v", err)
	}

	expected := []management.ServiceInstance{
		{
			ID:      "ocr-01",
			Type:    management.ServiceTypeOCR,
			Address: "localhost:50051",
			Enabled: true,
			Timeout: 30 * time.Second,
		},
		{
			ID:      "ocr-02",
			Type:    management.ServiceTypeOCR,
			Address: "localhost:50052",
			Enabled: false,
			Timeout: time.Minute,
		},
	}

	if len(instances) != len(expected) {
		test.Fatalf(
			"got %d services, want %d",
			len(instances),
			len(expected),
		)
	}

	for index := range expected {
		if instances[index] != expected[index] {
			test.Errorf(
				"service[%d] = %+v, want %+v",
				index,
				instances[index],
				expected[index],
			)
		}
	}
}

func TestLoadServicesInvalidConfiguration(test *testing.T) {
	validYAML := `
services:
  - id: ocr-01
    type: ocr
    address: localhost:50051
    enabled: true
    timeout: 30s
`

	testCases := []struct {
		name     string
		contents string
	}{
		{
			name:     "empty service list",
			contents: "services: []\n",
		},
		{
			name: "missing enabled",
			contents: strings.Replace(
				validYAML, "    enabled: true\n", "", 1,
			),
		},
		{
			name: "invalid timeout",
			contents: strings.Replace(
				validYAML, "timeout: 30s", "timeout: invalid", 1,
			),
		},
		{
			name:     "unknown field",
			contents: validYAML + "    adress: localhost:50051\n",
		},
		{
			name:     "malformed YAML",
			contents: "services: [\n",
		},
		{
			name:     "multiple YAML documents",
			contents: validYAML + "---\nservices: []\n",
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			configPath := writeServiceConfig(
				test,
				testCase.contents,
			)

			if _, err := LoadServices(configPath); err == nil {
				test.Fatal(
					"expected invalid service configuration to be rejected",
				)
			}
		})
	}
}

func TestLoadServicesMissingFile(test *testing.T) {
	missingPath := filepath.Join(
		test.TempDir(),
		"missing.yaml",
	)

	if _, err := LoadServices(missingPath); err == nil {
		test.Fatal("expected error for missing config file")
	}
}
