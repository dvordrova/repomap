package address

import (
	"context"
	"fmt"
	"time"

	"example.invalid/fulfilment/internal/model"
	"example.invalid/fulfilment/internal/telemetry"
	vendor "geo.example/locator"
)

type Configuration struct {
	ServiceURL string
	AccessKey  string
	RuleSet    string
}

type Verification struct {
	Token      string
	Normalized model.Address
}

type remoteVerifier interface {
	Validate(context.Context, vendor.Candidate) (vendor.Result, error)
}

type Verifier interface {
	Verify(context.Context, string, model.Address) (Verification, error)
}

type Adapter struct {
	remote   remoteVerifier
	recorder telemetry.Recorder
}

func New(configuration Configuration, recorder telemetry.Recorder) (*Adapter, error) {
	remote, err := vendor.New(configuration.ServiceURL, configuration.AccessKey, configuration.RuleSet)
	if err != nil {
		return nil, fmt.Errorf("configure address verification: %w", err)
	}
	return &Adapter{remote: remote, recorder: recorder}, nil
}

func (a *Adapter) Verify(ctx context.Context, orderID string, address model.Address) (Verification, error) {
	a.recorder.Record(telemetry.Event{Integration: "address", Operation: "verify", Outcome: "attempt", OrderID: orderID, At: time.Now()})
	result, err := a.remote.Validate(ctx, vendor.Candidate{
		Street: address.Street, City: address.City, PostalCode: address.PostalCode, Country: address.Country,
	})
	if err != nil {
		a.recorder.Record(telemetry.Event{Integration: "address", Operation: "verify", Outcome: "failure", OrderID: orderID, At: time.Now()})
		return Verification{}, fmt.Errorf("verify address: %w", err)
	}
	if !result.Deliverable {
		a.recorder.Record(telemetry.Event{Integration: "address", Operation: "verify", Outcome: "failure", OrderID: orderID, At: time.Now()})
		return Verification{}, fmt.Errorf("verify address: not deliverable")
	}
	return Verification{Token: result.ID, Normalized: model.Address{
		Street: result.Normalized.Street, City: result.Normalized.City,
		PostalCode: result.Normalized.PostalCode, Country: result.Normalized.Country,
	}}, nil
}
