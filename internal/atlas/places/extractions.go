package places

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
)

// addExtractions extends the existing places graph. A declared output folder
// is an entity; membership never promotes its manual files to generated code.
func (b *builder) addExtractions(graph *atlas.Graph) {
	programTargets := make(map[string]string, len(b.input.Facts.Targets))
	for _, target := range b.input.Facts.Targets {
		programTargets[target.ID] = target.ProgramTargetID
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Kind != facts.KindEntity {
			continue
		}
		entity := &atlas.EntityFacts{Name: fact.Symbol, Extractor: fact.Extractor, Status: fact.Value, Files: []string{}}
		targets := make(map[string]struct{})
		if target := programTargets[fact.TargetID]; target != "" {
			targets[target] = struct{}{}
		}
		for _, member := range fact.Evidence {
			entity.Files = append(entity.Files, member.Path)
			if file := b.files[member.Path]; file != nil {
				for target := range file.targets {
					targets[target] = struct{}{}
				}
				graph.Edges = append(graph.Edges, atlas.Edge{From: "entity:" + fact.ID, To: atlas.FileID(member.Path), Kind: "inventory", Count: 1, Witnesses: []atlas.Witness{},
					Evidence: &atlas.EdgeEvidence{Label: "file at or beneath referenced path", Path: member.Path, LineNo: 1}})
			}
		}
		place := atlas.Place{ID: "entity:" + fact.ID, Kind: atlas.PlaceEntity, Path: fact.Path, TargetIDs: sortedKeys(targets), Entity: entity,
			Given: fact.Extractor + ": " + fact.Symbol + " " + fact.Path + " (" + fact.Value + ")"}
		if fact.Anchor != nil {
			place.LineNo = fact.Anchor.Line
		}
		graph.Places = append(graph.Places, place)
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Kind != facts.KindRelation {
			continue
		}
		graph.Edges = append(graph.Edges, atlas.Edge{From: "entity:" + fact.Refs[0], To: "entity:" + fact.Refs[1], Kind: "observation", Count: 1, Witnesses: []atlas.Witness{},
			Evidence: &atlas.EdgeEvidence{Extractor: fact.Extractor, Label: fact.Key, Path: fact.Anchor.Path, LineNo: fact.Anchor.Line}})
	}
}
