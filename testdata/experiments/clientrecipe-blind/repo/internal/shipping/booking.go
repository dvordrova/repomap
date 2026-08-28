package shipping

import (
	"context"
	"fmt"
	"time"

	"example.invalid/fulfilment/internal/telemetry"
	vendor "shipping.example/parcel"
)

type Environment struct {
	Endpoint string
	Key      string
	Depot    string
}

type Booking struct {
	TrackingCode string
	LabelURL     string
}

type reservationFunc func(context.Context, vendor.Consignment) (vendor.Reservation, error)

type Booker struct {
	reserve  reservationFunc
	recorder telemetry.Recorder
}

func Open(environment Environment, recorder telemetry.Recorder) (*Booker, error) {
	broker, err := vendor.Dial(vendor.DialOptions{
		Endpoint: environment.Endpoint,
		APIKey:   environment.Key,
		Depot:    environment.Depot,
	})
	if err != nil {
		return nil, fmt.Errorf("open parcel booking: %w", err)
	}
	return &Booker{reserve: broker.Reserve, recorder: recorder}, nil
}

func (b *Booker) Book(ctx context.Context, orderID, addressToken string, weightGrams int) (Booking, error) {
	// This adapter deliberately relies on its caller's context and makes a
	// single attempt; it owns neither a timeout nor retry policy.
	b.recorder.Record(telemetry.Event{Integration: "parcel", Operation: "book", Outcome: "attempt", OrderID: orderID, At: time.Now()})
	reservation, err := b.reserve(ctx, vendor.Consignment{
		OrderID: orderID, AddressToken: addressToken, WeightGrams: weightGrams,
	})
	if err != nil {
		b.recorder.Record(telemetry.Event{Integration: "parcel", Operation: "book", Outcome: "failure", OrderID: orderID, At: time.Now()})
		return Booking{}, fmt.Errorf("book parcel: %w", err)
	}
	return Booking{TrackingCode: reservation.TrackingCode, LabelURL: reservation.LabelURL}, nil
}
