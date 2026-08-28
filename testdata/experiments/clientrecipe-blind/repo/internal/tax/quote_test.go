package tax

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/telemetry"
	vendor "revenue.example/taxquote"
)

type failingCalculator struct{}

func (failingCalculator) Calculate(context.Context, string, []vendor.Line) (vendor.Result, error) {
	return vendor.Result{}, errors.New("tax service unavailable")
}

func TestQuoteRecordsRemoteFailure(t *testing.T) {
	events := &telemetry.Memory{}
	quote := makeQuote(failingCalculator{}, events, time.Second)
	_, err := quote(context.Background(), "order-8", []model.Item{{SKU: "lamp", UnitCents: 3200, Quantity: 1}})
	if err == nil {
		t.Fatal("expected quotation error")
	}
	got := events.Events()
	if len(got) != 2 || got[0].Outcome != "attempt" || got[1].Outcome != "failure" {
		t.Fatalf("unexpected events: %+v", got)
	}
}
