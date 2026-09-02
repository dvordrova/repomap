package programcategorization

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

const (
	MaxRequestBytes  = llm.SemanticRecordByteLimit
	MaxResponseBytes = llm.ProviderResponseByteLimit
)

type batch struct {
	subjectRefs       []string
	documentationRefs []string
}

func splitBatch(value batch) (batch, batch, bool) {
	if len(value.subjectRefs) < 2 {
		return batch{}, batch{}, false
	}
	middle := len(value.subjectRefs) / 2
	return batch{
		subjectRefs:       append([]string(nil), value.subjectRefs[:middle]...),
		documentationRefs: append([]string(nil), value.documentationRefs...),
	}, batch{
		subjectRefs:       append([]string(nil), value.subjectRefs[middle:]...),
		documentationRefs: append([]string(nil), value.documentationRefs...),
	}, true
}

// batchesForProvider returns a deterministic disjoint cover of categorization
// subjects. A bounded number of owned refs keeps each semantic decision local;
// it is only a request-planning boundary and never samples or omits a subject.
// Complete reduced documentation is repeated in every request when it fits
// beside every indivisible subject. Otherwise its rows are partitioned
// losslessly across one-subject requests.
func (compilation Compilation) batchesForProvider(provider llm.Provider) ([]batch, error) {
	subjectRefs := make([]string, 0, len(compilation.subjects))
	for _, subject := range compilation.subjects {
		subjectRefs = append(subjectRefs, subject.ref)
	}
	if len(subjectRefs) == 0 {
		return []batch{}, nil
	}
	documentationRefs := make([]string, 0, len(compilation.documentationRows))
	for _, row := range compilation.documentationRows {
		documentationRefs = append(documentationRefs, row.ref)
	}

	canRepeatDocumentation := true
	for _, ref := range subjectRefs {
		fits, err := compilation.batchFits(provider, []string{ref}, documentationRefs)
		if err != nil {
			return nil, err
		}
		if !fits {
			canRepeatDocumentation = false
			break
		}
	}
	if canRepeatDocumentation {
		result, err := compilation.packSubjects(provider, subjectRefs, documentationRefs)
		if err != nil {
			return nil, err
		}
		if err := compilation.validatePlan(result); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Full reduced documentation does not fit beside at least one indivisible
	// subject. Partition only the documentation reservoir, then execute the
	// complete subject cover once for every documentation shard. This preserves
	// the ProgramIndex × documentation context instead of assigning arbitrary
	// documentation subsets to different subjects.
	documentationShards := make([][]string, 0)
	currentDocumentation := make([]string, 0)
	for _, documentationRef := range documentationRefs {
		probe := append(append([]string(nil), currentDocumentation...), documentationRef)
		fitsAll, err := compilation.documentationFitsEverySubject(provider, subjectRefs, probe)
		if err != nil {
			return nil, err
		}
		if fitsAll {
			currentDocumentation = probe
			continue
		}
		if len(currentDocumentation) == 0 {
			row := compilation.documentationByRef[documentationRef]
			return nil, fmt.Errorf(
				"program categorization: reduced documentation row %s (%s, %s) is indivisible beside at least one complete subject graph",
				documentationRef, row.path, row.kind,
			)
		}
		documentationShards = append(documentationShards, append([]string(nil), currentDocumentation...))
		currentDocumentation = []string{documentationRef}
		fitsAll, err = compilation.documentationFitsEverySubject(provider, subjectRefs, currentDocumentation)
		if err != nil {
			return nil, err
		}
		if !fitsAll {
			row := compilation.documentationByRef[documentationRef]
			return nil, fmt.Errorf(
				"program categorization: reduced documentation row %s (%s, %s) is indivisible beside at least one complete subject graph",
				documentationRef, row.path, row.kind,
			)
		}
	}
	if len(currentDocumentation) > 0 {
		documentationShards = append(documentationShards, append([]string(nil), currentDocumentation...))
	}
	result := make([]batch, 0)
	for _, shard := range documentationShards {
		shardBatches, err := compilation.packSubjects(provider, subjectRefs, shard)
		if err != nil {
			return nil, err
		}
		result = append(result, shardBatches...)
	}
	if err := compilation.validatePlan(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (compilation Compilation) packSubjects(
	provider llm.Provider,
	subjectRefs []string,
	documentationRefs []string,
) ([]batch, error) {
	result := make([]batch, 0)
	current := make([]string, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		result = append(result, batch{
			subjectRefs:       append([]string(nil), current...),
			documentationRefs: append([]string(nil), documentationRefs...),
		})
		current = current[:0]
	}
	for _, ref := range subjectRefs {
		if len(current) == ownedSubjectsPerRequest {
			flush()
		}
		probe := append(append([]string(nil), current...), ref)
		fits, err := compilation.batchFits(provider, probe, documentationRefs)
		if err != nil {
			return nil, err
		}
		if fits {
			current = probe
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf("program categorization: indivisible subject %s does not fit provider envelope", ref)
		}
		flush()
		current = append(current, ref)
	}
	flush()
	return result, nil
}

func (compilation Compilation) documentationFitsEverySubject(
	provider llm.Provider,
	subjectRefs []string,
	documentationRefs []string,
) (bool, error) {
	for _, subjectRef := range subjectRefs {
		fits, err := compilation.batchFits(provider, []string{subjectRef}, documentationRefs)
		if err != nil {
			return false, err
		}
		if !fits {
			return false, nil
		}
	}
	return true, nil
}

func (compilation Compilation) batchFits(
	provider llm.Provider,
	subjectRefs []string,
	documentationRefs []string,
) (bool, error) {
	if provider == nil {
		return false, fmt.Errorf("program categorization: provider is nil")
	}
	request, err := compilation.request(subjectRefs, documentationRefs)
	if err != nil {
		return false, err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("program categorization: encode provider request: %w", err)
	}
	prepared, err := provider.Prepare(llm.Prompt{
		System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
	}, limits())
	if err != nil {
		var resourceErr *llm.ResourceLimitError
		if errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes {
			return false, nil
		}
		return false, fmt.Errorf("program categorization: prepare provider request: %w", err)
	}
	if prepared.Len() > MaxRequestBytes {
		return false, fmt.Errorf(
			"program categorization: provider prepared %d bytes above the %d-byte shared envelope without returning request_bytes",
			prepared.Len(), MaxRequestBytes,
		)
	}
	return true, nil
}

func (compilation Compilation) validatePlan(plan []batch) error {
	subjectCounts := make(map[string]int, len(compilation.subjects))
	subjectDocumentation := make(map[string]map[string]int, len(compilation.subjects))
	for _, item := range plan {
		if len(item.subjectRefs) == 0 {
			return fmt.Errorf("program categorization: request plan contains an empty subject batch")
		}
		if len(item.subjectRefs) > ownedSubjectsPerRequest {
			return fmt.Errorf(
				"program categorization: request plan owns %d subjects above semantic request granularity %d",
				len(item.subjectRefs), ownedSubjectsPerRequest,
			)
		}
		for _, ref := range item.subjectRefs {
			subjectCounts[ref]++
			if subjectDocumentation[ref] == nil {
				subjectDocumentation[ref] = make(map[string]int)
			}
			for _, documentationRef := range item.documentationRefs {
				subjectDocumentation[ref][documentationRef]++
			}
		}
	}
	for _, subject := range compilation.subjects {
		if len(compilation.documentationRows) == 0 {
			if subjectCounts[subject.ref] != 1 {
				return fmt.Errorf("program categorization: subject %s occurs %d times in request plan", subject.ref, subjectCounts[subject.ref])
			}
			continue
		}
		for _, row := range compilation.documentationRows {
			if subjectDocumentation[subject.ref][row.ref] != 1 {
				return fmt.Errorf(
					"program categorization: subject/documentation pair %s/%s occurs %d times in request plan",
					subject.ref, row.ref, subjectDocumentation[subject.ref][row.ref],
				)
			}
		}
	}
	return nil
}

func limits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes: MaxRequestBytes, MaxResponseBytes: MaxResponseBytes,
		MaxOutputTokens: maxOutputTokens,
	}
}
