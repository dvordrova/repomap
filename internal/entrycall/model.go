// Package entrycall builds a compact, refs-only semantic projection of generic
// activity candidates reachable from exact process roots. Exact identities and
// restored evidence stay local; only the bounded catalog is provider-visible.
package entrycall

const (
	SubstrateVersion = 3
	RequestVersion   = 3
	ResponseVersion  = 5

	MaxRequestBytes  = 128 * 1024
	MaxResponseBytes = 64 * 1024
	// MaxRawSurfaceCandidates is the independent local reservoir bound. The
	// provider-visible compiler applies a deliberately smaller projection
	// bound; retaining the larger exact reservoir here prevents syntax walk
	// order from silently deciding which candidates can be classified.
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
	ClosedNoEntrypoints  ClosedReason = "no_entrypoints"
)

// Location is exact repository-local evidence. It is intentionally absent
// from the provider request and appears only in a locally restored Result.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// ExactRoot is live-run-only process authority. Its canonical node ID is
// excluded from JSON so an accidental marshal cannot expose it.
type ExactRoot struct {
	NodeID string `json:"-"`
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
	SurfaceCandidates []ExactSurfaceCandidate `json:"-"`
	Coverage          Coverage                `json:"-"`
}

func Unavailable(reason ClosedReason) Substrate {
	return Substrate{Version: SubstrateVersion, State: StateUnavailable, ClosedReason: reason}
}

// Snapshot returns an independently owned copy for the handoff from the exact
// program pass to the activity-surface cube.
func (substrate Substrate) Snapshot() Substrate {
	result := substrate
	result.Roots = append([]ExactRoot(nil), substrate.Roots...)
	result.SurfaceCandidates = append([]ExactSurfaceCandidate(nil), substrate.SurfaceCandidates...)
	for index := range result.SurfaceCandidates {
		result.SurfaceCandidates[index].Facts = append(
			[]ExactSurfaceFact(nil), substrate.SurfaceCandidates[index].Facts...,
		)
	}
	return result
}
