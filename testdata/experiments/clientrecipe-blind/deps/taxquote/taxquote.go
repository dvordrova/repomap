package taxquote

import (
	"context"
	"errors"
)

type Config struct {
	Host       string
	Credential string
	Tenant     string
}

type Line struct {
	SKU   string
	Cents int64
	Count int
}

type Result struct {
	QuoteID  string
	TaxCents int64
}

type Client struct{ config Config }

func NewClient(config Config) (*Client, error) {
	if config.Host == "" || config.Credential == "" || config.Tenant == "" {
		return nil, errors.New("taxquote: incomplete config")
	}
	return &Client{config: config}, nil
}

func (c *Client) Calculate(ctx context.Context, region string, lines []Line) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if region == "" || len(lines) == 0 {
		return Result{}, errors.New("taxquote: incomplete request")
	}
	var subtotal int64
	for _, line := range lines {
		subtotal += line.Cents * int64(line.Count)
	}
	return Result{QuoteID: "tax-" + region, TaxCents: subtotal / 10}, nil
}
