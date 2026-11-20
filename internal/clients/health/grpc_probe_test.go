package health

import (
	"context"
	"net"
	"testing"
	"time"

	"cvhub-vision-platform/internal/management"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// TestNewGRPCHealthProbe verifies initialization and configuration validation.
func TestNewGRPCHealthProbe(test *testing.T) {
	testCases := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{"valid timeout", time.Second, false},
		{"zero timeout", 0, true},
		{"negative timeout", -time.Second, true},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			probe, err := NewGRPCHealthProbe(testCase.timeout)

			if testCase.wantErr {
				if err == nil || probe != nil {
					test.Fatalf(
						"expected initialization failure, got probe=%v, err=%v",
						probe,
						err,
					)
				}
				return
			}

			if err != nil || probe == nil {
				test.Fatalf(
					"expected initialized probe, got probe=%v, err=%v",
					probe,
					err,
				)
			}

			if err := probe.Close(); err != nil {
				test.Fatalf("failed to close probe: %v", err)
			}
		})
	}
}

// TestGRPCHealthProbeCheck verifies real gRPC health checks,
// connection reuse, and connection cleanup.
func TestGRPCHealthProbeCheck(test *testing.T) {
	// Create a local test listener on an automatically assigned port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatalf("failed to create test listener: %v", err)
	}

	// Create a temporary gRPC server with the standard Health Service.
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()

	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		_ = grpcServer.Serve(listener)
	}()

	test.Cleanup(func() {
		grpcServer.Stop()
	})

	// Create the health probe.
	probe, err := NewGRPCHealthProbe(3 * time.Second)
	if err != nil {
		test.Fatalf("failed to create health probe: %v", err)
	}

	test.Cleanup(func() {
		if err := probe.Close(); err != nil {
			test.Errorf("failed to close health probe: %v", err)
		}
	})

	instance := management.ServiceInstance{
		ID:      "ocr-01",
		Type:    management.ServiceTypeOCR,
		Address: listener.Addr().String(),
		Enabled: true,
		Timeout: 30 * time.Second,
	}

	// Test 1: The server reports SERVING.
	healthServer.SetServingStatus(
		"",
		healthpb.HealthCheckResponse_SERVING,
	)

	status, err := probe.Check(context.Background(), instance)
	if err != nil {
		test.Fatalf("health check failed: %v", err)
	}

	if status != management.HealthStatusHealthy {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusHealthy,
		)
	}

	// Test 2: The server reports NOT_SERVING.
	healthServer.SetServingStatus(
		"",
		healthpb.HealthCheckResponse_NOT_SERVING,
	)

	status, err = probe.Check(context.Background(), instance)
	if err != nil {
		test.Fatalf("health check failed: %v", err)
	}

	if status != management.HealthStatusUnhealthy {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusUnhealthy,
		)
	}

	// Test 3: Verify connection reuse.
	probe.mu.Lock()
	connectionCount := len(probe.connections)
	probe.mu.Unlock()

	if connectionCount != 1 {
		test.Errorf(
			"cached connections = %d, want 1",
			connectionCount,
		)
	}

	// Test 4: Close the probe.
	if err := probe.Close(); err != nil {
		test.Fatalf("failed to close health probe: %v", err)
	}

	// Test 5: Closed probes must reject new checks.
	status, err = probe.Check(context.Background(), instance)

	if err == nil {
		test.Fatal("expected error after closing health probe")
	}

	if status != management.HealthStatusUnknown {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusUnknown,
		)
	}
}

// TestGRPCHealthProbeInvalidAddress verifies address validation.
func TestGRPCHealthProbeInvalidAddress(test *testing.T) {
	probe, err := NewGRPCHealthProbe(time.Second)
	if err != nil {
		test.Fatalf("failed to create health probe: %v", err)
	}
	defer probe.Close()

	instance := management.ServiceInstance{
		ID:      "ocr-01",
		Address: "  ",
	}

	status, err := probe.Check(context.Background(), instance)

	if err == nil {
		test.Fatal("expected error for an empty service address")
	}

	if status != management.HealthStatusUnknown {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusUnknown,
		)
	}
}

// TestGRPCHealthProbeCanceledContext verifies cancellation handling.
func TestGRPCHealthProbeCanceledContext(test *testing.T) {
	probe, err := NewGRPCHealthProbe(time.Second)
	if err != nil {
		test.Fatalf("failed to create health probe: %v", err)
	}
	defer probe.Close()

	requestContext, cancel := context.WithCancel(context.Background())
	cancel()

	instance := management.ServiceInstance{
		ID:      "ocr-01",
		Address: "localhost:50051",
	}

	status, err := probe.Check(requestContext, instance)

	if err == nil {
		test.Fatal("expected error for canceled context")
	}

	if status != management.HealthStatusUnknown {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusUnknown,
		)
	}
}

// TestGRPCHealthProbeUnavailable verifies connection failure handling.
func TestGRPCHealthProbeUnavailable(test *testing.T) {
	// Reserve an ephemeral port, then close it before checking.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatalf("failed to allocate test port: %v", err)
	}

	address := listener.Addr().String()

	if err := listener.Close(); err != nil {
		test.Fatalf("failed to release test port: %v", err)
	}

	probe, err := NewGRPCHealthProbe(100 * time.Millisecond)
	if err != nil {
		test.Fatalf("failed to create health probe: %v", err)
	}
	defer probe.Close()

	instance := management.ServiceInstance{
		ID:      "ocr-01",
		Address: address,
	}

	status, err := probe.Check(context.Background(), instance)

	if err == nil {
		test.Fatal("expected error when gRPC server is unavailable")
	}

	if status != management.HealthStatusUnknown {
		test.Errorf(
			"status = %q, want %q",
			status,
			management.HealthStatusUnknown,
		)
	}
}
