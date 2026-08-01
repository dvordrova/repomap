// Package repositoryatlas defines the language-neutral canonical core of the
// Repository Atlas. Source analyzers project their typed local facts into this
// contract; they do not delegate identity, membership, or ownership to a model.
package repositoryatlas

import "github.com/dvordrova/repomap/internal/evidence"

const Version = 1

type UnitKind string

const (
	UnitRepository UnitKind = "repository"
	UnitModule     UnitKind = "module"
	UnitService    UnitKind = "service"
	UnitApp        UnitKind = "app"
	UnitPackage    UnitKind = "package"
)

func (kind UnitKind) Valid() bool {
	switch kind {
	case UnitRepository, UnitModule, UnitService, UnitApp, UnitPackage:
		return true
	default:
		return false
	}
}

type EntityKind string

const (
	EntitySurface   EntityKind = "surface"
	EntityOperation EntityKind = "operation"
	EntityBoundary  EntityKind = "boundary"
	EntityResource  EntityKind = "resource"
	EntityContract  EntityKind = "contract"
)

func (kind EntityKind) Valid() bool {
	switch kind {
	case EntitySurface, EntityOperation, EntityBoundary, EntityResource, EntityContract:
		return true
	default:
		return false
	}
}

type RelationKind string

const RelationExposes RelationKind = "exposes"

func (kind RelationKind) Valid() bool {
	return kind == RelationExposes
}

type Phase string

const (
	PhaseRuntime     Phase = "runtime"
	PhaseStartup     Phase = "startup"
	PhaseShutdown    Phase = "shutdown"
	PhaseScheduled   Phase = "scheduled"
	PhaseBuild       Phase = "build"
	PhaseGeneration  Phase = "generation"
	PhaseMigration   Phase = "migration"
	PhaseDeploy      Phase = "deploy"
	PhaseTest        Phase = "test"
	PhaseDevelopment Phase = "development"
)

func (phase Phase) Valid() bool {
	switch phase {
	case PhaseRuntime, PhaseStartup, PhaseShutdown, PhaseScheduled, PhaseBuild,
		PhaseGeneration, PhaseMigration, PhaseDeploy, PhaseTest, PhaseDevelopment:
		return true
	default:
		return false
	}
}

type Authority string

const (
	AuthorityObserved   Authority = "observed"
	AuthorityResolved   Authority = "resolved"
	AuthorityInferred   Authority = "inferred"
	AuthorityPartial    Authority = "partial"
	AuthorityConflicted Authority = "conflicted"
	AuthorityUnknown    Authority = "unknown"
)

func (authority Authority) Valid() bool {
	switch authority {
	case AuthorityObserved, AuthorityResolved, AuthorityInferred, AuthorityPartial,
		AuthorityConflicted, AuthorityUnknown:
		return true
	default:
		return false
	}
}

type Atlas struct {
	Version      int           `json:"version"`
	Units        []Unit        `json:"units"`
	Entities     []Entity      `json:"entities"`
	Observations []Observation `json:"observations"`
	Evidence     []Evidence    `json:"evidence"`
	Relations    []Relation    `json:"relations"`
}

type Unit struct {
	ID       string   `json:"id"`
	Kind     UnitKind `json:"kind"`
	ParentID string   `json:"parent_id,omitempty"`
	Name     string   `json:"name"`
}

type Entity struct {
	ID     string     `json:"id"`
	Kind   EntityKind `json:"kind"`
	UnitID string     `json:"unit_id"`
}

type EntityRef struct {
	Kind EntityKind `json:"kind"`
	ID   string     `json:"id"`
}

// Evidence keeps source files and symbols in the evidence substrate. They are
// locators for a typed fact and are never Atlas entities themselves.
type Evidence struct {
	ID         string              `json:"id"`
	UnitID     string              `json:"unit_id"`
	Location   evidence.Location   `json:"location"`
	Symbol     string              `json:"symbol,omitempty"`
	Provenance evidence.Provenance `json:"provenance"`
}

// Observation binds a typed entity to exact evidence within an explicit Unit
// scope. Its subject and evidence may live on the scope Unit or descendants.
type Observation struct {
	ID           string    `json:"id"`
	UnitID       string    `json:"unit_id"`
	Subject      EntityRef `json:"subject"`
	EvidenceRefs []string  `json:"evidence_refs"`
}

// Relation is scoped by UnitID. Endpoints and evidence may live on that Unit
// or descendants, allowing truthful module/repository integrations without
// permitting references outside the declared scope.
type Relation struct {
	ID           string       `json:"id"`
	UnitID       string       `json:"unit_id"`
	Kind         RelationKind `json:"kind"`
	Source       EntityRef    `json:"source"`
	Target       EntityRef    `json:"target"`
	Phase        Phase        `json:"phase"`
	Authority    Authority    `json:"authority"`
	EvidenceRefs []string     `json:"evidence_refs"`
}
