package targetportfolio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

// Compile resolves every universally merged FileRef through the exact corpus.
// It does not rank, filter, merge duplicate FileRefs, or truncate candidates.
func Compile(snapshot corpus.Snapshot, candidates []Candidate) (Compilation, error) {
	return compile(snapshot, candidates, false, nil, false, nil)
}

// CompileWithExecutableAuthority resolves the complete exact set of
// executable-capable candidate refs in addition to the generic candidate
// surface. Authority refs are canonicalized in corpus order and deduplicated;
// every ref must name a current candidate. An explicitly empty set remains
// bound and provider-visible as a non-null empty array.
func CompileWithExecutableAuthority(
	snapshot corpus.Snapshot,
	candidates []Candidate,
	executableFileRefs []corpus.FileID,
) (Compilation, error) {
	return compile(snapshot, candidates, true, executableFileRefs, false, nil)
}

// CompileWithRequiredTargetAuthority binds canonical file representatives for
// exact targets established by deterministic language adapters. The provider
// may choose the default and retain additional guidance candidates, but it
// cannot suppress this set.
func CompileWithRequiredTargetAuthority(
	snapshot corpus.Snapshot,
	candidates []Candidate,
	requiredTargetFileRefs []corpus.FileID,
) (Compilation, error) {
	return compile(snapshot, candidates, false, nil, true, requiredTargetFileRefs)
}

func compile(
	snapshot corpus.Snapshot,
	candidates []Candidate,
	executableAuthorityBound bool,
	executableFileRefs []corpus.FileID,
	requiredAuthorityBound bool,
	requiredTargetFileRefs []corpus.FileID,
) (Compilation, error) {
	ownedCorpus, err := snapshot.Owned()
	if err != nil {
		return Compilation{}, fmt.Errorf("target portfolio: corpus: %w", err)
	}
	canonical, err := canonicalCandidates(ownedCorpus, candidates)
	if err != nil {
		return Compilation{}, err
	}
	visible, err := visibleCandidates(ownedCorpus, canonical)
	if err != nil {
		return Compilation{}, err
	}
	canonicalExecutableFileRefs := []corpus.FileID(nil)
	var requestExecutableFileRefs *[]corpus.FileID
	if executableAuthorityBound {
		canonicalExecutableFileRefs, err = canonicalExecutableRefs(canonical, executableFileRefs)
		if err != nil {
			return Compilation{}, err
		}
		requestRefs := cloneFileRefs(canonicalExecutableFileRefs)
		requestExecutableFileRefs = &requestRefs
	}
	canonicalRequiredTargetFileRefs := []corpus.FileID(nil)
	var requestRequiredTargetFileRefs *[]corpus.FileID
	if requiredAuthorityBound {
		canonicalRequiredTargetFileRefs, err = canonicalRequiredTargetRefs(canonical, requiredTargetFileRefs)
		if err != nil {
			return Compilation{}, fmt.Errorf("target portfolio: required target authority: %w", err)
		}
		requestRefs := cloneFileRefs(canonicalRequiredTargetFileRefs)
		requestRequiredTargetFileRefs = &requestRefs
	}
	request := Request{
		Candidates:             visible,
		ExecutableFileRefs:     requestExecutableFileRefs,
		RequiredTargetFileRefs: requestRequiredTargetFileRefs,
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("target portfolio: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf(
			"target portfolio: complete candidate request is %d bytes, limit is %d",
			len(wire), MaxRequestBytes,
		)
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return Compilation{}, fmt.Errorf("target portfolio: provider request contains credential-shaped content")
	}
	state, err := compileState(
		ownedCorpus, canonical, executableAuthorityBound, canonicalExecutableFileRefs,
		requiredAuthorityBound, canonicalRequiredTargetFileRefs, wire,
	)
	if err != nil {
		return Compilation{}, err
	}
	compilation := Compilation{
		Request:       request,
		RequestSHA256: sha256Hex(wire),
		wire:          append([]byte(nil), wire...),
		state:         append([]byte(nil), state...),
		corpus:        ownedCorpus,
		candidates:    cloneCandidates(canonical),

		executableAuthorityBound: executableAuthorityBound,
		executableFileRefs:       cloneFileRefs(canonicalExecutableFileRefs),
		requiredAuthorityBound:   requiredAuthorityBound,
		requiredTargetFileRefs:   cloneFileRefs(canonicalRequiredTargetFileRefs),
	}
	compilation.sealed = compilationSeal(compilation.state)
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

// ExecutionState returns the exact cache identity owned by this compilation.
// It binds prompt, preparation, and response-schema versions plus hashes of
// canonical corpus, candidate, executable-authority, and provider-request
// bytes.
func ExecutionState(compilation Compilation) ([]byte, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	return append([]byte(nil), compilation.state...), nil
}

func validateCompilation(compilation Compilation) error {
	if err := compilation.corpus.Validate(); err != nil {
		return fmt.Errorf("target portfolio: private corpus: %w", err)
	}
	canonical, err := canonicalCandidates(compilation.corpus, compilation.candidates)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonical, compilation.candidates) {
		return fmt.Errorf("target portfolio: candidate authority mismatch")
	}
	visible, err := visibleCandidates(compilation.corpus, canonical)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(compilation.Request.Candidates, visible) {
		return fmt.Errorf("target portfolio: visible candidate authority mismatch")
	}
	if compilation.executableAuthorityBound {
		canonicalExecutableFileRefs, err := canonicalExecutableRefs(canonical, compilation.executableFileRefs)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(canonicalExecutableFileRefs, compilation.executableFileRefs) ||
			compilation.Request.ExecutableFileRefs == nil ||
			!reflect.DeepEqual(*compilation.Request.ExecutableFileRefs, canonicalExecutableFileRefs) {
			return fmt.Errorf("target portfolio: executable authority mismatch")
		}
	} else if compilation.Request.ExecutableFileRefs != nil || len(compilation.executableFileRefs) != 0 {
		return fmt.Errorf("target portfolio: unexpected executable authority")
	}
	if compilation.requiredAuthorityBound {
		canonicalRequiredTargetFileRefs, err := canonicalRequiredTargetRefs(canonical, compilation.requiredTargetFileRefs)
		if err != nil {
			return fmt.Errorf("target portfolio: required target authority: %w", err)
		}
		if !reflect.DeepEqual(canonicalRequiredTargetFileRefs, compilation.requiredTargetFileRefs) ||
			compilation.Request.RequiredTargetFileRefs == nil ||
			!reflect.DeepEqual(*compilation.Request.RequiredTargetFileRefs, canonicalRequiredTargetFileRefs) {
			return fmt.Errorf("target portfolio: required target authority mismatch")
		}
	} else if compilation.Request.RequiredTargetFileRefs != nil || len(compilation.requiredTargetFileRefs) != 0 {
		return fmt.Errorf("target portfolio: unexpected required target authority")
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil {
		return fmt.Errorf("target portfolio: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes || !reflect.DeepEqual(wire, compilation.wire) ||
		compilation.RequestSHA256 != sha256Hex(wire) {
		return fmt.Errorf("target portfolio: request wire binding mismatch")
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return fmt.Errorf("target portfolio: provider request contains credential-shaped content")
	}
	wantState, err := compileState(
		compilation.corpus,
		canonical,
		compilation.executableAuthorityBound,
		compilation.executableFileRefs,
		compilation.requiredAuthorityBound,
		compilation.requiredTargetFileRefs,
		wire,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(wantState, compilation.state) ||
		compilation.sealed != compilationSeal(wantState) {
		return fmt.Errorf("target portfolio: compilation state binding mismatch")
	}
	return nil
}

func canonicalExecutableRefs(candidates []Candidate, executableFileRefs []corpus.FileID) ([]corpus.FileID, error) {
	return canonicalAuthorityRefs(candidates, executableFileRefs, "executable")
}

func canonicalRequiredTargetRefs(candidates []Candidate, requiredFileRefs []corpus.FileID) ([]corpus.FileID, error) {
	return canonicalAuthorityRefs(candidates, requiredFileRefs, "required target")
}

func canonicalAuthorityRefs(candidates []Candidate, fileRefs []corpus.FileID, label string) ([]corpus.FileID, error) {
	known := make(map[corpus.FileID]struct{}, len(candidates))
	for _, candidate := range candidates {
		known[candidate.FileRef] = struct{}{}
	}
	selected := make(map[corpus.FileID]struct{}, len(fileRefs))
	for _, fileRef := range fileRefs {
		if _, ok := known[fileRef]; !ok {
			return nil, fmt.Errorf("target portfolio: %s file_ref is outside the current candidate authority", label)
		}
		selected[fileRef] = struct{}{}
	}
	result := make([]corpus.FileID, 0, len(selected))
	for _, candidate := range candidates {
		if _, ok := selected[candidate.FileRef]; ok {
			result = append(result, candidate.FileRef)
		}
	}
	return result, nil
}

func canonicalCandidates(snapshot corpus.Snapshot, candidates []Candidate) ([]Candidate, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("target portfolio: merged candidate list is empty")
	}
	ordinals := make(map[corpus.FileID]int, len(snapshot.Entries))
	for index, entry := range snapshot.Entries {
		ordinals[entry.ID] = index
	}
	result := make([]Candidate, len(candidates))
	seen := make(map[corpus.FileID]struct{}, len(candidates))
	for index, candidate := range candidates {
		if _, ok := ordinals[candidate.FileRef]; !ok {
			return nil, fmt.Errorf("target portfolio: candidate file_ref is outside the corpus")
		}
		if _, duplicate := seen[candidate.FileRef]; duplicate {
			return nil, fmt.Errorf("target portfolio: duplicate candidate file_ref must be merged upstream")
		}
		seen[candidate.FileRef] = struct{}{}
		hypotheses := canonicalStrings(candidate.Hypotheses)
		if len(hypotheses) == 0 {
			return nil, fmt.Errorf("target portfolio: candidate has no hypotheses")
		}
		for _, hypothesis := range hypotheses {
			if err := validateHypothesis(hypothesis); err != nil {
				return nil, err
			}
		}
		result[index] = Candidate{FileRef: candidate.FileRef, Hypotheses: hypotheses}
	}
	sort.Slice(result, func(i, j int) bool {
		return ordinals[result[i].FileRef] < ordinals[result[j].FileRef]
	})
	return result, nil
}

func visibleCandidates(snapshot corpus.Snapshot, candidates []Candidate) ([]VisibleCandidate, error) {
	entries := make(map[corpus.FileID]corpus.Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ID] = entry
	}
	result := make([]VisibleCandidate, len(candidates))
	for index, candidate := range candidates {
		entry, ok := entries[candidate.FileRef]
		if !ok {
			return nil, fmt.Errorf("target portfolio: candidate file_ref is outside the corpus")
		}
		result[index] = VisibleCandidate{
			FileRef:    candidate.FileRef,
			Path:       entry.Path,
			Hypotheses: append([]string(nil), candidate.Hypotheses...),
		}
	}
	return result, nil
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	for _, value := range result {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func validateHypothesis(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len(value) > MaxRequestBytes || isAbsoluteLabel(value) {
		return fmt.Errorf("target portfolio: invalid candidate hypothesis")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("target portfolio: invalid candidate hypothesis")
		}
	}
	return nil
}

func isAbsoluteLabel(value string) bool {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') ||
		(value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' &&
		(value[2] == '/' || value[2] == '\\')
}

func compileState(
	snapshot corpus.Snapshot,
	candidates []Candidate,
	executableAuthorityBound bool,
	executableFileRefs []corpus.FileID,
	requiredAuthorityBound bool,
	requiredTargetFileRefs []corpus.FileID,
	request []byte,
) ([]byte, error) {
	corpusWire, err := snapshot.CanonicalJSON()
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode corpus state: %w", err)
	}
	candidateWire, err := json.Marshal(candidates)
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode candidate state: %w", err)
	}
	executableWire, err := json.Marshal(executableFileRefs)
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode executable authority state: %w", err)
	}
	requiredWire, err := json.Marshal(requiredTargetFileRefs)
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode required target authority state: %w", err)
	}
	state, err := json.Marshal(struct {
		Contract                 string `json:"contract"`
		PromptVersion            string `json:"prompt_version"`
		PreparationVersion       int    `json:"preparation_version"`
		ResponseSchemaVersion    int    `json:"response_schema_version"`
		CorpusBytesSHA256        string `json:"corpus_bytes_sha256"`
		CandidateBytesSHA256     string `json:"candidate_bytes_sha256"`
		ExecutableAuthorityBound bool   `json:"executable_authority_bound"`
		ExecutableRefsSHA256     string `json:"executable_file_refs_sha256"`
		RequiredAuthorityBound   bool   `json:"required_target_authority_bound"`
		RequiredRefsSHA256       string `json:"required_target_file_refs_sha256"`
		RequestBytesSHA256       string `json:"request_bytes_sha256"`
	}{
		Contract: executionContract, PromptVersion: PromptVersion,
		PreparationVersion: PreparationVersion, ResponseSchemaVersion: ResponseSchemaVersion,
		CorpusBytesSHA256: sha256Hex(corpusWire), CandidateBytesSHA256: sha256Hex(candidateWire),
		ExecutableAuthorityBound: executableAuthorityBound,
		ExecutableRefsSHA256:     sha256Hex(executableWire),
		RequiredAuthorityBound:   requiredAuthorityBound,
		RequiredRefsSHA256:       sha256Hex(requiredWire),
		RequestBytesSHA256:       sha256Hex(request),
	})
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode execution state: %w", err)
	}
	return state, nil
}

func compilationSeal(state []byte) string {
	return sha256Hex(append([]byte("file-target-portfolio-compilation-v1\x00"), state...))
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func cloneCandidate(value Candidate) Candidate {
	value.Hypotheses = append([]string(nil), value.Hypotheses...)
	return value
}

func cloneCandidates(values []Candidate) []Candidate {
	result := make([]Candidate, len(values))
	for index, value := range values {
		result[index] = cloneCandidate(value)
	}
	return result
}

func cloneFileRefs(values []corpus.FileID) []corpus.FileID {
	if values == nil {
		return nil
	}
	result := make([]corpus.FileID, len(values))
	copy(result, values)
	return result
}

func cloneVisibleCandidate(value VisibleCandidate) VisibleCandidate {
	value.Hypotheses = append([]string(nil), value.Hypotheses...)
	return value
}
