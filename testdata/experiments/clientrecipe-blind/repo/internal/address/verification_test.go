package address

import (
	"context"
	"testing"

	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/telemetry"
	vendor "geo.example/locator"
)

type rejectingVerifier struct{}

func (rejectingVerifier) Validate(context.Context, vendor.Candidate) (vendor.Result, error) {
	return vendor.Result{Deliverable: false}, nil
}

func TestAdapterRejectsUndeliverableAddress(t *testing.T) {
	events := &telemetry.Memory{}
	adapter := &Adapter{remote: rejectingVerifier{}, recorder: events}
	_, err := adapter.Verify(context.Background(), "order-12", model.Address{
		Street: "3 Market Street", City: "York", PostalCode: "Y1", Country: "GB",
	})
	if err == nil {
		t.Fatal("expected undeliverable address error")
	}
	got := events.Events()
	if len(got) != 2 || got[0].Outcome != "attempt" || got[1].Outcome != "failure" {
		t.Fatalf("unexpected events: %+v", got)
	}
}
