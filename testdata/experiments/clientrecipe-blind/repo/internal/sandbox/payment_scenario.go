// Package sandbox contains opt-in developer exercises that are not assembled
// into the fulfilment executable.
package sandbox

import (
	"context"

	vendor "acquirer.example/authorizer"
)

type PaymentScenario struct {
	Endpoint string
	Merchant string
	Secret   string
}

func (s PaymentScenario) Run(ctx context.Context) (vendor.Decision, error) {
	client, err := vendor.New(vendor.Options{
		BaseURL: s.Endpoint, Merchant: s.Merchant, Secret: s.Secret, UserAgent: "developer-sandbox",
	})
	if err != nil {
		return vendor.Decision{}, err
	}
	return client.Authorize(ctx, vendor.Charge{Reference: "sandbox-order", Cents: 100, Currency: "USD"})
}
