package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

type classificationBatch struct {
	compilation Compilation
}

// classificationBatches returns a deterministic disjoint cover of the
// complete candidate reservoir. Authority arrays are projected only to refs
// present in each batch; their bound-vs-unbound distinction is preserved.
func classificationBatches(compilation Compilation) ([]classificationBatch, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	result := make([]classificationBatch, 0)
	current := make([]Candidate, 0)
	packing := newClassificationPacking(compilation)
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		batchCompilation, err := compileSubset(compilation, current)
		if err != nil {
			return err
		}
		if len(batchCompilation.wire) > MaxRequestBytes && len(current) != 1 {
			return fmt.Errorf(
				"target portfolio: classification batch requires %d bytes; provider batch window is %d",
				len(batchCompilation.wire), MaxRequestBytes,
			)
		}
		if packing.size != len(batchCompilation.wire) {
			return fmt.Errorf(
				"target portfolio: classification packing size mismatch: planned %d, encoded %d",
				packing.size, len(batchCompilation.wire),
			)
		}
		result = append(result, classificationBatch{compilation: batchCompilation})
		current = current[:0]
		packing = newClassificationPacking(compilation)
		return nil
	}
	for index, candidate := range compilation.candidates {
		visible := compilation.Request.Candidates[index]
		if visible.FileRef != candidate.FileRef {
			return nil, fmt.Errorf("target portfolio: classification packing authority mismatch")
		}
		rowWire, err := json.Marshal(visible)
		if err != nil {
			return nil, fmt.Errorf("target portfolio: encode classification candidate: %w", err)
		}
		projected := packing.projectedSize(candidate.FileRef, len(rowWire))
		if projected <= MaxRequestBytes {
			current = append(current, cloneCandidate(candidate))
			packing.add(candidate.FileRef, len(rowWire))
			continue
		}
		if len(current) == 0 {
			// Preserve the complete indivisible row. The ordinary Run path asks
			// the provider to prepare it against the real semantic-record
			// envelope; this local packing window is diagnostic only.
			current = append(current, cloneCandidate(candidate))
			packing.add(candidate.FileRef, len(rowWire))
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		current = []Candidate{cloneCandidate(candidate)}
		packing.add(candidate.FileRef, len(rowWire))
		if packing.size > MaxRequestBytes {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("target portfolio: classification packing produced no batches")
	}
	covered := 0
	for _, batch := range result {
		covered += len(batch.compilation.candidates)
	}
	if covered != len(compilation.candidates) {
		return nil, fmt.Errorf("target portfolio: classification batches do not cover complete candidate authority")
	}
	return result, nil
}

func classificationBatchesWithFit(
	compilation Compilation,
	fits func([]byte) (bool, error),
) ([]classificationBatch, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	if fits == nil {
		return nil, fmt.Errorf("target portfolio: provider request envelope is missing")
	}
	result := make([]classificationBatch, 0)
	for start := 0; start < len(compilation.candidates); {
		low, high, accepted := 1, len(compilation.candidates)-start, 0
		var acceptedCompilation Compilation
		for low <= high {
			count := low + (high-low)/2
			probe, err := compileSubset(compilation, compilation.candidates[start:start+count])
			if err != nil {
				return nil, err
			}
			ok, fitErr := fits(probe.wire)
			if fitErr != nil {
				return nil, fmt.Errorf("target portfolio: prepare classification batch: %w", fitErr)
			}
			if !ok {
				high = count - 1
				continue
			}
			accepted = count
			acceptedCompilation = probe
			low = count + 1
		}
		if accepted == 0 {
			return nil, fmt.Errorf(
				"target portfolio: exact candidate %s is indivisible in the provider request envelope",
				compilation.candidates[start].FileRef,
			)
		}
		result = append(result, classificationBatch{compilation: acceptedCompilation})
		start += accepted
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("target portfolio: classification packing produced no batches")
	}
	return result, nil
}

type classificationPacking struct {
	size                int
	candidates          int
	requiredRefs        int
	executableRefs      int
	requiredAuthority   map[corpus.FileID]struct{}
	executableAuthority map[corpus.FileID]struct{}
	requiredBound       bool
	executableBound     bool
}

func newClassificationPacking(compilation Compilation) classificationPacking {
	packing := classificationPacking{
		size:                len(`{"candidates":[]}`),
		requiredAuthority:   make(map[corpus.FileID]struct{}, len(compilation.requiredTargetFileRefs)),
		executableAuthority: make(map[corpus.FileID]struct{}, len(compilation.executableFileRefs)),
		requiredBound:       compilation.requiredAuthorityBound,
		executableBound:     compilation.executableAuthorityBound,
	}
	if packing.requiredBound {
		packing.size += len(`,"required_target_file_refs":[]`)
	}
	if packing.executableBound {
		packing.size += len(`,"executable_file_refs":[]`)
	}
	for _, ref := range compilation.requiredTargetFileRefs {
		packing.requiredAuthority[ref] = struct{}{}
	}
	for _, ref := range compilation.executableFileRefs {
		packing.executableAuthority[ref] = struct{}{}
	}
	return packing
}

func (packing classificationPacking) projectedSize(ref corpus.FileID, rowBytes int) int {
	size := packing.size + rowBytes
	if packing.candidates > 0 {
		size++
	}
	refWire, _ := json.Marshal(ref)
	if _, required := packing.requiredAuthority[ref]; required {
		size += len(refWire)
		if packing.requiredRefs > 0 {
			size++
		}
	}
	if _, executable := packing.executableAuthority[ref]; executable {
		size += len(refWire)
		if packing.executableRefs > 0 {
			size++
		}
	}
	return size
}

func (packing *classificationPacking) add(ref corpus.FileID, rowBytes int) {
	packing.size = packing.projectedSize(ref, rowBytes)
	packing.candidates++
	if _, required := packing.requiredAuthority[ref]; required {
		packing.requiredRefs++
	}
	if _, executable := packing.executableAuthority[ref]; executable {
		packing.executableRefs++
	}
}

func compileSubset(compilation Compilation, candidates []Candidate) (Compilation, error) {
	executableRefs := authorityRefsInCandidates(compilation.executableFileRefs, candidates)
	requiredRefs := authorityRefsInCandidates(compilation.requiredTargetFileRefs, candidates)
	return compile(
		compilation.corpus,
		cloneCandidates(candidates),
		compilation.executableAuthorityBound,
		executableRefs,
		compilation.requiredAuthorityBound,
		requiredRefs,
	)
}

func authorityRefsInCandidates(authority []corpus.FileID, candidates []Candidate) []corpus.FileID {
	if authority == nil {
		return nil
	}
	allowed := make(map[corpus.FileID]struct{}, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.FileRef] = struct{}{}
	}
	result := make([]corpus.FileID, 0)
	for _, ref := range authority {
		if _, ok := allowed[ref]; ok {
			result = append(result, ref)
		}
	}
	if result == nil {
		return []corpus.FileID{}
	}
	return result
}

type defaultBatch struct {
	request   DefaultRequest
	wire      []byte
	authority map[corpus.FileID]VisibleCandidate
}

func defaultBatches(compilation Compilation, refs []corpus.FileID) ([]defaultBatch, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	if len(refs) < 2 {
		return nil, fmt.Errorf("target portfolio: default comparison requires at least two refs")
	}
	result := make([]defaultBatch, 0)
	current := make([]corpus.FileID, 0)
	currentSize := len(`{"phase":"default_choice","candidates":[]}`)
	authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
	for _, candidate := range compilation.Request.Candidates {
		authority[candidate.FileRef] = candidate
	}
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		batch, err := compileDefaultBatch(compilation, current)
		if err != nil {
			return err
		}
		if len(batch.wire) != currentSize {
			return fmt.Errorf(
				"target portfolio: default packing size mismatch: planned %d, encoded %d",
				currentSize, len(batch.wire),
			)
		}
		result = append(result, batch)
		current = current[:0]
		currentSize = len(`{"phase":"default_choice","candidates":[]}`)
		return nil
	}
	for _, ref := range refs {
		candidate, known := authority[ref]
		if !known {
			return nil, fmt.Errorf("target portfolio: default comparison cites unknown ref")
		}
		rowWire, err := json.Marshal(candidate)
		if err != nil {
			return nil, fmt.Errorf("target portfolio: encode default candidate: %w", err)
		}
		projected := currentSize + len(rowWire)
		if len(current) > 0 {
			projected++
		}
		if projected <= MaxDefaultRequestBytes {
			current = append(current, ref)
			currentSize = projected
			continue
		}
		if len(current) == 0 {
			current = append(current, ref)
			currentSize = projected
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		current = []corpus.FileID{ref}
		currentSize += len(rowWire)
		if currentSize > MaxDefaultRequestBytes {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return result, nil
}

func defaultBatchesWithFit(
	compilation Compilation,
	refs []corpus.FileID,
	fits func([]byte) (bool, error),
) ([]defaultBatch, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	if len(refs) < 2 {
		return nil, fmt.Errorf("target portfolio: default comparison requires at least two refs")
	}
	if fits == nil {
		return nil, fmt.Errorf("target portfolio: provider comparison envelope is missing")
	}
	result := make([]defaultBatch, 0)
	for start := 0; start < len(refs); {
		low, high, accepted := 1, len(refs)-start, 0
		var acceptedBatch defaultBatch
		for low <= high {
			count := low + (high-low)/2
			probe, err := compileDefaultBatch(compilation, refs[start:start+count])
			if err != nil {
				return nil, err
			}
			ok, fitErr := fits(probe.wire)
			if fitErr != nil {
				return nil, fmt.Errorf("target portfolio: prepare default comparison: %w", fitErr)
			}
			if !ok {
				high = count - 1
				continue
			}
			accepted = count
			acceptedBatch = probe
			low = count + 1
		}
		if accepted == 0 {
			return nil, fmt.Errorf(
				"target portfolio: exact default candidate %s is indivisible in the provider request envelope",
				refs[start],
			)
		}
		result = append(result, acceptedBatch)
		start += accepted
	}
	return result, nil
}

func defaultRequestForRefs(compilation Compilation, refs []corpus.FileID) (DefaultRequest, error) {
	authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
	for _, candidate := range compilation.Request.Candidates {
		authority[candidate.FileRef] = candidate
	}
	request := DefaultRequest{Phase: "default_choice", Candidates: make([]VisibleCandidate, 0, len(refs))}
	seen := make(map[corpus.FileID]struct{}, len(refs))
	for _, ref := range refs {
		candidate, ok := authority[ref]
		if !ok {
			return DefaultRequest{}, fmt.Errorf("target portfolio: default comparison cites unknown ref")
		}
		if _, duplicate := seen[ref]; duplicate {
			return DefaultRequest{}, fmt.Errorf("target portfolio: default comparison repeats a ref")
		}
		seen[ref] = struct{}{}
		request.Candidates = append(request.Candidates, cloneVisibleCandidate(candidate))
	}
	return request, nil
}

func compileDefaultBatch(compilation Compilation, refs []corpus.FileID) (defaultBatch, error) {
	request, err := defaultRequestForRefs(compilation, refs)
	if err != nil {
		return defaultBatch{}, err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return defaultBatch{}, fmt.Errorf("target portfolio: encode default comparison: %w", err)
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return defaultBatch{}, fmt.Errorf("target portfolio: default comparison contains credential-shaped content")
	}
	authority := make(map[corpus.FileID]VisibleCandidate, len(request.Candidates))
	for _, candidate := range request.Candidates {
		authority[candidate.FileRef] = cloneVisibleCandidate(candidate)
	}
	return defaultBatch{request: request, wire: append([]byte(nil), wire...), authority: authority}, nil
}

func (batch defaultBatch) buildPrompt() (llm.Prompt, error) {
	if err := validateDefaultBatch(batch); err != nil {
		return llm.Prompt{}, err
	}
	return llm.Prompt{
		System:             defaultPromptSystem,
		User:               fmt.Sprintf(defaultPromptUserShape, batch.wire),
		ResponseFormatJSON: true,
	}, nil
}

func (batch defaultBatch) resolve(raw []byte) (Selection, error) {
	if err := validateDefaultBatch(batch); err != nil {
		return Selection{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Selection{}, fmt.Errorf("target portfolio: default response exceeds bounded envelope")
	}
	if _, found := secretscan.Detect(string(raw)); found {
		return Selection{}, fmt.Errorf("target portfolio: default response contains credential-shaped content")
	}
	var response DefaultResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil || ensureJSONEOF(decoder) != nil {
		return Selection{}, fmt.Errorf("target portfolio: invalid default JSON response")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 1 || fields["default_file_ref"] == nil {
		return Selection{}, fmt.Errorf("target portfolio: default response must contain exactly default_file_ref")
	}
	candidate, known := batch.authority[response.DefaultFileRef]
	if !known {
		return Selection{}, fmt.Errorf("target portfolio: default response cites unknown file_ref")
	}
	defaultCandidate := cloneVisibleCandidate(candidate)
	return Selection{Default: &defaultCandidate, Targets: []VisibleCandidate{cloneVisibleCandidate(candidate)}}, nil
}

func validateDefaultBatch(batch defaultBatch) error {
	if len(batch.request.Candidates) == 0 || len(batch.authority) != len(batch.request.Candidates) {
		return fmt.Errorf("target portfolio: invalid default comparison authority")
	}
	for _, candidate := range batch.request.Candidates {
		if authority, ok := batch.authority[candidate.FileRef]; !ok || !reflect.DeepEqual(candidate, authority) {
			return fmt.Errorf("target portfolio: default comparison authority mismatch")
		}
	}
	wire, err := json.Marshal(batch.request)
	if err != nil || !reflect.DeepEqual(wire, batch.wire) {
		return fmt.Errorf("target portfolio: default comparison wire binding mismatch")
	}
	return nil
}
