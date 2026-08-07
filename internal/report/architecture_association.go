package report

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// ArchitectureAssociationVersion changes when the persisted association
// projection shape or its exact derivation rules change.
const ArchitectureAssociationVersion = 1

// ArchitectureAssociationProjection is the provider-free join of Atlas
// boundary/resource observations onto accepted component member scopes
// (Decision 225). It states only: "an observed boundary/resource callsite
// occurs in an exact member scope of this component". It never states
// runtime dependency, ownership, reachability, read/write/transaction
// semantics, endpoint identity, table/topic/bucket identity, or execution
// order. Canonical Atlas IDs are private: rows carry display-safe package
// paths and evidence locations only.
type ArchitectureAssociationProjection struct {
	Version    int                                 `json:"version"`
	Components []ArchitectureComponentAssociations `json:"components"`
	Total      int                                 `json:"total"` // observations in scope across all components
	Omissions  []ArchitectureAssociationOmission   `json:"omissions,omitempty"`
}

// ArchitectureComponentAssociations is one component's exact observation
// associations plus its structural neighbor rows (incoming/outgoing).
type ArchitectureComponentAssociations struct {
	ComponentID  componentmap.ComponentID          `json:"component_id"`
	Name         string                            `json:"name"`
	Incoming     []ArchitectureStructuralNeighbor  `json:"incoming,omitempty"`
	Outgoing     []ArchitectureStructuralNeighbor  `json:"outgoing,omitempty"`
	Associations []ArchitectureBoundaryResourceRow `json:"associations"`
	Omitted      int                               `json:"omitted"` // observations in scope but not associated (truthful count)
}

// ArchitectureStructuralNeighbor is one supported one-hop structural
// neighbor (incoming = depends-on-this, outgoing = this-depends-on).
type ArchitectureStructuralNeighbor struct {
	ComponentID componentmap.ComponentID `json:"component_id"`
	Name        string                   `json:"name"`
	Kind        string                   `json:"kind"` // incoming | outgoing
}

// ArchitectureBoundaryResourceRow is one association row: a boundary or
// resource observed in the component's exact member scope, with bounded
// witnesses and explicit limitations.
type ArchitectureBoundaryResourceRow struct {
	Kind           string `json:"kind"` // boundary | resource
	ImportedFamily string `json:"imported_family,omitempty"`
	// Family (Decision 236): the CLOSED generic touchpoint-family
	// classification of the imported path — database, broker/pub-sub,
	// filesystem, object-storage, cache-lock, config-secrets, process-OS,
	// HTTP-gRPC-SDK, other. Deterministic in the backend; never guessed
	// from component properties in JavaScript and never a per-repository
	// keyword table (DO-NOT-ADD).
	Family           string                           `json:"family,omitempty"`
	OwningUnit       string                           `json:"owning_unit"` // display-safe package path
	Witnesses        []ArchitectureAssociationWitness `json:"witnesses"`
	Paired           bool                             `json:"paired,omitempty"` // same callsite also observed as the other kind
	ObservationCount int                              `json:"observation_count"`
	// SourceRoles (Decision 225 / goal 05) reconcile every witness into a
	// closed production/test/tooling split, classified deterministically
	// from the exact callsite path — never from a model.
	SourceRoles ArchitectureSourceRoleCounts `json:"source_roles,omitempty"`
}

// ArchitectureSourceRoleCounts is the closed production/test/tooling
// reconciliation for one association row (goal 05 acceptance: counts must
// reconcile with observation_count).
type ArchitectureSourceRoleCounts struct {
	Production int `json:"production"`
	Test       int `json:"test"`
	Tooling    int `json:"tooling"`
}

// ArchitectureAssociationWitness is one exact callsite in the member scope.
type ArchitectureAssociationWitness struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol,omitempty"`
	Role   string `json:"role,omitempty"` // production | test | tooling
}

// classifySourceRole classifies a callsite path deterministically (goal 05
// source roles). Test paths: _test.go suffix and test/ trees. Tooling
// paths: contrib/examples/docs/tools/hack/scripts trees. Everything else is
// production. The classification is exact-path based, never a model claim.
func classifySourceRole(path string) string {
	if path == "" {
		return "production"
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.HasPrefix(lower, "test/") ||
		strings.HasPrefix(lower, "tests/") {
		return "test"
	}
	for _, segment := range []string{"/contrib/", "/examples/", "/example/", "/docs/", "/tools/", "/tool/", "/hack/", "/scripts/", "/integration_test"} {
		if strings.Contains(lower, segment) {
			return "tooling"
		}
	}
	for _, prefix := range []string{"contrib/", "examples/", "example/", "docs/", "tools/", "tool/", "hack/", "scripts/"} {
		if strings.HasPrefix(lower, prefix) {
			return "tooling"
		}
	}
	return "production"
}

// ArchitectureAssociationOmission lists observations that could not be
// associated (none when the join is complete) with the honest reason.
type ArchitectureAssociationOmission struct {
	Unit   string `json:"unit"`
	Reason string `json:"reason"`
}

// obsRow is one observation grouped by unit for association joining.
type obsRow struct {
	entityKind string
	unitPath   string
	evidence   []repositoryatlas.Evidence
}

// ProjectArchitectureAssociations joins Atlas boundary/resource observations
// onto canvas component member scopes deterministically (Decision 225).
// The join key is the canonical package path: a component member's fact
// value (package path) equals or prefixes (at a '/' boundary) the unit
// package path of an observation. No model call, no new stage.
func ProjectArchitectureAssociations(
	canvas *ArchitectureCanvas,
	atlas *repositoryatlas.Atlas,
) (*ArchitectureAssociationProjection, error) {
	if canvas == nil || atlas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf("architecture associations: unsupported canvas version %d", canvas.Version)
	}

	unitName := make(map[string]string, len(atlas.Units))
	for _, unit := range atlas.Units {
		unitName[unit.ID] = unit.Name
	}
	entityKind := make(map[string]string, len(atlas.Entities))
	for _, entity := range atlas.Entities {
		entityKind[entity.ID] = string(entity.Kind)
	}
	evidenceByID := make(map[string]repositoryatlas.Evidence, len(atlas.Evidence))
	for _, evidence := range atlas.Evidence {
		evidenceByID[evidence.ID] = evidence
	}

	// Component member scopes: exact package paths from member facts.
	type compScope struct {
		id    componentmap.ComponentID
		name  string
		paths []string
	}
	scopes := make([]compScope, 0, len(canvas.Components))
	for _, component := range canvas.Components {
		paths := make(map[string]struct{})
		for _, member := range component.Members {
			for _, fact := range member.Facts {
				if fact.Kind == componentmap.FactDeclaration && fact.Value != "" {
					paths[fact.Value] = struct{}{}
				}
			}
		}
		ordered := make([]string, 0, len(paths))
		for path := range paths {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		scopes = append(scopes, compScope{id: component.ID, name: component.Name, paths: ordered})
	}

	// Group observations per unit, then match to component scopes.
	byComponent := make(map[componentmap.ComponentID][]obsRow)
	matched := 0
	matchedUnits := make(map[string]struct{})
	for _, observation := range atlas.Observations {
		unitPath := unitName[observation.UnitID]
		if unitPath == "" {
			continue
		}
		kind := entityKind[observation.Subject.ID]
		if kind == "" {
			continue
		}
		evidence := make([]repositoryatlas.Evidence, 0, len(observation.EvidenceRefs))
		for _, ref := range observation.EvidenceRefs {
			if e, ok := evidenceByID[ref]; ok {
				evidence = append(evidence, e)
			}
		}
		row := obsRow{entityKind: kind, unitPath: unitPath, evidence: evidence}
		hit := false
		for _, scope := range scopes {
			if scopeContains(scope.paths, unitPath) {
				byComponent[scope.id] = append(byComponent[scope.id], row)
				hit = true
			}
		}
		if hit {
			matched++
			matchedUnits[unitPath] = struct{}{}
		}
	}

	projection := &ArchitectureAssociationProjection{
		Version: ArchitectureAssociationVersion,
		Total:   matched,
	}
	// Omissions: observations whose unit is not inside any component scope.
	for _, observation := range atlas.Observations {
		unitPath := unitName[observation.UnitID]
		if unitPath == "" {
			continue
		}
		if _, ok := matchedUnits[unitPath]; ok {
			continue
		}
		projection.Omissions = append(projection.Omissions, ArchitectureAssociationOmission{
			Unit:   unitPath,
			Reason: "unit is not a member of any accepted component scope",
		})
	}
	sort.Slice(projection.Omissions, func(i, j int) bool {
		return projection.Omissions[i].Unit < projection.Omissions[j].Unit
	})

	for _, scope := range scopes {
		rows := byComponent[scope.id]
		if len(rows) == 0 {
			continue
		}
		associations := aggregateAssociationRows(rows)
		entry := ArchitectureComponentAssociations{
			ComponentID:  scope.id,
			Name:         scope.name,
			Associations: associations,
			Omitted:      0,
		}
		// Decision 225: supported one-hop structural neighbors from the
		// canvas edges — incoming (this component is the target) and
		// outgoing (this component is the source) remain distinguishable.
		for _, edge := range canvas.StructuralEdges {
			if edge.ToComponentID == scope.id && edge.FromComponentID != "" {
				entry.Incoming = append(entry.Incoming, ArchitectureStructuralNeighbor{
					ComponentID: edge.FromComponentID,
					Kind:        "incoming",
				})
			}
			if edge.FromComponentID == scope.id && edge.ToComponentID != "" {
				entry.Outgoing = append(entry.Outgoing, ArchitectureStructuralNeighbor{
					ComponentID: edge.ToComponentID,
					Kind:        "outgoing",
				})
			}
		}
		sort.Slice(entry.Incoming, func(i, j int) bool {
			return string(entry.Incoming[i].ComponentID) < string(entry.Incoming[j].ComponentID)
		})
		sort.Slice(entry.Outgoing, func(i, j int) bool {
			return string(entry.Outgoing[i].ComponentID) < string(entry.Outgoing[j].ComponentID)
		})
		projection.Components = append(projection.Components, entry)
	}
	sort.Slice(projection.Components, func(i, j int) bool {
		return string(projection.Components[i].ComponentID) < string(projection.Components[j].ComponentID)
	})
	// Resolve structural neighbor names from the canvas components.
	nameByID := make(map[componentmap.ComponentID]string, len(canvas.Components))
	for _, component := range canvas.Components {
		nameByID[component.ID] = component.Name
	}
	for index := range projection.Components {
		for neighborIndex := range projection.Components[index].Incoming {
			projection.Components[index].Incoming[neighborIndex].Name = nameByID[projection.Components[index].Incoming[neighborIndex].ComponentID]
		}
		for neighborIndex := range projection.Components[index].Outgoing {
			projection.Components[index].Outgoing[neighborIndex].Name = nameByID[projection.Components[index].Outgoing[neighborIndex].ComponentID]
		}
	}
	return projection, nil
}

// scopeContains reports whether unitPath equals a scope path exactly.
// Decision 230 D9 (salvage contract v6): package paths match by exact
// package identity; prefix matching is not package ownership and a root
// package does not own its descendants.
func scopeContains(scopePaths []string, unitPath string) bool {
	// Decision 230 D9 (salvage contract v6): package paths match by EXACT
	// package identity. Prefix matching is not package ownership — a root
	// package scope must never absorb observations from its descendants
	// (casdoor «Запуск» inherited all 218 observations through the root
	// prefix). Descendant units reach their own exact component scopes.
	for _, path := range scopePaths {
		if unitPath == path {
			return true
		}
	}
	return false
}

// aggregateAssociationRows groups observation rows into per-component
// association rows by (kind, owning unit, imported family), pairing
// boundary and resource observations of the same callsite.
func aggregateAssociationRows(rows []obsRow) []ArchitectureBoundaryResourceRow {
	keyIndex := make(map[string]int)
	var out []ArchitectureBoundaryResourceRow
	for _, row := range rows {
		importedFamily := ""
		detailPath := ""
		for _, e := range row.evidence {
			if detail := e.Provenance.Detail; detail != "" {
				importedFamily = familyFromImportPath(detail)
				detailPath = detail
				break
			}
		}
		key := row.entityKind + "\x00" + row.unitPath + "\x00" + importedFamily
		idx, ok := keyIndex[key]
		if !ok {
			idx = len(out)
			keyIndex[key] = idx
			out = append(out, ArchitectureBoundaryResourceRow{
				Kind:           row.entityKind,
				ImportedFamily: importedFamily,
				// Decision 236: closed generic touchpoint family,
				// backend-owned (never guessed in JavaScript).
				Family:     touchpointFamilyFromImportPath(detailPath),
				OwningUnit: row.unitPath,
			})
		}
		for _, e := range row.evidence {
			role := classifySourceRole(e.Location.Path)
			out[idx].Witnesses = append(out[idx].Witnesses, ArchitectureAssociationWitness{
				Path: e.Location.Path, Line: e.Location.Line, Symbol: e.Symbol, Role: role,
			})
			switch role {
			case "test":
				out[idx].SourceRoles.Test++
			case "tooling":
				out[idx].SourceRoles.Tooling++
			default:
				out[idx].SourceRoles.Production++
			}
		}
		out[idx].ObservationCount++
	}
	for index := range out {
		sort.Slice(out[index].Witnesses, func(i, j int) bool {
			if out[index].Witnesses[i].Path != out[index].Witnesses[j].Path {
				return out[index].Witnesses[i].Path < out[index].Witnesses[j].Path
			}
			if out[index].Witnesses[i].Line != out[index].Witnesses[j].Line {
				return out[index].Witnesses[i].Line < out[index].Witnesses[j].Line
			}
			return out[index].Witnesses[i].Symbol < out[index].Witnesses[j].Symbol
		})
	}
	// Decision 229 D2: Boundary and Resource observations sharing the same
	// exact witness are one user-facing "Observed external/state
	// interaction" row (charter D6: two ontology roles over one fact).
	// Canonical records stay distinct; the projection marks the pair so the
	// UI can coalesce them and keep every witness.
	for i := range out {
		if out[i].Kind != "boundary" {
			continue
		}
		for j := range out {
			if out[j].Kind != "resource" || out[j].OwningUnit != out[i].OwningUnit ||
				out[j].ImportedFamily != out[i].ImportedFamily {
				continue
			}
			if associationWitnessSetsOverlap(out[i].Witnesses, out[j].Witnesses) {
				out[i].Paired = true
				out[j].Paired = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].OwningUnit != out[j].OwningUnit {
			return out[i].OwningUnit < out[j].OwningUnit
		}
		return out[i].ImportedFamily < out[j].ImportedFamily
	})
	return out
}

// associationWitnessSetsOverlap reports whether two association rows share
// at least one exact witness (path + line). Used for Decision 229 D2
// boundary/resource coalescing.
func associationWitnessSetsOverlap(left, right []ArchitectureAssociationWitness) bool {
	seen := make(map[string]struct{}, len(left))
	for _, witness := range left {
		seen[witness.Path+"\x00"+strconv.Itoa(witness.Line)] = struct{}{}
	}
	for _, witness := range right {
		if _, exists := seen[witness.Path+"\x00"+strconv.Itoa(witness.Line)]; exists {
			return true
		}
	}
	return false
}

// ensureMechanismFragment derives the Decision 226 vertical fragment when
// the report carries an accepted canvas. Provider-free; absent when no
// process entry with call_target proof exists.
func ensureMechanismFragment(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("mechanism fragment: report is required")
	}
	// Historical/manual render fixtures with unsupported canvas versions stay
	// displayable without a fragment; authorized manifest verification
	// independently rejects unsupported Canvas versions.
	if data.MechanismFragment == nil && data.ArchitectureCanvas != nil &&
		data.ArchitectureCanvas.Version != ArchitectureCanvasVersion {
		return nil
	}
	if data.MechanismFragment != nil {
		return ValidateMechanismFragment(
			data.ArchitectureCanvas,
			data.ArchitectureAssociations,
			data.RepositoryAtlas,
			data.MechanismFragment,
		)
	}
	fragment, err := ProjectMechanismFragment(
		data.ArchitectureCanvas,
		data.ArchitectureAssociations,
		data.RepositoryAtlas,
	)
	if err != nil {
		return err
	}
	data.MechanismFragment = fragment
	return nil
}

// ValidateMechanismFragment re-derives the exact fragment and rejects drift.
func ValidateMechanismFragment(
	canvas *ArchitectureCanvas,
	associations *ArchitectureAssociationProjection,
	atlas *repositoryatlas.Atlas,
	fragment *MechanismFragmentProjection,
) error {
	expected, err := ProjectMechanismFragment(canvas, associations, atlas)
	if err != nil {
		return err
	}
	if expected == nil {
		if fragment == nil {
			return nil
		}
		return fmt.Errorf("mechanism fragment: projection has no process entry")
	}
	if fragment == nil {
		return fmt.Errorf("mechanism fragment: projection is missing")
	}
	if fragment.Version != MechanismFragmentVersion {
		return fmt.Errorf("mechanism fragment: unsupported version %d", fragment.Version)
	}
	if !reflect.DeepEqual(fragment, expected) {
		return fmt.Errorf("mechanism fragment: persisted projection does not match exact local evidence")
	}
	return nil
}

// familyFromImportPath extracts the broad imported family from a canonical
// import path: stdlib first segment (net, os, database) or module root for
// external modules (github.com/org/...).
func familyFromImportPath(importPath string) string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return ""
	}
	first := strings.Split(importPath, "/")[0]
	if strings.Contains(first, ".") {
		parts := strings.Split(importPath, "/")
		if len(parts) >= 3 {
			return strings.Join(parts[:3], "/")
		}
		return importPath
	}
	return first
}

// touchpointFamilyFromImportPath classifies an imported canonical path into
// the CLOSED generic touchpoint-family taxonomy (Decision 236). The
// classification is deterministic, backend-owned and generic — the same
// rules apply to every repository, so it is never a per-repository keyword
// table. The first stdlib segment and a bounded set of well-known module
// prefixes map to a family; everything else is "other" (the raw
// imported_family stays available for evidence details).
func touchpointFamilyFromImportPath(importPath string) string {
	importPath = strings.ToLower(strings.TrimSpace(importPath))
	if importPath == "" {
		return ""
	}
	// Standard library first segments.
	switch {
	case strings.HasPrefix(importPath, "database/") || strings.HasPrefix(importPath, "database.") ||
		strings.HasPrefix(importPath, "github.com/jmoiron/sqlx") ||
		strings.HasPrefix(importPath, "github.com/jackc/") ||
		strings.HasPrefix(importPath, "github.com/go-sql-driver/") ||
		strings.HasPrefix(importPath, "github.com/lib/pq") ||
		strings.HasPrefix(importPath, "github.com/mattn/go-sqlite3") ||
		strings.HasPrefix(importPath, "github.com/mongodb/") ||
		strings.HasPrefix(importPath, "gorm.io/") ||
		strings.HasPrefix(importPath, "github.com/uptrace/bun") ||
		strings.HasPrefix(importPath, "github.com/go-gorm/"):
		return "database"
	case strings.HasPrefix(importPath, "net/http") || strings.HasPrefix(importPath, "google.golang.org/grpc") ||
		strings.HasPrefix(importPath, "github.com/gin-gonic/") ||
		strings.HasPrefix(importPath, "github.com/labstack/echo") ||
		strings.HasPrefix(importPath, "github.com/go-chi/") ||
		strings.HasPrefix(importPath, "github.com/gorilla/") ||
		strings.HasPrefix(importPath, "github.com/valyala/fasthttp") ||
		strings.HasPrefix(importPath, "golang.org/x/net") ||
		strings.HasPrefix(importPath, "github.com/go-resty/") ||
		strings.HasPrefix(importPath, "github.com/google/go-github") ||
		strings.HasPrefix(importPath, "github.com/bufbuild/") ||
		strings.HasPrefix(importPath, "google.golang.org/api"):
		return "HTTP-gRPC-SDK"
	case strings.HasPrefix(importPath, "kafka") || strings.HasPrefix(importPath, "github.com/segmentio/kafka-go") ||
		strings.HasPrefix(importPath, "github.com/Shopify/sarama") ||
		strings.HasPrefix(importPath, "github.com/rabbitmq/") ||
		strings.HasPrefix(importPath, "github.com/streadway/amqp") ||
		strings.HasPrefix(importPath, "github.com/nats-io/") ||
		strings.HasPrefix(importPath, "github.com/eclipse/paho") ||
		strings.HasPrefix(importPath, "google.golang.org/grpc") ||
		strings.HasPrefix(importPath, "github.com/apache/pulsar") ||
		strings.HasPrefix(importPath, "github.com/IBM/sarama") ||
		strings.HasPrefix(importPath, "github.com/nsqio/"):
		return "broker/pub-sub"
	case strings.HasPrefix(importPath, "os/exec") || strings.HasPrefix(importPath, "syscall") ||
		strings.HasPrefix(importPath, "github.com/mitchellh/go-ps") ||
		strings.HasPrefix(importPath, "github.com/shirou/gopsutil"):
		return "process-OS"
	case strings.HasPrefix(importPath, "os") || strings.HasPrefix(importPath, "io") ||
		strings.HasPrefix(importPath, "path/filepath") ||
		strings.HasPrefix(importPath, "github.com/spf13/afero") ||
		strings.HasPrefix(importPath, "github.com/fsnotify/") ||
		strings.HasPrefix(importPath, "github.com/bmatcuk/doublestar"):
		return "filesystem"
	case strings.HasPrefix(importPath, "github.com/aws/aws-sdk-go") ||
		strings.HasPrefix(importPath, "cloud.google.com/go/storage") ||
		strings.HasPrefix(importPath, "github.com/minio/") ||
		strings.HasPrefix(importPath, "github.com/azure/azure-sdk-for-go") ||
		strings.HasPrefix(importPath, "github.com/googleapis/gcs-fuse-go"):
		return "object-storage"
	case strings.HasPrefix(importPath, "github.com/redis/") ||
		strings.HasPrefix(importPath, "github.com/go-redis/") ||
		strings.HasPrefix(importPath, "github.com/bradfitz/gomemcache") ||
		strings.HasPrefix(importPath, "github.com/allegro/bigcache") ||
		strings.HasPrefix(importPath, "github.com/patrickmn/go-cache") ||
		strings.HasPrefix(importPath, "github.com/golang/groupcache") ||
		strings.HasPrefix(importPath, "github.com/hashicorp/golang-lru") ||
		strings.HasPrefix(importPath, "github.com/coocood/freecache"):
		return "cache-lock"
	case strings.HasPrefix(importPath, "github.com/spf13/viper") ||
		strings.HasPrefix(importPath, "github.com/kelseyhightower/envconfig") ||
		strings.HasPrefix(importPath, "github.com/caarlos0/env") ||
		strings.HasPrefix(importPath, "github.com/joho/godotenv") ||
		strings.HasPrefix(importPath, "github.com/hashicorp/vault") ||
		strings.HasPrefix(importPath, "gopkg.in/yaml") ||
		strings.HasPrefix(importPath, "github.com/knadh/koanf") ||
		strings.HasPrefix(importPath, "github.com/mitchellh/mapstructure"):
		return "config-secrets"
	}
	return "other"
}

// ensureArchitectureAssociations derives the Decision 225 association
// projection into the report. Historical/manual render fixtures without the
// projection stay displayable; the authorized manifest verification
// independently rejects unsupported Canvas versions.
func ensureArchitectureAssociations(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("architecture associations: report is required")
	}
	if data.ArchitectureAssociations != nil {
		return ValidateArchitectureAssociations(
			data.ArchitectureCanvas,
			data.RepositoryAtlas,
			data.ArchitectureAssociations,
		)
	}
	projection, err := ProjectArchitectureAssociations(
		data.ArchitectureCanvas,
		data.RepositoryAtlas,
	)
	if err != nil {
		return err
	}
	data.ArchitectureAssociations = projection
	return nil
}

// ValidateArchitectureAssociations re-derives the exact association
// projection from the same canvas + atlas and rejects any drift (the
// manifest DeepEqual round-trip depends on it).
func ValidateArchitectureAssociations(
	canvas *ArchitectureCanvas,
	atlas *repositoryatlas.Atlas,
	projection *ArchitectureAssociationProjection,
) error {
	if canvas == nil || atlas == nil {
		if projection != nil {
			return fmt.Errorf("architecture associations: projection has no canvas/atlas")
		}
		return nil
	}
	expected, err := ProjectArchitectureAssociations(canvas, atlas)
	if err != nil {
		return err
	}
	if projection == nil {
		if expected == nil || len(expected.Components) == 0 {
			return nil
		}
		return fmt.Errorf("architecture associations: projection is missing")
	}
	if projection.Version != ArchitectureAssociationVersion {
		return fmt.Errorf(
			"architecture associations: unsupported version %d",
			projection.Version,
		)
	}
	if !reflect.DeepEqual(projection, expected) {
		return fmt.Errorf("architecture associations: persisted projection does not match exact canvas+atlas join")
	}
	return nil
}
