// Package examples holds documentation-oriented snippets. Nothing in the
// production composition root imports it.
package examples

import (
	"context"

	vendor "revenue.example/taxquote"
)

func QuotingWalkthrough(ctx context.Context, host, token, tenant string) (int64, error) {
	client, err := vendor.NewClient(vendor.Config{Host: host, Credential: token, Tenant: tenant})
	if err != nil {
		return 0, err
	}
	result, err := client.Calculate(ctx, "training-region", []vendor.Line{{SKU: "sample", Cents: 2500, Count: 1}})
	if err != nil {
		return 0, err
	}
	return result.TaxCents, nil
}
