package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	vendor "acquirer.example/authorizer"
	"example.invalid/fulfilment/internal/telemetry"
)

type Settings struct {
	URL         string
	MerchantID  string
	APISecret   string
	MaxAttempts int
}

type Request struct {
	OrderID  string
	Cents    int64
	Currency string
}

type Approval struct{ Code string }

type sdk interface {
	Authorize(context.Context, vendor.Charge) (vendor.Decision, error)
}

type Gateway struct {
	client   sdk
	recorder telemetry.Recorder
	attempts int
	pause    func(time.Duration)
}

func Open(settings Settings, recorder telemetry.Recorder) (*Gateway, error) {
	client, err := vendor.New(vendor.Options{
		BaseURL:   settings.URL,
		Merchant:  settings.MerchantID,
		Secret:    settings.APISecret,
		UserAgent: "fulfilment-service/1",
	})
	if err != nil {
		return nil, fmt.Errorf("open payment gateway: %w", err)
	}
	if settings.MaxAttempts < 1 {
		settings.MaxAttempts = 1
	}
	return &Gateway{client: client, recorder: recorder, attempts: settings.MaxAttempts, pause: time.Sleep}, nil
}

func (g *Gateway) Authorize(ctx context.Context, request Request) (Approval, error) {
	charge := vendor.Charge{Reference: request.OrderID, Cents: request.Cents, Currency: request.Currency}
	var lastErr error
	for attempt := 1; attempt <= g.attempts; attempt++ {
		g.recorder.Record(telemetry.Event{Integration: "payment", Operation: "authorize", Outcome: "attempt", OrderID: request.OrderID, At: time.Now()})
		decision, err := g.client.Authorize(ctx, charge)
		if err == nil && decision.Approved {
			return Approval{Code: decision.ApprovalCode}, nil
		}
		if err == nil {
			err = errors.New("authorization declined")
		}
		lastErr = err
		g.recorder.Record(telemetry.Event{Integration: "payment", Operation: "authorize", Outcome: "failure", OrderID: request.OrderID, At: time.Now()})
		if attempt < g.attempts {
			g.pause(time.Duration(attempt) * time.Millisecond)
		}
	}
	return Approval{}, fmt.Errorf("authorize payment: %w", lastErr)
}
