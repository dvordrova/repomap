package vaultsdk

import (
	"context"
	"fmt"
)

type Config struct {
	Address string
	Token   string
}

type Client struct {
	config Config
}

func New(config Config) (*Client, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("vault address is required")
	}
	return &Client{config: config}, nil
}

func (client *Client) Read(_ context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("vault key is required")
	}
	return "value:" + key, nil
}

func (client *Client) Ping(context.Context) error { return nil }
