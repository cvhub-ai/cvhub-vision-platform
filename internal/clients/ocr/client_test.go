package ocr

import (
	"context"
	"errors"
	"testing"
	"time"

	ocrv1 "cvhub-vision-platform/internal/gen/ocr/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockOcrServiceClient implements the generated gRPC client interface.
type mockOcrServiceClient struct {
	getCapabilitiesFunc    func(context.Context, *ocrv1.GetCapabilitiesRequest) (*ocrv1.GetCapabilitiesResponse, error)
	processSingleImageFunc func(context.Context, *ocrv1.ProcessSingleImageRequest) (*ocrv1.ProcessSingleImageResponse, error)
	processBatchImagesFunc func(context.Context, *ocrv1.ProcessBatchImagesRequest) (*ocrv1.ProcessBatchImagesResponse, error)
}

func (mock *mockOcrServiceClient) GetCapabilities(
	requestContext context.Context,
	request *ocrv1.GetCapabilitiesRequest,
	_ ...grpc.CallOption,
) (*ocrv1.GetCapabilitiesResponse, error) {
	if mock.getCapabilitiesFunc == nil {
		return nil, status.Error(codes.Unimplemented, "GetCapabilities not configured in mock")
	}
	return mock.getCapabilitiesFunc(requestContext, request)
}

func (mock *mockOcrServiceClient) ProcessSingleImage(
	requestContext context.Context,
	request *ocrv1.ProcessSingleImageRequest,
	_ ...grpc.CallOption,
) (*ocrv1.ProcessSingleImageResponse, error) {
	if mock.processSingleImageFunc == nil {
		return nil, status.Error(codes.Unimplemented, "ProcessSingleImage not configured in mock")
	}
	return mock.processSingleImageFunc(requestContext, request)
}

func (mock *mockOcrServiceClient) ProcessBatchImages(
	requestContext context.Context,
	request *ocrv1.ProcessBatchImagesRequest,
	_ ...grpc.CallOption,
) (*ocrv1.ProcessBatchImagesResponse, error) {
	if mock.processBatchImagesFunc == nil {
		return nil, status.Error(codes.Unimplemented, "ProcessBatchImages not configured in mock")
	}
	return mock.processBatchImagesFunc(requestContext, request)
}

// assertRPCDeadline verifies that an RPC context has a valid deadline.
func assertRPCDeadline(test *testing.T, requestContext context.Context, timeout time.Duration) {
	test.Helper()

	deadline, exists := requestContext.Deadline()
	if !exists {
		test.Fatal("expected RPC context to have a deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > timeout {
		test.Fatalf("unexpected RPC deadline: %v remaining, configured timeout %v", remaining, timeout)
	}
}

// TestNewClient verifies client initialization and configuration validation.
func TestNewClient(test *testing.T) {
	test.Run("valid configuration", func(test *testing.T) {
		timeout := 30 * time.Second
		client, err := NewClient(" localhost:50051 ", timeout)
		if err != nil {
			test.Fatalf("NewClient returned unexpected error: %v", err)
		}
		test.Cleanup(func() {
			if err := client.Close(); err != nil {
				test.Errorf("failed to close client: %v", err)
			}
		})

		if client.connection == nil || client.service == nil {
			test.Fatal("expected initialized gRPC connection and service client")
		}
		if client.timeout != timeout {
			test.Errorf("timeout = %v, want %v", client.timeout, timeout)
		}
	})

	invalidCases := []struct {
		name    string
		address string
		timeout time.Duration
	}{
		{name: "empty address", address: "", timeout: time.Second},
		{name: "whitespace address", address: "  ", timeout: time.Second},
		{name: "zero timeout", address: "localhost:50051", timeout: 0},
		{name: "negative timeout", address: "localhost:50051", timeout: -time.Second},
	}

	for _, testCase := range invalidCases {
		test.Run(testCase.name, func(test *testing.T) {
			client, err := NewClient(testCase.address, testCase.timeout)
			if err == nil {
				if client != nil {
					_ = client.Close()
				}
				test.Fatal("expected NewClient to reject invalid configuration")
			}
			if client != nil {
				test.Fatal("expected nil client on configuration error")
			}
		})
	}
}

// TestClientClose verifies gRPC connection cleanup.
func TestClientClose(test *testing.T) {
	client, err := NewClient("localhost:50051", time.Second)
	if err != nil {
		test.Fatalf("NewClient failed: %v", err)
	}
	if err := client.Close(); err != nil {
		test.Fatalf("Close failed: %v", err)
	}
}

// TestClientGetCapabilities verifies success, error handling and timeout.
func TestClientGetCapabilities(test *testing.T) {
	test.Run("success", func(test *testing.T) {
		timeout := 30 * time.Second
		request := &ocrv1.GetCapabilitiesRequest{
			Context: &ocrv1.RequestContext{RequestId: "capabilities-001", ClientId: "vision-platform"},
		}
		expectedResponse := &ocrv1.GetCapabilitiesResponse{ServiceName: "ocr", ServiceVersion: "1.0.0"}
		mockService := &mockOcrServiceClient{
			getCapabilitiesFunc: func(requestContext context.Context, receivedRequest *ocrv1.GetCapabilitiesRequest) (*ocrv1.GetCapabilitiesResponse, error) {
				if receivedRequest != request {
					test.Error("GetCapabilities did not forward the original request")
				}
				assertRPCDeadline(test, requestContext, timeout)
				return expectedResponse, nil
			},
		}
		client := &Client{service: mockService, timeout: timeout}
		response, err := client.GetCapabilities(context.Background(), request)
		if err != nil {
			test.Fatalf("GetCapabilities failed: %v", err)
		}
		if response != expectedResponse {
			test.Fatalf("response = %p, want %p", response, expectedResponse)
		}
	})

	test.Run("gRPC error", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			getCapabilitiesFunc: func(context.Context, *ocrv1.GetCapabilitiesRequest) (*ocrv1.GetCapabilitiesResponse, error) {
				return nil, status.Error(codes.Unavailable, "OCR service unavailable")
			},
		}
		client := &Client{service: mockService, timeout: time.Second}
		response, err := client.GetCapabilities(context.Background(), &ocrv1.GetCapabilitiesRequest{})
		if response != nil || status.Code(err) != codes.Unavailable {
			test.Fatalf("got response %v, error %v; want nil and Unavailable", response, err)
		}
	})

	test.Run("timeout", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			getCapabilitiesFunc: func(requestContext context.Context, _ *ocrv1.GetCapabilitiesRequest) (*ocrv1.GetCapabilitiesResponse, error) {
				<-requestContext.Done()
				return nil, requestContext.Err()
			},
		}
		client := &Client{service: mockService, timeout: 20 * time.Millisecond}
		response, err := client.GetCapabilities(context.Background(), &ocrv1.GetCapabilitiesRequest{})
		if response != nil || !errors.Is(err, context.DeadlineExceeded) {
			test.Fatalf("got response %v, error %v; want nil and deadline exceeded", response, err)
		}
	})
}

// TestClientProcessSingleImage verifies success, error handling and timeout.
func TestClientProcessSingleImage(test *testing.T) {
	test.Run("success", func(test *testing.T) {
		timeout := 30 * time.Second
		request := &ocrv1.ProcessSingleImageRequest{
			Context: &ocrv1.RequestContext{RequestId: "single-001"},
			Image:   &ocrv1.ImageInput{ImageId: "image-001", Data: []byte("test-image"), MimeType: "image/png"},
			Options: &ocrv1.OcrOptions{Language: "en"},
		}
		expectedResponse := &ocrv1.ProcessSingleImageResponse{
			Result: &ocrv1.ImageResult{ImageWidth: 640, ImageHeight: 480},
		}
		mockService := &mockOcrServiceClient{
			processSingleImageFunc: func(requestContext context.Context, receivedRequest *ocrv1.ProcessSingleImageRequest) (*ocrv1.ProcessSingleImageResponse, error) {
				if receivedRequest != request {
					test.Error("ProcessSingleImage did not forward the original request")
				}
				assertRPCDeadline(test, requestContext, timeout)
				return expectedResponse, nil
			},
		}
		client := &Client{service: mockService, timeout: timeout}
		response, err := client.ProcessSingleImage(context.Background(), request)
		if err != nil {
			test.Fatalf("ProcessSingleImage failed: %v", err)
		}
		if response != expectedResponse {
			test.Fatalf("response = %p, want %p", response, expectedResponse)
		}
	})

	test.Run("gRPC error", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			processSingleImageFunc: func(context.Context, *ocrv1.ProcessSingleImageRequest) (*ocrv1.ProcessSingleImageResponse, error) {
				return nil, status.Error(codes.InvalidArgument, "invalid image")
			},
		}
		client := &Client{service: mockService, timeout: time.Second}
		response, err := client.ProcessSingleImage(context.Background(), &ocrv1.ProcessSingleImageRequest{})
		if response != nil || status.Code(err) != codes.InvalidArgument {
			test.Fatalf("got response %v, error %v; want nil and InvalidArgument", response, err)
		}
	})

	test.Run("timeout", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			processSingleImageFunc: func(requestContext context.Context, _ *ocrv1.ProcessSingleImageRequest) (*ocrv1.ProcessSingleImageResponse, error) {
				<-requestContext.Done()
				return nil, requestContext.Err()
			},
		}
		client := &Client{service: mockService, timeout: 20 * time.Millisecond}
		response, err := client.ProcessSingleImage(context.Background(), &ocrv1.ProcessSingleImageRequest{})
		if response != nil || !errors.Is(err, context.DeadlineExceeded) {
			test.Fatalf("got response %v, error %v; want nil and deadline exceeded", response, err)
		}
	})
}

// TestClientProcessBatchImages verifies batch responses, errors and timeout.
func TestClientProcessBatchImages(test *testing.T) {
	test.Run("success with per-image error", func(test *testing.T) {
		timeout := 30 * time.Second
		request := &ocrv1.ProcessBatchImagesRequest{
			Context: &ocrv1.RequestContext{RequestId: "batch-001"},
			Images: []*ocrv1.ImageInput{
				{ImageId: "image-001", MimeType: "image/png", Data: []byte("image-one")},
				{ImageId: "image-002", MimeType: "image/png", Data: []byte("image-two")},
			},
		}
		expectedResponse := &ocrv1.ProcessBatchImagesResponse{
			Results: []*ocrv1.BatchItemResult{
				{
					ImageId: "image-001",
					Outcome: &ocrv1.BatchItemResult_Result{
						Result: &ocrv1.ImageResult{ImageWidth: 640, ImageHeight: 480},
					},
				},
				{
					ImageId: "image-002",
					Outcome: &ocrv1.BatchItemResult_Error{
						Error: &ocrv1.ItemError{
							Code:    ocrv1.ItemErrorCode_ITEM_ERROR_CODE_INVALID_IMAGE,
							Message: "invalid image",
						},
					},
				},
			},
		}
		mockService := &mockOcrServiceClient{
			processBatchImagesFunc: func(requestContext context.Context, receivedRequest *ocrv1.ProcessBatchImagesRequest) (*ocrv1.ProcessBatchImagesResponse, error) {
				if receivedRequest != request {
					test.Error("ProcessBatchImages did not forward the original request")
				}
				assertRPCDeadline(test, requestContext, timeout)
				return expectedResponse, nil
			},
		}
		client := &Client{service: mockService, timeout: timeout}
		response, err := client.ProcessBatchImages(context.Background(), request)
		if err != nil {
			test.Fatalf("ProcessBatchImages failed: %v", err)
		}
		if response != expectedResponse {
			test.Fatalf("response = %p, want %p", response, expectedResponse)
		}
		if len(response.Results) != 2 || response.Results[1].GetError() == nil {
			test.Fatal("expected per-image error to remain in the batch response")
		}
	})

	test.Run("gRPC error", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			processBatchImagesFunc: func(context.Context, *ocrv1.ProcessBatchImagesRequest) (*ocrv1.ProcessBatchImagesResponse, error) {
				return nil, status.Error(codes.Unavailable, "OCR service unavailable")
			},
		}
		client := &Client{service: mockService, timeout: time.Second}
		response, err := client.ProcessBatchImages(context.Background(), &ocrv1.ProcessBatchImagesRequest{})
		if response != nil || status.Code(err) != codes.Unavailable {
			test.Fatalf("got response %v, error %v; want nil and Unavailable", response, err)
		}
	})

	test.Run("timeout", func(test *testing.T) {
		mockService := &mockOcrServiceClient{
			processBatchImagesFunc: func(requestContext context.Context, _ *ocrv1.ProcessBatchImagesRequest) (*ocrv1.ProcessBatchImagesResponse, error) {
				<-requestContext.Done()
				return nil, requestContext.Err()
			},
		}
		client := &Client{service: mockService, timeout: 20 * time.Millisecond}
		response, err := client.ProcessBatchImages(context.Background(), &ocrv1.ProcessBatchImagesRequest{})
		if response != nil || !errors.Is(err, context.DeadlineExceeded) {
			test.Fatalf("got response %v, error %v; want nil and deadline exceeded", response, err)
		}
	})
}
