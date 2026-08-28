// Package preview renders sample communication for local design review. It is
// deliberately absent from production assembly.
package preview

import (
	"context"

	vendor "messaging.example/receipt"
)

func SendSampleReceipt(ctx context.Context, token, host, designerEmail string) (string, error) {
	sender, err := vendor.NewSender(token, host)
	if err != nil {
		return "", err
	}
	delivery, err := sender.Dispatch(ctx, vendor.Message{
		To: designerEmail, Template: "order-receipt-draft",
		Fields: map[string]string{"order_id": "PREVIEW", "tracking_code": "SAMPLE"},
	})
	if err != nil {
		return "", err
	}
	return delivery.MessageID, nil
}
