package componentstudy

import (
	"context"
	"encoding/json"
	"fmt"
)

// Planner is the smallest provider-facing port consumed by the component
// study service. Implementations receive only a validated bounded bundle.
type Planner interface {
	Plan(ctx context.Context, bundleJSON []byte) ([]byte, error)
}

type Service struct {
	planner Planner
}

func NewService(planner Planner) *Service {
	return &Service{planner: planner}
}

func (s *Service) Plan(ctx context.Context, bundle Bundle) (Result, error) {
	if s == nil || s.planner == nil {
		return Result{}, fmt.Errorf("component study: planner is required")
	}
	if err := bundle.Validate(); err != nil {
		return Result{}, err
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return Result{}, fmt.Errorf("component study: marshal bundle: %w", err)
	}
	raw, err := s.planner.Plan(ctx, bundleJSON)
	if err != nil {
		return Result{}, err
	}
	return ParsePlan(bundle, raw)
}
