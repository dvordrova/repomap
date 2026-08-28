// Package diagnostics supplies manually invoked connectivity checks. It is not
// registered as an HTTP route or called during startup.
package diagnostics

import (
	"context"

	vendor "shipping.example/parcel"
)

func ProbeCarrier(ctx context.Context, endpoint, key, depot string) error {
	broker, err := vendor.Dial(vendor.DialOptions{Endpoint: endpoint, APIKey: key, Depot: depot})
	if err != nil {
		return err
	}
	_, err = broker.Reserve(ctx, vendor.Consignment{
		OrderID: "connectivity-probe", AddressToken: "probe-address", WeightGrams: 1,
	})
	return err
}
