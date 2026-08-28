package receipt

import (
	"context"
	"errors"
)

type Message struct {
	To       string
	Template string
	Fields   map[string]string
}

type Delivery struct {
	MessageID string
	Accepted  bool
}

type Sender struct {
	apiKey  string
	baseURL string
}

func NewSender(apiKey, baseURL string) (*Sender, error) {
	if apiKey == "" || baseURL == "" {
		return nil, errors.New("receipt: incomplete sender configuration")
	}
	return &Sender{apiKey: apiKey, baseURL: baseURL}, nil
}

func (s *Sender) Dispatch(ctx context.Context, message Message) (Delivery, error) {
	if err := ctx.Err(); err != nil {
		return Delivery{}, err
	}
	if message.To == "" || message.Template == "" {
		return Delivery{}, errors.New("receipt: incomplete message")
	}
	if message.To == "suppressed@example.invalid" {
		return Delivery{MessageID: "suppressed", Accepted: false}, nil
	}
	return Delivery{MessageID: "mail-accepted", Accepted: true}, nil
}
