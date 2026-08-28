package checkout

import (
	"context"
	"fmt"

	"example.invalid/fulfilment/internal/address"
	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/payment"
	"example.invalid/fulfilment/internal/shipping"
	"example.invalid/fulfilment/internal/tax"
)

type paymentAuthorizer interface {
	Authorize(context.Context, payment.Request) (payment.Approval, error)
}

type parcelBooker interface {
	Book(context.Context, string, string, int) (shipping.Booking, error)
}

type receiptSender interface {
	Send(context.Context, string, string, string, int64) error
}

type Service struct {
	payments  paymentAuthorizer
	taxes     tax.QuoteFunc
	addresses address.Verifier
	parcels   parcelBooker
	receipts  receiptSender
}

func New(payments paymentAuthorizer, taxes tax.QuoteFunc, addresses address.Verifier, parcels parcelBooker, receipts receiptSender) *Service {
	return &Service{payments: payments, taxes: taxes, addresses: addresses, parcels: parcels, receipts: receipts}
}

func (s *Service) Fulfil(ctx context.Context, order model.Order) (model.Fulfilment, error) {
	verification, err := s.addresses.Verify(ctx, order.ID, order.ShipTo)
	if err != nil {
		return model.Fulfilment{}, fmt.Errorf("fulfil order: %w", err)
	}
	taxCents, err := s.taxes(ctx, order.ID, order.Items)
	if err != nil {
		return model.Fulfilment{}, fmt.Errorf("fulfil order: %w", err)
	}
	totalCents := order.SubtotalCents() + taxCents
	approval, err := s.payments.Authorize(ctx, payment.Request{
		OrderID: order.ID, Cents: totalCents, Currency: order.Currency,
	})
	if err != nil {
		return model.Fulfilment{}, fmt.Errorf("fulfil order: %w", err)
	}
	booking, err := s.parcels.Book(ctx, order.ID, verification.Token, order.WeightGrams())
	if err != nil {
		return model.Fulfilment{}, fmt.Errorf("fulfil order: %w", err)
	}
	if err := s.receipts.Send(ctx, order.ID, order.Email, booking.TrackingCode, totalCents); err != nil {
		return model.Fulfilment{}, fmt.Errorf("fulfil order: %w", err)
	}
	return model.Fulfilment{
		OrderID: order.ID, ApprovalCode: approval.Code,
		TaxCents: taxCents, TrackingCode: booking.TrackingCode,
	}, nil
}
