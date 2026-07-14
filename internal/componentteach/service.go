package componentteach

import (
	"context"
	"encoding/json"
	"fmt"
)

// Teacher is the provider-facing port. It receives only a validated bounded
// Bundle encoded as JSON, never the local locator Index.
type Teacher interface {
	Teach(ctx context.Context, bundleJSON []byte) ([]byte, error)
}

type Service struct {
	teacher Teacher
}

func NewService(teacher Teacher) *Service {
	return &Service{teacher: teacher}
}

func (s *Service) Teach(ctx context.Context, bundle Bundle) (ParseResult, error) {
	if s == nil || s.teacher == nil {
		return ParseResult{}, fmt.Errorf("component teach: teacher is required")
	}
	if err := bundle.Validate(); err != nil {
		return ParseResult{}, err
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return ParseResult{}, fmt.Errorf("component teach: marshal bundle: %w", err)
	}
	raw, err := s.teacher.Teach(ctx, bundleJSON)
	if err != nil {
		return ParseResult{}, err
	}
	return ParseReport(bundle, raw)
}
