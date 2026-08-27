package legacy

import "example.com/legacysdk"

// Client predates the current bootstrap path and is intentionally unreachable.
type Client struct {
	sdk *legacysdk.Client
}

func NewClient() *Client { return &Client{sdk: legacysdk.New()} }

func (client *Client) Lookup(key string) string { return client.sdk.Lookup(key) }
