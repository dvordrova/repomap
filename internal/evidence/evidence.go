package evidence

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const GraphVersion = 2

type Certainty string

const (
	CertaintyUnknown    Certainty = "unknown"
	CertaintyHypothesis Certainty = "hypothesis"
	CertaintyPossible   Certainty = "possible"
	CertaintyStatic     Certainty = "static"
	CertaintyObserved   Certainty = "observed"
	CertaintyVerified   Certainty = "verified"
)

func (c Certainty) Valid() bool {
	switch c {
	case CertaintyUnknown,
		CertaintyHypothesis,
		CertaintyPossible,
		CertaintyStatic,
		CertaintyObserved,
		CertaintyVerified:
		return true
	default:
		return false
	}
}

type EntityKind string

const (
	EntityUnknown    EntityKind = "unknown"
	EntityQuery      EntityKind = "query"
	EntityFile       EntityKind = "file"
	EntityModule     EntityKind = "module"
	EntityPackage    EntityKind = "package"
	EntityFunction   EntityKind = "function"
	EntityMethod     EntityKind = "method"
	EntityType       EntityKind = "type"
	EntityInterface  EntityKind = "interface"
	EntityField      EntityKind = "field"
	EntityVariable   EntityKind = "variable"
	EntityConstant   EntityKind = "constant"
	EntityTest       EntityKind = "test"
	EntityEntrypoint EntityKind = "entrypoint"
	EntityReference  EntityKind = "reference"
)

func (kind EntityKind) Valid() bool {
	switch kind {
	case EntityUnknown,
		EntityQuery,
		EntityFile,
		EntityModule,
		EntityPackage,
		EntityFunction,
		EntityMethod,
		EntityType,
		EntityInterface,
		EntityField,
		EntityVariable,
		EntityConstant,
		EntityTest,
		EntityEntrypoint,
		EntityReference:
		return true
	default:
		return false
	}
}

// SourceScope says whether an analyzer-resolved entity belongs to the
// repository or only to its surrounding toolchain. External entities may omit
// source locations so absolute dependency and toolchain paths do not leak into
// a repository evidence graph.
type SourceScope string

const (
	SourceScopeRepository       SourceScope = "repository"
	SourceScopeStandardLibrary  SourceScope = "stdlib"
	SourceScopeDependency       SourceScope = "dependency"
	SourceScopeOutsideWorkspace SourceScope = "outside_workspace"
)

func (scope SourceScope) Valid() bool {
	switch scope {
	case "",
		SourceScopeRepository,
		SourceScopeStandardLibrary,
		SourceScopeDependency,
		SourceScopeOutsideWorkspace:
		return true
	default:
		return false
	}
}

type RelationKind string

const (
	RelationMatchesQuery     RelationKind = "matches_query"
	RelationResolvesTo       RelationKind = "resolves_to"
	RelationCalls            RelationKind = "calls"
	RelationRegisters        RelationKind = "registers_command"
	RelationDispatches       RelationKind = "dispatches"
	RelationCallback         RelationKind = "callback"
	RelationConstructs       RelationKind = "constructs"
	RelationStartsGoroutine  RelationKind = "starts_goroutine"
	RelationCancels          RelationKind = "cancels"
	RelationUsesCancellation RelationKind = "uses_cancellation"
	RelationJoins            RelationKind = "joins"
	RelationReturns          RelationKind = "returns"
	RelationReads            RelationKind = "reads"
	RelationWrites           RelationKind = "writes"
	RelationImplements       RelationKind = "implements"
	RelationReferences       RelationKind = "references"
)

func (kind RelationKind) Valid() bool {
	switch kind {
	case RelationMatchesQuery,
		RelationResolvesTo,
		RelationCalls,
		RelationRegisters,
		RelationDispatches,
		RelationCallback,
		RelationConstructs,
		RelationStartsGoroutine,
		RelationCancels,
		RelationUsesCancellation,
		RelationJoins,
		RelationReturns,
		RelationReads,
		RelationWrites,
		RelationImplements,
		RelationReferences:
		return true
	default:
		return false
	}
}

// ResolutionKind records how a relation target was selected. It is
// intentionally orthogonal to Certainty: a statically selected interface
// method can still be only possible at runtime, while an observed target may
// have been resolved without source types.
type ResolutionKind string

const (
	ResolutionUnknown         ResolutionKind = "unknown"
	ResolutionStatic          ResolutionKind = "static"
	ResolutionTypeInferred    ResolutionKind = "type_inferred"
	ResolutionFrameworkRule   ResolutionKind = "framework_rule"
	ResolutionRuntimeObserved ResolutionKind = "runtime_observed"
	ResolutionModelSuggested  ResolutionKind = "model_suggested"
	ResolutionUnresolved      ResolutionKind = "unresolved"
)

func (kind ResolutionKind) Valid() bool {
	switch kind {
	case "",
		ResolutionUnknown,
		ResolutionStatic,
		ResolutionTypeInferred,
		ResolutionFrameworkRule,
		ResolutionRuntimeObserved,
		ResolutionModelSuggested,
		ResolutionUnresolved:
		return true
	default:
		return false
	}
}

// InvocationMode says how control reaches a relation target. Relation.Kind
// still carries the semantic verb (calls, registers, constructs, ...).
type InvocationMode string

const (
	InvocationUnknown     InvocationMode = "unknown"
	InvocationSynchronous InvocationMode = "synchronous"
	InvocationAwait       InvocationMode = "await"
	InvocationGoroutine   InvocationMode = "goroutine"
	InvocationCallback    InvocationMode = "callback"
	InvocationDeferred    InvocationMode = "deferred"
)

func (mode InvocationMode) Valid() bool {
	switch mode {
	case "",
		InvocationUnknown,
		InvocationSynchronous,
		InvocationAwait,
		InvocationGoroutine,
		InvocationCallback,
		InvocationDeferred:
		return true
	default:
		return false
	}
}

type Location struct {
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	EndColumn int    `json:"end_column,omitempty"`
}

// Condition preserves a source branch predicate without claiming that it is
// true for the selected runtime scenario. It is intentionally syntax evidence,
// not a general path-condition solver.
type Condition struct {
	Expression string   `json:"expression,omitempty"`
	Location   Location `json:"location"`
}

type BuildContext struct {
	GOOS      string   `json:"goos,omitempty"`
	GOARCH    string   `json:"goarch,omitempty"`
	BuildTags []string `json:"build_tags,omitempty"`
}

type Provenance struct {
	Provider  string    `json:"provider"`
	Version   string    `json:"version,omitempty"`
	Operation string    `json:"operation"`
	Detail    string    `json:"detail,omitempty"`
	Location  *Location `json:"location,omitempty"`
}

type Scenario struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Command    []string          `json:"command,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Build      BuildContext      `json:"build,omitempty"`
}

// LocationSet is a provider-produced set of source locations with the static
// analysis context that made them visible. Consumers may filter the locations,
// but must not invent stronger certainty or discard provenance.
type LocationSet struct {
	Locations  []Location   `json:"locations"`
	Certainty  Certainty    `json:"certainty"`
	Provenance []Provenance `json:"provenance"`
	Scenarios  []Scenario   `json:"scenarios"`
}

func (s LocationSet) Validate() error {
	if !s.Certainty.Valid() {
		return fmt.Errorf("evidence: location set has invalid certainty %q", s.Certainty)
	}
	if len(s.Provenance) == 0 {
		return fmt.Errorf("evidence: location set has no provenance")
	}
	for index, provenance := range s.Provenance {
		if provenance.Provider == "" || provenance.Operation == "" {
			return fmt.Errorf("evidence: location set provenance[%d] is incomplete", index)
		}
	}
	seenScenarios := make(map[string]struct{}, len(s.Scenarios))
	for index, scenario := range s.Scenarios {
		if scenario.ID == "" || scenario.Name == "" {
			return fmt.Errorf("evidence: location set scenarios[%d] is incomplete", index)
		}
		if _, exists := seenScenarios[scenario.ID]; exists {
			return fmt.Errorf("evidence: location set has duplicate scenario %q", scenario.ID)
		}
		seenScenarios[scenario.ID] = struct{}{}
	}
	if len(s.Scenarios) == 0 {
		return fmt.Errorf("evidence: location set has no build scenario")
	}
	for index, location := range s.Locations {
		if location.Path == "" || location.Line <= 0 || location.Column <= 0 {
			return fmt.Errorf("evidence: location set locations[%d] is incomplete", index)
		}
	}
	return nil
}

type Entity struct {
	ID       string      `json:"id"`
	Kind     EntityKind  `json:"kind"`
	Name     string      `json:"name"`
	Language string      `json:"language,omitempty"`
	Scope    SourceScope `json:"scope,omitempty"`
	Location *Location   `json:"location,omitempty"`
}

type Relation struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Kind       RelationKind   `json:"kind"`
	Resolution ResolutionKind `json:"resolution,omitempty"`
	Invocation InvocationMode `json:"invocation,omitempty"`
	Certainty  Certainty      `json:"certainty"`
	Provenance []Provenance   `json:"provenance"`
	Scenarios  []string       `json:"scenarios,omitempty"`
}

type Graph struct {
	Version   int          `json:"version"`
	RepoPath  string       `json:"repo_path"`
	Query     string       `json:"query,omitempty"`
	Build     BuildContext `json:"build,omitempty"`
	Entities  []Entity     `json:"entities"`
	Relations []Relation   `json:"relations"`
	Scenarios []Scenario   `json:"scenarios,omitempty"`
	Warnings  []string     `json:"warnings,omitempty"`
}

type Summary struct {
	Entities    int               `json:"entities"`
	Relations   int               `json:"relations"`
	ByCertainty map[Certainty]int `json:"by_certainty"`
}

func NewGraph(repoPath, query string) Graph {
	return Graph{
		Version:   GraphVersion,
		RepoPath:  repoPath,
		Query:     query,
		Entities:  []Entity{},
		Relations: []Relation{},
	}
}

func (g *Graph) AddEntity(entity Entity) {
	if entity.ID == "" {
		return
	}
	for i := range g.Entities {
		if g.Entities[i].ID == entity.ID {
			return
		}
	}
	g.Entities = append(g.Entities, entity)
}

func (g *Graph) AddRelation(relation Relation) {
	if relation.From == "" || relation.To == "" {
		return
	}
	for i := range g.Relations {
		existing := &g.Relations[i]
		if existing.From == relation.From &&
			existing.To == relation.To &&
			existing.Kind == relation.Kind &&
			existing.Resolution == relation.Resolution &&
			existing.Invocation == relation.Invocation &&
			existing.Certainty == relation.Certainty {
			existing.Provenance = appendUniqueProvenance(existing.Provenance, relation.Provenance...)
			existing.Scenarios = appendUniqueStrings(existing.Scenarios, relation.Scenarios...)
			return
		}
	}
	relation.Provenance = appendUniqueProvenance(nil, relation.Provenance...)
	relation.Scenarios = appendUniqueStrings(nil, relation.Scenarios...)
	g.Relations = append(g.Relations, relation)
}

func (g Graph) Summary() Summary {
	summary := Summary{
		Entities:    len(g.Entities),
		Relations:   len(g.Relations),
		ByCertainty: make(map[Certainty]int),
	}
	for _, relation := range g.Relations {
		summary.ByCertainty[relation.Certainty]++
	}
	return summary
}

func (g *Graph) Sort() {
	sort.Slice(g.Entities, func(i, j int) bool {
		return g.Entities[i].ID < g.Entities[j].ID
	})
	sort.Slice(g.Relations, func(i, j int) bool {
		left := g.Relations[i]
		right := g.Relations[j]
		if left.From != right.From {
			return left.From < right.From
		}
		if left.To != right.To {
			return left.To < right.To
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Resolution != right.Resolution {
			return left.Resolution < right.Resolution
		}
		if left.Invocation != right.Invocation {
			return left.Invocation < right.Invocation
		}
		return left.Certainty < right.Certainty
	})
}

func (g Graph) Validate() error {
	if g.Version != GraphVersion {
		return fmt.Errorf("evidence: unsupported graph version %d", g.Version)
	}
	known := make(map[string]struct{}, len(g.Entities))
	for _, entity := range g.Entities {
		if entity.ID == "" {
			return fmt.Errorf("evidence: entity id is required")
		}
		if !entity.Kind.Valid() {
			return fmt.Errorf("evidence: entity %q has invalid kind %q", entity.ID, entity.Kind)
		}
		if !entity.Scope.Valid() {
			return fmt.Errorf("evidence: entity %q has invalid source scope %q", entity.ID, entity.Scope)
		}
		if entity.Scope == SourceScopeRepository {
			if entity.Location == nil || !repositoryRelativePath(entity.Location.Path) || entity.Location.Line <= 0 {
				return fmt.Errorf("evidence: repository entity %q requires a repository-relative location", entity.ID)
			}
		}
		if _, exists := known[entity.ID]; exists {
			return fmt.Errorf("evidence: duplicate entity id %q", entity.ID)
		}
		known[entity.ID] = struct{}{}
	}
	knownScenarios := make(map[string]struct{}, len(g.Scenarios))
	for _, scenario := range g.Scenarios {
		if scenario.ID == "" {
			return fmt.Errorf("evidence: scenario id is required")
		}
		if _, exists := knownScenarios[scenario.ID]; exists {
			return fmt.Errorf("evidence: duplicate scenario id %q", scenario.ID)
		}
		knownScenarios[scenario.ID] = struct{}{}
	}
	for _, relation := range g.Relations {
		if _, exists := known[relation.From]; !exists {
			return fmt.Errorf("evidence: relation source %q is unknown", relation.From)
		}
		if _, exists := known[relation.To]; !exists {
			return fmt.Errorf("evidence: relation target %q is unknown", relation.To)
		}
		if !relation.Kind.Valid() {
			return fmt.Errorf("evidence: relation %q -> %q has invalid kind %q", relation.From, relation.To, relation.Kind)
		}
		if !relation.Resolution.Valid() {
			return fmt.Errorf("evidence: relation %q -> %q has invalid resolution %q", relation.From, relation.To, relation.Resolution)
		}
		if !relation.Invocation.Valid() {
			return fmt.Errorf("evidence: relation %q -> %q has invalid invocation %q", relation.From, relation.To, relation.Invocation)
		}
		if !relation.Certainty.Valid() {
			return fmt.Errorf("evidence: relation %q -> %q has invalid certainty %q", relation.From, relation.To, relation.Certainty)
		}
		if len(relation.Provenance) == 0 {
			return fmt.Errorf("evidence: relation %q -> %q has no provenance", relation.From, relation.To)
		}
		for _, provenance := range relation.Provenance {
			if provenance.Provider == "" || provenance.Operation == "" {
				return fmt.Errorf("evidence: relation %q -> %q has incomplete provenance", relation.From, relation.To)
			}
		}
		for _, scenarioID := range relation.Scenarios {
			if _, exists := knownScenarios[scenarioID]; !exists {
				return fmt.Errorf("evidence: relation %q -> %q references unknown scenario %q", relation.From, relation.To, scenarioID)
			}
		}
	}
	return nil
}

func repositoryRelativePath(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		dst = append(dst, value)
	}
	return dst
}

func appendUniqueProvenance(dst []Provenance, values ...Provenance) []Provenance {
	for _, value := range values {
		duplicate := false
		for _, existing := range dst {
			if existing.Provider == value.Provider &&
				existing.Version == value.Version &&
				existing.Operation == value.Operation &&
				existing.Detail == value.Detail &&
				sameLocation(existing.Location, value.Location) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, value)
		}
	}
	return dst
}

func sameLocation(left, right *Location) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
