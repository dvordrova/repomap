package checkout

import (
	"context"
	"testing"

	"example.invalid/fulfilment/internal/address"
	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/payment"
	"example.invalid/fulfilment/internal/shipping"
)

type paymentSpy struct{ called bool }

func (s *paymentSpy) Authorize(context.Context, payment.Request) (payment.Approval, error) {
	s.called = true
	return payment.Approval{Code: "approved"}, nil
}

type addressSpy struct{ called bool }

func (s *addressSpy) Verify(_ context.Context, _ string, value model.Address) (address.Verification, error) {
	s.called = true
	return address.Verification{Token: "verified", Normalized: value}, nil
}

type parcelSpy struct{ called bool }

func (s *parcelSpy) Book(context.Context, string, string, int) (shipping.Booking, error) {
	s.called = true
	return shipping.Booking{TrackingCode: "tracking"}, nil
}

type receiptSpy struct{ called bool }

func (s *receiptSpy) Send(context.Context, string, string, string, int64) error {
	s.called = true
	return nil
}

func TestFulfilReachesEveryProductionIntegration(t *testing.T) {
	payments := &paymentSpy{}
	addresses := &addressSpy{}
	parcels := &parcelSpy{}
	receipts := &receiptSpy{}
	taxCalled := false
	service := New(payments, func(context.Context, string, []model.Item) (int64, error) {
		taxCalled = true
		return 125, nil
	}, addresses, parcels, receipts)

	result, err := service.Fulfil(context.Background(), model.Order{
		ID: "order-1", Email: "buyer@example.invalid", Currency: "USD",
		ShipTo: model.Address{Street: "1 Main St", City: "Town", PostalCode: "10001", Country: "US"},
		Items:  []model.Item{{SKU: "mug", UnitCents: 1500, Quantity: 2, WeightGrams: 250}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !payments.called || !taxCalled || !addresses.called || !parcels.called || !receipts.called {
		t.Fatalf("calls payment=%v tax=%v address=%v parcel=%v receipt=%v", payments.called, taxCalled, addresses.called, parcels.called, receipts.called)
	}
	if result.TrackingCode != "tracking" || result.TaxCents != 125 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
