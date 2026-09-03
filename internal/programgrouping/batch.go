package programgrouping

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// maxGroupingRequestBytes bounds one grouping request. It is not the provider
// envelope, which is far larger, and not the token window, which nothing here
// counts: it is the size past which the provider refuses the request outright.
// Pointing repomap at its own checkout built a 15.4 MB grouping request for
// cmd/repomap and got back "maximum context length is 1048576 tokens"; the
// requests that work are smaller by an order of magnitude — python-dotenv's
// 2.4 MB and chi's router package at 1.6 MB both answer. Three megabytes
// leaves both of those in one request and splits only what was failing.
//
// Nothing is sampled or omitted. The subject cover is the same, spread over
// more requests, and the merge phase that then runs can no longer lose a
// target when it goes wrong.
const maxGroupingRequestBytes = 3 << 20

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
	remaining := compilation.categorizedRefs
	for len(remaining) > 0 {
		length, err := compilation.largestFittingPrefix(provider, remaining)
		if err != nil {
			return nil, err
		}
		if length == 0 {
			return nil, compilation.indivisibleSubjectError(remaining[0])
		}
		result = append(result, batch{groupRefs: append([]string(nil), remaining[:length]...)})
		remaining = remaining[length:]
	}
	if err := compilation.validatePlan(result); err != nil {
		return nil, err
	}
	return result, nil
}

// largestFittingPrefix returns how many of the leading refs fit in one
// request, or zero when even the first one does not.
//
// A request grows with every ref it carries, so fitting is monotone in the
// prefix length and the boundary can be found by search rather than by
// re-encoding the whole request once per added ref. That earlier walk cost one
// full JSON encoding per subject — quadratic in the number of subjects, and
// the single largest local cost of a cached run. Trying the whole remainder
// first makes the ordinary repository, where everything fits in one request,
// cost exactly one probe. The partition is unchanged wherever the old walk
// produced one batch, so no cache key moves.
func (compilation Compilation) largestFittingPrefix(provider llm.Provider, refs []string) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	fits, err := compilation.groupingRequestFits(provider, refs)
	if err != nil {
		return 0, err
	}
	if fits {
		return len(refs), nil
	}
	fits, err = compilation.groupingRequestFits(provider, refs[:1])
	if err != nil || !fits {
		return 0, err
	}
	fitting, tooLarge := 1, len(refs)
	for step := 2; step < tooLarge; step *= 2 {
		fits, err = compilation.groupingRequestFits(provider, refs[:step])
		if err != nil {
			return 0, err
		}
		if !fits {
			tooLarge = step
			break
		}
		fitting = step
	}
	for fitting+1 < tooLarge {
		middle := fitting + (tooLarge-fitting)/2
		fits, err = compilation.groupingRequestFits(provider, refs[:middle])
		if err != nil {
			return 0, err
		}
		if fits {
			fitting = middle
			continue
		}
		tooLarge = middle
	}
	return fitting, nil
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
	wire, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("program grouping: encode provider request: %w", err)
	}
	if len(wire) > maxGroupingRequestBytes {
		return false, nil
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
