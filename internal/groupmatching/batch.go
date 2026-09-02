package groupmatching

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

type pairBatch struct {
	pairRef string
}

func (compilation Compilation) batchesForProvider(provider llm.Provider) ([]pairBatch, error) {
	if len(compilation.pairs) == 0 {
		return []pairBatch{}, nil
	}
	result := make([]pairBatch, 0, len(compilation.pairs))
	for _, pair := range compilation.pairs {
		// A pair without a locally valid witness candidate cannot satisfy the
		// response contract. Its empty result is deterministic; the model has
		// no semantic selection to make and is never asked to acknowledge it.
		if len(pair.witnessCandidates) == 0 {
			continue
		}
		if provider == nil {
			return nil, fmt.Errorf("group matching: provider is required")
		}
		fits, err := compilation.requestFits(provider, pair.ref)
		if err != nil {
			return nil, err
		}
		if !fits {
			return nil, compilation.indivisiblePairError(pair.ref)
		}
		result = append(result, pairBatch{pairRef: pair.ref})
	}
	if err := compilation.validatePlan(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (compilation Compilation) requestFits(provider llm.Provider, pairRef string) (bool, error) {
	request, err := compilation.request(pairRef)
	if err != nil {
		return false, err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("group matching: encode provider request: %w", err)
	}
	prepared, err := provider.Prepare(llm.Prompt{
		System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
	}, limits())
	if err != nil {
		var resourceErr *llm.ResourceLimitError
		if errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes {
			return false, nil
		}
		return false, fmt.Errorf("group matching: prepare provider request: %w", err)
	}
	if prepared.Len() > llm.SemanticRecordByteLimit {
		return false, fmt.Errorf(
			"group matching: provider prepared %d bytes above the shared request envelope without returning request_bytes",
			prepared.Len(),
		)
	}
	return true, nil
}

func (compilation Compilation) indivisiblePairError(ref string) error {
	request, err := compilation.request(ref)
	if err != nil {
		return err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("group matching: encode indivisible pair request: %w", err)
	}
	return fmt.Errorf(
		"group matching: cross-target group pair %s with its complete dossier is indivisible at %d semantic JSON bytes plus prompt in the configured provider request envelope",
		ref, len(wire),
	)
}

func (compilation Compilation) validatePlan(plan []pairBatch) error {
	counts := make(map[string]int, len(compilation.pairs))
	for _, item := range plan {
		if item.pairRef == "" {
			return fmt.Errorf("group matching: request plan contains an empty batch")
		}
		if _, known := compilation.pairByRef[item.pairRef]; !known {
			return fmt.Errorf("group matching: request plan cites unknown pair %q", item.pairRef)
		}
		pair := compilation.pairByRef[item.pairRef]
		if len(pair.witnessCandidates) == 0 {
			return fmt.Errorf("group matching: request plan includes candidate-free pair %s", item.pairRef)
		}
		counts[item.pairRef]++
	}
	for _, pair := range compilation.pairs {
		expected := 0
		if len(pair.witnessCandidates) > 0 {
			expected = 1
		}
		if counts[pair.ref] != expected {
			return fmt.Errorf(
				"group matching: pair %s occurs %d times in request plan, want %d for its candidate authority",
				pair.ref, counts[pair.ref], expected,
			)
		}
	}
	return nil
}

func limits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes:  llm.SemanticRecordByteLimit,
		MaxResponseBytes: llm.ProviderResponseByteLimit,
		MaxOutputTokens:  outputTokenCount,
	}
}
