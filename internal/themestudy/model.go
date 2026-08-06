// Package themestudy owns the deterministic, provider-free substrate of the
// Atlas-backed source-grounded Study theme layer (Decision 213). It produces a
// flat names-only f* file vocabulary, bounded exact a* seed-anchor source
// packs, executes local f* source expansion, validates the Theme Scout and
// Theme Adjudication semantic responses with item-local rejection, and reduces
// adjudicated candidates into clean canonical Study cards.
//
// This package never calls a model provider. Models only ever interpret the
// bounded request bundles this package compiles; everything here is local,
// exact, and backend-owned. Source bytes are provider evidence only and must
// never appear on a Study card.
package themestudy

// Language is the closed prose language of a theme-stage request. Prose is
// model-authored in this language; exact symbols, paths and refs never change.
type Language string

const (
	LanguageEnglish Language = "en"
	LanguageRussian Language = "ru"
)

func (language Language) Valid() bool {
	return language == LanguageEnglish || language == LanguageRussian
}

// Role is the closed role vocabulary of the f* file layer and of the a* seed
// anchors. These are internal producer roles and must never be rendered as
// user-facing UI labels.
type Role string

const (
	RoleProductionSource Role = "production_source"
	RoleTest             Role = "test"
	RoleDocumentation    Role = "documentation"
)

func (role Role) Valid() bool {
	switch role {
	case RoleProductionSource, RoleTest, RoleDocumentation:
		return true
	default:
		return false
	}
}

// ThemeKind is the closed enum of editorial grouping kinds a Theme Scout
// candidate may declare. It is editorial label only, never authority.
type ThemeKind string

const (
	KindUserJourney                 ThemeKind = "user_journey"
	KindCrossCuttingPolicy          ThemeKind = "cross_cutting_policy"
	KindSiblingImplementationFamily ThemeKind = "sibling_implementation_family"
	KindIntegrationFamily           ThemeKind = "integration_family"
	KindLifecycleConcern            ThemeKind = "lifecycle_concern"
	KindSharedDomainResponsibility  ThemeKind = "shared_domain_responsibility"
)

func (kind ThemeKind) Valid() bool {
	switch kind {
	case KindUserJourney, KindCrossCuttingPolicy, KindSiblingImplementationFamily,
		KindIntegrationFamily, KindLifecycleConcern, KindSharedDomainResponsibility:
		return true
	default:
		return false
	}
}

// FitClass is the closed internal per-anchor classification from Theme
// Adjudication. Only direct and supporting anchors publish as readings;
// weak and irrelevant never appear as readings and never reach the UI.
type FitClass string

const (
	FitDirect     FitClass = "direct"
	FitSupporting FitClass = "supporting"
	FitWeak       FitClass = "weak"
	FitIrrelevant FitClass = "irrelevant"
)

func (fit FitClass) Valid() bool {
	switch fit {
	case FitDirect, FitSupporting, FitWeak, FitIrrelevant:
		return true
	default:
		return false
	}
}

// RelationClaim is the only authority the Theme Scout stage may assert. It is
// exactly editorial_only; any other value rejects the candidate because a
// model may never create runtime facts, relations, or acceptance.
type RelationClaim string

const RelationClaimEditorialOnly RelationClaim = "editorial_only"

func (claim RelationClaim) Valid() bool {
	return claim == RelationClaimEditorialOnly
}

// Bounds are explicit producer-owned constants (safety bounds), never flags
// and never tuning knobs tuned to any one fixture.
const (
	MaxFileVocabularyBytes = 64 << 10
	MaxSeedAnchors         = 64
	MaxSeedSourceBytes     = 256 << 10
	MaxSourceObjectLines   = 200
	MaxSourceObjectBytes   = 32 << 10
	MaxExpansionFileLines  = 1200
	MaxExpansionFileBytes  = 128 << 10
	// MaxExpansionFiles is a generous per-run file ceiling; the honest bound
	// is MaxExpansionBytes. Requested files beyond the byte budget are
	// recorded under OmittedRefs, never dropped silently (D190/D195).
	MaxExpansionFiles = 128
	// MaxExpansionBytes bounds the source-evidence expansion the Scout's
	// requested files are allowed to occupy. It must fit the full requested
	// set on reference repositories (casdoor requests 50+ files once the
	// D214 resource-boundary seeds widen the catalog) while staying well
	// under the 1 MiB Adjudication wire bound that embeds it.
	MaxExpansionBytes          = 512 << 10
	MaxOmissionRepresentatives = 12

	// Scout portfolio bounds: desired 8-12 candidates, valid 1-12.
	MinScoutCandidates = 1
	MaxScoutCandidates = 12
	DesiredScoutMin    = 8
	DesiredScoutMax    = 12

	// Per-theme anchor bounds: desired 2-5, valid 1-5.
	MinThemeAnchors = 1
	MaxThemeAnchors = 5

	// Adjudication portfolio bounds: desired 4-8 themes, valid 1-8.
	MinFinalThemes  = 1
	MaxFinalThemes  = 8
	DesiredFinalMin = 4
	DesiredFinalMax = 8

	MaxQuestionRunes    = 200
	MaxTitleRunes       = 80
	MaxEditorialRunes   = 240
	MaxUnknownsPerTheme = 4
	MaxUnknownRunes     = 120
)

// FileRef is one item of the flat names-only f* file vocabulary. It is a lead
// only: a filename is never evidence and may only request local expansion.
type FileRef struct {
	Ref      string `json:"ref"`      // e.g. "f17"
	Path     string `json:"path"`     // exact repository-relative path
	Language string `json:"language"` // extension-based language id
	Role     Role   `json:"role"`     // production_source | test | documentation
}

// Omission is a closed, coverage-aware aggregate of files not advertised.
// It exactly partitions considered - advertised so coverage never silently
// becomes first-N.
type Omission struct {
	Reason          string   `json:"reason"` // eligible_not_advertisable | vocabulary_budget | seed_budget
	Count           int      `json:"count"`
	Representatives []string `json:"representative_refs,omitempty"`
}

// Vocabulary is the complete f* flat names-only layer (contract A).
type Vocabulary struct {
	Version         string     `json:"version"`
	Complete        bool       `json:"complete"`
	Considered      int        `json:"considered"`
	Advertised      int        `json:"advertised"`
	CandidateSHA256 string     `json:"candidate_sha256"`
	Files           []FileRef  `json:"files,omitempty"`
	Omissions       []Omission `json:"omissions,omitempty"`
}

// SeedSpec describes one a* seed requested from the compiled local substrate.
// Kind is "system_path" (span endpoints: caller + representative callsite +
// callee) or "focused" (single declaration anchored at Line/Symbol). For
// system_path seeds, CallerLine/CallLine/CalleeLine carry the exact positive
// lines of the three separated SourceObjects; focused seeds emit one
// declaration at Line.
type SeedSpec struct {
	Ref        string `json:"ref"`  // e.g. "a17"
	Path       string `json:"path"` // exact repository-relative path
	Line       int    `json:"line"` // positive line, > 0 (focused anchor or callsite for system_path)
	Symbol     string `json:"symbol"`
	Provenance string `json:"provenance"` // e.g. "d211_span_reading_target"
	Kind       string `json:"kind"`       // system_path | focused
	Role       Role   `json:"role"`
	// CanonicalSpanID is the canonical route-span identity this seed was
	// compiled from (when the seed derives from a span). It is an internal
	// binding used by the re-based four-stage browse derivation; it is never
	// model-visible and never serialized to a report.
	CanonicalSpanID string `json:"canonical_span_id,omitempty"`

	// system_path separation (exact positive lines, zero means "unused").
	CallerSymbol string `json:"caller_symbol,omitempty"`
	CallerLine   int    `json:"caller_line,omitempty"`
	CallLine     int    `json:"call_line,omitempty"`
	CalleeSymbol string `json:"callee_symbol,omitempty"`
	CalleeLine   int    `json:"callee_line,omitempty"`
}

// SourceRole marks one SourceObject's role within an a* seed source pack.
type SourceRole string

const (
	SourceRoleCaller      SourceRole = "caller"
	SourceRoleCallsite    SourceRole = "callsite"
	SourceRoleCallee      SourceRole = "callee"
	SourceRoleDeclaration SourceRole = "declaration"
	SourceRoleRelatedTest SourceRole = "related_test"
	SourceRoleRelatedDoc  SourceRole = "related_document"
)

// LineRange is an exact, explicit omitted range within an authorized file.
type LineRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// SourceObject is one bounded source object in a seed pack or expansion. Its
// lines are provider evidence only and must never be rendered on a Study card.
// ContentSHA256 hashes the visible Lines so nothing is silently altered.
type SourceObject struct {
	Role          SourceRole  `json:"role"`
	Path          string      `json:"path"`
	Line          int         `json:"line"`
	Symbol        string      `json:"symbol"`
	Provenance    string      `json:"provenance"`
	FullBody      bool        `json:"full_body"`
	Partial       bool        `json:"partial"`
	Lines         []string    `json:"lines"`
	Omitted       []LineRange `json:"omitted_ranges,omitempty"`
	ContentSHA256 string      `json:"content_sha256"`
}

// SeedPack is the bounded a* source pack for one seed (contract B). Source
// bytes are provider evidence only.
type SeedPack struct {
	Seed        SeedSpec       `json:"seed"`
	Objects     []SourceObject `json:"objects"`
	TotalBytes  int            `json:"total_bytes"`
	Limitations string         `json:"limitations,omitempty"`
}

// ExpansionFile is one locally expanded f* file (contract D). Relevance is
// never inferred from a filename alone; a requested file is expanded in
// full-or-indexed form, never filtered by name.
type ExpansionFile struct {
	Ref           string         `json:"ref"` // the requested f* ref
	Path          string         `json:"path"`
	Small         bool           `json:"small"`
	Objects       []SourceObject `json:"objects,omitempty"`
	Omitted       []LineRange    `json:"omitted_ranges,omitempty"`
	ExpandedLines int            `json:"expanded_lines"`
	TotalLines    int            `json:"total_lines"`
}

// SourceExpansion is the persisted provider-free expansion artifact
// (contract D). Persisting it makes the Adjudication request rebuildable and
// replayable provider-free.
type SourceExpansion struct {
	Version         string          `json:"version"`
	Requested       []string        `json:"requested_refs"`
	Files           []ExpansionFile `json:"files"`
	OmittedRefs     []string        `json:"omitted_refs,omitempty"`
	ExpandedLines   int             `json:"expanded_lines"`
	ExpandedBytes   int             `json:"expanded_bytes"`
	CandidateSHA256 string          `json:"candidate_sha256"`
	Revision        string          `json:"revision,omitempty"`
}

// ScoutCandidate is one Theme Scout proposal (contract C). Title, question and
// prose are editorial only. anchor_refs (a*) and expansion_file_refs (f*) carry
// request-local typed refs, never raw paths or canonical IDs. Ref is the
// request-local t* catalog ref assigned by the compile layer, never model prose.
type ScoutCandidate struct {
	Ref               string        `json:"ref,omitempty"`
	Title             string        `json:"title"`
	Question          string        `json:"question"`
	ThemeKind         ThemeKind     `json:"theme_kind"`
	AnchorRefs        []string      `json:"anchor_refs"`
	ExpansionFileRefs []string      `json:"expansion_file_refs,omitempty"`
	WhyItMatters      string        `json:"why_it_matters"`
	ExpectedLearning  string        `json:"expected_learning"`
	RelationClaim     RelationClaim `json:"relation_claim"`
	Focused           bool          `json:"focused,omitempty"`
}

// ScoutResponse is the decoded Theme Scout result for a request-local catalog
// (contract C). Only the Themes field is consumed; every other provider field
// is ignored and rejected as unrequested output.
type ScoutResponse struct {
	Themes []ScoutCandidate `json:"themes"`
}

// ScoutIssueCode is a closed item-local rejection reason for a candidate.
type ScoutIssueCode string

const (
	ScoutIssueUnrequestedOutput    ScoutIssueCode = "unrequested_output"
	ScoutIssueDecodeCandidate      ScoutIssueCode = "decode_candidate"
	ScoutIssueUnknownRef           ScoutIssueCode = "unknown_ref"
	ScoutIssueWrongKindRef         ScoutIssueCode = "wrong_kind_ref"
	ScoutIssueCrossRequestRef      ScoutIssueCode = "cross_request_ref"
	ScoutIssueDuplicateRef         ScoutIssueCode = "duplicate_ref"
	ScoutIssueDuplicateCandidate   ScoutIssueCode = "duplicate_candidate"
	ScoutIssueEmptyProse           ScoutIssueCode = "empty_prose"
	ScoutIssueInvalidThemeKind     ScoutIssueCode = "invalid_theme_kind"
	ScoutIssueInvalidRelationClaim ScoutIssueCode = "invalid_relation_claim"
	ScoutIssueProseTooLong         ScoutIssueCode = "prose_too_long"
	ScoutIssueInvalidAnchorCount   ScoutIssueCode = "invalid_anchor_count"
)

// ScoutIssue records one item-local rejected candidate.
type ScoutIssue struct {
	Position int            `json:"position"`
	Code     ScoutIssueCode `json:"code"`
}

// ScoutStatus is the persisted Theme Scout status (no prose, no source).
type ScoutStatus struct {
	State         string       `json:"state"` // prepared | accepted | accepted_partial | failed | unavailable
	Received      int          `json:"received"`
	Accepted      int          `json:"accepted"`
	Rejected      int          `json:"rejected"`
	Issues        []ScoutIssue `json:"issues,omitempty"`
	SeedCoverage  int          `json:"seed_coverage"`
	VocabCoverage int          `json:"vocab_coverage"`
	// Normalized records typed per-field editorial truncation counts
	// (Decision 224): title/question/why_it_matters/expected_learning. A
	// non-empty map means overlong provisional prose was bounded, never
	// silently dropped.
	Normalized map[string]int `json:"normalized,omitempty"`
}

// AnchorAssessment is one per-anchor classification from Theme Adjudication
// (contract E). fit and role are internal enums, never UX.
type AnchorAssessment struct {
	AnchorRef            string   `json:"anchor_ref"`
	Fit                  FitClass `json:"fit"`
	Role                 string   `json:"role,omitempty"`
	SupportedObservation string   `json:"supported_observation"`
}

// AdjudicatedTheme is one Source Review / Theme Adjudication result (contract E).
type AdjudicatedTheme struct {
	CandidateRef      string             `json:"candidate_ref"`
	FinalTitle        string             `json:"final_title"`
	FinalQuestion     string             `json:"final_question"`
	AnchorAssessments []AnchorAssessment `json:"anchor_assessments"`
	ReadingOrder      []string           `json:"reading_order"`
	Unknowns          []string           `json:"unknowns,omitempty"`
}

// AdjudicationResponse is the decoded Theme Adjudication result.
type AdjudicationResponse struct {
	Themes []AdjudicatedTheme `json:"themes"`
}

// AdjudicationIssueCode is a closed item-local rejection reason for a theme.
type AdjudicationIssueCode string

const (
	AdjIssueUnrequestedOutput      AdjudicationIssueCode = "unrequested_output"
	AdjIssueDecodeCandidate        AdjudicationIssueCode = "decode_candidate"
	AdjIssueUnknownCandidateRef    AdjudicationIssueCode = "unknown_candidate_ref"
	AdjIssueDuplicateCandidateRef  AdjudicationIssueCode = "duplicate_candidate_ref"
	AdjIssueEmptyFinalProse        AdjudicationIssueCode = "empty_final_prose"
	AdjIssueUnknownAnchor          AdjudicationIssueCode = "unknown_anchor"
	AdjIssueAnchorOutsideCandidate AdjudicationIssueCode = "anchor_outside_candidate"
	AdjIssueDuplicateAssessment    AdjudicationIssueCode = "duplicate_assessment"
	AdjIssueInvalidFit             AdjudicationIssueCode = "invalid_fit"
	AdjIssueEmptyObservation       AdjudicationIssueCode = "empty_observation"
	AdjIssueObservationTooLong     AdjudicationIssueCode = "observation_too_long"
	AdjIssueUnknownTooLong         AdjudicationIssueCode = "unknown_too_long"
	AdjIssueTooManyUnknowns        AdjudicationIssueCode = "too_many_unknowns"
	AdjIssueUnknownReadingRef      AdjudicationIssueCode = "unknown_reading_ref"
	AdjIssueNoDirect               AdjudicationIssueCode = "no_direct"
)

// Reading is one exact ordered reading on a Study card. It carries only the
// public editorial label plus exact path/symbol/line for source navigation.

// AdjudicationStatus is the persisted Theme Adjudication status (no prose, no
// source).
type AdjudicationStatus struct {
	State    string              `json:"state"` // prepared | accepted | accepted_partial | failed | unavailable
	Received int                 `json:"received"`
	Accepted int                 `json:"accepted"`
	Rejected int                 `json:"rejected"`
	Issues   []AdjudicationIssue `json:"issues,omitempty"`
	// Normalized records typed editorial truncation counts (Decision 224):
	// observation / unknown / unknowns_capped. Non-empty means overlong
	// bounded evidence was retained, never dropped as empty.
	Normalized map[string]int `json:"normalized,omitempty"`
}

// AdjudicationIssue records one item-local rejected theme.
type AdjudicationIssue struct {
	Position int                   `json:"position"`
	Code     AdjudicationIssueCode `json:"code"`
}

// It never carries source bytes. CanonicalSpanID is an internal binding used
// by the re-based browse derivation (published stage); it is never serialized
// to a report.
type Reading struct {
	Label  string `json:"label"`
	Symbol string `json:"symbol"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	// SupportedObservation is the bounded per-anchor model interpretation
	// over the supplied exact source (Decision 224 / D219 C). It is a
	// model interpretation, never a locally proven runtime fact, and is
	// bounded to MaxEditorialRunes by the adjudication normalizer.
	SupportedObservation string `json:"supported_observation,omitempty"`
	// Fit is the user-facing support role derived from the adjudicator's
	// classification: direct or supporting (weak/irrelevant never publish).
	Fit FitClass `json:"fit,omitempty"`
	// CanonicalSpanID is an internal binding used by the re-based browse
	// derivation (published stage). It is never serialized to a report.
	CanonicalSpanID string `json:"canonical_span_id,omitempty"`
}

// ThemeCard is the reduced, published Study card (contract F / projection).
// It contains editorial prose + exact readings + a badge, and zero source
// bytes. CanonicalIdentity is derived from accepted exact refs + local
// contract data, never model prose.
type ThemeCard struct {
	Ordinal          int       `json:"ordinal"`
	CanonicalID      string    `json:"canonical_id"`
	ThemeKind        ThemeKind `json:"theme_kind"`
	FinalTitle       string    `json:"final_title"`
	FinalQuestion    string    `json:"final_question"`
	WhyItMatters     string    `json:"why_it_matters"`
	ExpectedLearning string    `json:"expected_learning"`
	Readings         []Reading `json:"readings"`
	Badge            string    `json:"badge"` // editorial source-backed | partial
	DirectCount      int       `json:"direct_count"`
	SupportingCount  int       `json:"supporting_count"`
}
