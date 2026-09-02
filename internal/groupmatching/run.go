package groupmatching

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed prompt.md
var promptText string

// NeedsProvider reports whether the complete graph set contains at least one
// cross-target pair with a locally valid witness candidate. Candidate-free
// pairs are already proven unable to satisfy the response contract.
func NeedsProvider(indexes []groupindex.Index) (bool, error) {
	compilation, err := Compile(indexes)
	if err != nil {
		return false, err
	}
	for _, pair := range compilation.pairs {
		if len(pair.witnessCandidates) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// Run exhaustively considers every unordered cross-target group pair, presents
// only pairs with at least one locally valid witness candidate, restores
// accepted directed semantic edges, and delegates merge/sealing to
// groupindex.WithConnections. Candidate-free and model-sparse empty output are
// both legitimate empty results.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	indexes []groupindex.Index,
) ([]groupindex.Index, []groupindex.Diagnostic, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("group matching: context is required")
	}
	compilation, err := Compile(indexes)
	if err != nil {
		return nil, nil, err
	}
	if len(compilation.pairs) == 0 {
		matched, diagnostics, matchErr := groupindex.WithConnections(compilation.input, nil)
		return matched, canonicalDiagnostics(diagnostics), matchErr
	}
	plan, err := compilation.batchesForProvider(provider)
	if err != nil {
		return nil, nil, err
	}
	calls := make([]llm.Call[normalizedResponse], len(plan))
	for position := range plan {
		request, requestErr := compilation.request(plan[position].pairRef)
		if requestErr != nil {
			return nil, nil, requestErr
		}
		wire, encodeErr := json.Marshal(request)
		if encodeErr != nil {
			return nil, nil, fmt.Errorf("group matching: encode request: %w", encodeErr)
		}
		state, stateErr := cubeState(compilation, wire)
		if stateErr != nil {
			return nil, nil, stateErr
		}
		requestCopy := request
		calls[position] = llm.Call[normalizedResponse]{
			State: state,
			Prompt: llm.Prompt{
				System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
			},
			Limits: limits(),
			DecodeValidate: func(raw []byte) (normalizedResponse, error) {
				return normalizeResponse(raw, compilation, requestCopy)
			},
		}
	}
	outcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	if err != nil {
		return nil, nil, fmt.Errorf("group matching: model batch: %w", err)
	}
	if err := compilation.validatePlan(plan); err != nil {
		return nil, nil, err
	}
	var accepted []connectionInput
	var diagnostics []groupindex.Diagnostic
	for _, outcome := range outcomes {
		accepted = append(accepted, outcome.Value.connections...)
		diagnostics = append(diagnostics, outcome.Value.diagnostics...)
	}
	accepted = canonicalConnectionInputs(accepted)
	restored, err := compilation.restore(accepted)
	if err != nil {
		return nil, nil, err
	}
	matched, buildDiagnostics, err := groupindex.WithConnections(compilation.input, restored)
	if err != nil {
		return nil, nil, err
	}
	diagnostics = append(diagnostics, buildDiagnostics...)
	return matched, canonicalDiagnostics(diagnostics), nil
}

func cubeState(compilation Compilation, request []byte) ([]byte, error) {
	promptDigest := sha256.Sum256([]byte(strings.TrimSpace(promptText)))
	requestDigest := sha256.Sum256(request)
	indexDigests := make([]string, 0, len(compilation.input))
	for _, index := range compilation.input {
		indexDigests = append(indexDigests, index.SHA256)
	}
	sort.Strings(indexDigests)
	authorityDigest := sha256.Sum256([]byte(strings.Join(indexDigests, "\x00")))
	state := struct {
		Contract              string `json:"contract"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		PromptSHA256          string `json:"prompt_sha256"`
		GroupsIndexSetSHA256  string `json:"groups_index_set_sha256"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: preparationVersion,
		ResponseSchemaVersion: responseSchemaVersion,
		PromptSHA256:          hex.EncodeToString(promptDigest[:]),
		GroupsIndexSetSHA256:  hex.EncodeToString(authorityDigest[:]),
		RequestSHA256:         hex.EncodeToString(requestDigest[:]),
	}
	wire, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("group matching: encode cube state: %w", err)
	}
	return wire, nil
}
