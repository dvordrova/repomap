package places

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestOnlyObservedModuleBodiesEnterDeclarationChoices(t *testing.T) {
	index := programindex.Index{Objects: []programindex.Object{
		{ID: "caller", Name: "start", Kind: programindex.ObjectModule, Location: &programindex.Location{Path: "start.py", Line: 1, Column: 1}},
		{ID: "reader", Name: "config", Kind: programindex.ObjectModule, Location: &programindex.Location{Path: "config.py", Line: 1, Column: 1}},
		{ID: "passive", Name: "library", Kind: programindex.ObjectModule, Location: &programindex.Location{Path: "library.py", Line: 1, Column: 1}},
		{ID: "namespace", Name: "namespace", Kind: programindex.ObjectModule},
		{ID: "work", Name: "work", Kind: programindex.ObjectFunction, ContainerID: "passive", Location: &programindex.Location{Path: "library.py", Line: 3, Column: 1}},
	}, Relations: []programindex.Relation{
		{FromID: "caller", ToIDs: []string{"work"}, Kind: programindex.RelationCalls},
		{FromID: "reader", Kind: programindex.RelationReads},
		{FromID: "namespace", Kind: programindex.RelationCalls},
		{FromID: "passive", ToIDs: []string{"work"}, Kind: programindex.RelationContains},
		{FromID: "passive", ToIDs: []string{"reader"}, Kind: programindex.RelationImports},
	}}
	b := builder{}
	b.useTargetObjects(index)
	for _, id := range []string{"caller", "reader", "work"} {
		if b.symbolOf[id] == "" {
			t.Fatalf("lost source-located execution/declaration: %s", id)
		}
	}
	for _, id := range []string{"passive", "namespace"} {
		if b.symbolOf[id] != "" {
			t.Fatalf("import/containment or missing source became module activity: %s", id)
		}
	}
}
