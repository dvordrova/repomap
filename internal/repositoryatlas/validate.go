package repositoryatlas

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func (atlas Atlas) Validate() error {
	if atlas.Version != Version {
		return fmt.Errorf("repository atlas: unsupported version %d", atlas.Version)
	}
	units, err := validateUnits(atlas.Units)
	if err != nil {
		return err
	}
	entities, err := validateEntities(atlas.Entities, units)
	if err != nil {
		return err
	}
	evidenceByID, err := validateEvidence(atlas.Evidence, units)
	if err != nil {
		return err
	}
	if err := validateObservations(atlas.Observations, units, entities, evidenceByID); err != nil {
		return err
	}
	return validateRelations(atlas.Relations, units, entities, evidenceByID)
}

func validateUnits(values []Unit) (map[string]Unit, error) {
	units := make(map[string]Unit, len(values))
	repositories := 0
	for _, unit := range values {
		if unit.ID == "" || unit.Name == "" {
			return nil, fmt.Errorf("repository atlas: unit id and name are required")
		}
		if !unit.Kind.Valid() {
			return nil, fmt.Errorf("repository atlas: unit %q has invalid kind %q", unit.ID, unit.Kind)
		}
		if _, exists := units[unit.ID]; exists {
			return nil, fmt.Errorf("repository atlas: duplicate unit id %q", unit.ID)
		}
		if unit.Kind == UnitRepository {
			repositories++
			if unit.ParentID != "" {
				return nil, fmt.Errorf("repository atlas: repository unit %q cannot have a parent", unit.ID)
			}
		} else if unit.ParentID == "" {
			return nil, fmt.Errorf("repository atlas: unit %q requires a parent", unit.ID)
		}
		units[unit.ID] = unit
	}
	if repositories != 1 {
		return nil, fmt.Errorf("repository atlas: exactly one repository unit is required")
	}
	for _, unit := range values {
		if unit.Kind == UnitRepository {
			continue
		}
		parent, exists := units[unit.ParentID]
		if !exists {
			return nil, fmt.Errorf("repository atlas: unit %q has unknown parent %q", unit.ID, unit.ParentID)
		}
		if !validUnitParent(unit.Kind, parent.Kind) {
			return nil, fmt.Errorf("repository atlas: %s unit %q cannot be parented by %s", unit.Kind, unit.ID, parent.Kind)
		}
		seen := map[string]struct{}{unit.ID: {}}
		current := unit
		for current.ParentID != "" {
			if _, duplicate := seen[current.ParentID]; duplicate {
				return nil, fmt.Errorf("repository atlas: unit %q has a parent cycle", unit.ID)
			}
			seen[current.ParentID] = struct{}{}
			current = units[current.ParentID]
		}
		if current.Kind != UnitRepository {
			return nil, fmt.Errorf("repository atlas: unit %q is outside the repository topology", unit.ID)
		}
	}
	return units, nil
}

func validUnitParent(child, parent UnitKind) bool {
	switch child {
	case UnitModule:
		return parent == UnitRepository
	case UnitService, UnitApp:
		return parent == UnitRepository || parent == UnitModule
	case UnitPackage:
		return parent == UnitModule
	default:
		return false
	}
}

func validateEntities(values []Entity, units map[string]Unit) (map[string]Entity, error) {
	entities := make(map[string]Entity, len(values))
	for _, entity := range values {
		if entity.ID == "" || entity.UnitID == "" {
			return nil, fmt.Errorf("repository atlas: entity id and unit are required")
		}
		if !entity.Kind.Valid() {
			return nil, fmt.Errorf("repository atlas: entity %q has invalid kind %q", entity.ID, entity.Kind)
		}
		if _, exists := units[entity.UnitID]; !exists {
			return nil, fmt.Errorf("repository atlas: entity %q has unknown unit %q", entity.ID, entity.UnitID)
		}
		if _, exists := entities[entity.ID]; exists {
			return nil, fmt.Errorf("repository atlas: duplicate entity id %q", entity.ID)
		}
		entities[entity.ID] = entity
	}
	return entities, nil
}

func validateEvidence(values []Evidence, units map[string]Unit) (map[string]Evidence, error) {
	known := make(map[string]Evidence, len(values))
	for _, item := range values {
		if item.ID == "" || item.UnitID == "" {
			return nil, fmt.Errorf("repository atlas: evidence id and unit are required")
		}
		if _, exists := units[item.UnitID]; !exists {
			return nil, fmt.Errorf("repository atlas: evidence %q has unknown unit %q", item.ID, item.UnitID)
		}
		if _, exists := known[item.ID]; exists {
			return nil, fmt.Errorf("repository atlas: duplicate evidence id %q", item.ID)
		}
		if !repositoryRelativePath(item.Location.Path) || item.Location.Line <= 0 || item.Location.Column < 0 ||
			item.Location.EndLine < 0 || item.Location.EndColumn < 0 {
			return nil, fmt.Errorf("repository atlas: evidence %q requires a repository-relative source location", item.ID)
		}
		if item.Location.EndLine > 0 && item.Location.EndLine < item.Location.Line {
			return nil, fmt.Errorf("repository atlas: evidence %q has an invalid source range", item.ID)
		}
		if item.Provenance.Provider == "" || item.Provenance.Operation == "" {
			return nil, fmt.Errorf("repository atlas: evidence %q has incomplete provenance", item.ID)
		}
		known[item.ID] = item
	}
	return known, nil
}

func validateObservations(
	values []Observation,
	units map[string]Unit,
	entities map[string]Entity,
	evidenceByID map[string]Evidence,
) error {
	known := make(map[string]struct{}, len(values))
	for _, observation := range values {
		if observation.ID == "" {
			return fmt.Errorf("repository atlas: observation id is required")
		}
		if _, exists := known[observation.ID]; exists {
			return fmt.Errorf("repository atlas: duplicate observation id %q", observation.ID)
		}
		known[observation.ID] = struct{}{}
		if _, exists := units[observation.UnitID]; !exists {
			return fmt.Errorf("repository atlas: observation %q has unknown scope %q", observation.ID, observation.UnitID)
		}
		subject, err := resolveEntityRef(observation.Subject, entities)
		if err != nil {
			return fmt.Errorf("repository atlas: observation %q: %w", observation.ID, err)
		}
		if !unitInScope(subject.UnitID, observation.UnitID, units) {
			return fmt.Errorf("repository atlas: observation %q subject is outside scope %q", observation.ID, observation.UnitID)
		}
		if err := validateEvidenceRefs(observation.EvidenceRefs, observation.UnitID, units, evidenceByID); err != nil {
			return fmt.Errorf("repository atlas: observation %q: %w", observation.ID, err)
		}
	}
	return nil
}

func validateRelations(
	values []Relation,
	units map[string]Unit,
	entities map[string]Entity,
	evidenceByID map[string]Evidence,
) error {
	known := make(map[string]struct{}, len(values))
	for _, relation := range values {
		if relation.ID == "" {
			return fmt.Errorf("repository atlas: relation id is required")
		}
		if _, exists := known[relation.ID]; exists {
			return fmt.Errorf("repository atlas: duplicate relation id %q", relation.ID)
		}
		known[relation.ID] = struct{}{}
		if _, exists := units[relation.UnitID]; !exists {
			return fmt.Errorf("repository atlas: relation %q has unknown scope %q", relation.ID, relation.UnitID)
		}
		if !relation.Kind.Valid() || !relation.Phase.Valid() || !relation.Authority.Valid() {
			return fmt.Errorf("repository atlas: relation %q has invalid kind, phase, or authority", relation.ID)
		}
		source, err := resolveEntityRef(relation.Source, entities)
		if err != nil {
			return fmt.Errorf("repository atlas: relation %q source: %w", relation.ID, err)
		}
		target, err := resolveEntityRef(relation.Target, entities)
		if err != nil {
			return fmt.Errorf("repository atlas: relation %q target: %w", relation.ID, err)
		}
		if relation.Kind == RelationExposes && (source.Kind != EntitySurface || target.Kind != EntityOperation) {
			return fmt.Errorf("repository atlas: relation %q exposes requires surface to operation", relation.ID)
		}
		if !unitInScope(source.UnitID, relation.UnitID, units) || !unitInScope(target.UnitID, relation.UnitID, units) {
			return fmt.Errorf("repository atlas: relation %q endpoint is outside scope %q", relation.ID, relation.UnitID)
		}
		if err := validateEvidenceRefs(relation.EvidenceRefs, relation.UnitID, units, evidenceByID); err != nil {
			return fmt.Errorf("repository atlas: relation %q: %w", relation.ID, err)
		}
	}
	return nil
}

func resolveEntityRef(ref EntityRef, entities map[string]Entity) (Entity, error) {
	if !ref.Kind.Valid() || ref.ID == "" {
		return Entity{}, fmt.Errorf("invalid entity reference")
	}
	entity, exists := entities[ref.ID]
	if !exists {
		return Entity{}, fmt.Errorf("unknown entity %q", ref.ID)
	}
	if entity.Kind != ref.Kind {
		return Entity{}, fmt.Errorf("entity %q has kind %q, not %q", ref.ID, entity.Kind, ref.Kind)
	}
	return entity, nil
}

func validateEvidenceRefs(
	refs []string,
	scope string,
	units map[string]Unit,
	evidenceByID map[string]Evidence,
) error {
	if len(refs) == 0 {
		return fmt.Errorf("at least one evidence reference is required")
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("duplicate evidence reference %q", ref)
		}
		seen[ref] = struct{}{}
		item, exists := evidenceByID[ref]
		if !exists {
			return fmt.Errorf("unknown evidence %q", ref)
		}
		if !unitInScope(item.UnitID, scope, units) {
			return fmt.Errorf("evidence %q is outside scope %q", ref, scope)
		}
	}
	return nil
}

func unitInScope(unitID, scopeID string, units map[string]Unit) bool {
	currentID := unitID
	for currentID != "" {
		if currentID == scopeID {
			return true
		}
		currentID = units[currentID].ParentID
	}
	return false
}

func repositoryRelativePath(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && clean == value && !strings.HasPrefix(clean, "../")
}
