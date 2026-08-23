// Package targetportfolio compiles and reduces one file-addressed portfolio
// decision over the universally merged output of the initial target scouts.
package targetportfolio

import (
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	PreparationVersion    = 1
	ResponseSchemaVersion = 3

	// The provider receives the complete candidate set or no request at
	// all. There is deliberately no candidate-count or hypothesis-count
	// truncation hidden behind these byte envelopes.
	MaxRequestBytes         = 256 << 10
	MaxProviderRequestBytes = 2*MaxRequestBytes + 64<<10
	MaxResponseBytes        = 64 << 10
	MaxOutputTokens         = 64_000
)

const executionContract = "positive-file-target-portfolio-selection-v3"

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

// Request is the complete provider-visible bundle. Corpus identity and cache
// identity remain private.
type Request struct {
	Candidates []VisibleCandidate `json:"candidates"`
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

// Selection is a disjoint canonical-order partition. Targets contains only
// positive model selections. Omitted input candidates are restored locally as
// Unclassified and dropped from target execution. Default is non-nil exactly
// when Targets is non-empty, and then always points to a Target.
type Selection struct {
	Default      *VisibleCandidate
	Targets      []VisibleCandidate
	Unclassified []VisibleCandidate
}
