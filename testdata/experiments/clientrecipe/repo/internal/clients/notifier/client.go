package notifier

import (
	"context"

	"example.com/notifiersdk"
)

type Client struct {
	sender *notifiersdk.Sender
}

func NewClient(config Config) *Client {
	return &Client{sender: notifiersdk.New(config.Endpoint)}
}

func (client *Client) SendLaunch(ctx context.Context, message string) error {
	return client.sender.Send(ctx, message)
}
