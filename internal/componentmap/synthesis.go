package componentmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	SynthesisRequestVersion = 3
	SynthesisRecordVersion  = 3
	SynthesisPromptVersion  = "architecture-grounding-v5"

	maxSynthesisRequestBytes  = 1 << 20
	maxSynthesisPromptBytes   = maxSynthesisRequestBytes + (16 << 10)
	maxSynthesisResponseBytes = 256 << 10
	maxSynthesisRecordBytes   = 512 << 10
	maxSynthesisWarnings      = 128
	maxRevisionBytes          = 256
	maxProfileBytes           = 128
	maxModelBytes             = 256
)

// SynthesisCandidate is the provider-visible candidate shape. Display names
// without provenance are deliberately omitted; the provider receives only an
// opaque typed ID and exact local facts and bindings.
type SynthesisCandidate struct {
	ID             MemberID            `json:"id"`
	ParentID       *MemberID           `json:"parent_id,omitempty"`
	Participations []FlowParticipation `json:"flow_participations,omitempty"`
	Facts          []LocalFact         `json:"facts"`
}

// SynthesisFlow keeps a flow opaque while retaining its exact local facts.
type SynthesisFlow struct {
	ID    FlowID      `json:"id"`
	Facts []LocalFact `json:"facts"`
}

// SynthesisRequest is the complete model-visible payload. Its type has no
// place for raw repository trees, report styles, layout coordinates, or model-
// supplied relations.
type SynthesisRequest struct {
	Version               int                      `json:"version"`
	ContractVersion       int                      `json:"contract_version"`
	PromptVersion         string                   `json:"prompt_version"`
	RepositoryArchetype   RepositoryArchetype      `json:"repository_archetype"`
	GroundingMode         GroundingMode            `json:"grounding_mode"`
	AllowedPaths          []string                 `json:"allowed_paths"`
	BehaviorAnchors       []BehaviorAnchor         `json:"behavior_anchors,omitempty"`
	Flows                 []SynthesisFlow          `json:"flows,omitempty"`
	Candidates            []SynthesisCandidate     `json:"candidates"`
	Relations             []LocalRelation          `json:"supporting_relations,omitempty"`
	AnchorBindings        []FlowAnchorBinding      `json:"flow_anchor_bindings,omitempty"`
	ResearchFindings      []ResearchInterpretation `json:"accepted_research_findings,omitempty"`
	ResearchPolicyVersion string                   `json:"research_policy_version,omitempty"`
}

// SynthesisPrompt is the provider-neutral instruction plus the exact bounded
// request JSON. Transport adapters may wrap these strings in their native chat
// format but must not add repository material.
type SynthesisPrompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

// ResponseState keeps oversized provider output replayable without storing an
// unbounded response body.
type ResponseState string

const (
	ResponseCaptured         ResponseState = "captured"
	ResponseOversize         ResponseState = "oversize_omitted"
	ResponseSensitiveOmitted ResponseState = "sensitive_omitted"
)

// SynthesisMetadata is saved beside the singular provider call. Validation
// warnings and fallback are outcomes of local Apply, never provider claims.
type SynthesisMetadata struct {
	PromptVersion          string                   `json:"prompt_version"`
	Profile                string                   `json:"profile"`
	Model                  string                   `json:"model"`
	OutputLanguage         string                   `json:"output_language"`
	InputBytes             int                      `json:"input_bytes"`
	LatencyMillis          int64                    `json:"latency_ms"`
	InputTokens            int                      `json:"input_tokens,omitempty"`
	OutputTokens           int                      `json:"output_tokens,omitempty"`
	ValidationWarnings     []Diagnostic             `json:"validation_warnings,omitempty"`
	ValidationOutcome      ValidationOutcome        `json:"validation_outcome"`
	ArchitectureSource     ArchitectureSource       `json:"architecture_source"`
	ArchitectureLevel      int                      `json:"architecture_level"`
	Normalizations         []NormalizationOperation `json:"normalization_operations,omitempty"`
	OriginalProposalSHA256 string                   `json:"original_proposal_sha256,omitempty"`
	FallbackReason         FallbackReason           `json:"fallback_reason,omitempty"`
}

// SynthesisCall is one already-completed provider interaction. No provider
// interface or network operation lives in this package.
type SynthesisCall struct {
	Metadata      SynthesisMetadata `json:"metadata"`
	ResponseState ResponseState     `json:"response_state"`
	ResponseBytes int               `json:"response_bytes"`
	Response      []byte            `json:"response,omitempty"`
}

// SynthesisRecord intentionally has one optional Call field rather than call
// history. This represents one call for one exact bounded synthesis request.
type SynthesisRecord struct {
	Version            int            `json:"version"`
	RepositoryRevision string         `json:"repository_revision"`
	CacheKey           string         `json:"cache_key"`
	RequestSHA256      string         `json:"request_sha256"`
	Call               *SynthesisCall `json:"call,omitempty"`
}

type SynthesisResult struct {
	Landscape Landscape
	Record    SynthesisRecord
}

// BuildSynthesisRequest validates and canonically orders the bounded local
// inputs before encoding the exact bytes intended for a provider.
func BuildSynthesisRequest(bundle CandidateBundle) (SynthesisRequest, []byte, error) {
	if err := bundle.Validate(); err != nil {
		return SynthesisRequest{}, nil, err
	}

	request := SynthesisRequest{
		Version:               SynthesisRequestVersion,
		ContractVersion:       ContractVersion,
		PromptVersion:         SynthesisPromptVersion,
		RepositoryArchetype:   bundle.RepositoryArchetype,
		GroundingMode:         bundle.GroundingMode,
		AllowedPaths:          synthesisAllowedPaths(bundle),
		BehaviorAnchors:       cloneBehaviorAnchors(bundle.BehaviorAnchors),
		Candidates:            make([]SynthesisCandidate, 0, len(bundle.Candidates)),
		Flows:                 make([]SynthesisFlow, 0, len(bundle.Flows)),
		Relations:             cloneLocalRelations(bundle.Relations),
		AnchorBindings:        cloneFlowAnchorBindings(bundle.AnchorBindings),
		ResearchFindings:      cloneResearchInterpretations(bundle.ResearchFindings),
		ResearchPolicyVersion: bundle.ResearchPolicyVersion,
	}
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	for _, candidate := range candidates {
		cloned := cloneCandidate(candidate)
		request.Candidates = append(request.Candidates, SynthesisCandidate{
			ID: cloned.ID, ParentID: cloned.ParentID,
			Participations: cloned.Participations, Facts: cloned.Facts,
		})
	}
	flows := append([]Flow(nil), bundle.Flows...)
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	for _, flow := range flows {
		facts := make([]LocalFact, len(flow.Facts))
		for index, fact := range flow.Facts {
			facts[index] = cloneLocalFact(fact)
		}
		request.Flows = append(request.Flows, SynthesisFlow{ID: flow.ID, Facts: facts})
	}
	sort.Slice(request.Relations, func(i, j int) bool { return request.Relations[i].ID < request.Relations[j].ID })
	sort.Slice(request.BehaviorAnchors, func(i, j int) bool { return request.BehaviorAnchors[i].ID < request.BehaviorAnchors[j].ID })
	sort.Slice(request.AnchorBindings, func(i, j int) bool {
		left := string(request.AnchorBindings[i].FlowID) + "\x00" + request.AnchorBindings[i].AnchorID + "\x00" + request.AnchorBindings[i].MemberID.key()
		right := string(request.AnchorBindings[j].FlowID) + "\x00" + request.AnchorBindings[j].AnchorID + "\x00" + request.AnchorBindings[j].MemberID.key()
		return left < right
	})
	sort.Slice(request.ResearchFindings, func(i, j int) bool {
		return request.ResearchFindings[i].ID < request.ResearchFindings[j].ID
	})

	encoded, err := json.Marshal(request)
	if err != nil {
		return SynthesisRequest{}, nil, fmt.Errorf("componentmap: encode synthesis request: %w", err)
	}
	if len(encoded) > maxSynthesisRequestBytes {
		return SynthesisRequest{}, nil, fmt.Errorf(
			"componentmap: synthesis request is %d bytes, limit is %d",
			len(encoded), maxSynthesisRequestBytes,
		)
	}
	if synthesisJSONContainsCredential(encoded) {
		return SynthesisRequest{}, nil, fmt.Errorf("componentmap: synthesis request contains an obvious credential")
	}
	return request, encoded, nil
}

func synthesisAllowedPaths(bundle CandidateBundle) []string {
	paths := make(map[string]struct{})
	add := func(location *evidence.Location) {
		if location != nil && location.Path != "" {
			paths[location.Path] = struct{}{}
		}
	}
	for _, anchor := range bundle.BehaviorAnchors {
		add(&anchor.Location)
		add(anchor.Producer.Location)
	}
	for _, candidate := range bundle.Candidates {
		for _, fact := range candidate.Facts {
			add(fact.Location)
			for _, provenance := range fact.Provenance {
				add(provenance.Location)
			}
		}
	}
	for _, relation := range bundle.Relations {
		add(relation.Location)
		for _, provenance := range relation.Provenance {
			add(provenance.Location)
		}
	}
	for _, binding := range bundle.AnchorBindings {
		add(binding.Location)
		for _, provenance := range binding.Provenance {
			add(provenance.Location)
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

// BuildSynthesisPrompt exposes the actual versioned synthesis instruction used
// by provider adapters. The output schema is intentionally smaller than the
// local Landscape: evidence, relations, certainty, layout, and styling remain
// local authority and cannot be returned by the model.
func BuildSynthesisPrompt(bundle CandidateBundle) (SynthesisPrompt, error) {
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return SynthesisPrompt{}, err
	}
	system := fmt.Sprintf(`You create a compact conceptual architecture landscape from bounded local repository facts.

Use candidate IDs as opaque values. Do not rewrite them, infer new IDs, or mention members absent from the request.
Local facts, structural relations, flow participation, certainty, provenance, scenarios, and source locations are read-only evidence. They help grouping but must never be returned, upgraded, replaced, or converted into execution order.
accepted_research_findings are validated model interpretations bound to supplied exact member, flow, anchor, and evidence IDs. They may guide conceptual grouping and responsibility names, but they are not local facts and cannot create or strengthen evidence.

Return exactly one compact JSON proposal object with this shape:
{"version":%d,"subsystems":[{"name":"short name","description":"short purpose","components":[{"name":"short name","description":"short purpose","member_ids":[{"kind":"package","value":"opaque supplied value"}],"anchor_ids":["supplied-anchor-id"],"hypothesis":false}]}]}

The only allowed proposal fields are version, subsystems, subsystem name/description/components, component name/description/member_ids/anchor_ids/hypothesis, and member kind/value. Member and anchor IDs must be copied exactly from the request. Array order is the conceptual display order. Assign each supplied candidate at most once. Never repeat a member ID across components; for a cross-cutting member choose its single best conceptual home. Omit an uncertain member rather than duplicating it because local validation retains omissions separately. Every component must contain at least one supplied member ID.

Repository archetype and grounding mode are local facts. A primary pillar is one top-level subsystem; components are nested responsibilities and are not additional primary pillars. When grounding_mode is behavior_grounded or mixed, choose four to seven top-level primary subsystems when the supplied evidence supports that many, never more than eight. Prefer one to four nested components per subsystem and no more than eighteen in total. Every non-hypothesis nested component must cite at least one supplied behavior anchor ID. Set hypothesis true only when a component is explicitly conceptual or package-derived; do not use it merely to avoid available anchors. Separate extension families from support and tooling. Preserve unresolved frontiers. When grounding_mode is package_landscape, describe an honest static package landscape and do not imply behavioral verification.

Do not return edges, relations, flow definitions or transitions, fact payloads, repository paths, symbol details, test details, evidence, certainty, provenance, scenarios, source locations, coordinates, dimensions, ports, colors, styles, UI settings, markdown, or explanatory prose. Do not claim temporal or runtime behavior from static relations.`, ProposalVersion)
	user := "Bounded candidate request:\n" + string(requestJSON)
	if len(system)+len(user) > maxSynthesisPromptBytes {
		return SynthesisPrompt{}, fmt.Errorf("componentmap: synthesis prompt exceeds the local byte limit")
	}
	return SynthesisPrompt{Version: SynthesisPromptVersion, System: system, User: user}, nil
}

// SynthesisCacheKey binds one conceptual synthesis to the exact bounded local
// request as well as the repository revision and prompt contract.
func SynthesisCacheKey(repositoryRevision string, bundle CandidateBundle) (string, error) {
	return SynthesisCacheKeyForProvider(repositoryRevision, bundle, "", "")
}

// SynthesisCacheKeyForProvider additionally binds the cache to the configured
// provider profile and model. ResearchPolicyVersion is part of the canonical
// request whenever a normal adaptive run builds the bundle.
func SynthesisCacheKeyForProvider(repositoryRevision string, bundle CandidateBundle, profile, model string) (string, error) {
	return synthesisCacheKeyForProvider(repositoryRevision, bundle, profile, model, "")
}

// SynthesisCacheKeyForProviderAndLanguage additionally isolates provider
// responses whose human-readable prose was requested in another language.
// English intentionally retains the pre-language cache key so current English
// records remain path-compatible.
func SynthesisCacheKeyForProviderAndLanguage(
	repositoryRevision string,
	bundle CandidateBundle,
	profile string,
	model string,
	outputLanguage string,
) (string, error) {
	language, err := normalizeSynthesisOutputLanguage(outputLanguage)
	if err != nil {
		return "", err
	}
	if language == "en" {
		language = ""
	}
	return synthesisCacheKeyForProvider(repositoryRevision, bundle, profile, model, language)
}

func synthesisCacheKeyForProvider(
	repositoryRevision string,
	bundle CandidateBundle,
	profile string,
	model string,
	outputLanguage string,
) (string, error) {
	if err := validateSynthesisRevision(repositoryRevision); err != nil {
		return "", err
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "componentmap-synthesis\nrevision=%s\ncontract=%d\nprompt=%s\nprofile=%s\nmodel=%s\n",
		repositoryRevision, ContractVersion, SynthesisPromptVersion, profile, model)
	if outputLanguage != "" {
		fmt.Fprintf(hash, "language=%s\n", outputLanguage)
	}
	fmt.Fprintf(hash, "request=%s\n", sha256String(requestJSON))
	return "component-synthesis-" + hex.EncodeToString(hash.Sum(nil)), nil
}

// RecordSynthesisResponse evaluates an already-received response, records one
// bounded call, and returns the locally authoritative landscape. It performs
// no network I/O.
func RecordSynthesisResponse(
	bundle CandidateBundle,
	repositoryRevision string,
	profile string,
	model string,
	latency time.Duration,
	rawResponse []byte,
) (SynthesisResult, error) {
	return RecordSynthesisResponseForLanguage(
		bundle,
		repositoryRevision,
		profile,
		model,
		"en",
		latency,
		rawResponse,
	)
}

// RecordSynthesisResponseForLanguage records the language identity that shaped
// provider prose. Historical records without this field remain replayable, but
// active cache selection can distinguish them from explicitly authored output.
func RecordSynthesisResponseForLanguage(
	bundle CandidateBundle,
	repositoryRevision string,
	profile string,
	model string,
	outputLanguage string,
	latency time.Duration,
	rawResponse []byte,
) (SynthesisResult, error) {
	if latency < 0 {
		return SynthesisResult{}, fmt.Errorf("componentmap: synthesis latency cannot be negative")
	}
	if err := validateSynthesisLabel("profile", profile, maxProfileBytes); err != nil {
		return SynthesisResult{}, err
	}
	if err := validateSynthesisLabel("model", model, maxModelBytes); err != nil {
		return SynthesisResult{}, err
	}
	language, err := normalizeSynthesisOutputLanguage(outputLanguage)
	if err != nil {
		return SynthesisResult{}, err
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return SynthesisResult{}, err
	}
	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		return SynthesisResult{}, err
	}
	cacheKey, err := SynthesisCacheKeyForProviderAndLanguage(repositoryRevision, bundle, profile, model, language)
	if err != nil {
		return SynthesisResult{}, err
	}

	state := ResponseCaptured
	response := append([]byte(nil), rawResponse...)
	if len(rawResponse) > maxSynthesisResponseBytes {
		state = ResponseOversize
		response = nil
	} else if synthesisResponseContainsCredential(rawResponse) {
		state = ResponseSensitiveOmitted
		response = nil
	}
	landscape, err := evaluateSynthesisResponse(bundle, state, response)
	if err != nil {
		return SynthesisResult{}, err
	}
	record := SynthesisRecord{
		Version:            SynthesisRecordVersion,
		RepositoryRevision: repositoryRevision,
		CacheKey:           cacheKey,
		RequestSHA256:      sha256String(requestJSON),
		Call: &SynthesisCall{
			Metadata: SynthesisMetadata{
				PromptVersion: SynthesisPromptVersion,
				Profile:       profile, Model: model,
				OutputLanguage: language,
				InputBytes:     synthesisPromptSize(prompt), LatencyMillis: latency.Milliseconds(),
				ValidationWarnings:     cloneDiagnostics(landscape.Diagnostics),
				ValidationOutcome:      landscape.ValidationOutcome,
				ArchitectureSource:     landscape.Source,
				ArchitectureLevel:      landscape.Level,
				Normalizations:         append([]NormalizationOperation(nil), landscape.Normalizations...),
				OriginalProposalSHA256: landscape.OriginalProposalSHA256,
				FallbackReason:         landscape.FallbackReason,
			},
			ResponseState: state, ResponseBytes: len(rawResponse), Response: response,
		},
	}
	if err := validateSynthesisRecord(bundle, repositoryRevision, record); err != nil {
		return SynthesisResult{}, err
	}
	return SynthesisResult{Landscape: landscape, Record: record}, nil
}

// ReplaySynthesis strictly decodes one saved record, rebuilds the exact local
// request, and re-applies the saved provider response without a provider call.
func ReplaySynthesis(bundle CandidateBundle, repositoryRevision string, saved []byte) (Landscape, error) {
	if len(saved) == 0 || len(saved) > maxSynthesisRecordBytes {
		return Landscape{}, fmt.Errorf("componentmap: saved synthesis record is empty or too large")
	}
	var record SynthesisRecord
	if err := decodeStrictJSON(saved, &record); err != nil {
		return Landscape{}, fmt.Errorf("componentmap: decode synthesis record: %w", err)
	}
	if err := validateSynthesisRecord(bundle, repositoryRevision, record); err != nil {
		return Landscape{}, err
	}

	landscape, err := evaluateSynthesisResponse(bundle, record.Call.ResponseState, record.Call.Response)
	if err != nil {
		return Landscape{}, err
	}
	if !diagnosticsEqual(record.Call.Metadata.ValidationWarnings, landscape.Diagnostics) {
		return Landscape{}, fmt.Errorf("componentmap: saved synthesis validation warnings do not replay")
	}
	if record.Call.Metadata.FallbackReason != landscape.FallbackReason {
		return Landscape{}, fmt.Errorf("componentmap: saved synthesis fallback reason does not replay")
	}
	metadata := record.Call.Metadata
	if metadata.ValidationOutcome != landscape.ValidationOutcome || metadata.ArchitectureSource != landscape.Source ||
		metadata.ArchitectureLevel != landscape.Level || !reflect.DeepEqual(metadata.Normalizations, landscape.Normalizations) ||
		metadata.OriginalProposalSHA256 != landscape.OriginalProposalSHA256 {
		return Landscape{}, fmt.Errorf("componentmap: saved synthesis validation outcome does not replay")
	}
	return landscape, nil
}

// ReplayLegacyCapturedSynthesis revalidates a captured v1 response against the
// current local bundle. This is intentionally limited to persisted approved
// component-landscape-v2 and architecture-grounding-v3 records; no old
// validation outcome, cache key, or warning is trusted.
func ReplayLegacyCapturedSynthesis(bundle CandidateBundle, saved []byte) (Landscape, error) {
	if len(saved) == 0 || len(saved) > maxSynthesisRecordBytes {
		return Landscape{}, fmt.Errorf("componentmap: legacy synthesis record is empty or too large")
	}
	var record struct {
		Version            int    `json:"version"`
		RepositoryRevision string `json:"repository_revision"`
		CacheKey           string `json:"cache_key"`
		RequestSHA256      string `json:"request_sha256"`
		Call               *struct {
			Metadata struct {
				PromptVersion      string         `json:"prompt_version"`
				Profile            string         `json:"profile"`
				Model              string         `json:"model"`
				InputBytes         int            `json:"input_bytes"`
				LatencyMillis      int64          `json:"latency_ms"`
				ValidationWarnings []Diagnostic   `json:"validation_warnings"`
				FallbackReason     FallbackReason `json:"fallback_reason"`
			} `json:"metadata"`
			ResponseState ResponseState `json:"response_state"`
			ResponseBytes int           `json:"response_bytes"`
			Response      []byte        `json:"response"`
		} `json:"call"`
	}
	if err := decodeStrictJSON(saved, &record); err != nil {
		return Landscape{}, fmt.Errorf("componentmap: decode legacy synthesis record: %w", err)
	}
	if record.Version != 1 || record.Call == nil ||
		(record.Call.Metadata.PromptVersion != "component-landscape-v2" &&
			record.Call.Metadata.PromptVersion != "architecture-grounding-v3") {
		return Landscape{}, fmt.Errorf("componentmap: unsupported legacy synthesis record")
	}
	if record.Call.ResponseState != ResponseCaptured || record.Call.ResponseBytes != len(record.Call.Response) ||
		len(record.Call.Response) == 0 || len(record.Call.Response) > maxSynthesisResponseBytes {
		return Landscape{}, fmt.Errorf("componentmap: legacy synthesis response is not a bounded capture")
	}
	if synthesisResponseContainsCredential(record.Call.Response) {
		return Landscape{}, fmt.Errorf("componentmap: legacy synthesis response violates the obvious credential policy")
	}
	response := record.Call.Response
	if record.Call.Metadata.PromptVersion == "component-landscape-v2" {
		object, _, responseErr := extractProposalObject(response)
		if responseErr != nil {
			return Landscape{}, fmt.Errorf("componentmap: legacy synthesis response is invalid")
		}
		proposal, _, err := decodeProposalJSON(object)
		if err != nil || proposal.Version != 2 {
			return Landscape{}, fmt.Errorf("componentmap: legacy synthesis proposal is invalid")
		}
		proposal.Version = ProposalVersion
		response, err = json.Marshal(proposal)
		if err != nil {
			return Landscape{}, fmt.Errorf("componentmap: encode upgraded legacy synthesis proposal: %w", err)
		}
	}
	result, err := RecordSynthesisResponse(
		bundle,
		record.RepositoryRevision,
		record.Call.Metadata.Profile,
		record.Call.Metadata.Model,
		time.Duration(record.Call.Metadata.LatencyMillis)*time.Millisecond,
		response,
	)
	if err != nil {
		return Landscape{}, err
	}
	return result.Landscape, nil
}

func validateSynthesisRecord(bundle CandidateBundle, repositoryRevision string, record SynthesisRecord) error {
	if record.Version != SynthesisRecordVersion {
		return fmt.Errorf("componentmap: unsupported synthesis record version %d", record.Version)
	}
	if err := validateSynthesisRevision(repositoryRevision); err != nil {
		return err
	}
	if record.RepositoryRevision != repositoryRevision {
		return fmt.Errorf("componentmap: synthesis record repository revision does not match")
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return err
	}
	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		return err
	}
	if record.RequestSHA256 != sha256String(requestJSON) {
		return fmt.Errorf("componentmap: synthesis record request digest does not match")
	}
	if record.Call == nil {
		return fmt.Errorf("componentmap: synthesis record has no represented call")
	}
	metadata := record.Call.Metadata
	var expectedCacheKey string
	if metadata.OutputLanguage == "" {
		// Records written before output-language identity existed are still
		// valid historical report artifacts. After bounded replay validation,
		// active cache selection rejects their unknown language.
		expectedCacheKey, err = SynthesisCacheKeyForProvider(repositoryRevision, bundle, metadata.Profile, metadata.Model)
	} else {
		language, languageErr := normalizeSynthesisOutputLanguage(metadata.OutputLanguage)
		if languageErr != nil || language != metadata.OutputLanguage {
			return fmt.Errorf("componentmap: synthesis record output language is invalid")
		}
		expectedCacheKey, err = SynthesisCacheKeyForProviderAndLanguage(
			repositoryRevision,
			bundle,
			metadata.Profile,
			metadata.Model,
			language,
		)
	}
	if err != nil {
		return err
	}
	if record.CacheKey != expectedCacheKey {
		return fmt.Errorf("componentmap: synthesis record cache key does not match")
	}
	if metadata.PromptVersion != SynthesisPromptVersion {
		return fmt.Errorf("componentmap: synthesis record prompt version does not match")
	}
	if err := validateSynthesisLabel("profile", metadata.Profile, maxProfileBytes); err != nil {
		return err
	}
	if err := validateSynthesisLabel("model", metadata.Model, maxModelBytes); err != nil {
		return err
	}
	if metadata.InputBytes != synthesisPromptSize(prompt) {
		return fmt.Errorf("componentmap: synthesis record input byte count does not match")
	}
	if metadata.LatencyMillis < 0 {
		return fmt.Errorf("componentmap: synthesis record latency cannot be negative")
	}
	if metadata.InputTokens < 0 || metadata.OutputTokens < 0 {
		return fmt.Errorf("componentmap: synthesis record token counts cannot be negative")
	}
	if len(metadata.ValidationWarnings) > maxSynthesisWarnings {
		return fmt.Errorf("componentmap: synthesis record has too many validation warnings")
	}
	for index, warning := range metadata.ValidationWarnings {
		if err := validateDiagnostic(warning); err != nil {
			return fmt.Errorf("componentmap: synthesis validation warning[%d]: %w", index, err)
		}
	}
	if metadata.FallbackReason != "" && !validFallbackReason(metadata.FallbackReason) {
		return fmt.Errorf("componentmap: model-assisted synthesis has invalid fallback reason %q", metadata.FallbackReason)
	}
	if !validValidationOutcome(metadata.ValidationOutcome) || !validArchitectureSource(metadata.ArchitectureSource) ||
		metadata.ArchitectureLevel < 1 || metadata.ArchitectureLevel > 4 {
		return fmt.Errorf("componentmap: synthesis validation outcome metadata is invalid")
	}
	for index, operation := range metadata.Normalizations {
		if err := validateNormalizationOperation(operation); err != nil {
			return fmt.Errorf("componentmap: synthesis normalization[%d]: %w", index, err)
		}
	}
	if metadata.OriginalProposalSHA256 != "" && len(metadata.OriginalProposalSHA256) != 64 {
		return fmt.Errorf("componentmap: synthesis original proposal digest is malformed")
	}
	switch record.Call.ResponseState {
	case ResponseCaptured:
		if record.Call.ResponseBytes != len(record.Call.Response) || len(record.Call.Response) > maxSynthesisResponseBytes {
			return fmt.Errorf("componentmap: captured synthesis response byte count is invalid")
		}
		if synthesisResponseContainsCredential(record.Call.Response) {
			return fmt.Errorf("componentmap: captured synthesis response violates the obvious credential policy")
		}
	case ResponseOversize:
		if record.Call.ResponseBytes <= maxSynthesisResponseBytes || len(record.Call.Response) != 0 {
			return fmt.Errorf("componentmap: oversized synthesis response record is invalid")
		}
	case ResponseSensitiveOmitted:
		if record.Call.ResponseBytes < 1 || record.Call.ResponseBytes > maxSynthesisResponseBytes || len(record.Call.Response) != 0 {
			return fmt.Errorf("componentmap: sensitive synthesis response record is invalid")
		}
	default:
		return fmt.Errorf("componentmap: invalid synthesis response state %q", record.Call.ResponseState)
	}
	return nil
}

func normalizeSynthesisOutputLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return "en", nil
	case "ru":
		return "ru", nil
	default:
		return "", fmt.Errorf("componentmap: synthesis output language must be \"en\" or \"ru\"")
	}
}

func evaluateSynthesisResponse(bundle CandidateBundle, state ResponseState, response []byte) (Landscape, error) {
	if state == ResponseOversize {
		return synthesisResponseFallback(bundle, newDiagnostic(
			"response.too_large",
			"provider response exceeded the bounded synthesis response limit and was not retained",
		))
	}
	if state == ResponseSensitiveOmitted {
		return synthesisResponseFallback(bundle, newDiagnostic(
			"response.sensitive_omitted",
			"provider response matched the obvious credential policy and was not retained",
		))
	}
	object, normalization, responseErr := extractProposalObject(response)
	if responseErr != nil {
		return synthesisResponseFallback(bundle, newDiagnostic(responseErr.code, responseErr.message))
	}
	proposal, unknownFields, err := decodeProposalJSON(object)
	if err != nil {
		return synthesisResponseFallback(bundle, newDiagnostic(
			"response.invalid_proposal",
			"recovered json does not satisfy the bounded proposal schema",
		))
	}
	landscape, err := Apply(bundle, proposal)
	if err != nil {
		return Landscape{}, err
	}
	warnings := make([]Diagnostic, 0, 2)
	if normalization != nil {
		warnings = append(warnings, *normalization)
	}
	if unknownFields {
		warnings = append(warnings, newDiagnostic(
			"response.unknown_fields_ignored",
			"ignored bounded response fields outside the conceptual proposal contract",
		))
	}
	if len(warnings) > 0 {
		landscape.Diagnostics = append(warnings, landscape.Diagnostics...)
		if err := landscape.Validate(bundle); err != nil {
			return Landscape{}, err
		}
	}
	return landscape, nil
}

func synthesisResponseFallback(bundle CandidateBundle, warning Diagnostic) (Landscape, error) {
	landscape, err := Apply(bundle, Proposal{})
	if err != nil {
		return Landscape{}, err
	}
	landscape.Diagnostics = append([]Diagnostic{warning}, landscape.Diagnostics...)
	if err := landscape.Validate(bundle); err != nil {
		return Landscape{}, err
	}
	return landscape, nil
}

type synthesisResponseError struct {
	code    string
	message string
}

func extractProposalObject(raw []byte) ([]byte, *Diagnostic, *synthesisResponseError) {
	if len(raw) > maxSynthesisResponseBytes {
		return nil, nil, &synthesisResponseError{
			code: "response.too_large", message: "provider response exceeded the bounded synthesis response limit",
		}
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, &synthesisResponseError{code: "response.no_json", message: "provider response contains no json object"}
	}
	if json.Valid(trimmed) {
		if trimmed[0] != '{' {
			return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is json but not a proposal object"}
		}
		return append([]byte(nil), trimmed...), nil, nil
	}

	fenced := fencedJSONObjectCandidates(trimmed)
	switch len(fenced) {
	case 1:
		if len(jsonObjectCandidates(trimmed, 2)) > 1 {
			return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several json objects"}
		}
		diagnostic := newDiagnostic("response.fenced_json_extracted", "accepted one bounded proposal object from a markdown fence")
		return fenced[0], &diagnostic, nil
	case 0:
	default:
		return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several fenced json objects"}
	}

	embedded := jsonObjectCandidates(trimmed, 2)
	switch len(embedded) {
	case 1:
		diagnostic := newDiagnostic("response.embedded_json_extracted", "accepted one bounded proposal object embedded in provider prose")
		return embedded[0], &diagnostic, nil
	case 0:
		return nil, nil, &synthesisResponseError{code: "response.no_json", message: "provider response contains no recoverable json object"}
	default:
		return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several json objects"}
	}
}

// decodeProposalJSON is strict about all known field types while tolerating a
// weak model's harmless commentary fields. Unknown values are never copied to
// the Proposal and their names/content are not echoed in diagnostics.
func decodeProposalJSON(raw []byte) (Proposal, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return Proposal{}, false, fmt.Errorf("proposal is not an object")
	}
	unknown := hasUnknownFields(root, "version", "subsystems")

	var proposal Proposal
	if value, exists := root["version"]; exists {
		if err := json.Unmarshal(value, &proposal.Version); err != nil {
			return Proposal{}, unknown, fmt.Errorf("proposal version has invalid type")
		}
	}
	if value, exists := root["subsystems"]; exists {
		var rawSubsystems []json.RawMessage
		if err := json.Unmarshal(value, &rawSubsystems); err != nil {
			return Proposal{}, unknown, fmt.Errorf("proposal subsystems have invalid type")
		}
		proposal.Subsystems = make([]ProposedSubsystem, 0, len(rawSubsystems))
		for _, rawSubsystem := range rawSubsystems {
			subsystem, itemUnknown, err := decodeProposedSubsystem(rawSubsystem)
			if err != nil {
				return Proposal{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			proposal.Subsystems = append(proposal.Subsystems, subsystem)
		}
	}
	return proposal, unknown, nil
}

func decodeProposedSubsystem(raw json.RawMessage) (ProposedSubsystem, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ProposedSubsystem{}, false, fmt.Errorf("proposal subsystem is not an object")
	}
	unknown := hasUnknownFields(fields, "name", "description", "components")
	name, err := decodeProposalString(fields, "name")
	if err != nil {
		return ProposedSubsystem{}, unknown, err
	}
	description, err := decodeProposalString(fields, "description")
	if err != nil {
		return ProposedSubsystem{}, unknown, err
	}
	result := ProposedSubsystem{Name: name, Description: description}
	if value, exists := fields["components"]; exists {
		var rawComponents []json.RawMessage
		if err := json.Unmarshal(value, &rawComponents); err != nil {
			return ProposedSubsystem{}, unknown, fmt.Errorf("proposal components have invalid type")
		}
		result.Components = make([]ProposedComponent, 0, len(rawComponents))
		for _, rawComponent := range rawComponents {
			component, itemUnknown, err := decodeProposedComponent(rawComponent)
			if err != nil {
				return ProposedSubsystem{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			result.Components = append(result.Components, component)
		}
	}
	return result, unknown, nil
}

func decodeProposedComponent(raw json.RawMessage) (ProposedComponent, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ProposedComponent{}, false, fmt.Errorf("proposal component is not an object")
	}
	unknown := hasUnknownFields(fields, "name", "description", "member_ids", "anchor_ids", "hypothesis")
	name, err := decodeProposalString(fields, "name")
	if err != nil {
		return ProposedComponent{}, unknown, err
	}
	description, err := decodeProposalString(fields, "description")
	if err != nil {
		return ProposedComponent{}, unknown, err
	}
	result := ProposedComponent{Name: name, Description: description}
	if value, exists := fields["member_ids"]; exists {
		var rawMemberIDs []json.RawMessage
		if err := json.Unmarshal(value, &rawMemberIDs); err != nil {
			return ProposedComponent{}, unknown, fmt.Errorf("proposal member ids have invalid type")
		}
		result.MemberIDs = make([]MemberID, 0, len(rawMemberIDs))
		for _, rawMemberID := range rawMemberIDs {
			memberID, itemUnknown, err := decodeProposedMemberID(rawMemberID)
			if err != nil {
				return ProposedComponent{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			result.MemberIDs = append(result.MemberIDs, memberID)
		}
	}
	if value, exists := fields["anchor_ids"]; exists {
		if err := json.Unmarshal(value, &result.AnchorIDs); err != nil {
			return ProposedComponent{}, unknown, fmt.Errorf("proposal anchor ids have invalid type")
		}
	}
	if value, exists := fields["hypothesis"]; exists {
		if err := json.Unmarshal(value, &result.Hypothesis); err != nil {
			return ProposedComponent{}, unknown, fmt.Errorf("proposal hypothesis has invalid type")
		}
	}
	return result, unknown, nil
}

func cloneBehaviorAnchors(values []BehaviorAnchor) []BehaviorAnchor {
	if values == nil {
		return nil
	}
	result := make([]BehaviorAnchor, len(values))
	for index, value := range values {
		result[index] = value
		result[index].MemberIDs = append([]MemberID(nil), value.MemberIDs...)
		result[index].Limitations = append([]string(nil), value.Limitations...)
	}
	return result
}

func decodeProposedMemberID(raw json.RawMessage) (MemberID, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return MemberID{}, false, fmt.Errorf("proposal member id is not an object")
	}
	unknown := hasUnknownFields(fields, "kind", "value")
	kind, err := decodeProposalString(fields, "kind")
	if err != nil {
		return MemberID{}, unknown, err
	}
	value, err := decodeProposalString(fields, "value")
	if err != nil {
		return MemberID{}, unknown, err
	}
	return MemberID{Kind: MemberKind(kind), Value: value}, unknown, nil
}

func decodeProposalString(fields map[string]json.RawMessage, name string) (string, error) {
	value, exists := fields[name]
	if !exists {
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("proposal field has invalid string type")
	}
	return result, nil
}

func hasUnknownFields(fields map[string]json.RawMessage, allowed ...string) bool {
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	for field := range fields {
		if _, exists := known[field]; !exists {
			return true
		}
	}
	return false
}

func fencedJSONObjectCandidates(raw []byte) [][]byte {
	result := make([][]byte, 0, 2)
	for cursor := 0; cursor < len(raw) && len(result) < 2; {
		openOffset := bytes.Index(raw[cursor:], []byte("```"))
		if openOffset < 0 {
			break
		}
		contentStart := cursor + openOffset + 3
		closeOffset := bytes.Index(raw[contentStart:], []byte("```"))
		if closeOffset < 0 {
			break
		}
		contentEnd := contentStart + closeOffset
		content := bytes.TrimSpace(raw[contentStart:contentEnd])
		if newline := bytes.IndexByte(content, '\n'); newline >= 0 && strings.EqualFold(strings.TrimSpace(string(content[:newline])), "json") {
			content = bytes.TrimSpace(content[newline+1:])
		}
		result = append(result, jsonObjectCandidates(content, 2-len(result))...)
		cursor = contentEnd + 3
	}
	return result
}

func jsonObjectCandidates(raw []byte, limit int) [][]byte {
	result := make([][]byte, 0, limit)
	for index := 0; index < len(raw) && len(result) < limit; {
		relative := bytes.IndexByte(raw[index:], '{')
		if relative < 0 {
			break
		}
		start := index + relative
		decoder := json.NewDecoder(bytes.NewReader(raw[start:]))
		var candidate json.RawMessage
		if err := decoder.Decode(&candidate); err != nil || len(candidate) == 0 || candidate[0] != '{' {
			index = start + 1
			continue
		}
		result = append(result, append([]byte(nil), candidate...))
		consumed := int(decoder.InputOffset())
		if consumed <= 0 {
			consumed = 1
		}
		index = start + consumed
	}
	return result
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing json value")
		}
		return err
	}
	return nil
}

func validateSynthesisRevision(revision string) error {
	if len(revision) == 0 || len(revision) > maxRevisionBytes || strings.TrimSpace(revision) != revision || !utf8.ValidString(revision) {
		return fmt.Errorf("componentmap: repository revision is empty, malformed, or too long")
	}
	for _, char := range revision {
		if char <= 0x20 || char == 0x7f {
			return fmt.Errorf("componentmap: repository revision contains whitespace or control characters")
		}
	}
	return nil
}

func validateSynthesisLabel(field, value string, limit int) error {
	if err := validateDisplayText(field, value, limit, true); err != nil {
		return fmt.Errorf("componentmap: synthesis %w", err)
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("componentmap: synthesis %s contains control whitespace", field)
	}
	return nil
}

func sha256String(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func synthesisPromptSize(prompt SynthesisPrompt) int {
	return len(prompt.System) + len(prompt.User)
}

func synthesisJSONContainsCredential(encoded []byte) bool {
	if _, sensitive := secretscan.Detect(string(encoded)); sensitive {
		return true
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return true
	}
	var inspect func(any) bool
	inspect = func(current any) bool {
		switch typed := current.(type) {
		case string:
			_, sensitive := secretscan.Detect(typed)
			return sensitive
		case []any:
			for _, item := range typed {
				if inspect(item) {
					return true
				}
			}
		case map[string]any:
			for key, item := range typed {
				if inspect(key) || inspect(item) {
					return true
				}
			}
		}
		return false
	}
	return inspect(value)
}

func synthesisResponseContainsCredential(response []byte) bool {
	if _, sensitive := secretscan.Detect(string(response)); sensitive {
		return true
	}
	for _, object := range jsonObjectCandidates(response, 2) {
		if synthesisJSONContainsCredential(object) {
			return true
		}
	}
	return false
}

func cloneDiagnostics(values []Diagnostic) []Diagnostic {
	if values == nil {
		return nil
	}
	result := make([]Diagnostic, len(values))
	for index, value := range values {
		result[index] = value
		if value.Member != nil {
			member := *value.Member
			result[index].Member = &member
		}
	}
	return result
}

func diagnosticsEqual(left, right []Diagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !reflect.DeepEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}
