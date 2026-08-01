package repositoryatlas

import (
	"encoding/json"
	"sort"
)

// CanonicalJSON returns a deterministic encoding of a validated deep copy. It
// never reorders or aliases caller-owned slices.
func CanonicalJSON(atlas Atlas) ([]byte, error) {
	canonical, err := Canonical(atlas)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(canonical, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// Canonical validates atlas and returns a deeply copied, deterministically
// ordered value.
func Canonical(atlas Atlas) (Atlas, error) {
	if err := atlas.Validate(); err != nil {
		return Atlas{}, err
	}
	return canonicalCopy(atlas), nil
}

func canonicalCopy(atlas Atlas) Atlas {
	canonical := Atlas{
		Version:      atlas.Version,
		Units:        append([]Unit{}, atlas.Units...),
		Entities:     append([]Entity{}, atlas.Entities...),
		Observations: make([]Observation, len(atlas.Observations)),
		Evidence:     append([]Evidence{}, atlas.Evidence...),
		Relations:    make([]Relation, len(atlas.Relations)),
	}
	for index := range canonical.Evidence {
		if atlas.Evidence[index].Provenance.Location != nil {
			location := *atlas.Evidence[index].Provenance.Location
			canonical.Evidence[index].Provenance.Location = &location
		}
	}
	for index, observation := range atlas.Observations {
		canonical.Observations[index] = observation
		canonical.Observations[index].EvidenceRefs = append([]string{}, observation.EvidenceRefs...)
		sort.Strings(canonical.Observations[index].EvidenceRefs)
	}
	for index, relation := range atlas.Relations {
		canonical.Relations[index] = relation
		canonical.Relations[index].EvidenceRefs = append([]string{}, relation.EvidenceRefs...)
		sort.Strings(canonical.Relations[index].EvidenceRefs)
	}
	sort.Slice(canonical.Units, func(i, j int) bool { return canonical.Units[i].ID < canonical.Units[j].ID })
	sort.Slice(canonical.Entities, func(i, j int) bool { return canonical.Entities[i].ID < canonical.Entities[j].ID })
	sort.Slice(canonical.Observations, func(i, j int) bool {
		return canonical.Observations[i].ID < canonical.Observations[j].ID
	})
	sort.Slice(canonical.Evidence, func(i, j int) bool { return canonical.Evidence[i].ID < canonical.Evidence[j].ID })
	sort.Slice(canonical.Relations, func(i, j int) bool { return canonical.Relations[i].ID < canonical.Relations[j].ID })
	return canonical
}
