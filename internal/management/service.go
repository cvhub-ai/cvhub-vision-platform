package management

import "time"

// ServiceType identifies the capability provided by a service.
type ServiceType string

const (
	ServiceTypeOCR ServiceType = "ocr"
)

// IsValid reports whether the service type is supported.
func (serviceType ServiceType) IsValid() bool {
	switch serviceType {
	case ServiceTypeOCR:
		return true
	default:
		return false
	}
}

// ServiceInstance represents a configured vision service instance.
type ServiceInstance struct {
	ID      string
	Type    ServiceType
	Address string
	Enabled bool
	Timeout time.Duration
}
