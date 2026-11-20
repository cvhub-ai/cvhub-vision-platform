package management

import (
	"context"
	"fmt"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// HealthStatus represents the runtime health of a service.
type HealthStatus string

const (
	HealthStatusUnknown   HealthStatus = "unknown"
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthChecker checks service health using the standard gRPC protocol.
type HealthChecker struct {
	client  healthpb.HealthClient
	timeout time.Duration
}

// NewHealthChecker creates a health checker for a gRPC service.
func NewHealthChecker(
	client healthpb.HealthClient,
	timeout time.Duration,
) (*HealthChecker, error) {
	if client == nil {
		return nil, fmt.Errorf("gRPC health client cannot be nil")
	}

	if timeout <= 0 {
		return nil, fmt.Errorf("health check timeout must be positive")
	}

	return &HealthChecker{
		client:  client,
		timeout: timeout,
	}, nil
}

// Check retrieves the overall health status of the gRPC server.
func (healthChecker *HealthChecker) Check(
	ctx context.Context,
) (HealthStatus, error) {
	requestContext, cancel := context.WithTimeout(
		ctx,
		healthChecker.timeout,
	)
	defer cancel()

	response, err := healthChecker.client.Check(
		requestContext,
		&healthpb.HealthCheckRequest{
			Service: "",
		},
	)
	if err != nil {
		return HealthStatusUnknown, fmt.Errorf(
			"gRPC health check failed: %w",
			err,
		)
	}

	if response == nil {
		return HealthStatusUnknown, fmt.Errorf(
			"gRPC health check returned an empty response",
		)
	}

	switch response.GetStatus() {
	case healthpb.HealthCheckResponse_SERVING:
		return HealthStatusHealthy, nil

	case healthpb.HealthCheckResponse_NOT_SERVING:
		return HealthStatusUnhealthy, nil

	case healthpb.HealthCheckResponse_UNKNOWN,
		healthpb.HealthCheckResponse_SERVICE_UNKNOWN:
		return HealthStatusUnknown, nil

	default:
		return HealthStatusUnknown, fmt.Errorf(
			"unexpected gRPC health status: %s",
			response.GetStatus(),
		)
	}
}
