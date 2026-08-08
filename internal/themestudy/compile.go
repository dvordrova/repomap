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
	// Decision 239: an otherwise valid Scout theme with more than five exact
	// anchors is normalized item-locally instead of being discarded whole;
	// the typed result/status contract advances 3→4. D239 also removes the
	// duplicated Vocabulary/SeedPack source projection from the persisted
	// Scout request. The current request artifact stores the exact wire once
	// plus private metadata and restores the same in-memory request losslessly.
	// Older request versions fail closed; git retains their implementation.
	// Theme-input experiment T1 advances request 3→4 and cache v1→v2 because
	// the wire now exposes generic artifact roles and production-aware ordering.
	ScoutRequestVersion        = 4
	ScoutResultVersion         = 4
	AdjudicationRequestVersion = 2
	AdjudicationResultVersion  = 3
)

// Theme prompt contract identities — the short SHA-256 of the exact
// language-independent prompt template text (owner directive 2026-08-07:
// short prompt SHA instead of a hand-bumped version). Any edit to the
// system text or the user shape automatically changes the identity, so
// cache keys and saved-record replays fail closed on their own. The request
// bundle JSON is NOT part of the identity — it is already bound by the
// exact request digest.
var (
	ScoutPromptVersion        = "theme-scout-prompt-" + shortSHA256(scoutPromptSystem+scoutPromptUserShape)
	AdjudicationPromptVersion = "theme-adjudication-prompt-" + shortSHA256(adjudicationPromptSystem+adjudicationPromptUserShape)
)

// Decision 233 (Archive 9): the reduced portfolio gains alternate
// co-projection and the concentration diagnostic (StudyThemesVersion 2).
// Decision 235 (v11): theme equivalence accounting lands with the
// final-reducer changes (StudyThemesVersion 3).
const StudyThemesVersion = 3

// Decision 232 (Archive 9): prompt contract v2 — target-cardinality
// wording, duplicate normalization, backend-owned anchor role,
// observation only for direct/supporting, unreviewed anchors.
// (Prompt constants advanced to SHA identity above; this comment records the
// v2 rationale that still applies.)
const (
	ScoutCacheContract           = "theme-scout-accepted-v2"
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

// ScoutRequest is the compiled, bounded in-memory Theme Scout request
// (contract C): the model-visible wire plus the backend-owned identity that
// binds it. The current artifact stores the exact wire once and only the private
// metadata needed to losslessly restore this value; source bytes appear only
// once on disk and remain provider evidence, never card content.
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
	// WireBreakdown is the exact byte breakdown of the model-visible wire
	// sections (Archive 12 P0): candidates / anchor_evidence / sources /
	// envelope. Backend-owned diagnostics for incident investigation,
	// never model-visible.
	WireBreakdown map[string]int `json:"wire_breakdown,omitempty"`
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
// span. AnchorRef (a*) is the exact seed the span compiled to, when the span
// owns a single reading target — the model never has to guess which anchor a
// question belongs to (Phase 3 validation audit). It carries no canonical IDs.
type ScoutSpanQuestion struct {
	Kind      string `json:"kind"` // focused | system_path
	Question  string `json:"question"`
	AnchorRef string `json:"anchor_ref,omitempty"`
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
// canonical identities or bookkeeping (advertised counts, candidate SHA,
// omission accounting stay local — Phase 2 prompt cleanup: only fields the
// model can reason over reach the wire).
type wireVocabulary struct {
	Files []FileRef `json:"files,omitempty"`
}

// wireSeedPackResult is the wire-safe projection of the seed packs: every
// field the provider needs, with canonical span bindings and byte/omission
// bookkeeping removed (Phase 2 prompt cleanup).
type wireSeedPackResult struct {
	Packs []wireSeedPack `json:"packs"`
}

type wireSeedPack struct {
	Seed        wireSeedSpec   `json:"seed"`
	Objects     []SourceObject `json:"objects"`
	Limitations string         `json:"limitations,omitempty"`
}

// wireSeedSpec is SeedSpec minus the internal CanonicalSpanID binding and
// the backend-owned provenance tag (Phase 2: provenance is bookkeeping the
// model cannot act on).
type wireSeedSpec struct {
	Ref    string `json:"ref"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
	Kind   string `json:"kind"`
	Role   Role   `json:"role"`
}

// wireScoutFrom projects the local compile inputs into the provider-visible
// bundle: the f* vocabulary and the a* seed packs keep every field the
// provider needs, while internal CanonicalSpanID bindings, provenance tags
// and byte/omission bookkeeping stay local and never reach the wire.
func wireScoutFrom(vocabulary Vocabulary, packs SeedPackResult, context ScoutContext) wireScout {
	wire := wireScout{
		Context: context,
		Vocabulary: wireVocabulary{
			Files: vocabulary.Files,
		},
		SeedPacks: wireSeedPackResult{
			Packs: make([]wireSeedPack, 0, len(packs.Packs)),
		},
	}
	for _, pack := range packs.Packs {
		wire.SeedPacks.Packs = append(wire.SeedPacks.Packs, wireSeedPack{
			Seed: wireSeedSpec{
				Ref: pack.Seed.Ref, Path: pack.Seed.Path, Line: pack.Seed.Line,
				Symbol: pack.Seed.Symbol, Kind: pack.Seed.Kind, Role: pack.Seed.Role,
			},
			Objects:     pack.Objects,
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
	Language   Language           `json:"language"`
	Candidates []wireAdjCandidate `json:"candidates"`
	// AnchorEvidence carries the exact bounded source object for every a*
	// anchor the candidates actually reference (union of candidate
	// anchor_refs) — the same seed evidence the Scout used, never a bare
	// symbol. f* expanded sources below are additional context, not a
	// replacement for anchor evidence (Archive 12 P0, owner directive).
	AnchorEvidence map[string]wireAnchorEvidence `json:"anchor_evidence"`
	// Sources is the COMPACT provider wire for the locally expanded f*
	// files: only path/partial/lines/omitted ranges per object. Backend
	// bookkeeping (hashes, byte totals, artifact version, revision,
	// provenance) stays in the persisted SourceExpansion artifact and is
	// never model-visible (Archive 12 P0, owner directive).
	Sources map[string]wireExpandedFile `json:"sources"`
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

type wireAnchorEvidence struct {
	Symbol string           `json:"symbol"`
	Source wireSourceObject `json:"source"`
}

// wireSourceObject is the compact model-visible projection of one exact
// bounded source object: location + lines + explicit omitted ranges only.
type wireSourceObject struct {
	Path    string      `json:"path"`
	Line    int         `json:"line"`
	Symbol  string      `json:"symbol,omitempty"`
	Partial bool        `json:"partial,omitempty"`
	Lines   []string    `json:"lines"`
	Omitted []LineRange `json:"omitted_ranges,omitempty"`
}

// wireExpandedFile is the compact model-visible projection of one expanded
// f* file: path + object windows, no bookkeeping.
type wireExpandedFile struct {
	Path    string             `json:"path"`
	Partial bool               `json:"partial,omitempty"`
	Objects []wireSourceObject `json:"objects,omitempty"`
}

// CompileAdjudication compiles the bounded Source Review / Theme Adjudication
// request (contract E) from the Scout-accepted candidates, the locally
// expanded sources, the exact anchor identities, and the exact a* seed
// evidence packs the Scout already received. It performs no I/O and never
// calls a provider. Only anchors referenced by the candidates enter the wire
// (Archive 12 P0: no whole-catalog anchor dump).
func CompileAdjudication(
	language Language,
	candidates []ScoutCandidate,
	expansion SourceExpansion,
	anchors map[string]AnchorInfo,
	seedPacks []SeedPack,
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
	usedAnchors := make(map[string]struct{})
	for _, candidate := range candidates {
		for _, ref := range candidate.AnchorRefs {
			usedAnchors[ref] = struct{}{}
		}
	}
	seedEvidenceByRef := make(map[string]SeedPack, len(seedPacks))
	for _, pack := range seedPacks {
		seedEvidenceByRef[pack.Seed.Ref] = pack
	}

	wire := wireAdjudication{
		Language:       language,
		AnchorEvidence: make(map[string]wireAnchorEvidence, len(usedAnchors)),
		Sources:        make(map[string]wireExpandedFile, len(expansion.Files)),
	}
	for _, candidate := range candidates {
		wire.Candidates = append(wire.Candidates, wireAdjCandidate{
			Ref: candidate.Ref, Title: candidate.Title, Question: candidate.Question,
			ThemeKind: candidate.ThemeKind, AnchorRefs: append([]string(nil), candidate.AnchorRefs...),
			ExpansionFileRefs: append([]string(nil), candidate.ExpansionFileRefs...),
			WhyItMatters:      candidate.WhyItMatters, ExpectedLearning: candidate.ExpectedLearning,
		})
	}
	// Anchor evidence: the exact bounded source object from the Scout seed
	// pack for every referenced anchor. A referenced anchor without a seed
	// pack still carries its backend-owned identity (symbol), never a bare
	// ref with no location.
	for ref := range usedAnchors {
		info, ok := anchors[ref]
		if !ok {
			continue
		}
		evidence := wireAnchorEvidence{Symbol: info.Symbol}
		if pack, ok := seedEvidenceByRef[ref]; ok {
			for _, object := range pack.Objects {
				evidence.Source = wireSourceObject{
					Path: object.Path, Line: object.Line, Symbol: object.Symbol,
					Partial: object.Partial, Lines: object.Lines, Omitted: object.Omitted,
				}
				break
			}
		}
		wire.AnchorEvidence[ref] = evidence
	}
	// Compact f* source projection: model-visible windows only.
	for _, file := range expansion.Files {
		if file.Closed {
			continue
		}
		entry := wireExpandedFile{Path: file.Path, Partial: !file.Small}
		for _, object := range file.Objects {
			entry.Objects = append(entry.Objects, wireSourceObject{
				Path: object.Path, Line: object.Line, Symbol: object.Symbol,
				Partial: object.Partial, Lines: object.Lines, Omitted: object.Omitted,
			})
		}
		wire.Sources[file.Ref] = entry
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
	// Archive 12 P0 (owner directive): exact request-byte breakdown by wire
	// section — incident investigation without re-parsing the wire.
	breakdown, breakdownErr := wireBreakdownBytes(wire)
	if breakdownErr != nil {
		return AdjudicationRequest{}, fmt.Errorf("theme adjudication: wire breakdown: %w", breakdownErr)
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
		WireBreakdown: breakdown,
	}, nil
}

// wireBreakdownBytes reports the exact JSON byte size of each model-visible
// wire section plus the shared envelope.
func wireBreakdownBytes(wire wireAdjudication) (map[string]int, error) {
	breakdown := make(map[string]int, 4)
	section := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		breakdown[key] = len(encoded)
		return nil
	}
	if err := section("candidates", wire.Candidates); err != nil {
		return nil, err
	}
	if err := section("anchor_evidence", wire.AnchorEvidence); err != nil {
		return nil, err
	}
	if err := section("sources", wire.Sources); err != nil {
		return nil, err
	}
	breakdown["total"] = breakdown["candidates"] + breakdown["anchor_evidence"] + breakdown["sources"]
	return breakdown, nil
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
A Study theme is a question that one or more exact source anchors can help answer. It is not required to be a proven runtime path.
You may group anchors because they help explain one user journey, cross-cutting policy, sibling implementation family, integration family, lifecycle concern, or shared domain responsibility.
Use a* source refs as current support. Use f* names-only refs only to request local source expansion. A names-only file is never evidence.
The backend-owned role on every a* and f* item is a navigation priority, not runtime proof. Prefer primary_production_entry, production_core, effect_integration_boundary, and public_api when explaining the product. Use example, test, fixture, generated, playground_preview_evaluator, experimental, or documentation items only when they materially clarify the selected production theme.
Do not claim execution order, ownership, reachability, or data flow unless the supplied exact evidence establishes it.
Return themes in decreasing usefulness for a developer trying to understand this repository, with the most useful theme first.
Each additional theme must add a materially distinct learning outcome. Do not restate individual direct calls and do not pad.
Return exactly one JSON object and no markdown. Keep all enum values and refs unchanged. Write model-authored prose in the requested language.`

const scoutPromptUserShape = `Requested prose language: %s.
Most repositories need no more than about %d materially distinct, high-value themes. Use fewer when they cover the important learning outcomes; return more only when additional themes add substantial distinct understanding. Do not pad toward a target.
Prefer a small set of distinct anchor_refs that together support one coherent learning outcome; return 1 to %d per theme. Use a single anchor when it is sufficient on its own. Do not add anchors merely to reach a count, and do not repeat refs within a theme.
theme_kind is one of: user_journey, cross_cutting_policy, sibling_implementation_family, integration_family, lifecycle_concern, shared_domain_responsibility.
The backend retains at most 80 Unicode characters for title, 200 for question, and 240 each for why_it_matters and expected_learning. Stay below these existing limits. Use a compact title and short, complete sentences; do not pad prose toward a limit or leave a sentence unfinished.
Response schema: {"themes":[{"title":"...","question":"...","theme_kind":"...","anchor_refs":["a1"],"expansion_file_refs":["f1"],"why_it_matters":"...","expected_learning":"..."}]}
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
			request.Language, ScoutCardinalityPrior, MaxThemeAnchors,
			request.WireJSON,
		),
	}
}

const adjudicationPromptSystem = `Review proposed Study themes against their exact source evidence.
For each candidate, anchor_evidence contains exact bounded source for its anchors. sources contains additional context requested during local expansion; it supplements anchor evidence rather than replacing it.
Keep only anchors that materially help answer the candidate question. For every retained anchor, classify its support as direct or supporting and write one short observation bounded to the supplied source.
You may narrow or rewrite a title or question when the retained evidence supports a more precise learning outcome. Omit a candidate when the supplied evidence does not support a useful source-backed theme.
Do not infer execution order, causality, ownership, reachability, or data flow beyond what the supplied evidence establishes. Do not retain an anchor because its filename or symbol name merely sounds relevant.
Return exactly one JSON object and no markdown. Keep all refs unchanged. Write model-authored prose in the requested language.`

const adjudicationPromptUserShape = `Requested prose language: %s.
This request contains %d candidate themes. Review each candidate independently. Return every candidate that remains a useful source-backed theme after review; omit unsupported candidates. Do not create placeholders or pad the result.
Within each returned theme, order readings in the order you recommend a developer inspect them. Include at least one direct reading. Include only unknowns that materially qualify what the retained readings establish.
The backend retains at most 240 Unicode characters for each observation and 120 for each unknown, and accepts at most 4 unknowns per theme. Stay below these existing limits. Keep final_title and final_question concise and complete. Use short, complete sentences for observations and unknowns; do not pad prose toward a limit or leave a sentence unfinished.
Response schema: {"themes":[{"candidate_ref":"t1","final_title":"...","final_question":"...","readings":[{"anchor_ref":"a1","support":"direct","observation":"..."}],"unknowns":["..."]}]}
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
			request.Language, len(request.Candidates), request.WireJSON,
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
