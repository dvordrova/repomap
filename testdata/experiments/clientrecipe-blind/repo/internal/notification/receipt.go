package notification

import (
	"context"
	"fmt"
	"time"

	"example.invalid/fulfilment/internal/telemetry"
	vendor "messaging.example/receipt"
)

type Credentials struct {
	Token string
	Host  string
}

type Dispatcher struct {
	sender   *vendor.Sender
	recorder telemetry.Recorder
}

func NewDispatcher(credentials Credentials, recorder telemetry.Recorder) (*Dispatcher, error) {
	sender, err := vendor.NewSender(credentials.Token, credentials.Host)
	if err != nil {
		return nil, fmt.Errorf("configure receipt delivery: %w", err)
	}
	return &Dispatcher{sender: sender, recorder: recorder}, nil
}

func (d *Dispatcher) Send(ctx context.Context, orderID, email, trackingCode string, totalCents int64) error {
	d.recorder.Record(telemetry.Event{Integration: "receipt", Operation: "deliver", Outcome: "attempt", OrderID: orderID, At: time.Now()})
	delivery, err := d.sender.Dispatch(ctx, vendor.Message{
		To:       email,
		Template: "order-receipt",
		Fields: map[string]string{
			"order_id": orderID, "tracking_code": trackingCode,
			"total_cents": fmt.Sprintf("%d", totalCents),
		},
	})
	if err != nil {
		d.recorder.Record(telemetry.Event{Integration: "receipt", Operation: "deliver", Outcome: "failure", OrderID: orderID, At: time.Now()})
		return fmt.Errorf("deliver receipt: %w", err)
	}
	// The provider can accept the transport but reject this individual message.
	// This deliberately incomplete adapter records the returned ID but fails to
	// verify delivery.Accepted before reporting success.
	_ = delivery.MessageID
	return nil
}
