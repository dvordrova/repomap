package places

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
)

// Reuse the already extracted observations. No files are reread and no launch
// command is guessed from a framework name. Exact source and target identities
// stay attached even when a manifest is not one of the code files.
func (b *builder) addSourceFacts(graph *atlas.Graph) {
	targets := make(map[string]facts.Target, len(b.input.Facts.Targets))
	for _, target := range b.input.Facts.Targets {
		targets[target.ID] = target
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Kind != facts.KindEntrypoint && fact.Kind != facts.KindManifest {
			continue
		}
		if fact.Anchor == nil || fact.Anchor.Line < 1 {
			continue
		}
		// Some manifest observations name a configuration file excluded from
		// the corpus. Such a reference does not authorize reading or sending it.
		if _, present := b.entries[fact.Anchor.Path]; !present {
			continue
		}
		target, ok := targets[fact.TargetID]
		if !ok || target.ProgramTargetID == "" {
			continue
		}
		graph.Places = append(graph.Places, atlas.Place{
			ID: "fact:" + fact.ID, Kind: atlas.PlaceSourceFact, Path: fact.Anchor.Path,
			LineNo: fact.Anchor.Line, Column: fact.Anchor.Column,
			TargetIDs: []string{target.ProgramTargetID}, Given: string(fact.Kind) + ": " + fact.Key,
			SourceFact: &atlas.SourceFact{Kind: string(fact.Kind), Name: fact.Symbol,
				Key: fact.Key, Value: fact.Value, ObjectID: fact.ObjectID,
				Language: target.Language, Component: target.Name, ComponentKind: target.Kind, Root: target.Root},
		})
	}
}
