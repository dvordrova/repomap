package symbol

import (
	"context"
	"encoding/json"
	"fmt"
)

// Explainer is owned by the consumer so the rest of repomap can use a fixed
// DeepSeek response in tests without depending on HTTP details.
type Explainer interface {
	ExplainSymbol(context.Context, []byte) ([]byte, error)
}

type Service struct {
	explainer Explainer
}

type Explanation struct {
	Raw        []byte      `json:"-"`
	Parsed     ParseResult `json:"parsed"`
	Evaluation Evaluation  `json:"evaluation"`
}

func NewService(explainer Explainer) *Service {
	return &Service{explainer: explainer}
}

func (s *Service) Explain(ctx context.Context, bundle Bundle) (Explanation, error) {
	if s == nil || s.explainer == nil {
		return Explanation{}, fmt.Errorf("symbol: explainer is required")
	}
	if err := bundle.Validate(); err != nil {
		return Explanation{}, fmt.Errorf("symbol: invalid bundle: %w", err)
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return Explanation{}, fmt.Errorf("symbol: marshal bundle: %w", err)
	}
	raw, err := s.explainer.ExplainSymbol(ctx, bundleJSON)
	if err != nil {
		return Explanation{}, fmt.Errorf("symbol: explain: %w", err)
	}
	parsed, err := ParseReport(bundle, raw)
	if err != nil {
		return Explanation{}, fmt.Errorf("symbol: parse explanation: %w", err)
	}
	return Explanation{Raw: raw, Parsed: parsed, Evaluation: Evaluate(parsed)}, nil
}
