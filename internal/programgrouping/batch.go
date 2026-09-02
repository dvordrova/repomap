package programgrouping

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

type batch struct {
	groupRefs []string
}

func splitBatch(value batch) (batch, batch, bool) {
	if len(value.groupRefs) < 2 {
		return batch{}, batch{}, false
	}
	middle := len(value.groupRefs) / 2
	return batch{groupRefs: append([]string(nil), value.groupRefs[:middle]...)},
		batch{groupRefs: append([]string(nil), value.groupRefs[middle:]...)}, true
}

func (compilation Compilation) batchesForProvider(provider llm.Provider) ([]batch, error) {
	if provider == nil {
		return nil, fmt.Errorf("program grouping: provider is required")
	}
	if len(compilation.categorizedRefs) == 0 {
		return []batch{}, nil
	}
	result := make([]batch, 0)
	current := make([]string, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		result = append(result, batch{groupRefs: append([]string(nil), current...)})
		current = current[:0]
	}
	for _, ref := range compilation.categorizedRefs {
		probe := append(append([]string(nil), current...), ref)
		fits, err := compilation.groupingRequestFits(provider, probe)
		if err != nil {
			return nil, err
		}
		if fits {
			current = probe
			continue
		}
		if len(current) == 0 {
			return nil, compilation.indivisibleSubjectError(ref)
		}
		flush()
		fits, err = compilation.groupingRequestFits(provider, []string{ref})
		if err != nil {
			return nil, err
		}
		if !fits {
			return nil, compilation.indivisibleSubjectError(ref)
		}
		current = []string{ref}
	}
	flush()
	if err := compilation.validatePlan(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (compilation Compilation) indivisibleSubjectError(ref string) error {
	request, err := compilation.request(phaseGrouping, []string{ref}, proposalSet{})
	if err != nil {
		return err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("program grouping: encode indivisible subject request: %w", err)
	}
	return fmt.Errorf(
		"program grouping: categorized subject %s with its complete incident graph is indivisible at %d semantic JSON bytes plus prompt in the configured provider request envelope",
		ref, len(wire),
	)
}

func (compilation Compilation) groupingRequestFits(provider llm.Provider, refs []string) (bool, error) {
	request, err := compilation.request(phaseGrouping, refs, proposalSet{})
	if err != nil {
		return false, err
	}
	return requestFits(provider, request)
}

func requestFits(provider llm.Provider, request Request) (bool, error) {
	if provider == nil {
		return false, fmt.Errorf("program grouping: provider is required")
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("program grouping: encode provider request: %w", err)
	}
	prepared, err := provider.Prepare(llm.Prompt{
		System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
	}, limits())
	if err != nil {
		var resourceErr *llm.ResourceLimitError
		if errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes {
			return false, nil
		}
		return false, fmt.Errorf("program grouping: prepare provider request: %w", err)
	}
	if prepared.Len() > llm.SemanticRecordByteLimit {
		return false, fmt.Errorf(
			"program grouping: provider prepared %d bytes above the shared request envelope without returning request_bytes",
			prepared.Len(),
		)
	}
	return true, nil
}

func (compilation Compilation) validatePlan(plan []batch) error {
	counts := make(map[string]int, len(compilation.categorizedRefs))
	for _, item := range plan {
		if len(item.groupRefs) == 0 {
			return fmt.Errorf("program grouping: request plan contains an empty batch")
		}
		for _, ref := range item.groupRefs {
			if _, known := compilation.categorizedRefSet[ref]; !known {
				return fmt.Errorf("program grouping: request plan cites unknown or unclassified ref %q", ref)
			}
			counts[ref]++
		}
	}
	for _, ref := range compilation.categorizedRefs {
		if counts[ref] != 1 {
			return fmt.Errorf("program grouping: categorized subject %s occurs %d times in request plan", ref, counts[ref])
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
