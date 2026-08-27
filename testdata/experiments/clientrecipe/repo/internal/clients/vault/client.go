package vault

import (
	"context"
	"fmt"
	"time"

	"example.com/launchservice/internal/observability"
	"example.com/vaultsdk"
)

type Client struct {
	sdk     *vaultsdk.Client
	timeout time.Duration
	retries int
	metrics *observability.Metrics
}

func NewClient(config Config, metrics *observability.Metrics) (*Client, error) {
	sdk, err := vaultsdk.New(vaultsdk.Config{Address: config.Address, Token: config.Token})
	if err != nil {
		return nil, fmt.Errorf("create Vault SDK client: %w", err)
	}
	return &Client{sdk: sdk, timeout: config.Timeout, retries: config.Retries, metrics: metrics}, nil
}

func (client *Client) ReadSecret(ctx context.Context, key string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= client.retries; attempt++ {
		client.metrics.RecordAttempt("vault")
		requestContext, cancel := context.WithTimeout(ctx, client.timeout)
		value, err := client.sdk.Read(requestContext, key)
		cancel()
		if err == nil {
			return value, nil
		}
		lastErr = err
		client.metrics.RecordFailure("vault")
		observability.ClientFailure("vault", "read_secret", err)
	}
	return "", fmt.Errorf("read Vault secret: %w", lastErr)
}

func (client *Client) HealthCheck(ctx context.Context) error {
	requestContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	return client.sdk.Ping(requestContext)
}
