package sourceexplain

import (
	"context"
	"encoding/json"
	"fmt"
)

type Assessor interface {
	AssessSource(ctx context.Context, bundleJSON []byte) ([]byte, error)
}

type Service struct {
	assessor Assessor
}

type Explanation struct {
	Raw        []byte      `json:"-"`
	Parsed     ParseResult `json:"parsed"`
	Evaluation Evaluation  `json:"evaluation"`
}

func NewService(assessor Assessor) *Service {
	return &Service{assessor: assessor}
}

func (s *Service) Explain(ctx context.Context, bundle Bundle) (Explanation, error) {
	if s == nil || s.assessor == nil {
		return Explanation{}, fmt.Errorf("source explain: assessor is required")
	}
	if err := bundle.Validate(); err != nil {
		return Explanation{}, err
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return Explanation{}, fmt.Errorf("source explain: marshal bundle: %w", err)
	}
	raw, err := s.assessor.AssessSource(ctx, bundleJSON)
	if err != nil {
		return Explanation{}, err
	}
	explanation := Explanation{Raw: append([]byte{}, raw...)}
	parsed, err := ParseReport(bundle, raw)
	if err != nil {
		return explanation, err
	}
	explanation.Parsed = parsed
	explanation.Evaluation = Evaluate(parsed)
	return explanation, nil
}
