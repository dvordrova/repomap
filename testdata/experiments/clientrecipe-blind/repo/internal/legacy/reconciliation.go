// Package legacy preserves a retired settlement workflow for reference while
// old records age out. The current executable has no dependency on it.
package legacy

import (
	"context"
	"fmt"

	vendor "acquirer.example/authorizer"
)

type Reconciler struct{ gateway *vendor.Client }

func NewReconciler(endpoint, merchant, secret string) (*Reconciler, error) {
	client, err := vendor.New(vendor.Options{
		BaseURL: endpoint, Merchant: merchant, Secret: secret, UserAgent: "legacy-reconciler",
	})
	if err != nil {
		return nil, fmt.Errorf("open legacy payment reconciliation: %w", err)
	}
	return &Reconciler{gateway: client}, nil
}

func (r *Reconciler) Check(ctx context.Context, reference string) error {
	_, err := r.gateway.Authorize(ctx, vendor.Charge{Reference: reference, Cents: 1, Currency: "USD"})
	return err
}
