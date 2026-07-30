// Package guidedtour builds and validates a bounded editorial tour over
// already saved repository evidence. It performs no repository analysis and
// contains no model or network client.
package guidedtour

import "github.com/dvordrova/repomap/internal/evidence"

const (
	BundleVersion   = 1
	ProposalVersion = 1
	StoryVersion    = 1
	RecordVersion   = 1
	PromptVersion   = "guided-tour-editor-json-v5"
)

type CandidateKind string

const (
	CandidateSavedTrace         CandidateKind = "saved_trace"
	CandidateSuggestedDirection CandidateKind = "suggested_direction"
)

type OrderingBasis string

const (
	OrderingTrace     OrderingBasis = "trace_order"
	OrderingEditorial OrderingBasis = "editorial"
)

// Bundle is the complete bounded input visible to the optional editor.
type Bundle struct {
	Version       int         `json:"version"`
	RepoName      string      `json:"repo_name"`
	CanvasVersion int         `json:"canvas_version"`
	Candidates    []Candidate `json:"candidates"`
	Components    []Component `json:"components"`
}

type Candidate struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Kind          CandidateKind `json:"kind"`
	Trigger       string        `json:"trigger"`
	Summary       string        `json:"summary"`
	OrderingBasis OrderingBasis `json:"ordering_basis"`
	Beats         []Beat        `json:"beats"`
	Gaps          []Gap         `json:"gaps"`
}

type Beat struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Label        string        `json:"label"`
	Detail       string        `json:"detail"`
	Sequence     int           `json:"sequence"`
	ComponentIDs []string      `json:"component_ids"`
	SurfaceIDs   []string      `json:"surface_ids"`
	FlowID       string        `json:"flow_id"`
	FlowStepIDs  []string      `json:"flow_step_ids"`
	Evidence     []EvidenceRef `json:"evidence"`
}

type EvidenceRef struct {
	ID       string             `json:"id"`
	Kind     string             `json:"kind"`
	Label    string             `json:"label"`
	Location *evidence.Location `json:"location,omitempty"`
}

type Gap struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Detail   string        `json:"detail"`
	Evidence []EvidenceRef `json:"evidence"`
}

type Component struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Prompt is provider-neutral. Transport adapters may wrap it, but must not
// add repository material or relax the response contract.
type Prompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

// Proposal is the exact model response contract. It deliberately has no
// fields for repository references, evidence, components, or locations.
type Proposal struct {
	Version     int                  `json:"version"`
	CandidateID string               `json:"candidate_id"`
	Title       string               `json:"title"`
	Summary     string               `json:"summary"`
	Steps       []ProposedStep       `json:"steps"`
	GapSummary  []ProposedGapSummary `json:"gap_summary"`
}

type ProposedStep struct {
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	BeatIDs     []string `json:"beat_ids"`
}

type ProposedGapSummary struct {
	Explanation string   `json:"explanation"`
	GapIDs      []string `json:"gap_ids"`
}

// Story is a locally materialized proposal. Every repository-bearing field is
// copied or derived from the selected candidate's referenced beats and gaps.
type Story struct {
	Version       int               `json:"version"`
	CandidateID   string            `json:"candidate_id"`
	CandidateName string            `json:"candidate_name"`
	CandidateKind CandidateKind     `json:"candidate_kind"`
	Trigger       string            `json:"trigger"`
	OrderingBasis OrderingBasis     `json:"ordering_basis"`
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	Steps         []StoryStep       `json:"steps"`
	GapSummary    []StoryGapSummary `json:"gap_summary"`
	Components    []Component       `json:"components"`
}

type StoryStep struct {
	Title        string        `json:"title"`
	Explanation  string        `json:"explanation"`
	BeatIDs      []string      `json:"beat_ids"`
	Beats        []Beat        `json:"beats"`
	ComponentIDs []string      `json:"component_ids"`
	Components   []Component   `json:"components"`
	SurfaceIDs   []string      `json:"surface_ids"`
	FlowIDs      []string      `json:"flow_ids"`
	FlowStepIDs  []string      `json:"flow_step_ids"`
	Evidence     []EvidenceRef `json:"evidence"`
}

type StoryGapSummary struct {
	Explanation string        `json:"explanation"`
	GapIDs      []string      `json:"gap_ids"`
	Gaps        []Gap         `json:"gaps"`
	Evidence    []EvidenceRef `json:"evidence"`
}

// Record binds one validated editorial proposal to the exact canonical input
// bundle used to produce it.
type Record struct {
	Version      int      `json:"version"`
	BundleSHA256 string   `json:"bundle_sha256"`
	Proposal     Proposal `json:"proposal"`
}
