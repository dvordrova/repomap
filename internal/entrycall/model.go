// Package entrycall builds a compact, refs-only semantic projection of exact
// direct calls reachable from process main declarations. The exact substrate
// and restored evidence stay local; only Request is provider-visible.
package entrycall

import (
	"path"
	"sort"
	"strings"
	"unicode"
)

const (
	SubstrateVersion = 2
	RequestVersion   = 2
	ResultVersion    = 3
	StatusVersion    = 3

	MaxRoots                   = 4
	MaxDepth                   = 3
	MaxOutgoingFamiliesPerNode = 12
	MaxNodesPerRoot            = 32
	MaxFamiliesPerRoot         = 48
	MaxNodes                   = 128
	MaxFamilies                = 192
	// Every family advertised inside one root is already bounded by
	// MaxFamiliesPerRoot. Selection has no smaller, second resource ceiling.
	MaxSelectedFamiliesPerRoot = MaxFamiliesPerRoot
	MaxRepresentativeCallsites = 3
	MaxLabelRunes              = 96
	MaxRequestBytes            = 128 * 1024
	MaxResponseBytes           = 64 * 1024
	MaxArtifactBytes           = 256 * 1024

	// MaxRawSurfaceCandidates is the independent local reservoir bound. The
	// provider-visible compiler applies a deliberately smaller projection
	// bound; retaining the larger exact reservoir here prevents call-family
	// ranking from silently deciding which syntax candidates can be classified.
	MaxRawSurfaceCandidates        = 2_048
	MaxRawSurfaceFactsPerCandidate = 16
)

type State string

const (
	StateReady       State = "ready"
	StateUnavailable State = "unavailable"
)

type ClosedReason string

const (
	ClosedSSAUnavailable ClosedReason = "ssa_unavailable"
	ClosedIndexLimit     ClosedReason = "index_limit"
	ClosedSidecarLimit   ClosedReason = "sidecar_limit"
	ClosedNoEntrypoints  ClosedReason = "no_entrypoints"
)

type Invocation string

const (
	InvocationSynchronous Invocation = "synchronous"
	InvocationGoroutine   Invocation = "goroutine"
	InvocationDeferred    Invocation = "deferred"
)

func (invocation Invocation) Valid() bool {
	switch invocation {
	case InvocationSynchronous, InvocationGoroutine, InvocationDeferred:
		return true
	default:
		return false
	}
}

// Location is exact repository-local evidence. It is intentionally absent
// from the provider request and appears only in a locally restored Result.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// The Exact* types form a live-run-only substrate. Their fields are excluded
// from JSON so an accidental marshal cannot expose canonical IDs or paths.
type ExactRoot struct {
	NodeID string `json:"-"`
}

type ExactNode struct {
	ID          string   `json:"-"`
	Label       string   `json:"-"`
	Declaration Location `json:"-"`
	External    bool     `json:"-"`
}

type ExactFamily struct {
	ID           string     `json:"-"`
	CallerID     string     `json:"-"`
	CalleeID     string     `json:"-"`
	Invocation   Invocation `json:"-"`
	WitnessCount int        `json:"-"`
	Callsites    []Location `json:"-"`
}

type ExactFrontier struct {
	CallerID                  string `json:"-"`
	DynamicInvokesExcluded    int    `json:"-"`
	NonStaticCallsExcluded    int    `json:"-"`
	UnidentifiedCallsExcluded int    `json:"-"`
}

type SurfaceCandidateForm string

const (
	SurfaceCandidateDirectCall     SurfaceCandidateForm = "direct_call"
	SurfaceCandidateKeyedComposite SurfaceCandidateForm = "keyed_composite"
)

func (form SurfaceCandidateForm) Valid() bool {
	switch form {
	case SurfaceCandidateDirectCall, SurfaceCandidateKeyedComposite:
		return true
	default:
		return false
	}
}

type SurfaceFactKind string

const (
	SurfaceFactString   SurfaceFactKind = "string"
	SurfaceFactToken    SurfaceFactKind = "token"
	SurfaceFactCallable SurfaceFactKind = "callable"
)

func (kind SurfaceFactKind) Valid() bool {
	switch kind {
	case SurfaceFactString, SurfaceFactToken, SurfaceFactCallable:
		return true
	default:
		return false
	}
}

// ExactSurfaceFact is private local authority retained from already-loaded Go
// syntax, types, and SSA. Every field is JSON-opaque so canonical values and
// locations cannot cross the provider boundary by accidental marshaling.
type ExactSurfaceFact struct {
	ID       string          `json:"-"`
	Kind     SurfaceFactKind `json:"-"`
	Position int             `json:"-"`
	Label    string          `json:"-"`
	Value    string          `json:"-"`
	Location Location        `json:"-"`
}

// ExactSurfaceCandidate is one exact callsite or keyed composite associated
// with one process root by the local static reachability closure. The semantic
// compiler may advertise bounded refs for these fields, but the model never
// owns their values, locations, IDs, or order.
type ExactSurfaceCandidate struct {
	ID         string               `json:"-"`
	RootNodeID string               `json:"-"`
	Form       SurfaceCandidateForm `json:"-"`
	Sketch     string               `json:"-"`
	Site       Location             `json:"-"`
	Facts      []ExactSurfaceFact   `json:"-"`
}

type Coverage struct {
	RootsConsidered                      int `json:"-"`
	NodesConsidered                      int `json:"-"`
	FamiliesConsidered                   int `json:"-"`
	WitnessesIndexed                     int `json:"-"`
	SurfaceCandidatesConsidered          int `json:"-"`
	SurfaceCandidatesIndexed             int `json:"-"`
	SurfaceCandidateLimitExcluded        int `json:"-"`
	SurfaceCandidateFactsConsidered      int `json:"-"`
	SurfaceCandidateFactsIndexed         int `json:"-"`
	SurfaceCandidateFactLimitExcluded    int `json:"-"`
	UnsafeSurfaceCandidateFactsExcluded  int `json:"-"`
	UnreachableSurfaceCandidatesExcluded int `json:"-"`
}

type Substrate struct {
	Version           int                     `json:"-"`
	State             State                   `json:"-"`
	ClosedReason      ClosedReason            `json:"-"`
	Roots             []ExactRoot             `json:"-"`
	Nodes             []ExactNode             `json:"-"`
	Families          []ExactFamily           `json:"-"`
	Frontiers         []ExactFrontier         `json:"-"`
	SurfaceCandidates []ExactSurfaceCandidate `json:"-"`
	Coverage          Coverage                `json:"-"`
}

func Unavailable(reason ClosedReason) Substrate {
	return Substrate{Version: SubstrateVersion, State: StateUnavailable, ClosedReason: reason}
}

// Snapshot returns an independently owned copy for the handoff from the
// surface SSA pass to the optional semantic experiment.
func (substrate Substrate) Snapshot() Substrate {
	result := substrate
	result.Roots = append([]ExactRoot(nil), substrate.Roots...)
	result.Nodes = append([]ExactNode(nil), substrate.Nodes...)
	result.Families = append([]ExactFamily(nil), substrate.Families...)
	for index := range result.Families {
		result.Families[index].Callsites = append([]Location(nil), substrate.Families[index].Callsites...)
	}
	result.Frontiers = append([]ExactFrontier(nil), substrate.Frontiers...)
	result.SurfaceCandidates = append([]ExactSurfaceCandidate(nil), substrate.SurfaceCandidates...)
	for index := range result.SurfaceCandidates {
		result.SurfaceCandidates[index].Facts = append(
			[]ExactSurfaceFact(nil), substrate.SurfaceCandidates[index].Facts...,
		)
	}
	return result
}

func PackageLabel(packagePath, receiver, name string) string {
	packageName := path.Base(strings.TrimSpace(packagePath))
	if packageName == "." || packageName == "/" {
		packageName = "package"
	}
	symbol := strings.TrimSpace(name)
	if receiver = strings.TrimSpace(receiver); receiver != "" {
		symbol = receiver + "." + symbol
	}
	return sanitizeLabel(packageName + " · " + symbol)
}

func sanitizeLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), r == '/', r == '\\':
			return ' '
		default:
			return r
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > MaxLabelRunes {
		value = strings.TrimSpace(string(runes[:MaxLabelRunes]))
	}
	if value == "" {
		return "symbol"
	}
	return value
}

func sortLocations(locations []Location) {
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Path != locations[j].Path {
			return locations[i].Path < locations[j].Path
		}
		if locations[i].Line != locations[j].Line {
			return locations[i].Line < locations[j].Line
		}
		return locations[i].Column < locations[j].Column
	})
}
