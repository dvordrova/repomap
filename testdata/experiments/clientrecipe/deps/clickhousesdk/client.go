package clickhousesdk

import (
	"context"
	"fmt"
)

type Client struct {
	dsn string
}

func Open(dsn string) (*Client, error) {
	if dsn == "" {
		return nil, fmt.Errorf("ClickHouse DSN is required")
	}
	return &Client{dsn: dsn}, nil
}

func (*Client) RecentLaunches(context.Context, int) ([]string, error) {
	return []string{"launch-001", "launch-002"}, nil
}
