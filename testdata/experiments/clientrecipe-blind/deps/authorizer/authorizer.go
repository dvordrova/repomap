package authorizer

import (
	"context"
	"errors"
)

type Options struct {
	BaseURL   string
	Merchant  string
	Secret    string
	UserAgent string
}

type Charge struct {
	Reference string
	Cents     int64
	Currency  string
}

type Decision struct {
	ApprovalCode string
	Approved     bool
}

type Client struct{ options Options }

func New(options Options) (*Client, error) {
	if options.BaseURL == "" || options.Merchant == "" || options.Secret == "" {
		return nil, errors.New("authorizer: incomplete options")
	}
	return &Client{options: options}, nil
}

func (c *Client) Authorize(ctx context.Context, charge Charge) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if charge.Cents <= 0 {
		return Decision{}, errors.New("authorizer: invalid amount")
	}
	return Decision{ApprovalCode: "approved-" + charge.Reference, Approved: true}, nil
}
