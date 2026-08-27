package clickhouse

import (
	"context"
	"fmt"

	"example.com/clickhousesdk"
	"example.com/launchservice/internal/observability"
)

type Client struct {
	sdk     *clickhousesdk.Client
	metrics *observability.Metrics
}

func NewClient(config Config, metrics *observability.Metrics) (*Client, error) {
	sdk, err := clickhousesdk.Open(config.DSN)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse client: %w", err)
	}
	return &Client{sdk: sdk, metrics: metrics}, nil
}

func (client *Client) RecentLaunches(ctx context.Context, limit int) ([]string, error) {
	client.metrics.RecordAttempt("clickhouse")
	launches, err := client.sdk.RecentLaunches(ctx, limit)
	if err != nil {
		client.metrics.RecordFailure("clickhouse")
		observability.ClientFailure("clickhouse", "recent_launches", err)
		return nil, fmt.Errorf("query recent launches: %w", err)
	}
	return launches, nil
}
