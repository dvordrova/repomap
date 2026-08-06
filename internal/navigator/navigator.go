// Package navigator compiles a request-scoped, provider-neutral projection of
// a canonical Repository Atlas. Canonical identities and source locators stay
// in a private catalog; the wire contains only short request-local refs.
package navigator

import (
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

const (
	// Decision 232 (Archive 9): Navigator v2 — the model returns only
	// version, catalog_ref and exactly one action_ref; the backend
	// restores the trail, both endpoint entities, all evidence refs, the
	// operation and the canonical action record from its own catalog.
	Version = 2
)

// Limits are selected by the caller. Compile never substitutes defaults,
// trims a section, or expands a search to make an input fit.
type Limits struct {
	MaxWireBytes      int `json:"max_wire_bytes"`
	MaxResponseBytes  int `json:"max_response_bytes"`
	MaxUnitLabelBytes int `json:"max_unit_label_bytes"`
	MaxSeeds          int `json:"max_seeds"`
	MaxDirectTrails   int `json:"max_direct_trails"`
	MaxIntersections  int `json:"max_intersections"`
	MaxEvidence       int `json:"max_evidence"`
	MaxGaps           int `json:"max_gaps"`
	MaxActions        int `json:"max_actions"`
}

type ProvenGap struct {
	Key         string
	Meaning     string
	Subject     repositoryatlas.EntityRef
	EvidenceIDs []string
}

// Action is one backend-owned operation the caller has explicitly made
// available for this request. Navigator never invents an action.
type Action struct {
	Key       string
	Operation string
	Target    repositoryatlas.EntityRef
}

type Input struct {
	Atlas       repositoryatlas.Atlas
	Question    string
	ScopeUnitID string
	Seeds       []repositoryatlas.EntityRef
	Gaps        []ProvenGap
	Actions     []Action
	Limits      Limits
}

// Compiled keeps its canonical catalog private. Callers can send WireJSON and
// later validate a response through this exact value.
type Compiled struct {
	wire             []byte
	wireSHA256       string
	catalogSHA256    string
	catalogRef       string
	maxWireBytes     int
	maxResponseBytes int
	catalog          privateCatalog
	actions          map[string]ResolvedAction
}

func (compiled Compiled) WireJSON() []byte {
	return append([]byte(nil), compiled.wire...)
}

func (compiled Compiled) WireSHA256() string    { return compiled.wireSHA256 }
func (compiled Compiled) CatalogSHA256() string { return compiled.catalogSHA256 }
func (compiled Compiled) CatalogRef() string    { return compiled.catalogRef }
func (compiled Compiled) MaxWireBytes() int     { return compiled.maxWireBytes }

type ResourceLimitError struct {
	Section string
	Limit   int
	Actual  int
}

func (err *ResourceLimitError) Error() string {
	if err == nil {
		return "navigator: resource limit exceeded"
	}
	return fmt.Sprintf(
		"navigator: %s requires %d item(s)/byte(s); explicit limit is %d",
		err.Section,
		err.Actual,
		err.Limit,
	)
}

type ReferenceError struct {
	Field    string
	Position int
	Code     string
}

func (err *ReferenceError) Error() string {
	if err == nil {
		return "navigator response: invalid reference"
	}
	return fmt.Sprintf("navigator response: %s[%d]: %s", err.Field, err.Position, err.Code)
}

type ResolvedResponse struct {
	Entities              []repositoryatlas.EntityRef
	RelationIDs           []string
	IntersectionEntityIDs []string
	EvidenceIDs           []string
	GapKeys               []string
	ActionKeys            []string
	Actions               []ResolvedAction
}

// ResolvedAction restores only the backend-owned meaning and canonical target
// that were advertised in this exact compiled request. Provider output can
// select the request-local action ref, but cannot author either value.
type ResolvedAction struct {
	Key       string
	Operation string
	Target    repositoryatlas.EntityRef
}

// EntityRole gives a request-local entity ref bounded backend-owned meaning.
// The two specific startup roles are emitted only from an exact resolved
// Surface -exposes/startup-> Operation relation already present in Atlas.
type EntityRole string

const (
	EntityRoleGenericSurface   EntityRole = "generic_surface"
	EntityRoleGenericOperation EntityRole = "generic_operation"
	EntityRoleGenericBoundary  EntityRole = "generic_boundary"
	EntityRoleGenericResource  EntityRole = "generic_resource"
	EntityRoleGenericContract  EntityRole = "generic_contract"
	EntityRoleProcessEntry     EntityRole = "process_entry"
	EntityRoleApplicationStart EntityRole = "application_start"
)

type wireProjection struct {
	Version       int                `json:"version"`
	CatalogRef    string             `json:"catalog_ref"`
	Question      string             `json:"question"`
	ScopeRef      string             `json:"scope_ref"`
	Units         []wireUnit         `json:"units"`
	Entities      []wireEntity       `json:"entities"`
	SeedRefs      []string           `json:"seed_refs"`
	DirectTrails  []wireTrail        `json:"direct_trails"`
	Intersections []wireIntersection `json:"intersections"`
	Evidence      []wireEvidence     `json:"evidence"`
	Gaps          []wireGap          `json:"gaps"`
	Actions       []wireAction       `json:"actions"`
}

type wireUnit struct {
	Ref       string                   `json:"ref"`
	Kind      repositoryatlas.UnitKind `json:"kind"`
	Label     string                   `json:"label"`
	ParentRef string                   `json:"parent_ref,omitempty"`
}

type wireEntity struct {
	Ref     string                     `json:"ref"`
	Kind    repositoryatlas.EntityKind `json:"kind"`
	Role    EntityRole                 `json:"role"`
	UnitRef string                     `json:"unit_ref"`
}

type wireTrail struct {
	Ref          string                       `json:"ref"`
	SourceRef    string                       `json:"source_ref"`
	TargetRef    string                       `json:"target_ref"`
	Kind         repositoryatlas.RelationKind `json:"kind"`
	Phase        repositoryatlas.Phase        `json:"phase"`
	Authority    repositoryatlas.Authority    `json:"authority"`
	EvidenceRefs []string                     `json:"evidence_refs"`
}

type wireIntersection struct {
	Ref       string   `json:"ref"`
	EntityRef string   `json:"entity_ref"`
	SeedRefs  []string `json:"seed_refs"`
	TrailRefs []string `json:"trail_refs"`
}

type wireEvidence struct {
	Ref          string   `json:"ref"`
	SubjectRefs  []string `json:"subject_refs"`
	ExactLocator bool     `json:"exact_locator"`
}

type wireGap struct {
	Ref          string   `json:"ref"`
	Meaning      string   `json:"meaning"`
	SubjectRef   string   `json:"subject_ref"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type wireAction struct {
	Ref       string `json:"ref"`
	Operation string `json:"operation"`
	TargetRef string `json:"target_ref"`
}

type catalogKind string

const (
	catalogUnit         catalogKind = "unit"
	catalogEntity       catalogKind = "entity"
	catalogTrail        catalogKind = "trail"
	catalogIntersection catalogKind = "intersection"
	catalogEvidence     catalogKind = "evidence"
	catalogGap          catalogKind = "gap"
	catalogAction       catalogKind = "action"
)

type catalogEntry struct {
	Ref         string                     `json:"ref"`
	Kind        catalogKind                `json:"kind"`
	CanonicalID string                     `json:"canonical_id"`
	EntityKind  repositoryatlas.EntityKind `json:"entity_kind,omitempty"`
	EntityRole  EntityRole                 `json:"entity_role,omitempty"`
}

type catalogMaterial struct {
	Version          int            `json:"version"`
	ProjectionSHA256 string         `json:"projection_sha256"`
	ScopeUnitID      string         `json:"scope_unit_id"`
	Limits           Limits         `json:"limits"`
	Entries          []catalogEntry `json:"entries"`
}

type privateCatalog struct {
	entries          map[string]catalogEntry
	byCanonical      map[string][]catalogEntry
	outsideCanonical map[string]struct{}
}

type responseEnvelope struct {
	Version          int      `json:"version"`
	CatalogRef       string   `json:"catalog_ref"`
	EntityRefs       []string `json:"entity_refs"`
	TrailRefs        []string `json:"trail_refs"`
	IntersectionRefs []string `json:"intersection_refs"`
	EvidenceRefs     []string `json:"evidence_refs"`
	GapRefs          []string `json:"gap_refs"`
	ActionRefs       []string `json:"action_refs"`
}

func marshalCanonical(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}
