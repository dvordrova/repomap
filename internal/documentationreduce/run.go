package documentationreduce

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	preparationVersion    = 1
	responseSchemaVersion = 1
	maxOutputTokens       = 128_000
)

//go:embed prompt.md
var sourcePrompt string

//go:embed merge_prompt.md
var mergePrompt string

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
	Version      int            `json:"version"`
	Batch        batchWire      `json:"batch"`
	ContentTrust string         `json:"content_trust"`
	Documents    []documentWire `json:"documents"`
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
	Version      int                  `json:"version"`
	Level        int                  `json:"level"`
	Batch        batchWire            `json:"batch"`
	ContentTrust string               `json:"content_trust"`
	Candidates   []mergeCandidateWire `json:"candidates"`
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
	units   []documentUnit
	request sourceRequest
	wire    []byte
	allowed map[string]documentAuthority
}

type normalizedReduction struct {
	overview string
	sources  []responseSource
}

type mergeBatch struct {
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
	var executed []sourceBatch
	_, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
		ctx, executor, provider, batches,
		func(plan []sourceBatch) ([]llm.Call[normalizedReduction], error) {
			materialized, materializeErr := materializeSourcePlan(plan)
			if materializeErr != nil {
				return nil, materializeErr
			}
			executed = materialized
			calls := make([]llm.Call[normalizedReduction], len(materialized))
			for position := range materialized {
				batch := materialized[position]
				calls[position] = llm.Call[normalizedReduction]{
					State: cubeState("source", snapshot.SHA256, batch.wire),
					Prompt: llm.Prompt{
						System: strings.TrimSpace(sourcePrompt), User: string(batch.wire),
						ResponseFormatJSON: true,
					},
					Limits: limits(),
					DecodeValidate: func(raw []byte) (normalizedReduction, error) {
						return normalizeResponse(raw, batch.allowed)
					},
				}
			}
			return calls, nil
		},
		splitSourceBatch,
	)
	if err != nil {
		return Result{}, fmt.Errorf("documentation reduce: source batches: %w", err)
	}
	if len(executed) != len(outcomes) {
		return Result{}, fmt.Errorf("documentation reduce: source execution plan was not materialized")
	}
	candidates := make([]normalizedReduction, 0, len(outcomes))
	for _, outcome := range outcomes {
		if !emptyReduction(outcome.Value) {
			candidates = append(candidates, outcome.Value)
		}
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
	if len(candidates) != 1 {
		return Result{}, fmt.Errorf("documentation reduce: convergent reduction did not produce one result")
	}
	sources, err := restoreSources(candidates[0].sources, authority)
	if err != nil {
		return Result{}, err
	}
	return sealResult(snapshot, candidates[0].overview, sources)
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
	current := make([]documentUnit, 0)
	for _, unit := range units {
		candidate := append(append([]documentUnit(nil), current...), unit)
		fits, err := sourceUnitsFit(provider, candidate)
		if err != nil {
			return nil, err
		}
		if fits {
			current = candidate
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf("documentation reduce: indivisible document slice does not fit provider request")
		}
		packed = append(packed, sourceBatch{units: current})
		current = []documentUnit{unit}
	}
	if len(current) > 0 {
		packed = append(packed, sourceBatch{units: current})
	}
	return materializeSourcePlan(packed)
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
		ContentTrust: "untrusted_repository_text", Documents: documents,
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
			ContentTrust: "untrusted_repository_text", Documents: documents,
		}
		wire, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: encode source batch: %w", err)
		}
		result[batchPosition] = sourceBatch{
			units:   append([]documentUnit(nil), planned.units...),
			request: request, wire: wire, allowed: allowed,
		}
	}
	return result, nil
}

func splitSourceBatch(batch sourceBatch) (sourceBatch, sourceBatch, bool) {
	if len(batch.units) > 1 {
		middle := len(batch.units) / 2
		return sourceBatch{units: append([]documentUnit(nil), batch.units[:middle]...)},
			sourceBatch{units: append([]documentUnit(nil), batch.units[middle:]...)}, true
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
	return sourceBatch{units: []documentUnit{left}}, sourceBatch{units: []documentUnit{right}}, true
}

func mergeTournament(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	guidanceSHA string,
	authority map[string]documentAuthority,
	candidates []normalizedReduction,
) ([]normalizedReduction, error) {
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
		var executed []mergeBatch
		_, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
			ctx, executor, provider, batches,
			func(plan []mergeBatch) ([]llm.Call[normalizedReduction], error) {
				materialized, materializeErr := materializeMergePlan(plan, level, authority)
				if materializeErr != nil {
					return nil, materializeErr
				}
				executed = materialized
				calls := make([]llm.Call[normalizedReduction], len(materialized))
				for position := range materialized {
					batch := materialized[position]
					calls[position] = llm.Call[normalizedReduction]{
						State: cubeState("merge", guidanceSHA, batch.wire),
						Prompt: llm.Prompt{
							System: strings.TrimSpace(mergePrompt), User: string(batch.wire),
							ResponseFormatJSON: true,
						},
						Limits: limits(),
						DecodeValidate: func(raw []byte) (normalizedReduction, error) {
							return normalizeResponse(raw, batch.allowed)
						},
					}
				}
				return calls, nil
			},
			splitMergeBatch,
		)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: merge level %d: %w", level, err)
		}
		if len(executed) != len(outcomes) {
			return nil, fmt.Errorf("documentation reduce: merge execution plan was not materialized")
		}
		next := make([]normalizedReduction, 0, len(outcomes))
		for _, outcome := range outcomes {
			if !emptyReduction(outcome.Value) {
				next = append(next, outcome.Value)
			}
		}
		if len(next) == 0 {
			return nil, nil
		}
		next = canonicalCandidates(next)
		after, err := candidateFootprint(next)
		if err != nil {
			return nil, err
		}
		if len(next) >= len(candidates) && after >= before {
			return nil, fmt.Errorf(
				"documentation reduce: merge level %d made no count or byte progress; no context was truncated",
				level,
			)
		}
		candidates = next
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
	current := make([]normalizedReduction, 0)
	for _, candidate := range candidates {
		proposed := append(append([]normalizedReduction(nil), current...), candidate)
		fits, err := mergeCandidatesFit(provider, proposed, level)
		if err != nil {
			return nil, err
		}
		if fits {
			current = proposed
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf("documentation reduce: indivisible merge candidate does not fit provider request; no context was truncated")
		}
		packed = append(packed, mergeBatch{candidates: current})
		current = []normalizedReduction{candidate}
	}
	if len(current) > 0 {
		packed = append(packed, mergeBatch{candidates: current})
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
		ContentTrust: "untrusted_repository_summary",
		Candidates:   mergeCandidateWires(candidates),
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
			ContentTrust: "untrusted_repository_summary",
			Candidates:   mergeCandidateWires(planned.candidates),
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
	return mergeBatch{candidates: append([]normalizedReduction(nil), batch.candidates[:middle]...)},
		mergeBatch{candidates: append([]normalizedReduction(nil), batch.candidates[middle:]...)}, true
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

func normalizeResponse(
	raw []byte,
	allowed map[string]documentAuthority,
) (normalizedReduction, error) {
	if len(raw) == 0 || len(raw) > llm.ProviderResponseByteLimit {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: response exceeds bounded envelope")
	}
	normalizedJSON, err := llm.NormalizeJSON(raw)
	if err != nil {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: invalid response JSON: %w", err)
	}
	var response modelResponse
	decoder := json.NewDecoder(bytes.NewReader(normalizedJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: response has trailing data")
	}
	if response.Sources == nil {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: response sources are missing")
	}
	if response.Overview != "" && !validText(response.Overview) {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: response overview is invalid")
	}

	type sourceSets struct {
		claims   []string
		concepts []string
	}
	byRef := make(map[string]sourceSets)
	for _, source := range response.Sources {
		if _, known := allowed[source.Ref]; !known {
			continue
		}
		if source.Claims == nil || source.Concepts == nil {
			return normalizedReduction{}, fmt.Errorf("documentation reduce: response source sets are missing")
		}
		claims, err := canonicalizeText(source.Claims)
		if err != nil {
			return normalizedReduction{}, fmt.Errorf("documentation reduce: response claims: %w", err)
		}
		concepts, err := canonicalizeText(source.Concepts)
		if err != nil {
			return normalizedReduction{}, fmt.Errorf("documentation reduce: response concepts: %w", err)
		}
		if len(claims)+len(concepts) == 0 {
			continue
		}
		sets := byRef[source.Ref]
		sets.claims = append(sets.claims, claims...)
		sets.concepts = append(sets.concepts, concepts...)
		byRef[source.Ref] = sets
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		left, right := allowed[refs[i]], allowed[refs[j]]
		if left.path != right.path {
			return left.path < right.path
		}
		return refs[i] < refs[j]
	})
	sources := make([]responseSource, 0, len(refs))
	for _, ref := range refs {
		sets := byRef[ref]
		claims, _ := canonicalizeText(sets.claims)
		concepts, _ := canonicalizeText(sets.concepts)
		sources = append(sources, responseSource{Ref: ref, Claims: claims, Concepts: concepts})
	}
	if response.Overview != "" && len(sources) == 0 {
		return normalizedReduction{}, fmt.Errorf("documentation reduce: response overview has no known source")
	}
	return normalizedReduction{overview: response.Overview, sources: sources}, nil
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
	_, err = provider.Prepare(llm.Prompt{
		System: strings.TrimSpace(systemPrompt), User: string(wire), ResponseFormatJSON: true,
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
