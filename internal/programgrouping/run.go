package programgrouping

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

//go:embed prompt.md
var promptText string

// Run groups one exact enriched ProgramIndex and immediately seals the result
// through groupindex.Build. Response-row and build diagnostics are combined;
// malformed or unsupported model rows never become graph authority.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	index programindex.Index,
) (groupindex.Index, []groupindex.Diagnostic, error) {
	if ctx == nil {
		return groupindex.Index{}, nil, fmt.Errorf("program grouping: context is required")
	}
	compilation, err := Compile(index)
	if err != nil {
		return groupindex.Index{}, nil, err
	}
	if len(compilation.categorizedRefs) == 0 {
		grouped, diagnostics, buildErr := groupindex.Build(compilation.index, groupindex.Proposals{})
		return grouped, canonicalDiagnostics(diagnostics), buildErr
	}

	plan, err := compilation.batchesForProvider(provider)
	if err != nil {
		return groupindex.Index{}, nil, err
	}
	finalPlan, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
		ctx, executor, provider, plan,
		func(items []batch) ([]llm.Call[proposalSet], error) {
			calls := make([]llm.Call[proposalSet], len(items))
			for position := range items {
				request, requestErr := compilation.request(phaseGrouping, items[position].groupRefs, proposalSet{})
				if requestErr != nil {
					return nil, requestErr
				}
				wire, encodeErr := json.Marshal(request)
				if encodeErr != nil {
					return nil, fmt.Errorf("program grouping: encode request: %w", encodeErr)
				}
				state, stateErr := cubeState(compilation.index.SHA256, phaseGrouping, wire)
				if stateErr != nil {
					return nil, stateErr
				}
				requestCopy := request
				calls[position] = llm.Call[proposalSet]{
					State: state,
					Prompt: llm.Prompt{
						System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
					},
					Limits: limits(),
					DecodeValidate: func(raw []byte) (proposalSet, error) {
						return normalizeResponse(raw, compilation, requestCopy)
					},
				}
			}
			return calls, nil
		},
		splitBatch,
	)
	if err != nil {
		return groupindex.Index{}, nil, fmt.Errorf("program grouping: model batch: %w", err)
	}
	if err := compilation.validatePlan(finalPlan); err != nil {
		return groupindex.Index{}, nil, err
	}

	combined := proposalSet{}
	for position, outcome := range outcomes {
		namespaced := namespaceProposalSet(outcome.Value, fmt.Sprintf("b%d:", position+1))
		combined.groups = append(combined.groups, namespaced.groups...)
		combined.connections = append(combined.connections, namespaced.connections...)
		combined.diagnostics = append(combined.diagnostics, namespaced.diagnostics...)
	}
	combined = canonicalProposalSet(combined)
	if len(finalPlan) > 1 && len(combined.groups) > 1 {
		combined, err = runMergeTournament(ctx, executor, provider, compilation, combined)
		if err != nil {
			return groupindex.Index{}, nil, err
		}
	}

	grouped, buildDiagnostics, err := groupindex.Build(compilation.index, combined.groupIndexProposals())
	if err != nil {
		return groupindex.Index{}, nil, err
	}
	diagnostics := append([]groupindex.Diagnostic(nil), combined.diagnostics...)
	diagnostics = append(diagnostics, buildDiagnostics...)
	return grouped, canonicalDiagnostics(diagnostics), nil
}

func cubeState(indexSHA string, requestPhase phase, request []byte) ([]byte, error) {
	promptDigest := sha256.Sum256([]byte(strings.TrimSpace(promptText)))
	requestDigest := sha256.Sum256(request)
	state := struct {
		Contract              string `json:"contract"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		Phase                 phase  `json:"phase"`
		PromptSHA256          string `json:"prompt_sha256"`
		ProgramIndexSHA256    string `json:"program_index_sha256"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: preparationVersion,
		ResponseSchemaVersion: responseSchemaVersion, Phase: requestPhase,
		PromptSHA256: hex.EncodeToString(promptDigest[:]), ProgramIndexSHA256: indexSHA,
		RequestSHA256: hex.EncodeToString(requestDigest[:]),
	}
	wire, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("program grouping: encode cube state: %w", err)
	}
	return wire, nil
}
