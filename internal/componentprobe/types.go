// Package componentprobe collects a bounded, source-grounded dossier for the
// first research question selected by componentstudy. It is an experimental
// cube: repository survey, model planning, and presentation stay outside.
package componentprobe

import (
	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const BundleVersion = 1

const (
	RoundInitial  = 1
	RoundFrontier = 2
)

const (
	hardMaxSymbols             = 3
	hardMaxCallsPerDirection   = 6
	hardMaxProvenancePerFact   = 3
	hardMaxSourceLines         = 96
	hardMaxSourceBytes         = 16 * 1024
	hardMaxCallsiteWindows     = 6
	hardMaxCallsiteLinesBefore = 8
	hardMaxCallsiteLinesAfter  = 12
	hardMaxCallsiteWindowBytes = 8 * 1024
	hardMaxCallsiteBytes       = 24 * 1024
	hardMaxCallsiteFileBytes   = 1024 * 1024
	hardMaxFrontier            = 6
)

type Provider interface {
	analyzer.ExactSymbolAnalyzer
	testevidence.ReferenceFinder
}

type Status string

const (
	StatusConnected Status = "connected"
	StatusFrontier  Status = "frontier"
	StatusBlocked   Status = "blocked"
)

type Direction string

const (
	DirectionIncoming  Direction = "incoming"
	DirectionOutgoing  Direction = "outgoing"
	DirectionReference Direction = "reference"
)

type FrontierKind string

const (
	FrontierCallEndpoint  FrontierKind = "call_endpoint"
	FrontierTestReference FrontierKind = "test_reference"
)

type ArtifactKind string

const (
	ArtifactStructural ArtifactKind = "structural"
	ArtifactSource     ArtifactKind = "source"
	ArtifactTests      ArtifactKind = "tests"
)

type EvidenceKind string

const (
	EvidenceResolution    EvidenceKind = "resolution"
	EvidenceCandidate     EvidenceKind = "candidate"
	EvidenceIncomingCall  EvidenceKind = "incoming_call"
	EvidenceOutgoingCall  EvidenceKind = "outgoing_call"
	EvidenceSourceLine    EvidenceKind = "source_line"
	EvidenceTestReference EvidenceKind = "test_reference"
)

type SupportBasis string

const (
	SupportOrientationHypothesis SupportBasis = "orientation_hypothesis"
	SupportStaticNavigation      SupportBasis = "static_navigation"
	SupportSource                SupportBasis = "source_supported"
	SupportTestNavigation        SupportBasis = "test_reference_navigation"
)

// Options may only tighten the experiment's hard bounds. Zero values select
// the bounded defaults.
type Options struct {
	MaxCallsiteWindows     int
	CallsiteLinesBefore    int
	CallsiteLinesAfter     int
	MaxCallsiteWindowBytes int
	MaxCallsiteBytes       int
	MaxFrontier            int
}

type Bundle struct {
	Version         int              `json:"version"`
	Round           int              `json:"round"`
	Parent          *Parent          `json:"parent,omitempty"`
	Status          Status           `json:"status"`
	Focus           Focus            `json:"focus"`
	SymbolProbes    []SymbolProbe    `json:"symbol_probes"`
	CallsiteWindows []CallsiteWindow `json:"callsite_windows"`
	Frontier        []Frontier       `json:"frontier"`
	Warnings        []Warning        `json:"warnings"`
}

type Parent struct {
	BundleSHA256       string `json:"bundle_sha256"`
	AcceptedFrontierID string `json:"accepted_frontier_id"`
}

type Focus struct {
	Goal            componentstudy.Goal            `json:"goal"`
	Component       componentstudy.Component       `json:"component"`
	PrimaryQuestion componentstudy.Question        `json:"primary_question"`
	SelectedFiles   []componentstudy.FileCandidate `json:"selected_files"`
	SupportBasis    SupportBasis                   `json:"support_basis"`
}

// SymbolProbe keeps the existing bounded artifacts unchanged. EvidenceIndex
// gives their local IDs a probe-scoped globally unique ID for later model calls.
type SymbolProbe struct {
	ID             string                         `json:"id"`
	SelectedSymbol componentstudy.SymbolCandidate `json:"selected_symbol"`
	Structural     symbol.Bundle                  `json:"structural"`
	Source         sourcecard.Card                `json:"source"`
	Tests          testevidence.Bundle            `json:"tests"`
	EvidenceIndex  []EvidenceRef                  `json:"evidence_index"`
}

type EvidenceRef struct {
	ID      string         `json:"id"`
	Kind    EvidenceKind   `json:"kind"`
	LocalID string         `json:"local_id"`
	Origin  EvidenceOrigin `json:"origin"`
	Basis   SupportBasis   `json:"support_basis"`
}

type EvidenceOrigin struct {
	ProbeID  string       `json:"probe_id"`
	Artifact ArtifactKind `json:"artifact"`
	LocalID  string       `json:"local_id"`
}

type CallsiteWindow struct {
	EvidenceID string                `json:"evidence_id"`
	Direction  Direction             `json:"direction"`
	Caller     evidence.Entity       `json:"caller"`
	Callee     evidence.Entity       `json:"callee"`
	Callsite   evidence.Location     `json:"callsite"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
	Origin     EvidenceOrigin        `json:"origin"`
	Basis      SupportBasis          `json:"support_basis"`
	StartLine  int                   `json:"start_line"`
	EndLine    int                   `json:"end_line"`
	Lines      []SourceLine          `json:"lines"`
	Truncated  bool                  `json:"truncated"`
}

type SourceLine struct {
	EvidenceID string `json:"evidence_id"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// Frontier is a next-hop candidate, not a claim that runtime crosses it.
// Test references are additionally marked navigation-only.
type Frontier struct {
	ID             string                `json:"id"`
	Kind           FrontierKind          `json:"kind"`
	Direction      Direction             `json:"direction"`
	Name           string                `json:"name"`
	EntityKind     evidence.EntityKind   `json:"entity_kind"`
	Location       evidence.Location     `json:"location"`
	Certainty      evidence.Certainty    `json:"certainty"`
	Provenance     []evidence.Provenance `json:"provenance"`
	Origins        []EvidenceOrigin      `json:"origins"`
	Basis          SupportBasis          `json:"support_basis"`
	NavigationOnly bool                  `json:"navigation_only"`
	RuntimeProof   bool                  `json:"runtime_proof"`
}

type Warning struct {
	Code     string `json:"code"`
	SymbolID string `json:"symbol_id,omitempty"`
	Message  string `json:"message"`
}
