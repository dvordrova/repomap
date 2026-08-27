package notifiersdk

import "context"

type Sender struct {
	endpoint string
}

func New(endpoint string) *Sender { return &Sender{endpoint: endpoint} }

func (sender *Sender) Send(context.Context, string) error { return nil }
