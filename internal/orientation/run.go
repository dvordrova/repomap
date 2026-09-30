package orientation

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	// StageName labels rejected rows and the cache state of this stage.
	StageName = "orientation"

	executionContract     = "repomap.orientation.v2"
	preparationVersion    = 5
	promptVersion         = 11
	responseSchemaVersion = 2
	// maxOutputTokens is measured: 117 accepted orientation exchanges
	// answered in at most 1,260 output tokens (median 848). The shared
	// 128,000 reservation took that much of the 1,048,576-token window from
	// the request.
	maxOutputTokens = 16384
)

// The stage asks the model once: the overview (summary, roles, run recipe
// and which target's Main flow to read). That Main flow is walked by code
// (path.go), the categorizer choosing at each split.
var (
	//go:embed overview-prompt.md
	overviewPrompt string
	//go:embed overview-response-example.json
	overviewExample string
)

// Input is everything the stage may show the model. Groups is the complete
// matched GroupsIndex set, one index per analyzed target.
type Input struct {
	RepositoryName string
	Facts          facts.Result
	Claims         claims.Result
	Groups         []groupindex.Index
	// Graph is the already-built source graph used by atlas reading. Only
	// the seeds' rows enter the orientation request.
	Graph atlas.Graph
	// Categorizer chooses where the Main flow continues at a split
	// (walkFlow); without one the flow ends at its first split, a named
	// fork.
	Categorizer llm.Categorizer
}

// Run makes the overview call and, when it names a target, walks that
// target's Main flow; it validates every returned row against the request's
// catalogue, restores accepted refs to exact ids, and seals the result
// against the inputs. Rejected rows are returned, never repaired, and never
// abort the run. A split the categorizer leaves undecided ends the flow
// there and is journaled.
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
	overview, cat, err := buildOverview(input)
	if err != nil {
		return Result{}, nil, err
	}
	wire, err := encodeWire(overview)
	if err != nil {
		return Result{}, nil, fmt.Errorf("orientation: encode request: %w", err)
	}
	outcome, err := ask(ctx, executor, provider, input, digests, wire, "overview", overviewPrompt, overviewExample,
		func(raw []byte) (normalized, error) { return normalizeOverview(raw, cat) })
	if err != nil {
		rejected, refused := refusal(ctx, err, outcome, sectionRequest, len(wire))
		if !refused {
			return Result{}, nil, fmt.Errorf("orientation: model call: %w", err)
		}
		result, sealErr := Empty(input.Facts.SHA256, input.Claims.SHA256, digests, len(rejected))
		return result, rejected, sealErr
	}
	accepted := outcome.Value
	rejected := accepted.rejected
	var flow MainFlow
	if accepted.flowTarget != "" {
		walk, err := walkFlow(ctx, executor, input.Categorizer, input, accepted.flowTarget)
		if err != nil {
			return Result{}, nil, err
		}
		flow = walk.flow
		rejected = append(rejected, walk.rejected...)
	}
	result, err := Seal(Result{
		FactsSHA256:   input.Facts.SHA256,
		ClaimsSHA256:  input.Claims.SHA256,
		GroupsSHA256s: digests,
		Summary:       accepted.summary,
		SummaryRefs:   accepted.summaryRefs,
		Roles:         accepted.roles,
		RunRecipe:     accepted.recipe,
		MainFlow:      flow,
		RejectedCount: len(rejected),
	})
	if err != nil {
		return Result{}, nil, fmt.Errorf("orientation: seal: %w", err)
	}
	return result, rejected, nil
}

// encodeWire writes a request as the model reads it: a call's "->" and a
// "<lambda>" stay as written, not \u003e and \u003c.
func encodeWire(request any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(request); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// ask is one cube call: the prepared request must fit the provider's
// envelope, and the answer is the decoder's.
func ask(ctx context.Context, executor llm.Executor, provider llm.Provider, input Input, digests []string, wire []byte,
	part, system, example string, decode func([]byte) (normalized, error)) (llm.Outcome[normalized], error) {
	prompt := llm.Prompt{System: strings.TrimSpace(system), User: string(wire), ResponseFormatJSON: true, ResponseExample: example}
	if err := requestFits(provider, prompt); err != nil {
		return llm.Outcome[normalized]{}, fmt.Errorf("orientation: provider request preparation: %w", err)
	}
	return llm.ExecuteJSON(ctx, executor, provider, llm.Call[normalized]{
		State:          cubeState(input, digests, part, wire),
		Prompt:         prompt,
		Limits:         limits(),
		DecodeValidate: decode,
	})
}

// refusal turns a failed call into journaled rows when the failure is the
// model's answer or the provider's size or context refusal: a response whose
// every row was refused journals each row's own reason, one that is not the
// requested JSON one row, and a size refusal one row under section. Local
// and transport errors keep their error path.
func refusal(ctx context.Context, err error, outcome llm.Outcome[normalized], section string, requestBytes int) ([]RejectedRow, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	var none *noOutputError
	if errors.As(err, &none) {
		return append([]RejectedRow(nil), none.rejected...), true
	}
	for _, refused := range outcome.ResponseRejections {
		if refused.Kind == "response_validation" {
			raw, _ := json.Marshal(string(outcome.Response))
			return []RejectedRow{{Stage: StageName, Section: "response", Raw: raw, Reason: err.Error()}}, true
		}
	}
	var resource *llm.ResourceLimitError
	if errors.As(err, &resource) {
		raw, _ := json.Marshal(struct {
			RequestBytes int    `json:"request_bytes,omitempty"`
			Resource     string `json:"resource"`
		}{requestBytes, fmt.Sprint(resource.Kind)})
		return []RejectedRow{{Stage: StageName, Section: section, Raw: raw, Reason: err.Error()}}, true
	}
	return nil, false
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
		programTargets[target.ID] = struct{}{}
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

// requestFits returns nil when the provider accepts the request, a
// *llm.ResourceLimitError when it is too large, and any other error as is.
func requestFits(provider llm.Provider, prompt llm.Prompt) error {
	bounds := limits()
	prepared, err := llm.Prepare(provider, prompt, bounds)
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

func cubeState(input Input, groupDigests []string, part string, wire []byte) []byte {
	requestDigest := sha256.Sum256(wire)
	state, _ := json.Marshal(struct {
		Contract              string   `json:"contract"`
		Part                  string   `json:"part"`
		PreparationVersion    int      `json:"preparation_version"`
		PromptVersion         int      `json:"prompt_version"`
		ResponseSchemaVersion int      `json:"response_schema_version"`
		FactsSHA256           string   `json:"facts_sha256"`
		ClaimsSHA256          string   `json:"claims_sha256"`
		GroupsSHA256s         []string `json:"groups_sha256s"`
		RequestSHA256         string   `json:"request_sha256"`
	}{
		Contract: executionContract, Part: part, PreparationVersion: preparationVersion,
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
