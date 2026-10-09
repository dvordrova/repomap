package orientation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestManifestProviderReferencesKeepCompleteCommandsAtRepositoryRoot(t *testing.T) {
	root := "/private/tmp/selected-repository"
	command := "cd \"" + root + "\" && ./configure --enable-json --enable-fts5 --enable-session --enable-column-metadata --enable-threadsafe --enable-shared --enable-static && make -f " + root + "/Makefile sqlite3"
	input := Input{RepositoryRoot: root}
	builder := newRequestBuilder(input)
	original := facts.Fact{ID: "a1", Kind: facts.KindManifest, Value: command, Key: "variable.CONFIGURE", Anchor: &facts.Anchor{Path: "Makefile", Line: 8}}
	row := builder.factWire(original)
	want := strings.ReplaceAll(strings.ReplaceAll(command, "\""+root+"\"", "\".\""), root+"/", "./")
	if row.Value != want || row.Key != original.Key || row.Anchor != original.Anchor.String() || original.Value != command {
		t.Fatalf("portable complete manifest command or original source changed: row=%#v original=%#v", row, original)
	}
	if row := builder.factWire(facts.Fact{Kind: facts.KindManifest, Value: root}); row.Value != "." {
		t.Fatalf("checkout root variable = %q", row.Value)
	}
	unrelated := root + "-other/build"
	if row := builder.factWire(facts.Fact{Kind: facts.KindManifest, Value: unrelated}); row.Value != unrelated {
		t.Fatalf("a different authored directory was changed: %q", row.Value)
	}
	for _, value := range []string{"/backup" + root + "/file", "https://example" + root + "/file"} {
		if got := portableManifestValue(value, root); got != value {
			t.Fatalf("a matching suffix in another path changed: %q => %q", value, got)
		}
	}
	for value, want := range map[string]string{
		"cd " + root + " && ./configure":            "cd . && ./configure",
		"-I" + root + "/include -L" + root + "/lib": "-I./include -L./lib",
	} {
		if got := portableManifestValue(value, root); got != want {
			t.Fatalf("known checkout reference: %q => %q, want %q", value, got, want)
		}
	}
}

// freqtrade's build_helpers module is t2.n1, t3.n59 and t4.n16, and its one
// merged place keeps t1's numbering (t1.n1781). Looked up by their own
// qualified ids, t2 and t3 found no place and went without their calls.
func TestAMergedDeclarationPlaceGivesEachOwningTargetItsEvidence(t *testing.T) {
	location := &programindex.Location{Path: "build_helpers/tool.py", Line: 10, Column: 1}
	index := func(targetID, subjectID string, seed bool) groupindex.Index {
		value := groupindex.Index{Target: programindex.Target{ID: targetID}, Subjects: []groupindex.Subject{{
			ID: subjectID, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: "main", Kind: programindex.ObjectFunction, Location: location},
		}}}
		if seed {
			value.Target.Seeds = []programindex.TargetSeed{{ObjectID: subjectID}}
		}
		return value
	}
	input := Input{
		Facts:  facts.Result{Targets: []facts.Target{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}}},
		Groups: []groupindex.Index{index("t1", "n1781", false), index("t2", "n1", true), index("t3", "n59", true)},
		Graph: atlas.Graph{Places: []atlas.Place{{
			ID: "s1", Kind: atlas.PlaceSymbol, Path: "build_helpers/tool.py", LineNo: 10, TargetIDs: []string{"t2", "t3"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "t1.n1781", Name: "main", Kind: "function"},
				Calls: []atlas.SymbolCall{{Name: "update_tiers", Kind: "calls", Line: 11, Column: 5}}},
		}}},
	}
	wire, _, err := buildOverview(input)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, row := range wire.Seeds {
		raw, _ := json.Marshal(row.Calls)
		got[row.Ref] = strings.Contains(string(raw), "update_tiers")
	}
	if want := map[string]bool{"t2.n1": true, "t3.n59": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("seed rows = %v, want %v", got, want)
	}
}

// Targets holding the same fact see it once; a response row naming one of
// them keeps that target's own fact.
func TestASharedFactIsOneRowAndEachTargetKeepsItsOwnID(t *testing.T) {
	fixture := newFixture(t)
	alpha, beta := fixture.targetID("alpha"), fixture.targetID("beta")
	rows := append([]facts.Fact(nil), fixture.input.Facts.Facts...)
	for _, target := range []string{beta, alpha} {
		rows = append(rows, facts.Fact{
			ID: facts.NewFactID(target, facts.KindRegistration, "shared/cli.py", "shared"), Kind: facts.KindRegistration, TargetID: target,
			Anchor: &facts.Anchor{Path: "shared/cli.py", Line: 4}, Key: "add_parser", Path: "trade", Text: "argparse.add_parser",
		})
	}
	fixture.input.Facts.Facts = rows
	sealed, err := facts.Seal(fixture.input.Facts)
	if err != nil {
		t.Fatal(err)
	}
	fixture.input.Facts = sealed
	own := make(map[string]string)
	for _, fact := range sealed.Facts {
		if fact.Key == "add_parser" {
			own[fact.TargetID] = fact.ID
		}
	}
	wire, cat, err := buildOverview(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	var shared []factWire
	for _, row := range wire.Facts {
		if row.Key == "add_parser" {
			shared = append(shared, row)
		}
		if len(row.Targets) > 1 && row.Key != "add_parser" {
			t.Fatalf("distinct facts were joined: %+v", row)
		}
	}
	if len(shared) != 1 || shared[0].Ref != own[alpha] || !reflect.DeepEqual(shared[0].Targets, []string{alpha, beta}) {
		t.Fatalf("shared fact rows = %+v, own ids %v", shared, own)
	}
	if len(wire.Facts) != 10 {
		t.Fatalf("fact rows = %d, want the fixture's 9 and one shared", len(wire.Facts))
	}
	if _, advertised := cat.facts[own[beta]]; advertised {
		t.Fatal("the second copy is a ref the request never shows")
	}
	ref := shared[0].Ref
	raw := encodeResponse(t, map[string]any{
		"summary": "Alpha and Beta parse a trade command.", "summary_refs": []string{ref},
		"roles":      []any{map[string]any{"target": beta, "role": "Command line", "purpose": "Parses trade.", "refs": []string{ref}}},
		"run_recipe": []any{map[string]any{"target": beta, "command": "python cli.py trade", "refs": []string{ref, fixture.refs(t).fact("entrypoint")}}},
	})
	result, err := normalizeOverview(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.summaryRefs, []string{own[alpha]}) ||
		len(result.roles) != 1 || !reflect.DeepEqual(result.roles[0].FactIDs, []string{own[beta]}) ||
		len(result.recipe) != 1 || result.recipe[0].FactIDs[0] != own[beta] {
		t.Fatalf("restored ids: summary %v roles %+v recipe %+v; own %v", result.summaryRefs, result.roles, result.recipe, own)
	}
}

// One (from, to, kind) is one row, and no label or sentence is lost.
func TestConnectionsOfOneKindAreOneRowWithEveryLabelAndSentence(t *testing.T) {
	rows := collapseConnections([]groupConnection{
		{from: "t1.g2", to: "t2.g1", kind: "calls", label: "fetches items", summary: "Alpha fetches items from Beta's API."},
		{from: "t1.g2", to: "t2.g1", kind: "calls", label: "fetches items", summary: "fetches items"},
		{from: "t1.g2", to: "t2.g1", kind: "calls", label: "posts orders", summary: "posts orders"},
		{from: "t1.g2", to: "t2.g1", kind: "calls", label: "fetches items", summary: "Alpha polls Beta for new items."},
		{from: "t1.g2", to: "t2.g1", kind: "calls", label: "fetches items", summary: "Alpha fetches items from Beta's API."},
		{from: "t1.g2", to: "t2.g1", kind: "reads", label: "fetches items", summary: "fetches items"},
		{from: "t1.g10", to: "t1.g1", kind: "calls", label: "starts", summary: "starts"},
	})
	want := []connectionWire{
		{From: "t1.g2", To: "t2.g1", Kind: "calls", Labels: []string{"fetches items", "posts orders"},
			Sentences: []string{"Alpha fetches items from Beta's API.", "Alpha polls Beta for new items."}},
		{From: "t1.g2", To: "t2.g1", Kind: "reads", Labels: []string{"fetches items"}},
		{From: "t1.g10", To: "t1.g1", Kind: "calls", Labels: []string{"starts"}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("connections =\n%+v\nwant\n%+v", rows, want)
	}
}
