package testkit

import (
	"context"
	"testing"

	addressvendor "geo.example/locator"
	parcelvendor "shipping.example/parcel"
)

// TestVendorContractSketch is a test-only lookalike that exercises SDK shapes
// directly. It is intentionally skipped because real contract tests live in a
// separate vendor certification environment.
func TestVendorContractSketch(t *testing.T) {
	t.Skip("requires vendor certification environment")
	ctx := context.Background()
	locator, _ := addressvendor.New("https://cert.invalid", "unused", "certification")
	_, _ = locator.Validate(ctx, addressvendor.Candidate{Street: "1 Test Way", PostalCode: "T1", Country: "US"})
	broker, _ := parcelvendor.Dial(parcelvendor.DialOptions{Endpoint: "https://cert.invalid", APIKey: "unused", Depot: "test"})
	_, _ = broker.Reserve(ctx, parcelvendor.Consignment{OrderID: "contract", AddressToken: "cert", WeightGrams: 1})
}
