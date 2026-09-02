// Package targetportfolio compiles and reduces one file-addressed portfolio
// decision over the universally merged output of the initial target scouts.
package targetportfolio

import (
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	PreparationVersion    = 4
	ResponseSchemaVersion = 7

	// MaxRequestBytes is one classification-batch packing window, not an
	// aggregate candidate-authority bound. Run forms as many deterministic
	// disjoint batches and default-choice rounds as required to cover the
	// complete retained reservoir.
	MaxRequestBytes = 256 << 10
	// AdvisoryCompleteRequestBytes is the former one-call threshold. Crossing
	// it emits a warning only; it never rejects or truncates candidates.
	AdvisoryCompleteRequestBytes = 256 << 10
	// MaxDefaultRequestBytes and MaxProviderRequestBytes retain the former
	// packing estimates for diagnostics and provider-neutral helpers. Ordinary
	// execution uses llm.SemanticRecordByteLimit; neither value is acceptance
	// authority for an indivisible row.
	MaxDefaultRequestBytes  = 2*MaxRequestBytes + 8<<10
	MaxProviderRequestBytes = 2*MaxDefaultRequestBytes + 64<<10
	MaxResponseBytes        = llm.ProviderResponseByteLimit
	MaxOutputTokens         = 128_000
)

const executionContract = "positive-file-target-portfolio-selection-with-native-authority-v8"

// Candidate is the common output of the initial scouts after their dumb
// FileRef merge. Keep the alias so the portfolio does not invent a second
// wire shape for the same value.
type Candidate = analysistarget.FileCandidate

// VisibleCandidate adds the exact repository-relative path resolved locally
// from the corpus. It is both the provider-visible option and the value
// restored into Selection.
type VisibleCandidate struct {
	FileRef    corpus.FileID `json:"file_ref"`
	Path       string        `json:"path"`
	Hypotheses []string      `json:"hypotheses"`
}

// Request is either the complete locally retained reservoir or one bounded
// provider-visible classification projection. Corpus identity and cache
// identity remain private.
type Request struct {
	Candidates []VisibleCandidate `json:"candidates"`

	// RequiredTargetFileRefs is present when deterministic language adapters
	// have established exact native targets. It contains one canonical
	// representative per target, with one ref allowed to represent several
	// targets. Every ref must survive the portfolio; the model chooses their
	// default and may additionally retain repository-guidance candidates.
	RequiredTargetFileRefs *[]corpus.FileID `json:"required_target_file_refs,omitempty"`

	// ExecutableFileRefs is present only when exact executable authority is
	// bound. A pointer preserves the material distinction between an unbound
	// generic compilation (field omitted) and an exactly known library-only
	// surface (non-null empty array).
	ExecutableFileRefs *[]corpus.FileID `json:"executable_file_refs,omitempty"`
}

// Compilation owns the exact candidate and cache authority. Only Request may
// cross the provider boundary.
type Compilation struct {
	Request       Request
	RequestSHA256 string

	wire       []byte
	state      []byte
	corpus     corpus.Snapshot
	candidates []Candidate
	sealed     string

	executableAuthorityBound bool
	executableFileRefs       []corpus.FileID
	requiredAuthorityBound   bool
	requiredTargetFileRefs   []corpus.FileID
}

type Prompt struct {
	Version string
	System  string
	User    string
}

// Response is deliberately file-refs-only. The schema has exactly these two
// fields; unlike private execution state, it exposes no version or request
// identity to the model.
type Response struct {
	DefaultFileRef *corpus.FileID  `json:"default_file_ref"`
	TargetFileRefs []corpus.FileID `json:"target_file_refs"`
}

// DefaultRequest compares already accepted target candidates. It may choose
// one ref but cannot add, remove, or reclassify any target membership.
type DefaultRequest struct {
	Phase      string             `json:"phase"`
	Candidates []VisibleCandidate `json:"candidates"`
}

// DefaultResponse is deliberately one closed ref. The complete selected set
// is restored locally and never has to fit in this response.
type DefaultResponse struct {
	DefaultFileRef corpus.FileID `json:"default_file_ref"`
}

// Selection is a disjoint canonical-order partition. Targets contains only
// positive model selections. Omitted input candidates are restored locally as
// Unclassified and dropped from target execution. Default is non-nil exactly
// when Targets is non-empty, and then always points to a Target.
type Selection struct {
	Default      *VisibleCandidate
	Targets      []VisibleCandidate
	Unclassified []VisibleCandidate
}

// Execution owns one complete multi-call portfolio result plus every
// provider outcome in deterministic execution order. Callers may aggregate
// accounting without confusing the complete local authority with one batch.
type Execution struct {
	Selection Selection
	Outcomes  []llm.Outcome[Selection]
}
