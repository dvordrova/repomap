package report

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/repositoryatlas/goadapter"
)

func readRepositoryAtlasArtifact(runDir string) (*repositoryatlas.Atlas, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("repository atlas: open run directory: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(repositoryatlas.ArtifactFilename); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("repository atlas: inspect artifact: %w", err)
	}
	encoded, err := readManifestFile(
		root,
		repositoryatlas.ArtifactFilename,
		repositoryatlas.MaxArtifactBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("repository atlas: read artifact: %w", err)
	}
	atlas, err := repositoryatlas.DecodeCanonicalJSON(encoded)
	if err != nil {
		return nil, err
	}
	return &atlas, nil
}

func validateRepositoryAtlasForReport(data *ReportData) error {
	if data == nil || data.RepositoryAtlas == nil {
		return nil
	}
	atlas := data.RepositoryAtlas
	if err := atlas.Validate(); err != nil {
		return err
	}
	repositoryUnits := 0
	for _, unit := range atlas.Units {
		if unit.Kind != repositoryatlas.UnitRepository {
			continue
		}
		repositoryUnits++
		// A mandatory persisted-artifact scan may replace a secret-bearing
		// snapshot with a closed marker, leaving the presentation name empty.
		// Keep that absence truthful; compare only when an independent report
		// repository name survived artifact preparation.
		if data.RepoName != "" && unit.Name != data.RepoName {
			return fmt.Errorf(
				"repository atlas: repository unit %q does not match report repository %q",
				unit.Name,
				data.RepoName,
			)
		}
	}
	if repositoryUnits != 1 {
		return fmt.Errorf("repository atlas: report requires exactly one repository unit")
	}

	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		openable[path] = struct{}{}
	}
	evidenceByID := make(map[string]repositoryatlas.Evidence, len(atlas.Evidence))
	for _, item := range atlas.Evidence {
		if _, ok := openable[item.Location.Path]; !ok {
			return fmt.Errorf(
				"repository atlas: evidence %q path is not report-openable",
				item.ID,
			)
		}
		evidenceByID[item.ID] = item
	}

	triggers := make(map[string]DiscoveredTrigger)
	if data.DiscoveredSurfaces != nil {
		triggers = make(map[string]DiscoveredTrigger, len(data.DiscoveredSurfaces.Triggers))
		for _, trigger := range data.DiscoveredSurfaces.Triggers {
			triggers[trigger.ID] = trigger
		}
	}
	observations := make(map[string][]repositoryatlas.Observation)
	for _, observation := range atlas.Observations {
		observations[observation.Subject.ID] = append(observations[observation.Subject.ID], observation)
	}
	entityByID := make(map[string]repositoryatlas.Entity, len(atlas.Entities))
	surfaceIDs := make(map[string]struct{})
	operationIDs := make(map[string]struct{})
	boundaryIDs := make(map[string]struct{})
	resourceIDs := make(map[string]struct{})
	for _, entity := range atlas.Entities {
		entityByID[entity.ID] = entity
		switch entity.Kind {
		case repositoryatlas.EntitySurface:
			surfaceIDs[entity.ID] = struct{}{}
		case repositoryatlas.EntityOperation:
			operationIDs[entity.ID] = struct{}{}
		case repositoryatlas.EntityBoundary:
			boundaryIDs[entity.ID] = struct{}{}
		case repositoryatlas.EntityResource:
			resourceIDs[entity.ID] = struct{}{}
		default:
			return fmt.Errorf("repository atlas: entity %q is outside the persisted process-entry or resource-boundary vertical", entity.ID)
		}
	}
	for surfaceID := range surfaceIDs {
		entity := entityByID[surfaceID]
		trigger, ok := triggers[entity.ID]
		if !ok || trigger.Kind != "process_entry" || trigger.ProvisionalID ||
			trigger.Resolution != "exact" || trigger.Certainty != "static" ||
			trigger.ProcessEntrypoint.Location == nil {
			return fmt.Errorf("repository atlas: surface %q has no matching exact process trigger", entity.ID)
		}
		entityObservations := observations[entity.ID]
		if len(entityObservations) != 1 {
			return fmt.Errorf("repository atlas: surface %q requires one exact observation", entity.ID)
		}
		for _, evidenceRef := range entityObservations[0].EvidenceRefs {
			item := evidenceByID[evidenceRef]
			location := trigger.ProcessEntrypoint.Location
			if item.Location.Path != location.Path || item.Location.Line != location.Line ||
				item.Location.Column != location.Column || item.Symbol != trigger.ProcessEntrypoint.ID ||
				!triggerHasAtlasEvidence(trigger, item) {
				return fmt.Errorf("repository atlas: surface %q evidence does not match its trigger", entity.ID)
			}
		}
	}

	pairedSurfaces := make(map[string]int, len(surfaceIDs))
	pairedOperations := make(map[string]int, len(operationIDs))
	for _, relation := range atlas.Relations {
		source := entityByID[relation.Source.ID]
		target := entityByID[relation.Target.ID]
		if relation.Kind != repositoryatlas.RelationExposes ||
			source.Kind != repositoryatlas.EntitySurface || target.Kind != repositoryatlas.EntityOperation ||
			source.UnitID != target.UnitID || relation.UnitID != source.UnitID ||
			relation.Phase != repositoryatlas.PhaseStartup ||
			relation.Authority != repositoryatlas.AuthorityResolved {
			return fmt.Errorf("repository atlas: relation %q is outside the exact process-entry contract", relation.ID)
		}
		sourceObservations := observations[source.ID]
		targetObservations := observations[target.ID]
		if len(sourceObservations) != 1 || len(targetObservations) != 1 ||
			!slices.Equal(relation.EvidenceRefs, sourceObservations[0].EvidenceRefs) ||
			!slices.Equal(relation.EvidenceRefs, targetObservations[0].EvidenceRefs) {
			return fmt.Errorf("repository atlas: relation %q evidence does not match its exact observations", relation.ID)
		}
		pairedSurfaces[source.ID]++
		pairedOperations[target.ID]++
	}
	for surfaceID := range surfaceIDs {
		if pairedSurfaces[surfaceID] != 1 {
			return fmt.Errorf("repository atlas: surface %q requires one exact operation relation", surfaceID)
		}
	}
	for operationID := range operationIDs {
		if pairedOperations[operationID] != 1 {
			return fmt.Errorf("repository atlas: operation %q requires one exact surface relation", operationID)
		}
	}
	// D214 resource-boundary vertical: every boundary/resource entity must
	// carry at least one observation bound to adapter-owned exact boundary
	// evidence at exact call sites. Canonical identities never leak.
	for _, boundaryID := range append(append([]string{}, boundaryKeys(boundaryIDs)...), resourceKeys(resourceIDs)...) {
		entity := entityByID[boundaryID]
		entityObservations := observations[boundaryID]
		if len(entityObservations) < 1 {
			return fmt.Errorf("repository atlas: entity %q requires at least one exact boundary observation", boundaryID)
		}
		for _, entityObservation := range entityObservations {
			if len(entityObservation.EvidenceRefs) != 1 {
				return fmt.Errorf("repository atlas: entity %q requires one exact boundary evidence ref per observation", boundaryID)
			}
			item, ok := evidenceByID[entityObservation.EvidenceRefs[0]]
			if !ok {
				return fmt.Errorf("repository atlas: entity %q references unknown boundary evidence", boundaryID)
			}
			if item.Provenance.Provider != goadapter.BoundaryObservationEvidenceProvider ||
				item.Provenance.Operation != goadapter.BoundaryObservationEvidenceOperation ||
				item.Location.Path == "" || item.Location.Line <= 0 || item.Location.Column <= 0 ||
				item.UnitID != entity.UnitID {
				return fmt.Errorf("repository atlas: entity %q has non-boundary evidence", boundaryID)
			}
		}
	}
	return nil
}

func boundaryKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	return result
}

func resourceKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	return result
}

func triggerHasAtlasEvidence(trigger DiscoveredTrigger, item repositoryatlas.Evidence) bool {
	locationMatched := false
	for _, candidate := range trigger.Evidence {
		if candidate.Kind == "process_entry_declaration" && candidate.Location != nil &&
			candidate.Location.Path == item.Location.Path && candidate.Location.Line == item.Location.Line &&
			candidate.Location.Column == item.Location.Column {
			locationMatched = true
			break
		}
	}
	if !locationMatched {
		return false
	}
	for _, candidate := range trigger.Provenance {
		if candidate.Provider == item.Provenance.Provider && candidate.Version == item.Provenance.Version &&
			candidate.Operation == item.Provenance.Operation {
			return true
		}
	}
	return false
}
