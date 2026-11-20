package ocr

import (
	"context"
	"fmt"
	"strings"
	"time"

	ocrv1 "cvhub-vision-platform/internal/gen/ocr/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client manages gRPC communication with the OCR service.
type Client struct {
	connection *grpc.ClientConn
	service    ocrv1.OcrServiceClient
	timeout    time.Duration
}

// NewClient creates an OCR gRPC client for the specified service address.
func NewClient(address string, timeout time.Duration) (*Client, error) {
	address = strings.TrimSpace(address)

	if address == "" {
		return nil, fmt.Errorf("OCR service address cannot be empty")
	}

	if timeout <= 0 {
		return nil, fmt.Errorf("OCR service timeout must be positive")
	}

	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OCR service: %w", err)
	}

	return &Client{
		connection: connection,
		service:    ocrv1.NewOcrServiceClient(connection),
		timeout:    timeout,
	}, nil
}

// GetCapabilities retrieves the OCR service capabilities.
func (client *Client) GetCapabilities(
	ctx context.Context,
	request *ocrv1.GetCapabilitiesRequest,
) (*ocrv1.GetCapabilitiesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	response, err := client.service.GetCapabilities(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("OCR GetCapabilities RPC failed: %w", err)
	}

	return response, nil
}

// Close releases the underlying gRPC connection.
func (client *Client) Close() error {
	return client.connection.Close()
}

// ProcessSingleImage send a single image to the OCR service
func (client *Client) ProcessSingleImage(
	ctx context.Context,
	request *ocrv1.ProcessSingleImageRequest,
) (*ocrv1.ProcessSingleImageResponse, error) {
	requestContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	response, err := client.service.ProcessSingleImage(requestContext, request)
	if err != nil {
		return nil, fmt.Errorf("OCR ProcessSingleImage RPC failed: %w", err)
	}

	return response, nil
}

// ProcessBatchImages sends multiple images to the OCR service.
func (client *Client) ProcessBatchImages(
	ctx context.Context,
	request *ocrv1.ProcessBatchImagesRequest,
) (*ocrv1.ProcessBatchImagesResponse, error) {
	requestContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	response, err := client.service.ProcessBatchImages(
		requestContext,
		request,
	)
	if err != nil {
		return nil, fmt.Errorf("OCR ProcessBatchImages RPC failed: %w", err)
	}

	return response, nil
}
