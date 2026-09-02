package snapshot

import "encoding/json"

// OwnSnapshot returns an independently owned copy of a deterministic snapshot.
// It is the live handoff used when an already scoped target projection enters
// the ordinary orientation pipeline without rebuilding repository facts.
func OwnSnapshot(source Snapshot) (Snapshot, error) {
	wire, err := json.Marshal(source)
	if err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	if err := json.Unmarshal(wire, &result); err != nil {
		return Snapshot{}, err
	}
	result.FilteredFiles = append([]string(nil), source.FilteredFiles...)
	if source.GoTargetAdvisory != nil {
		advisory := *source.GoTargetAdvisory
		advisory.Examples = append([]string(nil), source.GoTargetAdvisory.Examples...)
		result.GoTargetAdvisory = &advisory
	}
	if source.GoTargetSelection != nil {
		selection := *source.GoTargetSelection
		selection.Examples = append([]string(nil), source.GoTargetSelection.Examples...)
		result.GoTargetSelection = &selection
	}
	if source.TargetCatalog != nil {
		catalog := source.TargetCatalog.Snapshot()
		result.TargetCatalog = &catalog
	}
	return result, nil
}
