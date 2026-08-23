// Package surfacediscovery builds exact, target-scoped Go program facts for
// the ordinary repomap pipeline. It does not assign framework, route, worker,
// transport, or other product semantics.
package surfacediscovery

import (
	"sort"

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
)

const (
	// DefaultDirectCallDepth bounds exact repository-local call edges outward
	// from the selected analysis target. The declaration inventory remains
	// complete independently of this edge bound.
	DefaultDirectCallDepth = 10
	// DefaultDirectCallEdgeLimit is the ordinary target-scoped graph ceiling.
	DefaultDirectCallEdgeLimit = 10_000
)

type Options struct {
	RepoPath  string
	GoTarget  string
	BuildTags []string

	DirectCallDepth     int
	DirectCallEdgeLimit int

	// Optional exact sidecars reuse the same package load and SSA instruction
	// pass. None of them is persisted from this package.
	CaptureEntryCallSubstrate  bool
	CaptureExternalCallIndex   bool
	CaptureCoreObjectIndex     bool
	CaptureDynamicHandoffIndex bool
	Progress                   func(PhaseProgress)
}

type PhaseProgress struct {
	Phase         string
	State         string
	ElapsedMillis int64
	Completed     int
	Total         int
	Detail        string
}

type PhaseMetric struct {
	Phase         string `json:"phase"`
	LatencyMillis int64  `json:"latency_ms"`
	Completed     int    `json:"completed,omitempty"`
	Total         int    `json:"total,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

func DefaultOptions(repoPath, resolvedGoTarget string) Options {
	return Options{
		RepoPath: repoPath, GoTarget: resolvedGoTarget,
		DirectCallDepth: DefaultDirectCallDepth, DirectCallEdgeLimit: DefaultDirectCallEdgeLimit,
	}
}

type Scenario struct {
	ID      string   `json:"id"`
	GOOS    string   `json:"goos"`
	GOARCH  string   `json:"goarch"`
	Tags    []string `json:"tags"`
	GoFlags string   `json:"go_flags,omitempty"`
}

type Symbol struct {
	ID            string   `json:"id"`
	EquivalentIDs []string `json:"equivalent_ids,omitempty"`
	Package       string   `json:"package"`
	Name          string   `json:"name"`
	Location      Location `json:"location"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// PackageDiagnostic is bounded, repository-relative evidence explaining why
// a selected package could not become an exact SSA program.
type PackageDiagnostic struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Message  string    `json:"message"`
	Package  string    `json:"package"`
	Location *Location `json:"location,omitempty"`
}

type ProgramCoverage struct {
	PackageDiagnostics []PackageDiagnostic `json:"package_diagnostics"`
	Phases             []PhaseMetric       `json:"phases,omitempty"`
}

type Result struct {
	Coverage ProgramCoverage `json:"program_coverage"`

	// These exact artifacts are live-run handoffs. Raw source graphs and model
	// catalogs are not serialized from surface discovery.
	DirectCallIndex     *DirectCallIndex        `json:"-"`
	EntryCallSubstrate  *entrycall.Substrate    `json:"-"`
	ExternalCallIndex   *ExternalCallIndex      `json:"-"`
	CoreObjectIndex     *gocoreobject.Index     `json:"-"`
	DynamicHandoffIndex *godynamichandoff.Index `json:"-"`
}

func (r *Result) normalize() {
	if r.Coverage.PackageDiagnostics == nil {
		r.Coverage.PackageDiagnostics = []PackageDiagnostic{}
	}
	if r.Coverage.Phases == nil {
		r.Coverage.Phases = []PhaseMetric{}
	}
}

func compactStrings(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}
	values := append([]string(nil), input...)
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
