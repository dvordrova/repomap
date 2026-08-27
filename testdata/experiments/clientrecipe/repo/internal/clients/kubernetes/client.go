package kubernetes

import (
	"context"
	"fmt"

	"example.com/kubernetessdk"
	"example.com/launchservice/internal/observability"
)

type Client struct {
	sdk     *kubernetessdk.Client
	retries int
	metrics *observability.Metrics
}

type Factory func(kubernetessdk.Config) (*kubernetessdk.Client, error)

func NewClient(config Config, metrics *observability.Metrics) (*Client, error) {
	return newWithFactory(config, metrics, kubernetessdk.New)
}

func newWithFactory(config Config, metrics *observability.Metrics, factory Factory) (*Client, error) {
	sdk, err := factory(kubernetessdk.Config{Server: config.Server})
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes SDK client: %w", err)
	}
	return &Client{sdk: sdk, retries: config.Retries, metrics: metrics}, nil
}

func (client *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= client.retries; attempt++ {
		client.metrics.RecordAttempt("kubernetes")
		namespaces, err := client.sdk.Namespaces(ctx)
		if err == nil {
			return namespaces, nil
		}
		lastErr = err
		client.metrics.RecordFailure("kubernetes")
		observability.ClientFailure("kubernetes", "list_namespaces", err)
	}
	return nil, fmt.Errorf("list Kubernetes namespaces: %w", lastErr)
}
