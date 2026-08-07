package themestudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Contract versions and prompt versions of the two semantic theme stages.
// They are producer-owned constants, never flags and never tuning knobs.
const (
	// Decision 235 (v11): the Study rebase (final Architecture context,
	// populated span questions) changes the Scout/Adjudication request
	// bytes and prompts — request 1→2, result 2→3, prompts v2→v3.
	ScoutRequestVersion        = 2
	ScoutResultVersion         = 3
	AdjudicationRequestVersion = 2
	AdjudicationResultVersion  = 3
	// Decision 235 (v11): rebased Architecture context + non-empty span
	// questions — prompt contract v3.
	ScoutPromptVersion        = "theme-scout-prompt-v3"
	AdjudicationPromptVersion = "theme-adjudication-prompt-v3"
	// Decision 233 (Archive 9): the reduced portfolio gains alternate
	// co-projection and the concentration diagnostic (StudyThemesVersion 2).
	// Decision 235 (v11): theme equivalence accounting lands with the
	// final-reducer changes (StudyThemesVersion 3).
	StudyThemesVersion = 3
	// Decision 232 (Archive 9): prompt contract v2 — target-cardinality
	// wording, duplicate normalization, backend-owned anchor role,
	// observation only for direct/supporting, unreviewed anchors.
	// (Prompt constants advanced to v3 above; this comment records the v2
	// rationale that still applies.)
	ScoutCacheContract           = "theme-scout-accepted-v1"
	AdjudicationCacheContract    = "theme-adjudication-accepted-v1"
	ScoutStage                   = "theme_scout"
	AdjudicationStage            = "theme_adjudication"
	MaxScoutRequestArtifactBytes = 384 << 10
	MaxScoutResultArtifactBytes  = 256 << 10
	MaxScoutStatusArtifactBytes  = 64 << 10
	MaxExpansionArtifactBytes    = 384 << 10
	MaxAdjRequestArtifactBytes   = 1 << 20
	MaxAdjResultArtifactBytes    = 384 << 10
	MaxAdjStatusArtifactBytes    = 64 << 10
	MaxStudyThemesArtifactBytes  = 256 << 10
)

// ScoutRequest is the compiled, bounded Theme Scout request (contract C). It
// is the exact artifact payload: the model-visible wire plus the backend-owned
// identity that binds it. Source bytes appear only inside seed pack objects and
// are provider evidence, never card content.
type ScoutRequest struct {
	Version       int            `json:"version"`
	PromptVersion string         `json:"prompt_version"`
	Language      Language       `json:"language"`
	Vocabulary    Vocabulary     `json:"vocabulary"`
	SeedPacks     SeedPackResult `json:"seed_packs"`
	// WireSHA256 hashes the exact model-visible wire JSON below.
	WireSHA256 string `json:"wire_sha256"`
	// WireJSON is the exact bounded model-visible request bundle.
	WireJSON string `json:"wire_json"`
	// CatalogSHA256 binds the complete canonical candidate set (eligible file
	// paths + seed anchor identities) so a different substrate misses.
	CatalogSHA256 string `json:"catalog_sha256"`
}

// ScoutResult is the validated Theme Scout result (contract C accepted
// output). It persists the accepted candidates plus item-local rejection
// diagnostics; zero accepted candidates is a semantic failure and never
// produces this record.
type ScoutResult struct {
	Version       int              `json:"version"`
	State         string           `json:"state"` // accepted | accepted_partial
	PromptVersion string           `json:"prompt_version"`
	Language      Language         `json:"language"`
	CatalogSHA256 string           `json:"catalog_sha256"`
	WireSHA256    string           `json:"wire_sha256"`
	Candidates    []ScoutCandidate `json:"candidates"`
	Status        ScoutStatus      `json:"status"`
}

// ScoutStatusRecord is the persisted Theme Scout status artifact. It is a
// thin wrapper so the status file remains bounded and self-describing.
// UnavailableCode and FailureCode are closed backend codes written by the run
// wiring layer (offline / insufficient_catalog / provider failure); they never
// carry provider prose.
type ScoutStatusRecord struct {
	Version         int         `json:"version"`
	State           string      `json:"state"`
	PromptVersion   string      `json:"prompt_version"`
	Language        Language    `json:"language"`
	CatalogSHA256   string      `json:"catalog_sha256"`
	UnavailableCode string      `json:"unavailable_code,omitempty"`
	FailureCode     string      `json:"failure_code,omitempty"`
	Status          ScoutStatus `json:"status"`
}

// AdjudicationRequest is the compiled, bounded Source Review / Theme
// Adjudication request (contract E). The model reviews each accepted Scout
// candidate against the locally expanded f* sources and the exact anchor
// identities; it may narrow, reorder, or reject. Source bytes appear only in
// the expansion and are provider evidence.
type AdjudicationRequest struct {
	Version       int                   `json:"version"`
	PromptVersion string                `json:"prompt_version"`
	Language      Language              `json:"language"`
	Candidates    []ScoutCandidate      `json:"candidates"` // accepted Scout candidates (t* catalog)
	Expansion     SourceExpansion       `json:"expansion"`  // contract D, provider-free, persisted
	Anchors       map[string]AnchorInfo `json:"anchors"`    // a* ref -> exact backend-owned identity
	WireSHA256    string                `json:"wire_sha256"`
	WireJSON      string                `json:"wire_json"`
	CatalogSHA256 string                `json:"catalog_sha256"`
}

// AdjudicationResult is the validated Theme Adjudication result (contract E
// accepted output). Zero accepted themes is a semantic failure and never
// produces this record.
type AdjudicationResult struct {
	Version       int                `json:"version"`
	State         string             `json:"state"` // accepted | accepted_partial
	PromptVersion string             `json:"prompt_version"`
	Language      Language           `json:"language"`
	CatalogSHA256 string             `json:"catalog_sha256"`
	WireSHA256    string             `json:"wire_sha256"`
	Themes        []AdjudicatedTheme `json:"themes"`
	Status        AdjudicationStatus `json:"status"`
}

// AdjudicationStatusRecord is the persisted Theme Adjudication status artifact.
// FailureCode is a closed backend code written by the run wiring layer; it
// never carries provider prose.
type AdjudicationStatusRecord struct {
	Version       int                `json:"version"`
	State         string             `json:"state"`
	PromptVersion string             `json:"prompt_version"`
	Language      Language           `json:"language"`
	CatalogSHA256 string             `json:"catalog_sha256"`
	FailureCode   string             `json:"failure_code,omitempty"`
	Status        AdjudicationStatus `json:"status"`
}

// StudyThemes is the reduced, published theme portfolio artifact
// (contract F). Cards carry editorial prose + exact readings + a badge and
// zero source bytes. The digest binds the complete artifact.
type StudyThemes struct {
	Version     string         `json:"version"`
	Revision    string         `json:"revision,omitempty"`
	ScoutSHA256 string         `json:"scout_catalog_sha256,omitempty"`
	AdjSHA256   string         `json:"adjudication_catalog_sha256,omitempty"`
	Cards       []ThemeCard    `json:"cards"`
	Omitted     int            `json:"omitted"`
	Partial     bool           `json:"partial"`
	Diagnostics map[string]int `json:"diagnostics,omitempty"`
}

// AnchorRefs returns the exact set of advertised a* seed refs.
func (request ScoutRequest) AnchorRefs() map[string]struct{} {
	refs := make(map[string]struct{}, len(request.SeedPacks.Packs))
	for _, pack := range request.SeedPacks.Packs {
		refs[pack.Seed.Ref] = struct{}{}
	}
	return refs
}

// FileRefs returns the exact set of advertised f* file refs.
func (request ScoutRequest) FileRefs() map[string]struct{} {
	refs := make(map[string]struct{}, len(request.Vocabulary.Files))
	for _, file := range request.Vocabulary.Files {
		refs[file.Ref] = struct{}{}
	}
	return refs
}

// digestJSON hashes the canonical JSON encoding of a value.
func digestJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return contentSHA256Bytes(encoded), nil
}

func contentSHA256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ScoutContext is the compact, bounded context block of the Scout wire: only
// local exact labels, never source bytes, raw trees, or canonical identities.
type ScoutContext struct {
	RepositoryName string              `json:"repository_name"`
	Architecture   ScoutArchContext    `json:"architecture,omitempty"`
	SpanQuestions  []ScoutSpanQuestion `json:"span_questions,omitempty"`
}

// ScoutArchContext is a compact labels-only projection of the accepted local
// Architecture Canvas (tolerant of failed/absent Architecture).
type ScoutArchContext struct {
	Title          string   `json:"title,omitempty"`
	Subtitle       string   `json:"subtitle,omitempty"`
	SubsystemNames []string `json:"subsystem_names,omitempty"`
	ComponentNames []string `json:"component_names,omitempty"`
}

// ScoutSpanQuestion is one backend-owned question label for a canonical route
// span. It carries no canonical IDs.
type ScoutSpanQuestion struct {
	Kind     string `json:"kind"` // focused | system_path
	Question string `json:"question"`
}

// wireScout is the exact model-visible request bundle (contract C). It is a
// projection of the local ScoutRequest: canonical identities (CanonicalSpanID
// bindings) never reach the provider, so the wire carries its own
// wire-safe copies of the vocabulary and the seed packs.
type wireScout struct {
	Context    ScoutContext       `json:"context"`
	Vocabulary wireVocabulary     `json:"vocabulary"`
	SeedPacks  wireSeedPackResult `json:"seed_packs"`
}

// wireVocabulary is the names-only f* vocabulary projection. It never carries
// canonical identities.
type wireVocabulary struct {
	Version         string     `json:"version"`
	Advertised      int        `json:"advertised"`
	CandidateSHA256 string     `json:"candidate_sha256"`
	Files           []FileRef  `json:"files,omitempty"`
	Omissions       []Omission `json:"omissions,omitempty"`
}

// wireSeedPackResult is the wire-safe projection of the seed packs: every
// field the provider needs, with canonical span bindings removed.
type wireSeedPackResult struct {
	Packs      []wireSeedPack `json:"packs"`
	TotalBytes int            `json:"total_bytes"`
	Omitted    int            `json:"omitted"`
}

type wireSeedPack struct {
	Seed        wireSeedSpec   `json:"seed"`
	Objects     []SourceObject `json:"objects"`
	TotalBytes  int            `json:"total_bytes"`
	Limitations string         `json:"limitations,omitempty"`
}

// wireSeedSpec is SeedSpec minus the internal CanonicalSpanID binding.
type wireSeedSpec struct {
	Ref        string `json:"ref"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Symbol     string `json:"symbol"`
	Provenance string `json:"provenance"`
	Kind       string `json:"kind"`
	Role       Role   `json:"role"`
}

// wireScoutFrom projects the local compile inputs into the provider-visible
// bundle: the f* vocabulary and the a* seed packs keep every field the
// provider needs, while the internal CanonicalSpanID bindings (canonical Atlas
// identities) are stripped and never reach the wire.
func wireScoutFrom(vocabulary Vocabulary, packs SeedPackResult, context ScoutContext) wireScout {
	wire := wireScout{
		Context: context,
		Vocabulary: wireVocabulary{
			Version: vocabulary.Version, Advertised: vocabulary.Advertised,
			CandidateSHA256: vocabulary.CandidateSHA256,
			Files:           vocabulary.Files, Omissions: vocabulary.Omissions,
		},
		SeedPacks: wireSeedPackResult{
			Packs:      make([]wireSeedPack, 0, len(packs.Packs)),
			TotalBytes: packs.TotalBytes,
		},
	}
	for _, pack := range packs.Packs {
		wire.SeedPacks.Packs = append(wire.SeedPacks.Packs, wireSeedPack{
			Seed: wireSeedSpec{
				Ref: pack.Seed.Ref, Path: pack.Seed.Path, Line: pack.Seed.Line,
				Symbol: pack.Seed.Symbol, Provenance: pack.Seed.Provenance,
				Kind: pack.Seed.Kind, Role: pack.Seed.Role,
			},
			Objects:     pack.Objects,
			TotalBytes:  pack.TotalBytes,
			Limitations: pack.Limitations,
		})
	}
	return wire
}

// CompileScout compiles the bounded Theme Scout request (contract C). The
// vocabulary and seed packs are already exact and bounded; this step derives
// the wire, its digest, and the catalog digest that binds the request. It
// performs no I/O and never calls a provider.
func CompileScout(
	language Language,
	vocabulary Vocabulary,
	packs SeedPackResult,
	context ScoutContext,
	revision string,
) (ScoutRequest, error) {
	if !language.Valid() {
		return ScoutRequest{}, fmt.Errorf("theme scout: unsupported language %q", language)
	}
	if vocabulary.Version == "" || vocabulary.CandidateSHA256 == "" {
		return ScoutRequest{}, fmt.Errorf("theme scout: vocabulary is incomplete")
	}
	if len(vocabulary.Files) == 0 {
		return ScoutRequest{}, fmt.Errorf("theme scout: vocabulary is empty")
	}
	if len(packs.Packs) == 0 {
		return ScoutRequest{}, fmt.Errorf("theme scout: seed packs are empty")
	}
	wire := wireScoutFrom(vocabulary, packs, context)
	wireJSON, err := json.Marshal(wire)
	if err != nil {
		return ScoutRequest{}, fmt.Errorf("theme scout: encode wire: %w", err)
	}
	if len(wireJSON) > MaxScoutRequestArtifactBytes {
		return ScoutRequest{}, fmt.Errorf("theme scout: wire exceeds %d bytes", MaxScoutRequestArtifactBytes)
	}
	catalogDigest, err := scoutCatalogDigest(vocabulary, packs)
	if err != nil {
		return ScoutRequest{}, err
	}
	return ScoutRequest{
		Version:       ScoutRequestVersion,
		PromptVersion: ScoutPromptVersion,
		Language:      language,
		Vocabulary:    vocabulary,
		SeedPacks:     packs,
		WireSHA256:    contentSHA256Bytes(wireJSON),
		WireJSON:      string(wireJSON),
		CatalogSHA256: catalogDigest,
	}, nil
}

// scoutCatalogDigest binds the complete canonical candidate set of the Scout
// layer: eligible file paths (complete candidate set, not only advertised) and
// every seed anchor identity. A different substrate therefore misses.
func scoutCatalogDigest(vocabulary Vocabulary, packs SeedPackResult) (string, error) {
	canonical := struct {
		Files []string `json:"files"`
		Seeds []string `json:"seeds"`
	}{
		Files: make([]string, 0, len(vocabulary.Files)),
	}
	for _, file := range vocabulary.Files {
		canonical.Files = append(canonical.Files, file.Path)
	}
	for _, pack := range packs.Packs {
		seed := pack.Seed
		canonical.Seeds = append(canonical.Seeds, fmt.Sprintf(
			"%s|%s|%d|%s|%s", seed.Ref, seed.Path, seed.Line, seed.Symbol, seed.Kind,
		))
	}
	sort.Strings(canonical.Files)
	sort.Strings(canonical.Seeds)
	digest, err := digestJSON(canonical)
	if err != nil {
		return "", fmt.Errorf("theme scout: catalog digest: %w", err)
	}
	return digest, nil
}

// wireAdjudication is the exact model-visible Source Review request bundle
// (contract E). It is per-candidate partitioned and shard-ready by
// construction: each candidate section carries only its own anchors and the
// expanded sources are shared once.
type wireAdjudication struct {
	Language   Language              `json:"language"`
	Candidates []wireAdjCandidate    `json:"candidates"`
	Expansion  SourceExpansion       `json:"expansion"`
	Anchors    map[string]wireAnchor `json:"anchors"`
}

type wireAdjCandidate struct {
	Ref               string    `json:"ref"` // t* ref
	Title             string    `json:"title"`
	Question          string    `json:"question"`
	ThemeKind         ThemeKind `json:"theme_kind"`
	AnchorRefs        []string  `json:"anchor_refs"`
	ExpansionFileRefs []string  `json:"expansion_file_refs,omitempty"`
	WhyItMatters      string    `json:"why_it_matters"`
	ExpectedLearning  string    `json:"expected_learning"`
}

type wireAnchor struct {
	Symbol string `json:"symbol"`
}

// CompileAdjudication compiles the bounded Source Review / Theme Adjudication
// request (contract E) from the Scout-accepted candidates, the locally
// expanded sources, and the exact anchor identities. It performs no I/O and
// never calls a provider.
func CompileAdjudication(
	language Language,
	candidates []ScoutCandidate,
	expansion SourceExpansion,
	anchors map[string]AnchorInfo,
) (AdjudicationRequest, error) {
	if !language.Valid() {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: unsupported language %q", language)
	}
	if len(candidates) == 0 {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: no candidates")
	}
	if expansion.Version == "" || expansion.CandidateSHA256 == "" {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: expansion is incomplete")
	}
	if len(anchors) == 0 {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: anchor identities are missing")
	}
	wire := wireAdjudication{
		Language: language, Expansion: expansion,
		Anchors: make(map[string]wireAnchor, len(anchors)),
	}
	for ref, info := range anchors {
		wire.Anchors[ref] = wireAnchor{Symbol: info.Symbol}
	}
	for _, candidate := range candidates {
		wire.Candidates = append(wire.Candidates, wireAdjCandidate{
			Ref: candidate.Ref, Title: candidate.Title, Question: candidate.Question,
			ThemeKind: candidate.ThemeKind, AnchorRefs: append([]string(nil), candidate.AnchorRefs...),
			ExpansionFileRefs: append([]string(nil), candidate.ExpansionFileRefs...),
			WhyItMatters:      candidate.WhyItMatters, ExpectedLearning: candidate.ExpectedLearning,
		})
	}
	wireJSON, err := json.Marshal(wire)
	if err != nil {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: encode wire: %w", err)
	}
	if len(wireJSON) > MaxAdjRequestArtifactBytes {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: wire exceeds %d bytes", MaxAdjRequestArtifactBytes)
	}
	catalogDigest, err := adjudicationCatalogDigest(candidates, expansion, anchors)
	if err != nil {
		return AdjudicationRequest{}, err
	}
	return AdjudicationRequest{
		Version:       AdjudicationRequestVersion,
		PromptVersion: AdjudicationPromptVersion,
		Language:      language,
		Candidates:    append([]ScoutCandidate(nil), candidates...),
		Expansion:     expansion,
		Anchors:       cloneAnchorMap(anchors),
		WireSHA256:    contentSHA256Bytes(wireJSON),
		WireJSON:      string(wireJSON),
		CatalogSHA256: catalogDigest,
	}, nil
}

func adjudicationCatalogDigest(
	candidates []ScoutCandidate,
	expansion SourceExpansion,
	anchors map[string]AnchorInfo,
) (string, error) {
	canonical := struct {
		Candidates []string          `json:"candidates"`
		Files      []string          `json:"files"`
		Anchors    map[string]string `json:"anchors"`
	}{Anchors: make(map[string]string, len(anchors))}
	for _, candidate := range candidates {
		refs := append([]string(nil), candidate.AnchorRefs...)
		sort.Strings(refs)
		canonical.Candidates = append(canonical.Candidates, strings.Join(refs, ","))
	}
	for _, file := range expansion.Files {
		canonical.Files = append(canonical.Files, file.Path)
	}
	for ref, info := range anchors {
		canonical.Anchors[ref] = fmt.Sprintf("%s|%d|%s", info.Path, info.Line, info.Symbol)
	}
	sort.Strings(canonical.Candidates)
	sort.Strings(canonical.Files)
	digest, err := digestJSON(canonical)
	if err != nil {
		return "", fmt.Errorf("theme adjudication: catalog digest: %w", err)
	}
	return digest, nil
}

func cloneAnchorMap(source map[string]AnchorInfo) map[string]AnchorInfo {
	if source == nil {
		return nil
	}
	out := make(map[string]AnchorInfo, len(source))
	for ref, info := range source {
		out[ref] = info
	}
	return out
}

// AssignCandidateRefs assigns the request-local t* catalog refs of the
// accepted Scout candidates in canonical order. Refs are stable across locales
// and derived from the candidate position, never from model prose.
func AssignCandidateRefs(candidates []ScoutCandidate) {
	for index := range candidates {
		candidates[index].Ref = refName("t", index+1)
	}
}

// candidateRefs returns the t* refs of the candidates in canonical order.
func candidateRefs(candidates []ScoutCandidate) []string {
	refs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		refs = append(refs, candidate.Ref)
	}
	return refs
}

// candidateByRef keys the accepted candidates by their t* refs.
func candidateByRef(candidates []ScoutCandidate) map[string]*ScoutCandidate {
	byRef := make(map[string]*ScoutCandidate, len(candidates))
	for index := range candidates {
		byRef[candidates[index].Ref] = &candidates[index]
	}
	return byRef
}

// ScoutPrompt is the bounded Theme Scout prompt (contract C). It follows
// docs/DEEPSEEK_API_NOTES.md: JSON mode, the word "json" plus an example shape
// in the prompt, explicit disabled thinking on the official DeepSeek endpoint.
type ScoutPrompt struct {
	Version  string   `json:"version"`
	Language Language `json:"language"`
	System   string   `json:"system"`
	User     string   `json:"user"`
}

// AdjudicationPrompt is the bounded Source Review / Theme Adjudication prompt
// (contract E).
type AdjudicationPrompt struct {
	Version  string   `json:"version"`
	Language Language `json:"language"`
	System   string   `json:"system"`
	User     string   `json:"user"`
}

const scoutPromptSystem = `You are proposing useful Study themes for a developer inside a large unfamiliar repository.
A Study theme is a question that several exact source anchors can help answer together. It is not required to be a proven runtime path.
You may group anchors because they participate in one user journey, cross-cutting policy, sibling implementation family, integration family, lifecycle concern, or shared domain responsibility.
Use a* source refs as current support. Use f* names-only refs only to request local source expansion. A names-only file is never evidence.
Do not claim execution order, ownership, reachability or data flow unless an exact supplied relation proves it; relation_claim must always be "editorial_only".
Propose meaningfully distinct themes. Do not restate individual direct calls and do not pad.
Return exactly one JSON object and no markdown. Keep all enum values and refs unchanged. Write model-authored prose in the requested language.`

const scoutPromptUserShape = `Requested prose language: %s.
Aim for %d-%d themes when distinct evidence supports them. Return %d-%d; fewer is better than overlap or filler. Use %d-%d anchor_refs per theme; a one-anchor focused theme is permitted only when marked "focused":true and must not dominate. Exact duplicate anchor or file refs are normalized and counted by the backend; do not repeat them.
theme_kind is one of: user_journey, cross_cutting_policy, sibling_implementation_family, integration_family, lifecycle_concern, shared_domain_responsibility.
Response schema: {"themes":[{"title":"...","question":"...","theme_kind":"...","anchor_refs":["a1"],"expansion_file_refs":["f1"],"why_it_matters":"...","expected_learning":"...","relation_claim":"editorial_only","focused":false}]}
Request bundle JSON:
%s`

// BuildScoutPrompt builds the exact bounded Theme Scout prompt.
func BuildScoutPrompt(request ScoutRequest) ScoutPrompt {
	return ScoutPrompt{
		Version:  ScoutPromptVersion,
		Language: request.Language,
		System:   scoutPromptSystem,
		User: fmt.Sprintf(
			scoutPromptUserShape,
			request.Language, DesiredScoutMin, DesiredScoutMax,
			MinScoutCandidates, MaxScoutCandidates, MinThemeAnchors, MaxThemeAnchors,
			request.WireJSON,
		),
	}
}

const adjudicationPromptSystem = `Review each proposed Study theme against its exact source packs.
For the anchors you assess, classify: direct, supporting, weak, or irrelevant. Anchors you do not assess are treated by the backend as unreviewed — counted, not published, never fatal.
Write one bounded supported observation from supplied source for direct and supporting anchors. The theme may remain editorial, but its question must be answerable by the accepted anchors together.
Do not infer execution order without an exact relation. Do not retain an anchor merely because its filename sounds relevant.
You may narrow or rewrite the title/question, remove weak anchors, reorder the reading path, or reject the complete theme. Do not pad.
Return exactly one JSON object and no markdown. Keep all enum values and refs unchanged. Write model-authored prose in the requested language.`

const adjudicationPromptUserShape = `Requested prose language: %s.
Review %d-%d candidate themes (valid %d-%d, no padding). For each candidate return exactly one section.
Assess the anchors that matter; fit is one of: direct, supporting, weak, irrelevant. Only direct and supporting anchors publish as readings; keep at least one direct anchor. Anchor role is backend-owned: do not return a role field. Supported observations are required only for direct and supporting anchors; weak and irrelevant anchors may carry an optional short rejection reason. Exact duplicate assessments are normalized and counted by the backend.
reading_order is an ordered subset of the candidate's own anchor_refs. unknowns are bounded and optional.
Response schema: {"themes":[{"candidate_ref":"t1","final_title":"...","final_question":"...","anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"..."}],"reading_order":["a1"],"unknowns":["..."]}]}
Request bundle JSON:
%s`

// BuildAdjudicationPrompt builds the exact bounded Theme Adjudication prompt.
func BuildAdjudicationPrompt(request AdjudicationRequest) AdjudicationPrompt {
	return AdjudicationPrompt{
		Version:  AdjudicationPromptVersion,
		Language: request.Language,
		System:   adjudicationPromptSystem,
		User: fmt.Sprintf(
			adjudicationPromptUserShape,
			request.Language, DesiredFinalMin, DesiredFinalMax,
			MinFinalThemes, MaxFinalThemes, request.WireJSON,
		),
	}
}

// RefsForExpansion returns the deduplicated f* refs the Scout candidates
// requested for local expansion, in canonical order.
func RefsForExpansion(candidates []ScoutCandidate) []string {
	seen := make(map[string]struct{})
	var refs []string
	for _, candidate := range candidates {
		for _, ref := range candidate.ExpansionFileRefs {
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

// ExpansionFilesForRefs resolves the requested f* refs to their exact file
// entries from the vocabulary, in the request's canonical order.
func ExpansionFilesForRefs(vocabulary Vocabulary, refs []string) []FileRef {
	byRef := make(map[string]FileRef, len(vocabulary.Files))
	for _, file := range vocabulary.Files {
		byRef[file.Ref] = file
	}
	out := make([]FileRef, 0, len(refs))
	for _, ref := range refs {
		if file, ok := byRef[ref]; ok {
			out = append(out, file)
		}
	}
	return out
}
