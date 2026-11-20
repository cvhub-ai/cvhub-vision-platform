package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"cvhub-vision-platform/internal/management"

	"gopkg.in/yaml.v3"
)

// servicesConfig represents the root YAML configuration.
type servicesConfig struct {
	Services []serviceEntry `yaml:"services"`
}

// serviceEntry represents a service definition in YAML.
type serviceEntry struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Address string `yaml:"address"`
	Enabled *bool  `yaml:"enabled"`
	Timeout string `yaml:"timeout"`
}

// LoadServices loads service instances from a YAML configuration file.
func LoadServices(configPath string) ([]management.ServiceInstance, error) {
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	// Decode YAML configuration.
	decoder := yaml.NewDecoder(configFile)
	decoder.KnownFields(true)

	var configuration servicesConfig

	if err := decoder.Decode(&configuration); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Reject additional YAML documents.
	var extraDocument any
	if err := decoder.Decode(&extraDocument); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("configuration must contain only one YAML document")
		}
		return nil, fmt.Errorf("failed to decode additional YAML document: %w", err)
	}

	if len(configuration.Services) == 0 {
		return nil, fmt.Errorf("no services defined in configuration")
	}

	instances := make(
		[]management.ServiceInstance,
		0,
		len(configuration.Services),
	)

	for _, entry := range configuration.Services {
		if entry.Enabled == nil {
			return nil, fmt.Errorf(
				"service %q: enabled field is required",
				entry.ID,
			)
		}

		// Convert duration string into time.Duration.
		timeout, err := time.ParseDuration(entry.Timeout)
		if err != nil {
			return nil, fmt.Errorf(
				"service %q: invalid timeout %q: %w",
				entry.ID,
				entry.Timeout,
				err,
			)
		}

		instance := management.ServiceInstance{
			ID:      entry.ID,
			Type:    management.ServiceType(entry.Type),
			Address: entry.Address,
			Enabled: *entry.Enabled,
			Timeout: timeout,
		}

		instances = append(instances, instance)
	}

	return instances, nil
}
