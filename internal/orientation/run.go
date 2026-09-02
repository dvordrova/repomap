package orientation

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	// StageName labels rejected rows and the cache state of this stage.
	StageName = "orientation"

	executionContract     = "repomap.orientation.v1"
	preparationVersion    = 1
	promptVersion         = 1
	responseSchemaVersion = 1
	maxOutputTokens       = 128_000
)

//go:embed prompt.md
var promptText string

// Input is everything the stage may show the model. Groups is the complete
// matched GroupsIndex set, one index per analyzed target.
type Input struct {
	RepositoryName string
	Facts          facts.Result
	Claims         claims.Result
	Groups         []groupindex.Index
}

type preparedRequest struct {
	wire    []byte
	catalog catalog
	dropped []RejectedRow
}

// Run makes exactly one model call, validates every returned row against the
// advertised catalog, restores accepted refs to exact ids, and seals the
// result against the inputs. Rejected rows are returned, never repaired, and
// never abort the run.
func Run(ctx context.Context, executor llm.Executor, provider llm.Provider, input Input) (Result, []RejectedRow, error) {
	if err := validateInput(input); err != nil {
		return Result{}, nil, fmt.Errorf("orientation: input: %w", err)
	}
	digests := groupDigests(input.Groups)
	if len(input.Facts.Targets) == 0 {
		result, err := Empty(input.Facts.SHA256, input.Claims.SHA256, digests, 0)
		return result, []RejectedRow{}, err
	}
	if provider == nil {
		return Result{}, nil, fmt.Errorf("orientation: provider is nil")
	}
	prepared, err := prepareRequest(provider, input)
	if err != nil {
		return Result{}, nil, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[normalized]{
		State: cubeState(input, digests, prepared.wire),
		Prompt: llm.Prompt{
			System: strings.TrimSpace(promptText), User: string(prepared.wire), ResponseFormatJSON: true,
		},
		Limits: limits(),
		DecodeValidate: func(raw []byte) (normalized, error) {
			return normalize(raw, prepared.catalog)
		},
	})
	if err != nil {
		return Result{}, nil, fmt.Errorf("orientation: model call: %w", err)
	}
	rejected := append(append([]RejectedRow{}, prepared.dropped...), outcome.Value.rejected...)
	result, err := Seal(Result{
		FactsSHA256:   input.Facts.SHA256,
		ClaimsSHA256:  input.Claims.SHA256,
		GroupsSHA256s: digests,
		Summary:       outcome.Value.summary,
		SummaryRefs:   outcome.Value.summaryRefs,
		Roles:         outcome.Value.roles,
		RunRecipe:     outcome.Value.recipe,
		MainFlow:      outcome.Value.flow,
		RejectedCount: len(rejected),
	})
	if err != nil {
		return Result{}, nil, fmt.Errorf("orientation: seal: %w", err)
	}
	return result, rejected, nil
}

func validateInput(input Input) error {
	if err := input.Facts.Validate(); err != nil {
		return err
	}
	if err := input.Claims.Validate(); err != nil {
		return err
	}
	if err := groupindex.ValidateSet(input.Groups); err != nil {
		return err
	}
	programTargets := make(map[string]struct{}, len(input.Facts.Targets))
	for _, target := range input.Facts.Targets {
		programTargets[target.ProgramTargetID] = struct{}{}
	}
	for _, index := range input.Groups {
		if _, known := programTargets[index.Target.ID]; !known {
			return fmt.Errorf("groups index for %q has no facts target", index.Target.ID)
		}
	}
	return nil
}

func groupDigests(indexes []groupindex.Index) []string {
	digests := make([]string, 0, len(indexes))
	for _, index := range indexes {
		digests = append(digests, index.SHA256)
	}
	return digests
}

// prepareRequest walks the shape levels from complete to smallest and keeps
// the first one the provider accepts. The chosen level is deterministic for
// one input, so the request bytes and the cache key are too.
func prepareRequest(provider llm.Provider, input Input) (preparedRequest, error) {
	var lastLimit error
	for level, shape := range requestShapes {
		wire, cat, err := encodeRequest(input, shape)
		if err != nil {
			return preparedRequest{}, err
		}
		fitErr := requestFits(provider, wire)
		if fitErr == nil {
			return preparedRequest{wire: wire, catalog: cat, dropped: droppedRows(input, level)}, nil
		}
		var resourceErr *llm.ResourceLimitError
		if !errors.As(fitErr, &resourceErr) || resourceErr.Kind != llm.ResourceLimitRequestBytes {
			return preparedRequest{}, fmt.Errorf("orientation: provider request preparation: %w", fitErr)
		}
		lastLimit = fitErr
	}
	return preparedRequest{}, fmt.Errorf(
		"orientation: request does not fit the provider request limit after dropping claims, group members, and bulk fact kinds; no fact was truncated: %w",
		lastLimit,
	)
}

func encodeRequest(input Input, shape requestShape) ([]byte, catalog, error) {
	wire, cat, err := buildRequest(input, shape)
	if err != nil {
		return nil, catalog{}, err
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, catalog{}, fmt.Errorf("orientation: encode request: %w", err)
	}
	return encoded, cat, nil
}

// requestFits returns nil when the provider accepts the request, a
// *llm.ResourceLimitError when it is too large, and any other error as is.
func requestFits(provider llm.Provider, wire []byte) error {
	bounds := limits()
	prepared, err := provider.Prepare(llm.Prompt{
		System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
	}, bounds)
	if err != nil {
		return err
	}
	if prepared.Len() > bounds.MaxRequestBytes {
		return llm.NewResourceLimitError(llm.ResourceLimitError{
			Stage: StageName + "_prepare", Kind: llm.ResourceLimitRequestBytes,
			Limit: bounds.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true,
		})
	}
	return nil
}

func cubeState(input Input, groupDigests []string, wire []byte) []byte {
	requestDigest := sha256.Sum256(wire)
	state, _ := json.Marshal(struct {
		Contract              string   `json:"contract"`
		PreparationVersion    int      `json:"preparation_version"`
		PromptVersion         int      `json:"prompt_version"`
		ResponseSchemaVersion int      `json:"response_schema_version"`
		FactsSHA256           string   `json:"facts_sha256"`
		ClaimsSHA256          string   `json:"claims_sha256"`
		GroupsSHA256s         []string `json:"groups_sha256s"`
		RequestSHA256         string   `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: preparationVersion,
		PromptVersion: promptVersion, ResponseSchemaVersion: responseSchemaVersion,
		FactsSHA256: input.Facts.SHA256, ClaimsSHA256: input.Claims.SHA256,
		GroupsSHA256s: sortedDigests(groupDigests), RequestSHA256: hex.EncodeToString(requestDigest[:]),
	})
	return state
}

func sortedDigests(digests []string) []string {
	sorted := append([]string{}, digests...)
	sort.Strings(sorted)
	return sorted
}

func limits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes:  llm.SemanticRecordByteLimit,
		MaxResponseBytes: llm.ProviderResponseByteLimit,
		MaxOutputTokens:  maxOutputTokens,
	}
}
