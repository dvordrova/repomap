package tax

import (
	"context"
	"fmt"
	"time"

	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/telemetry"
	vendor "revenue.example/taxquote"
)

type Deployment struct {
	Origin       string
	Credential   string
	Organization string
	Deadline     time.Duration
}

type QuoteFunc func(context.Context, string, []model.Item) (int64, error)

type calculator interface {
	Calculate(context.Context, string, []vendor.Line) (vendor.Result, error)
}

func Connect(deployment Deployment, recorder telemetry.Recorder) (QuoteFunc, error) {
	client, err := vendor.NewClient(vendor.Config{
		Host:       deployment.Origin,
		Credential: deployment.Credential,
		Tenant:     deployment.Organization,
	})
	if err != nil {
		return nil, fmt.Errorf("connect tax quotation: %w", err)
	}
	if deployment.Deadline <= 0 {
		deployment.Deadline = 750 * time.Millisecond
	}
	return makeQuote(client, recorder, deployment.Deadline), nil
}

func makeQuote(client calculator, recorder telemetry.Recorder, deadline time.Duration) QuoteFunc {
	return func(ctx context.Context, orderID string, items []model.Item) (int64, error) {
		ctx, cancel := context.WithTimeout(ctx, deadline)
		defer cancel()
		recorder.Record(telemetry.Event{Integration: "tax", Operation: "quote", Outcome: "attempt", OrderID: orderID, At: time.Now()})
		lines := make([]vendor.Line, 0, len(items))
		for _, item := range items {
			lines = append(lines, vendor.Line{SKU: item.SKU, Cents: item.UnitCents, Count: item.Quantity})
		}
		result, err := client.Calculate(ctx, "destination", lines)
		if err != nil {
			recorder.Record(telemetry.Event{Integration: "tax", Operation: "quote", Outcome: "failure", OrderID: orderID, At: time.Now()})
			return 0, fmt.Errorf("quote tax: %w", err)
		}
		return result.TaxCents, nil
	}
}
