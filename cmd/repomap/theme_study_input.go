package main

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// themeStudyAtlasClosure records the exact deterministic reduction applied to
// the private Atlas scaffold before the Theme pipeline compiles its task-shaped
// span product. The authoritative saved Atlas is never changed.
type themeStudyAtlasClosure struct {
	ObservedUnits        int
	RetainedUnits        int
	ObservedEntities     int
	RetainedEntities     int
	ObservedEvidence     int
	RetainedEvidence     int
	ObservedObservations int
	RetainedObservations int
	ObservedRelations    int
	RetainedRelations    int
}

// shapeThemeStudyCompileInput keeps only the exact Atlas dependency closure
// needed by the Theme pipeline's already-derived surfaces and evidence facts.
// Atlas Study is an internal span compiler for Theme Scout; Scout consumes its
// Architecture labels and route-span questions, not the complete repository
// unit catalog. Keeping all workspace packages here made large, valid
// repositories fail the local MaxUnits guard before a Scout request existed.
//
// The closure is identity-based and has no first-N behavior: required surface
// entities and exact evidence facts seed it; related observations/relations
// join only with all of their typed dependencies; every retained unit brings
// its complete ancestor chain. The original Atlas remains authoritative and
// unchanged in ReportData and on disk.
func shapeThemeStudyCompileInput(
	input atlasstudy.Input,
) (atlasstudy.Input, themeStudyAtlasClosure, error) {
	canonical, err := repositoryatlas.Canonical(input.Atlas)
	if err != nil {
		return atlasstudy.Input{}, themeStudyAtlasClosure{}, fmt.Errorf(
			"theme study input closure: repository Atlas: %w", err,
		)
	}
	stats := themeStudyAtlasClosure{
		ObservedUnits: len(canonical.Units), ObservedEntities: len(canonical.Entities),
		ObservedEvidence: len(canonical.Evidence), ObservedObservations: len(canonical.Observations),
		ObservedRelations: len(canonical.Relations),
	}

	units := make(map[string]repositoryatlas.Unit, len(canonical.Units))
	for _, unit := range canonical.Units {
		units[unit.ID] = unit
	}
	entities := make(map[string]repositoryatlas.Entity, len(canonical.Entities))
	for _, entity := range canonical.Entities {
		entities[entity.ID] = entity
	}
	evidenceByID := make(map[string]repositoryatlas.Evidence, len(canonical.Evidence))
	for _, item := range canonical.Evidence {
		evidenceByID[item.ID] = item
	}

	retainedEntities := make(map[string]struct{})
	retainedEvidence := make(map[string]struct{})
	retainedUnits := make(map[string]struct{})
	for _, surface := range input.Surfaces {
		entity, ok := entities[surface.ID]
		if !ok || entity.Kind != repositoryatlas.EntitySurface || entity.UnitID != surface.UnitID {
			return atlasstudy.Input{}, stats, fmt.Errorf(
				"theme study input closure: surface %q does not match the exact Atlas", surface.ID,
			)
		}
		retainedEntities[entity.ID] = struct{}{}
		retainedUnits[entity.UnitID] = struct{}{}
	}
	for _, fact := range input.Evidence {
		item, ok := evidenceByID[fact.ID]
		if !ok {
			return atlasstudy.Input{}, stats, fmt.Errorf(
				"theme study input closure: evidence %q does not match the exact Atlas", fact.ID,
			)
		}
		retainedEvidence[item.ID] = struct{}{}
		retainedUnits[item.UnitID] = struct{}{}
	}

	// Preserve the complete typed neighborhood of every required entity or
	// evidence item. Fixed-point expansion is deterministic because membership,
	// not traversal order, determines the final closed set.
	changed := true
	for changed {
		changed = false
		addEntity := func(id string) {
			if _, exists := retainedEntities[id]; !exists {
				retainedEntities[id] = struct{}{}
				changed = true
			}
		}
		addEvidence := func(id string) {
			if _, exists := retainedEvidence[id]; !exists {
				retainedEvidence[id] = struct{}{}
				changed = true
			}
		}
		addUnit := func(id string) {
			if _, exists := retainedUnits[id]; !exists {
				retainedUnits[id] = struct{}{}
				changed = true
			}
		}
		for _, observation := range canonical.Observations {
			relevant := false
			if _, ok := retainedEntities[observation.Subject.ID]; ok {
				relevant = true
			}
			for _, evidenceID := range observation.EvidenceRefs {
				if _, ok := retainedEvidence[evidenceID]; ok {
					relevant = true
					break
				}
			}
			if !relevant {
				continue
			}
			addEntity(observation.Subject.ID)
			addUnit(observation.UnitID)
			for _, evidenceID := range observation.EvidenceRefs {
				addEvidence(evidenceID)
			}
		}
		for _, relation := range canonical.Relations {
			relevant := false
			if _, ok := retainedEntities[relation.Source.ID]; ok {
				relevant = true
			}
			if _, ok := retainedEntities[relation.Target.ID]; ok {
				relevant = true
			}
			for _, evidenceID := range relation.EvidenceRefs {
				if _, ok := retainedEvidence[evidenceID]; ok {
					relevant = true
					break
				}
			}
			if !relevant {
				continue
			}
			addEntity(relation.Source.ID)
			addEntity(relation.Target.ID)
			addUnit(relation.UnitID)
			for _, evidenceID := range relation.EvidenceRefs {
				addEvidence(evidenceID)
			}
		}
		for entityID := range retainedEntities {
			entity, ok := entities[entityID]
			if !ok {
				return atlasstudy.Input{}, stats, fmt.Errorf(
					"theme study input closure: relation references unknown entity %q", entityID,
				)
			}
			addUnit(entity.UnitID)
		}
		for evidenceID := range retainedEvidence {
			item, ok := evidenceByID[evidenceID]
			if !ok {
				return atlasstudy.Input{}, stats, fmt.Errorf(
					"theme study input closure: relation references unknown evidence %q", evidenceID,
				)
			}
			addUnit(item.UnitID)
		}
		for unitID := range retainedUnits {
			unit, ok := units[unitID]
			if !ok {
				return atlasstudy.Input{}, stats, fmt.Errorf(
					"theme study input closure: dependency references unknown unit %q", unitID,
				)
			}
			if unit.ParentID != "" {
				addUnit(unit.ParentID)
			}
		}
	}
	// A valid Atlas always owns exactly one repository root. Keep it even when
	// the semantic shelf has no surface/evidence principal of its own.
	for _, unit := range canonical.Units {
		if unit.Kind == repositoryatlas.UnitRepository {
			retainedUnits[unit.ID] = struct{}{}
		}
	}

	shaped := repositoryatlas.Atlas{Version: canonical.Version}
	for _, unit := range canonical.Units {
		if _, ok := retainedUnits[unit.ID]; ok {
			shaped.Units = append(shaped.Units, unit)
		}
	}
	for _, entity := range canonical.Entities {
		if _, ok := retainedEntities[entity.ID]; ok {
			shaped.Entities = append(shaped.Entities, entity)
		}
	}
	for _, item := range canonical.Evidence {
		if _, ok := retainedEvidence[item.ID]; ok {
			shaped.Evidence = append(shaped.Evidence, item)
		}
	}
	allEvidenceRetained := func(refs []string) bool {
		for _, id := range refs {
			if _, ok := retainedEvidence[id]; !ok {
				return false
			}
		}
		return true
	}
	for _, observation := range canonical.Observations {
		_, subject := retainedEntities[observation.Subject.ID]
		_, scope := retainedUnits[observation.UnitID]
		if subject && scope && allEvidenceRetained(observation.EvidenceRefs) {
			shaped.Observations = append(shaped.Observations, observation)
		}
	}
	for _, relation := range canonical.Relations {
		_, source := retainedEntities[relation.Source.ID]
		_, target := retainedEntities[relation.Target.ID]
		_, scope := retainedUnits[relation.UnitID]
		if source && target && scope && allEvidenceRetained(relation.EvidenceRefs) {
			shaped.Relations = append(shaped.Relations, relation)
		}
	}
	shaped, err = repositoryatlas.Canonical(shaped)
	if err != nil {
		return atlasstudy.Input{}, stats, fmt.Errorf(
			"theme study input closure: reduced Atlas: %w", err,
		)
	}
	stats.RetainedUnits = len(shaped.Units)
	stats.RetainedEntities = len(shaped.Entities)
	stats.RetainedEvidence = len(shaped.Evidence)
	stats.RetainedObservations = len(shaped.Observations)
	stats.RetainedRelations = len(shaped.Relations)
	if input.Limits.MaxUnits > 0 && len(shaped.Units) > input.Limits.MaxUnits {
		return atlasstudy.Input{}, stats, &atlasstudy.ResourceLimitError{
			Section: "units", Limit: input.Limits.MaxUnits, Actual: len(shaped.Units),
		}
	}
	input.Atlas = shaped
	return input, stats, nil
}
