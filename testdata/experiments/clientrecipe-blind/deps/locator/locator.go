package locator

import (
	"context"
	"errors"
)

type Candidate struct {
	Street     string
	City       string
	PostalCode string
	Country    string
}

type Result struct {
	ID          string
	Deliverable bool
	Normalized  Candidate
}

type Service struct {
	endpoint string
	key      string
	profile  string
}

func New(endpoint, key string, profile string) (*Service, error) {
	if endpoint == "" || key == "" || profile == "" {
		return nil, errors.New("locator: incomplete configuration")
	}
	return &Service{endpoint: endpoint, key: key, profile: profile}, nil
}

func (s *Service) Validate(ctx context.Context, candidate Candidate) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if candidate.Street == "" || candidate.PostalCode == "" || candidate.Country == "" {
		return Result{Deliverable: false}, nil
	}
	return Result{ID: "address-verified", Deliverable: true, Normalized: candidate}, nil
}
