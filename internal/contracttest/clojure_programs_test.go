package contracttest

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The Clojure fixture's JVM program hands its functions over three ways the
// facts read as registrations (src/example/core.clj): a Quil sketch given
// draw-greeting and on-key under the keywords :draw and :key-pressed, a
// websocket given a trailing map of two handlers, and a future starting
// greet-many on its own thread. Each keyword entry is a registration of its
// own, named by its keyword, and each is asked what its callable becomes; the
// future is asked on its own with the statement as written. A preset reader
// answers the key handler interaction, the message handler request and the
// started greeting continuous: three inputs, each handled by its function.
func TestCumulativeClojureKeywordHandoffsAndFutures(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("Clojure discovery: %v %v", targets, err)
	}
	result, err := clojureproject.Build(t.Context(), root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	const core = "src/example/core.clj"
	var registrations []string
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		if fact.Anchor.Path == core && fact.Anchor.Line > 161 {
			registrations = append(registrations, fmt.Sprintf("%d %s %s %s %s %s", fact.Anchor.Line, fact.Key, fact.Text, fact.Symbol, fact.Path, fact.Invocation))
		}
	}
	want := []string{
		"167 quil.core/sketch quil.core.sketch.draw example.core/draw-greeting  ",
		"167 quil.core/sketch quil.core.sketch.key-pressed example.core/on-key  ",
		"175 hato.websocket/websocket hato.websocket.websocket.on-close example.core/close-feed ws://localhost:8080/feed ",
		"175 hato.websocket/websocket hato.websocket.websocket.on-message example.core/receive-greeting ws://localhost:8080/feed ",
		"180 clojure.core/future  example.core/greet-many  goroutine",
	}
	if slices.Sort(registrations); !slices.Equal(registrations, want) {
		t.Fatalf("registrations:\n have %q\n want %q", registrations, want)
	}
	// The tests the :test alias runs are no dead code: no entrypoint
	// reaches them, their runner runs them.
	for _, fact := range layer.OfKind(facts.KindDeadModule) {
		if strings.HasPrefix(fact.Path, "test/") {
			t.Fatalf("a test source is judged dead: %s", fact.Path)
		}
	}
	// The run recipe's evidence: every alias's main options, quoted with
	// the line that writes the alias.
	var manifest []string
	for _, fact := range layer.OfKind(facts.KindManifest) {
		if strings.HasSuffix(fact.Key, "main-opts") {
			manifest = append(manifest, fmt.Sprintf("%s:%d %s = %s", fact.Anchor.Path, fact.Anchor.Line, fact.Key, fact.Value))
		}
	}
	if want := []string{
		"deps.edn:3 aliases.run.main-opts = -m example.core",
		"deps.edn:4 aliases.test.main-opts = -m cognitect.test-runner",
		"deps.edn:6 aliases.web.main-opts = -m shadow.cljs.devtools.cli",
	}; !slices.Equal(manifest, want) {
		t.Fatalf("alias rows:\n have %q\n want %q", manifest, want)
	}

	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		symbol, _ := item["symbol"].(string)
		switch {
		case column == "binds" && symbol == "quil.core.sketch.key-pressed":
			return "interaction", true
		case column == "binds" && symbol == "hato.websocket.websocket.on-message":
			return "request", true
		case column == "starts":
			return "continuous", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "package", Name: index.Target.Name, Root: "."}, root, preset)
	if got, want := startedItems(preset), []string{"(future (greet-many names)) | example.core/greet-many | example.core/warm-greetings"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("starting statements asked:\n%q\nwant\n%q", got, want)
	}
	var bound []string
	for _, item := range preset.asked["binds"] {
		if symbol, _ := item["symbol"].(string); strings.HasPrefix(symbol, "quil.") || strings.HasPrefix(symbol, "hato.") {
			bound = append(bound, symbol)
		}
	}
	slices.Sort(bound)
	if want := []string{"hato.websocket.websocket.on-close", "hato.websocket.websocket.on-message", "quil.core.sketch.draw", "quil.core.sketch.key-pressed"}; !slices.Equal(bound, want) {
		t.Fatalf("keyword entries asked what their callable becomes: %q, want %q", bound, want)
	}
	// Each keyword entry is named from its words, the keyword first:
	// on-key's entry offers key-pressed, not only the call's word.
	var named []string
	for column, items := range preset.asked {
		if !strings.HasSuffix(column, ".name") {
			continue
		}
		for _, item := range items {
			if external, _ := item["external"].(string); external == "quil.core.sketch.key-pressed" {
				for _, word := range item["words"].([]any) {
					value, _ := word.(map[string]any)["value"].(string)
					named = append(named, value)
				}
			}
		}
	}
	if want := []string{"quil.core/sketch", "key-pressed", "Greeter"}; !slices.Equal(named, want) {
		t.Fatalf("key-pressed's entry is named from %q, want %q", named, want)
	}
	var inputs []string
	for _, row := range inputRows(projected, core) {
		inputs = append(inputs, row.kind+" "+row.handler+" "+row.at)
	}
	if want := []string{
		"interaction example.core/on-key src/example/core.clj:167",
		"request example.core/receive-greeting src/example/core.clj:175",
		"continuous example.core/greet-many src/example/core.clj:180",
	}; !slices.Equal(inputs, want) {
		t.Fatalf("inputs:\n have %q\n want %q", inputs, want)
	}
	// on-key hands (:key event) to service/command-for, which looks it up
	// in key->command: its keys are the keys key-pressed takes, listed
	// under it with the command each names, and the table is no input of
	// its own (othello's key->command).
	names := map[string]string{}
	for _, operation := range projected.Operations {
		names[operation.ID] = operation.Name
	}
	for position, operation := range projected.Operations {
		if operation.Location.Path != core || operation.Name != "key-pressed" {
			continue
		}
		var keys []string
		for _, id := range projected.Reach[position].SubArguments {
			keys = append(keys, names[id])
		}
		if !slices.Equal(keys, []string{"n", "u"}) {
			t.Fatalf("key-pressed's keys: %q, want n and u", keys)
		}
	}
	for _, operation := range projected.Operations {
		if operation.Location.Path == "src/example/service.cljc" && (operation.Kind != "interaction" || !operation.HandlerUnknown || !projected.Launch.Nested[operation.ID]) {
			t.Fatalf("a key of key->command is no nested value of key-pressed: %+v", operation)
		}
	}
	if written := writtenRows(projected, "src/example/service.cljc"); written["n"] != ":n :new-greeting" || written["u"] != ":u :undo" {
		t.Fatalf("each key's row as written: %q", written)
	}
}

// The shadow-cljs.edn's :app build is the fixture's browser program: the
// run discovers it beside the JVM project, it starts at example.web/init,
// and it hands refresh! to the browser's own setInterval, a registration
// like any callable handed to an outside function.
func TestCumulativeClojureShadowBuild(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	builds, err := clojureproject.ScoutShadow(repository)
	if err != nil || len(builds) != 1 {
		t.Fatalf("shadow-cljs builds: %v %v", builds, err)
	}
	result, err := clojureproject.Build(t.Context(), root, repository, builds[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, fact := range layer.Facts {
		switch fact.Kind {
		case facts.KindEntrypoint, facts.KindRegistration:
			got = append(got, fmt.Sprintf("%s %s:%d %s %s %s", fact.Kind, fact.Anchor.Path, fact.Anchor.Line, fact.Key, fact.Symbol, fact.Text))
		case facts.KindManifest:
			if strings.HasPrefix(fact.Key, "builds.") {
				got = append(got, fmt.Sprintf("%s %s:%d %s %s", fact.Kind, fact.Anchor.Path, fact.Anchor.Line, fact.Key, fact.Value))
			}
		}
	}
	slices.Sort(got)
	want := []string{
		"entrypoint src/example/web.cljs:10 callable example.web/init ",
		"manifest shadow-cljs.edn:2 builds.app.output-dir public/js",
		"manifest shadow-cljs.edn:2 builds.app.target browser",
		"manifest shadow-cljs.edn:4 builds.app.modules.main.init-fn example.web/init",
		"registration src/example/web.cljs:11 js/setInterval example.web/refresh! js.setInterval",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("web build facts:\n have %q\n want %q", got, want)
	}
	// ClojureScript reads a forward declaration as cljs.core/declare: no
	// var of its own, as in the JVM view (othello's (declare negamax)).
	var ticks []string
	for _, object := range index.Objects {
		if object.Name == "example.web/tick" {
			ticks = append(ticks, fmt.Sprintf("%s %d", object.Kind, object.Location.Line))
		}
	}
	if !slices.Equal(ticks, []string{"function 19"}) {
		t.Fatalf("example.web/tick declarations: %q, want its defn alone", ticks)
	}
}
