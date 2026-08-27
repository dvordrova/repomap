package kubernetessdk

import (
	"context"
	"fmt"
)

type Config struct {
	Server string
}

type Client struct {
	server string
}

func New(config Config) (*Client, error) {
	if config.Server == "" {
		return nil, fmt.Errorf("Kubernetes server is required")
	}
	return &Client{server: config.Server}, nil
}

func (*Client) Namespaces(context.Context) ([]string, error) {
	return []string{"default", "launch"}, nil
}
