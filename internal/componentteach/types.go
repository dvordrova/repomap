// Package componentteach builds the bounded, source-grounded teaching input
// for one component research question. It deliberately contains no repository,
// provider, or presentation I/O.
package componentteach

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/componentprobe"
)

const (
	BundleVersion         = 1
	IndexVersion          = 1
	SelectionTraceVersion = 1
	BudgetVersion         = 1
	ReportVersion         = 1

	MaxModelBytes = 48 * 1024
)

const (
	maxSliceLines = 96
	maxSliceBytes = 8 * 1024
	maxItemText   = 2 * 1024
	maxItems      = 16
)

var teacherEvidenceIDPattern = regexp.MustCompile(`^teach-[0-9a-f]{20}$`)
var frontierIDPattern = regexp.MustCompile(`^frontier-[0-9a-f]{20}$`)

type SupportBasis string

const (
	SupportOrientationHypothesis SupportBasis = "orientation_hypothesis"
	SupportStaticActiveBuild     SupportBasis = "static_active_build"
	SupportSource                SupportBasis = "source_supported"
	SupportTestNavigation        SupportBasis = "test_navigation_only"
)

type EvidenceKind string

const (
	EvidenceOrientationNote EvidenceKind = "orientation_note"
	EvidenceStaticRelation  EvidenceKind = "static_relation"
	EvidenceSourceSlice     EvidenceKind = "source_slice"
	EvidenceCallsiteSlice   EvidenceKind = "callsite_slice"
	EvidenceTestReference   EvidenceKind = "test_reference"
)

type Component struct {
	Name              string       `json:"name"`
	PurposeHypothesis string       `json:"purpose_hypothesis"`
	SupportBasis      SupportBasis `json:"support_basis"`
}

type PrimaryQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Why      string `json:"why"`
}

// Bundle is the only teacher-visible artifact. Locator metadata intentionally
// lives in Index, never alongside model-visible evidence.
type Bundle struct {
	Version               int             `json:"version"`
	GoalObjective         string          `json:"goal_objective"`
	Component             Component       `json:"component"`
	PrimaryQuestion       PrimaryQuestion `json:"primary_question"`
	Evidence              []EvidenceItem  `json:"evidence"`
	UnresolvedFrontierIDs []string        `json:"unresolved_frontier_ids"`
	UnresolvedFrontiers   []FrontierHint  `json:"unresolved_frontiers"`
	Warnings              []string        `json:"warnings"`
}

// FrontierHint makes an opaque next-hop ID meaningful to the teacher without
// exposing its local source locator or pretending it is runtime proof.
type FrontierHint struct {
	ID             string       `json:"id"`
	Kind           string       `json:"kind"`
	Direction      string       `json:"direction"`
	Name           string       `json:"name"`
	EntityKind     string       `json:"entity_kind"`
	SupportBasis   SupportBasis `json:"support_basis"`
	NavigationOnly bool         `json:"navigation_only"`
}

type EvidenceItem struct {
	ID                string       `json:"id"`
	Kind              EvidenceKind `json:"kind"`
	SupportBasis      SupportBasis `json:"support_basis"`
	Summary           string       `json:"summary"`
	Content           []string     `json:"content,omitempty"`
	Caller            string       `json:"caller,omitempty"`
	Callee            string       `json:"callee,omitempty"`
	Direction         string       `json:"direction,omitempty"`
	ActiveBuildCaveat string       `json:"active_build_caveat,omitempty"`
	NavigationOnly    bool         `json:"navigation_only,omitempty"`
}

type LocatorKind string

const (
	LocatorEvidence LocatorKind = "evidence"
	LocatorFrontier LocatorKind = "frontier"
)

type Index struct {
	Version int            `json:"version"`
	Entries []LocatorEntry `json:"entries"`
}

type LocatorEntry struct {
	ID        string      `json:"id"`
	Kind      LocatorKind `json:"kind"`
	Path      string      `json:"path"`
	StartLine int         `json:"start_line"`
	EndLine   int         `json:"end_line"`
	Column    int         `json:"column,omitempty"`
	Origins   []Origin    `json:"origins"`
}

type Origin struct {
	Round    int                         `json:"round"`
	ProbeID  string                      `json:"probe_id"`
	Artifact componentprobe.ArtifactKind `json:"artifact"`
	LocalID  string                      `json:"local_id"`
}

type Budget struct {
	Version       int `json:"version"`
	MaxModelBytes int `json:"max_model_bytes"`
}

func DefaultBudget() Budget {
	return Budget{Version: BudgetVersion, MaxModelBytes: MaxModelBytes}
}

type SelectionReason string

const (
	SelectionWithinBudget         SelectionReason = "within_budget"
	SelectionDuplicate            SelectionReason = "duplicate"
	SelectionModelBytesLimit      SelectionReason = "model_bytes_limit"
	SelectionRemoteValidationFail SelectionReason = "remote_validation_failed"
)

type SelectionDecision struct {
	Kind           EvidenceKind    `json:"kind"`
	ID             string          `json:"id"`
	Included       bool            `json:"included"`
	Reason         SelectionReason `json:"reason"`
	EstimatedBytes int             `json:"estimated_bytes"`
}

type SelectionTrace struct {
	Version             int                 `json:"version"`
	Budget              Budget              `json:"budget"`
	Decisions           []SelectionDecision `json:"decisions"`
	EstimatedModelBytes int                 `json:"estimated_model_bytes"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type Item struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
	FrontierIDs []string `json:"frontier_ids,omitempty"`
}

type Report struct {
	Version                  int    `json:"version"`
	PrimaryQuestionID        string `json:"primary_question_id"`
	MentalModel              []Item `json:"mental_model"`
	LifecycleSteps           []Item `json:"lifecycle_steps"`
	Boundaries               []Item `json:"boundaries"`
	DesignNotes              []Item `json:"design_notes"`
	FailuresAndObservability []Item `json:"failures_and_observability"`
	TestsAndChecks           []Item `json:"tests_and_checks"`
	Unknowns                 []Item `json:"unknowns"`
	NextDive                 []Item `json:"next_dive"`
}

type ParseResult struct {
	Report      Report       `json:"report"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (b Budget) Validate() error {
	if b.Version != BudgetVersion {
		return fmt.Errorf("component teach: unsupported budget version %d", b.Version)
	}
	if b.MaxModelBytes <= 0 || b.MaxModelBytes > MaxModelBytes {
		return fmt.Errorf("component teach: max_model_bytes must be between 1 and %d", MaxModelBytes)
	}
	return nil
}

func validLocatorPath(path string) bool {
	local := filepath.FromSlash(path)
	return path != "" && !strings.Contains(path, `\`) && !filepath.IsAbs(local) &&
		filepath.IsLocal(local) && filepath.ToSlash(filepath.Clean(local)) == path
}
