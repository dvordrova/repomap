package parcel

import (
	"context"
	"errors"
)

type DialOptions struct {
	Endpoint string
	APIKey   string
	Depot    string
}

type Consignment struct {
	OrderID      string
	AddressToken string
	WeightGrams  int
}

type Reservation struct {
	TrackingCode string
	LabelURL     string
}

type Broker struct{ options DialOptions }

func Dial(options DialOptions) (*Broker, error) {
	if options.Endpoint == "" || options.APIKey == "" || options.Depot == "" {
		return nil, errors.New("parcel: incomplete dial options")
	}
	return &Broker{options: options}, nil
}

func (b *Broker) Reserve(ctx context.Context, consignment Consignment) (Reservation, error) {
	if err := ctx.Err(); err != nil {
		return Reservation{}, err
	}
	if consignment.OrderID == "" || consignment.AddressToken == "" {
		return Reservation{}, errors.New("parcel: incomplete consignment")
	}
	return Reservation{TrackingCode: "track-" + consignment.OrderID, LabelURL: b.options.Endpoint + "/labels/" + consignment.OrderID}, nil
}
