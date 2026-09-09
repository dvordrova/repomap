package documentationreduce

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

const (
	requestVersion        = 1
	executionContract     = "repomap.documentation-reduce.v1"
	preparationVersion    = 2
	responseSchemaVersion = 2
	// Use the shared output allowance; the configured provider ceiling still
	// applies. A truncated completion is refused and its batch is split.
	maxOutputTokens = llm.DefaultMaxOutputTokens
)

//go:embed prompt.md
var sourcePrompt string

//go:embed merge_prompt.md
var mergePrompt string

//go:embed response-example.json
var responseExample string

type batchWire struct {
	Ordinal int `json:"ordinal"`
	Count   int `json:"count"`
}

type partWire struct {
	Ordinal int `json:"ordinal"`
	Count   int `json:"count"`
}

type documentWire struct {
	Ref     string                         `json:"ref"`
	Path    string                         `json:"path"`
	Kind    readmetargetscout.GuidanceKind `json:"kind"`
	Part    partWire                       `json:"part"`
	Content string                         `json:"content"`
}

type sourceRequest struct {
	Version        int            `json:"version"`
	Batch          batchWire      `json:"batch"`
	ContentTrust   string         `json:"content_trust"`
	Documents      []documentWire `json:"documents"`
	PartialContext bool           `json:"partial_context,omitempty"`
}

type responseSource struct {
	Ref      string   `json:"ref"`
	Claims   []string `json:"claims"`
	Concepts []string `json:"concepts"`
}

type modelResponse struct {
	Overview string           `json:"overview"`
	Sources  []responseSource `json:"sources"`
}

type mergeCandidateWire struct {
	Ref      string           `json:"ref"`
	Overview string           `json:"overview"`
	Sources  []responseSource `json:"sources"`
}

type mergeRequest struct {
	Version        int                  `json:"version"`
	Level          int                  `json:"level"`
	Batch          batchWire            `json:"batch"`
	ContentTrust   string               `json:"content_trust"`
	Candidates     []mergeCandidateWire `json:"candidates"`
	PartialContext bool                 `json:"partial_context,omitempty"`
}

type documentAuthority struct {
	path string
	kind readmetargetscout.GuidanceKind
}

type documentUnit struct {
	ref     string
	path    string
	kind    readmetargetscout.GuidanceKind
	content string
}

type sourceBatch struct {
	partial bool
	units   []documentUnit
	request sourceRequest
	wire    []byte
	allowed map[string]documentAuthority
}

type normalizedReduction struct {
	overview        string
	sources         []responseSource
	rejected        []llm.ResponseRejection
	accepted        []string
	refusedSources  []string
	unlocatedSource bool
}

type mergeBatch struct {
	partial    bool
	candidates []normalizedReduction
	request    mergeRequest
	wire       []byte
	allowed    map[string]documentAuthority
}

// Run exhaustively reduces the complete documentation_collect handoff. Large
// documents are split only into lossless UTF-8 slices, then every accepted
// shard is convergently reduced until one source-bound result remains.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	guidance readmetargetscout.GuidanceSnapshot,
) (Result, error) {
	snapshot, err := guidance.Snapshot()
	if err != nil {
		return Result{}, fmt.Errorf("documentation reduce: input: %w", err)
	}
	if len(snapshot.Documents) == 0 {
		return sealResult(snapshot, "", nil)
	}
	if provider == nil {
		return Result{}, fmt.Errorf("documentation reduce: provider is nil")
	}

	authority, units := compileAuthority(snapshot)
	batches, err := packSourceBatches(provider, units)
	if err != nil {
		return Result{}, err
	}
	var candidates []normalizedReduction
	unavailable := false
	for len(batches) > 0 {
		var fresh []sourceBatch
		for _, batch := range batches {
			if len(batch.wire) == 0 {
				fresh = append(fresh, batch)
			}
		}
		fresh, err = materializeSourcePlan(fresh)
		if err != nil {
			return Result{}, err
		}
		// Existing requests keep their original context and exact cache key;
		// only new complete children acquire the current partial-window scope.
		for i := range batches {
			if len(batches[i].wire) == 0 {
				batches[i], fresh = fresh[0], fresh[1:]
			}
		}
		calls := make([]llm.Call[normalizedReduction], len(batches))
		var hinted []sourceBatch
		replanned := false
		for i, batch := range batches {
			calls[i] = reductionCall("source", snapshot.SHA256, sourcePrompt, batch.wire, batch.allowed)
			found, err := llm.RecallAdaptiveSplit(executor, provider, calls[i])
			if err != nil {
				return Result{}, err
			}
			if found {
				if left, right, ok := splitSourceBatch(batch); ok {
					hinted = append(hinted, left, right)
					replanned = true
					continue
				}
			}
			hinted = append(hinted, batch)
		}
		if replanned {
			batches = hinted
			continue
		}
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(calls))
		}
		responses := llm.ExecuteJSONEach(ctx, executor, provider, calls)
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		var next []sourceBatch
		for i, response := range responses {
			if response.Err == nil {
				if !emptyReduction(response.Outcome.Value) {
					candidates = append(candidates, response.Outcome.Value)
				}
				continue
			}
			if left, right, ok := splitSourceBatch(batches[i]); ok {
				eligible, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], response.Outcome, response.Err)
				if err != nil {
					return Result{}, err
				}
				if eligible {
					next = append(next, left, right)
					continue
				}
			}
			if !unavailableReduction(response) {
				return Result{}, response.Err
			}
			unavailable = true
		}
		batches = next
	}
	if len(candidates) == 0 {
		return sealResult(snapshot, "", nil)
	}
	if len(candidates) > 1 {
		candidates, err = mergeTournament(ctx, executor, provider, snapshot.SHA256, authority, candidates)
		if err != nil {
			return Result{}, err
		}
	}
	if len(candidates) == 0 {
		return sealResult(snapshot, "", nil)
	}
	combined := joinReductions(candidates)
	if unavailable {
		combined.overview = ""
	}
	sources, err := restoreSources(combined.sources, authority)
	if err != nil {
		return Result{}, err
	}
	return sealResult(snapshot, combined.overview, sources)
}

func compileAuthority(
	snapshot readmetargetscout.GuidanceSnapshot,
) (map[string]documentAuthority, []documentUnit) {
	authority := make(map[string]documentAuthority, len(snapshot.Documents))
	units := make([]documentUnit, 0, len(snapshot.Documents))
	for position, document := range snapshot.Documents {
		ref := "d" + strconv.Itoa(position+1)
		authority[ref] = documentAuthority{path: document.Path, kind: document.Kind}
		units = append(units, documentUnit{
			ref: ref, path: document.Path, kind: document.Kind, content: document.Content,
		})
	}
	return authority, units
}

func packSourceBatches(provider llm.Provider, documents []documentUnit) ([]sourceBatch, error) {
	units := make([]documentUnit, 0, len(documents))
	for _, document := range documents {
		parts, err := splitSourceUnitToFit(provider, document)
		if err != nil {
			return nil, err
		}
		units = append(units, parts...)
	}
	packed := make([]sourceBatch, 0)
	for start := 0; start < len(units); {
		count, err := largestFittingPrefix(len(units)-start, func(count int) (bool, error) {
			return sourceUnitsFit(provider, units[start:start+count])
		})
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("documentation reduce: indivisible document slice does not fit provider request")
		}
		packed = append(packed, sourceBatch{units: units[start : start+count]})
		start += count
	}
	return materializeSourcePlan(packed)
}

// Prepared request size grows with each appended row. Bracket the first
// refusal by doubling, then search only that bracket. Searching the complete
// remaining tail for every small window would repeatedly encode most of the
// reservoir, even if the number of probes looked small.
func largestFittingPrefix(count int, fits func(int) (bool, error)) (int, error) {
	best, probe := 0, 1
	for probe <= count {
		ok, err := fits(probe)
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		best = probe
		if best == count {
			return best, nil
		}
		if probe > count/2 {
			probe = count
		} else {
			probe *= 2
		}
	}
	for low, high := best+1, probe-1; low <= high; {
		middle := low + (high-low)/2
		ok, err := fits(middle)
		if err != nil {
			return 0, err
		}
		if ok {
			best, low = middle, middle+1
		} else {
			high = middle - 1
		}
	}
	return best, nil
}

func splitSourceUnitToFit(provider llm.Provider, unit documentUnit) ([]documentUnit, error) {
	fits, err := sourceUnitsFit(provider, []documentUnit{unit})
	if err != nil {
		return nil, err
	}
	if fits {
		return []documentUnit{unit}, nil
	}
	leftContent, rightContent, ok := splitUTF8(unit.content)
	if !ok {
		return nil, fmt.Errorf(
			"documentation reduce: document %q has an indivisible slice outside the provider request envelope; no content was truncated",
			unit.path,
		)
	}
	left, right := unit, unit
	left.content, right.content = leftContent, rightContent
	leftParts, err := splitSourceUnitToFit(provider, left)
	if err != nil {
		return nil, err
	}
	rightParts, err := splitSourceUnitToFit(provider, right)
	if err != nil {
		return nil, err
	}
	return append(leftParts, rightParts...), nil
}

func sourceUnitsFit(provider llm.Provider, units []documentUnit) (bool, error) {
	documents := make([]documentWire, len(units))
	for position, unit := range units {
		documents[position] = documentWire{
			Ref: unit.ref, Path: unit.path, Kind: unit.kind,
			Part: partWire{Ordinal: math.MaxInt, Count: math.MaxInt}, Content: unit.content,
		}
	}
	request := sourceRequest{
		Version:      requestVersion,
		Batch:        batchWire{Ordinal: math.MaxInt, Count: math.MaxInt},
		ContentTrust: "untrusted_repository_text", Documents: documents, PartialContext: true,
	}
	return requestFits(provider, sourcePrompt, request)
}

func materializeSourcePlan(plan []sourceBatch) ([]sourceBatch, error) {
	partCount := make(map[string]int)
	for _, batch := range plan {
		for _, unit := range batch.units {
			partCount[unit.ref]++
		}
	}
	partOrdinal := make(map[string]int)
	result := make([]sourceBatch, len(plan))
	for batchPosition, planned := range plan {
		documents := make([]documentWire, len(planned.units))
		allowed := make(map[string]documentAuthority)
		for position, unit := range planned.units {
			partOrdinal[unit.ref]++
			documents[position] = documentWire{
				Ref: unit.ref, Path: unit.path, Kind: unit.kind,
				Part:    partWire{Ordinal: partOrdinal[unit.ref], Count: partCount[unit.ref]},
				Content: unit.content,
			}
			allowed[unit.ref] = documentAuthority{path: unit.path, kind: unit.kind}
		}
		request := sourceRequest{
			Version:      requestVersion,
			Batch:        batchWire{Ordinal: batchPosition + 1, Count: len(plan)},
			ContentTrust: "untrusted_repository_text", Documents: documents, PartialContext: planned.partial,
		}
		wire, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: encode source batch: %w", err)
		}
		result[batchPosition] = sourceBatch{
			partial: planned.partial,
			units:   append([]documentUnit(nil), planned.units...),
			request: request, wire: wire, allowed: allowed,
		}
	}
	return result, nil
}

func splitSourceBatch(batch sourceBatch) (sourceBatch, sourceBatch, bool) {
	if len(batch.units) > 1 {
		middle := len(batch.units) / 2
		return sourceBatch{partial: true, units: append([]documentUnit(nil), batch.units[:middle]...)},
			sourceBatch{partial: true, units: append([]documentUnit(nil), batch.units[middle:]...)}, true
	}
	if len(batch.units) != 1 {
		return sourceBatch{}, sourceBatch{}, false
	}
	leftContent, rightContent, ok := splitUTF8(batch.units[0].content)
	if !ok {
		return sourceBatch{}, sourceBatch{}, false
	}
	left, right := batch.units[0], batch.units[0]
	left.content, right.content = leftContent, rightContent
	return sourceBatch{partial: true, units: []documentUnit{left}}, sourceBatch{partial: true, units: []documentUnit{right}}, true
}

func reductionCall(phase, guidanceSHA, prompt string, wire []byte, allowed map[string]documentAuthority) llm.Call[normalizedReduction] {
	return llm.Call[normalizedReduction]{State: cubeState(phase, guidanceSHA, wire),
		Prompt: llm.Prompt{System: strings.TrimSpace(prompt), User: string(wire), ResponseFormatJSON: true, ResponseExample: responseExample},
		Limits: limits(), DecodeValidate: func(raw []byte) (normalizedReduction, error) { return normalizeResponse(raw, allowed) }}
}

func unavailableReduction(response llm.EachResult[normalizedReduction]) bool {
	var resource *llm.ResourceLimitError
	if errors.As(response.Err, &resource) || errors.Is(response.Err, context.Canceled) || errors.Is(response.Err, context.DeadlineExceeded) {
		return false
	}
	for _, rejected := range response.Outcome.ResponseRejections {
		if rejected.Kind == "response_validation" || rejected.Kind == "response_envelope" || rejected.Kind == "provider_failed" {
			return true
		}
	}
	return false
}

// Joining keeps accepted source statements. Only one accepted reduction can
// supply an overview; concatenation must not invent a repository-wide summary.
func joinReductions(candidates []normalizedReduction) normalizedReduction {
	if len(candidates) == 1 {
		return candidates[0]
	}
	result := normalizedReduction{}
	byRef := make(map[string]responseSource)
	for _, candidate := range candidates {
		for _, source := range candidate.sources {
			kept := byRef[source.Ref]
			kept.Ref = source.Ref
			kept.Claims, kept.Concepts = append(kept.Claims, source.Claims...), append(kept.Concepts, source.Concepts...)
			byRef[source.Ref] = kept
		}
	}
	for _, source := range byRef {
		source.Claims, _ = canonicalizeText(source.Claims)
		source.Concepts, _ = canonicalizeText(source.Concepts)
		result.sources = append(result.sources, source)
	}
	sort.Slice(result.sources, func(i, j int) bool { return result.sources[i].Ref < result.sources[j].Ref })
	return result
}

func keepRejectedMergeSources(result normalizedReduction, originals []normalizedReduction) normalizedReduction {
	if len(result.refusedSources) == 0 && !result.unlocatedSource {
		return result
	}
	refused := make(map[string]bool)
	for _, ref := range result.refusedSources {
		refused[ref] = true
	}
	kept := normalizedReduction{}
	for _, original := range originals {
		for _, source := range original.sources {
			if result.unlocatedSource || refused[source.Ref] {
				kept.sources = append(kept.sources, source)
			}
		}
	}
	// These are previously accepted statements, not a replacement interpretation
	// of raw source. The new overview, if any, is still the model's own answer.
	result.sources = joinReductions([]normalizedReduction{result, kept}).sources
	if result.unlocatedSource {
		result.overview = ""
	}
	return result
}

func mergeTournament(ctx context.Context, executor llm.Executor, provider llm.Provider, guidanceSHA string, authority map[string]documentAuthority, candidates []normalizedReduction) ([]normalizedReduction, error) {
	candidates = canonicalCandidates(candidates)
	for level := 1; len(candidates) > 1; level++ {
		before, err := candidateFootprint(candidates)
		if err != nil {
			return nil, err
		}
		batches, err := packMergeBatches(provider, candidates, level, authority)
		if err != nil {
			return nil, err
		}
		var accepted, retained []normalizedReduction
		for len(batches) > 0 {
			var fresh []mergeBatch
			for _, batch := range batches {
				if len(batch.wire) == 0 {
					fresh = append(fresh, batch)
				}
			}
			fresh, err = materializeMergePlan(fresh, level, authority)
			if err != nil {
				return nil, err
			}
			for i := range batches {
				if len(batches[i].wire) == 0 {
					batches[i], fresh = fresh[0], fresh[1:]
				}
			}
			calls := make([]llm.Call[normalizedReduction], len(batches))
			var hinted []mergeBatch
			replanned := false
			for i, batch := range batches {
				calls[i] = reductionCall("merge", guidanceSHA, mergePrompt, batch.wire, batch.allowed)
				found, err := llm.RecallAdaptiveSplit(executor, provider, calls[i])
				if err != nil {
					return nil, err
				}
				if found {
					if left, right, ok := splitMergeBatch(batch); ok {
						hinted = append(hinted, left, right)
						replanned = true
						continue
					}
				}
				hinted = append(hinted, batch)
			}
			if replanned {
				batches = hinted
				continue
			}
			if executor.PlanNotice != nil {
				executor.PlanNotice(len(calls))
			}
			responses := llm.ExecuteJSONEach(ctx, executor, provider, calls)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			var next []mergeBatch
			for i, response := range responses {
				if response.Err == nil {
					value := keepRejectedMergeSources(response.Outcome.Value, batches[i].candidates)
					if !emptyReduction(value) {
						accepted = append(accepted, value)
					}
					continue
				}
				if left, right, ok := splitMergeBatch(batches[i]); ok {
					eligible, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], response.Outcome, response.Err)
					if err != nil {
						return nil, err
					}
					if eligible {
						next = append(next, left, right)
						continue
					}
				}
				if !unavailableReduction(response) {
					return nil, response.Err
				}
				for _, original := range batches[i].candidates {
					original.overview = ""
					retained = append(retained, original)
				}
			}
			batches = next
		}
		if len(retained) > 0 {
			return append(accepted, retained...), nil
		}
		accepted = canonicalCandidates(accepted)
		after, err := candidateFootprint(accepted)
		if err != nil {
			return nil, err
		}
		if len(accepted) >= len(candidates) && after >= before {
			return accepted, nil
		}
		candidates = accepted
	}
	return candidates, nil
}

func packMergeBatches(
	provider llm.Provider,
	candidates []normalizedReduction,
	level int,
	authority map[string]documentAuthority,
) ([]mergeBatch, error) {
	packed := make([]mergeBatch, 0)
	for start := 0; start < len(candidates); {
		count, err := largestFittingPrefix(len(candidates)-start, func(count int) (bool, error) {
			return mergeCandidatesFit(provider, candidates[start:start+count], level)
		})
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("documentation reduce: indivisible merge candidate does not fit provider request; no context was truncated")
		}
		packed = append(packed, mergeBatch{candidates: candidates[start : start+count]})
		start += count
	}
	return materializeMergePlan(packed, level, authority)
}

func mergeCandidatesFit(
	provider llm.Provider,
	candidates []normalizedReduction,
	level int,
) (bool, error) {
	request := mergeRequest{
		Version: requestVersion, Level: level,
		Batch:        batchWire{Ordinal: math.MaxInt, Count: math.MaxInt},
		ContentTrust: "untrusted_repository_summary", PartialContext: true,
		Candidates: mergeCandidateWires(candidates),
	}
	return requestFits(provider, mergePrompt, request)
}

func materializeMergePlan(
	plan []mergeBatch,
	level int,
	authority map[string]documentAuthority,
) ([]mergeBatch, error) {
	result := make([]mergeBatch, len(plan))
	for position, planned := range plan {
		request := mergeRequest{
			Version: requestVersion, Level: level,
			Batch:        batchWire{Ordinal: position + 1, Count: len(plan)},
			ContentTrust: "untrusted_repository_summary", PartialContext: planned.partial,
			Candidates: mergeCandidateWires(planned.candidates),
		}
		wire, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: encode merge batch: %w", err)
		}
		allowed := make(map[string]documentAuthority)
		for _, candidate := range planned.candidates {
			for _, source := range candidate.sources {
				if document, known := authority[source.Ref]; known {
					allowed[source.Ref] = document
				}
			}
		}
		result[position] = mergeBatch{
			partial:    planned.partial,
			candidates: append([]normalizedReduction(nil), planned.candidates...),
			request:    request, wire: wire, allowed: allowed,
		}
	}
	return result, nil
}

func splitMergeBatch(batch mergeBatch) (mergeBatch, mergeBatch, bool) {
	if len(batch.candidates) < 2 {
		return mergeBatch{}, mergeBatch{}, false
	}
	middle := len(batch.candidates) / 2
	return mergeBatch{partial: true, candidates: append([]normalizedReduction(nil), batch.candidates[:middle]...)},
		mergeBatch{partial: true, candidates: append([]normalizedReduction(nil), batch.candidates[middle:]...)}, true
}

func mergeCandidateWires(candidates []normalizedReduction) []mergeCandidateWire {
	result := make([]mergeCandidateWire, len(candidates))
	for position, candidate := range candidates {
		result[position] = mergeCandidateWire{
			Ref: "r" + strconv.Itoa(position+1), Overview: candidate.overview,
			Sources: cloneResponseSources(candidate.sources),
		}
	}
	return result
}

func (result normalizedReduction) ResponseRejections() []llm.ResponseRejection {
	return result.rejected
}
func (result normalizedReduction) AcceptedRowKeys() []string {
	if len(result.rejected) > 0 {
		return result.accepted
	}
	return nil
}

func normalizeResponse(raw []byte, allowed map[string]documentAuthority) (normalizedReduction, error) {
	result := normalizedReduction{accepted: []string{}}
	if len(raw) == 0 || len(raw) > llm.ProviderResponseByteLimit {
		return result, fmt.Errorf("documentation reduce: response exceeds bounded envelope")
	}
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return result, err
	}
	var response struct {
		Overview json.RawMessage   `json:"overview"`
		Sources  []json.RawMessage `json:"sources"`
	}
	if json.Unmarshal(normalized, &response) != nil || response.Sources == nil {
		return result, fmt.Errorf("documentation reduce: response sources must be an array")
	}
	badSources := make(map[string]bool)
	currentRef := ""
	invalid := false
	reject := func(position, reason string) {
		if currentRef == "" {
			invalid = true
		} else if _, known := allowed[currentRef]; known {
			badSources[currentRef] = true
			invalid = true
		}
		result.rejected = append(result.rejected, llm.ResponseRejection{Kind: "documentation_rejected", Count: 1, Samples: []string{position}, Reason: reason})
	}
	if json.Unmarshal(response.Overview, &result.overview) != nil {
		reject("overview", "overview must be text")
		result.overview = ""
	}
	result.overview = strings.TrimSpace(result.overview)
	if result.overview != "" && !validText(result.overview) {
		reject("overview", "overview has invalid text")
		result.overview = ""
	}
	textSet := func(raw json.RawMessage, position string) []string {
		var entries []json.RawMessage
		if json.Unmarshal(raw, &entries) != nil || entries == nil {
			reject(position, "source text set must be an array")
			return nil
		}
		var values []string
		for i, rawEntry := range entries {
			var text string
			if json.Unmarshal(rawEntry, &text) != nil {
				reject(fmt.Sprintf("%s[%d]", position, i), "claim or concept must be text")
				continue
			}
			text = strings.TrimSpace(text)
			if !validText(text) {
				reject(fmt.Sprintf("%s[%d]", position, i), "claim or concept must be non-empty text")
				continue
			}
			values = append(values, text)
		}
		return values
	}
	byRef := make(map[string]responseSource)
	for i, rawSource := range response.Sources {
		currentRef = ""
		position := fmt.Sprintf("sources[%d]", i)
		var source struct {
			Ref      string          `json:"ref"`
			Claims   json.RawMessage `json:"claims"`
			Concepts json.RawMessage `json:"concepts"`
		}
		if json.Unmarshal(rawSource, &source) != nil || source.Ref == "" {
			result.unlocatedSource = true
			reject(position, "source must have a string ref")
			continue
		}
		currentRef = source.Ref
		if _, known := allowed[source.Ref]; !known {
			reject(position, "source ref was not advertised")
			continue
		}
		claims, concepts := textSet(source.Claims, position+".claims"), textSet(source.Concepts, position+".concepts")
		if len(claims)+len(concepts) == 0 {
			continue
		}
		value := byRef[source.Ref]
		value.Ref = source.Ref
		value.Claims, value.Concepts = append(value.Claims, claims...), append(value.Concepts, concepts...)
		byRef[source.Ref] = value
	}
	for ref, source := range byRef {
		source.Claims, _ = canonicalizeText(source.Claims)
		source.Concepts, _ = canonicalizeText(source.Concepts)
		result.sources = append(result.sources, source)
		if !badSources[ref] {
			result.accepted = append(result.accepted, ref)
		}
	}
	for ref := range badSources {
		if _, known := allowed[ref]; known {
			result.refusedSources = append(result.refusedSources, ref)
		}
	}
	sort.Strings(result.refusedSources)
	sort.Strings(result.accepted)
	sort.Slice(result.sources, func(i, j int) bool {
		a, b := allowed[result.sources[i].Ref].path, allowed[result.sources[j].Ref].path
		if a != b {
			return a < b
		}
		return result.sources[i].Ref < result.sources[j].Ref
	})
	currentRef = ""
	if result.overview != "" && len(result.sources) == 0 {
		reject("overview", "overview has no accepted source")
		result.overview = ""
	}
	if result.overview != "" {
		result.accepted = append(result.accepted, "")
	}
	if invalid && result.overview == "" && len(result.sources) == 0 {
		return result, fmt.Errorf("documentation reduce: no usable response: %s", result.rejected[0].Reason)
	}
	return result, nil
}

func restoreSources(
	values []responseSource,
	authority map[string]documentAuthority,
) ([]Source, error) {
	result := make([]Source, 0, len(values))
	for _, value := range values {
		document, known := authority[value.Ref]
		if !known {
			return nil, fmt.Errorf("documentation reduce: restored response contains unknown source ref")
		}
		result = append(result, Source{
			Path: document.path, Kind: document.kind,
			Claims:   append([]string(nil), value.Claims...),
			Concepts: append([]string(nil), value.Concepts...),
		})
	}
	return canonicalSources(result)
}

func requestFits(provider llm.Provider, systemPrompt string, request any) (bool, error) {
	wire, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("documentation reduce: encode provider request: %w", err)
	}
	_, err = llm.Prepare(provider, llm.Prompt{
		System: strings.TrimSpace(systemPrompt), User: string(wire), ResponseFormatJSON: true, ResponseExample: responseExample,
	}, limits())
	if err == nil {
		return true, nil
	}
	var resourceErr *llm.ResourceLimitError
	if errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes {
		return false, nil
	}
	return false, fmt.Errorf("documentation reduce: provider request preparation: %w", err)
}

func cubeState(phase, guidanceSHA string, requestWire []byte) []byte {
	requestDigest := sha256.Sum256(requestWire)
	state, _ := json.Marshal(struct {
		Contract              string `json:"contract"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		Phase                 string `json:"phase"`
		GuidanceSHA256        string `json:"guidance_sha256"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: preparationVersion,
		ResponseSchemaVersion: responseSchemaVersion, Phase: phase,
		GuidanceSHA256: guidanceSHA, RequestSHA256: hex.EncodeToString(requestDigest[:]),
	})
	return state
}

func limits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes:  llm.SemanticRecordByteLimit,
		MaxResponseBytes: llm.ProviderResponseByteLimit,
		MaxOutputTokens:  maxOutputTokens,
	}
}

func canonicalCandidates(values []normalizedReduction) []normalizedReduction {
	result := make([]normalizedReduction, len(values))
	for position, value := range values {
		result[position] = normalizedReduction{
			overview: value.overview, sources: cloneResponseSources(value.sources),
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, _ := json.Marshal(modelResponse{Overview: result[i].overview, Sources: result[i].sources})
		right, _ := json.Marshal(modelResponse{Overview: result[j].overview, Sources: result[j].sources})
		return string(left) < string(right)
	})
	return result
}

func candidateFootprint(values []normalizedReduction) (int, error) {
	wire, err := json.Marshal(mergeCandidateWires(values))
	if err != nil {
		return 0, fmt.Errorf("documentation reduce: encode merge footprint: %w", err)
	}
	return len(wire), nil
}

func cloneResponseSources(values []responseSource) []responseSource {
	result := make([]responseSource, len(values))
	for position, value := range values {
		result[position] = responseSource{
			Ref:      value.Ref,
			Claims:   append([]string(nil), value.Claims...),
			Concepts: append([]string(nil), value.Concepts...),
		}
	}
	if result == nil {
		return []responseSource{}
	}
	return result
}

func emptyReduction(value normalizedReduction) bool {
	return value.overview == "" && len(value.sources) == 0
}

func splitUTF8(value string) (string, string, bool) {
	if value == "" || !utf8.ValidString(value) {
		return "", "", false
	}
	middle := len(value) / 2
	for _, separator := range []string{"\n\n", "\n"} {
		if boundary := nearestTextBoundary(value, middle, separator); boundary > 0 && boundary < len(value) {
			return value[:boundary], value[boundary:], true
		}
	}
	for middle < len(value) && !utf8.RuneStart(value[middle]) {
		middle++
	}
	if middle == 0 || middle == len(value) {
		return "", "", false
	}
	return value[:middle], value[middle:], true
}

func nearestTextBoundary(value string, middle int, separator string) int {
	left := strings.LastIndex(value[:middle], separator)
	if left >= 0 {
		left += len(separator)
	}
	right := strings.Index(value[middle:], separator)
	if right >= 0 {
		right += middle + len(separator)
	}
	switch {
	case left <= 0:
		return right
	case right <= 0 || middle-left <= right-middle:
		return left
	default:
		return right
	}
}
