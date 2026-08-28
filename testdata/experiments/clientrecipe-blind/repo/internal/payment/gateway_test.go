package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	vendor "acquirer.example/authorizer"
	"example.invalid/fulfilment/internal/telemetry"
)

type flakyAuthorizer struct{ calls int }

func (f *flakyAuthorizer) Authorize(context.Context, vendor.Charge) (vendor.Decision, error) {
	f.calls++
	if f.calls == 1 {
		return vendor.Decision{}, errors.New("temporary outage")
	}
	return vendor.Decision{Approved: true, ApprovalCode: "ok-17"}, nil
}

func TestGatewayRetriesAndRecordsFailure(t *testing.T) {
	remote := &flakyAuthorizer{}
	events := &telemetry.Memory{}
	gateway := &Gateway{client: remote, recorder: events, attempts: 2, pause: func(time.Duration) {}}

	approval, err := gateway.Authorize(context.Background(), Request{OrderID: "order-17", Cents: 4200, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if approval.Code != "ok-17" || remote.calls != 2 {
		t.Fatalf("approval=%+v calls=%d", approval, remote.calls)
	}
	got := events.Events()
	if len(got) != 3 || got[0].Outcome != "attempt" || got[1].Outcome != "failure" || got[2].Outcome != "attempt" {
		t.Fatalf("unexpected events: %+v", got)
	}
}
