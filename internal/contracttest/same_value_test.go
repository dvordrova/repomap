package contracttest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// spellingPairs are, for the calls of one declaration given one string
// word each, which earlier call's word each call reads the same value as
// (ProgramIndex SameValueAs), by word: "storage-class" -> "storageClass".
// A call reading its own value maps to "".
func spellingPairs(t *testing.T, index programindex.Index, path, declaration string) map[string]string {
	t.Helper()
	var caller string
	for _, object := range index.Objects {
		if (object.Name == declaration || strings.HasSuffix(object.Name, "/"+declaration)) && object.Location != nil && object.Location.Path == path && object.Kind.Callable() {
			caller = object.ID
		}
	}
	if caller == "" {
		t.Fatalf("no callable %s in %s", declaration, path)
	}
	words := map[string]string{}
	for _, relation := range index.Relations {
		if relation.FromID != caller {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.Kind == programindex.PatternLiteralString && argument.Value != "" {
					words[pattern.ID] = argument.Value
					break
				}
			}
		}
	}
	pairs := map[string]string{}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if word, ok := words[pattern.ID]; ok {
				pairs[word] = words[pattern.SameValueAs]
			}
		}
	}
	return pairs
}

// One value read under two spellings is one fact in the code: the later
// call names the earlier (ProgramIndex SameValueAs). Go's ReplicaOptions
// (internal/storefixture/replica_options.go) reads storageClass in an
// if/else-if chain whose arms are written again but for the word and
// forcePathStyle in one || of two reads compared the same way; region,
// read once, endpoint and bucket, whose arms store different fields, and
// user and password, joined by &&, each read their own value.
func TestEveryLanguageKeepsTheSpellingsOfOneValue(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "spellings")
		index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		got := spellingPairs(t, index, "internal/storefixture/replica_options.go", "ReplicaOptions")
		want := map[string]string{
			"storageClass": "", "storage-class": "storageClass",
			"forcePathStyle": "", "force-path-style": "forcePathStyle",
			"region": "", "endpoint": "", "bucket": "", "user": "", "password": "",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("spellings read as one value: %v, want %v", got, want)
		}
		assertProgramIndexRoundTrip(t, index)
	})
	// TypeScript's replicaOptions (src/replica-options.ts) reads
	// storageClass with ??, forcePathStyle with || and verbose in an
	// if/else-if chain written again but for the word; region once,
	// endpoint and bucket in arms setting different values, user and
	// password joined by &&.
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := jstsproject.Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		got := spellingPairs(t, index, "src/replica-options.ts", "replicaOptions")
		want := map[string]string{
			"storageClass": "", "storage-class": "storageClass",
			"forcePathStyle": "", "force-path-style": "forcePathStyle",
			"region": "", "verbose": "", "v": "verbose", "endpoint": "", "bucket": "", "user": "", "password": "",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("spellings read as one value: %v, want %v", got, want)
		}
		assertProgramIndexRoundTrip(t, index)
	})
	// Clojure's replica-options (src/example/core.clj) reads storageClass
	// with `or`; region once, user and password joined by `and`.
	t.Run("clojure", func(t *testing.T) {
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
		got := spellingPairs(t, index, "src/example/core.clj", "replica-options")
		want := map[string]string{"storageClass": "", "storage-class": "storageClass", "region": "", "user": "", "password": ""}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("spellings read as one value: %v, want %v", got, want)
		}
		assertProgramIndexRoundTrip(t, index)
	})
	// kvd's loadConfig reads the dbfilename directive under its older
	// spelling too, one || of two strcasecmp calls compared the same way;
	// its else-if arms compare other directives and write other fields.
	t.Run("c", func(t *testing.T) {
		index := buildCIndex(t, loadCFixture(t), "c:kvd")
		got := spellingPairs(t, index, "kvd.c", "loadConfig")
		want := map[string]string{
			"r": "", "port": "", "dbfilename": "", "dbfile": "dbfilename", "max-entry-value": "", "persist": "", "never": "", "always": "",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("spellings read as one value: %v, want %v", got, want)
		}
		assertProgramIndexRoundTrip(t, index)
	})
}

// Two spellings of one value both answered setting are one input, named by
// the first spelling as written, the other kept as its alias with its site
// and call; a spelling answered another kind keeps its own answer and its
// own input, and nothing new is asked.
func TestGoSpellingsOfOneSettingAreOneInput(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	app := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "spelling-inputs")
	index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	meta := reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}
	const file = "internal/storefixture/replica_options.go"
	read := func(forcePathStyle string) (groupindex.Index, *inputsPreset) {
		preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
			symbol, _ := item["symbol"].(string)
			if column != "enters" || symbol != "net/url.Values.Get" {
				return "", false
			}
			if strings.Contains(callText(item), `"force-path-style"`) {
				return forcePathStyle, true
			}
			return "setting", true
		}}
		return readInputs(t, graph, index, meta, root, preset), preset
	}
	type spelled struct {
		kind, name string
		aliases    []string
	}
	rows := func(projected groupindex.Index) []spelled {
		var result []spelled
		for _, operation := range projected.Operations {
			if operation.Location.Path != file {
				continue
			}
			row := spelled{kind: operation.Kind, name: operation.Name}
			for _, alias := range operation.Aliases {
				row.aliases = append(row.aliases, alias.Name+" "+alias.Written)
			}
			result = append(result, row)
		}
		slices.SortFunc(result, func(a, b spelled) int { return strings.Compare(a.kind+" "+a.name, b.kind+" "+b.name) })
		return result
	}

	projected, preset := read("setting")
	want := []spelled{
		{kind: "setting", name: "bucket"},
		{kind: "setting", name: "endpoint"},
		{kind: "setting", name: "forcePathStyle", aliases: []string{`force-path-style query.Get("force-path-style")`}},
		{kind: "setting", name: "password"},
		{kind: "setting", name: "region"},
		{kind: "setting", name: "storageClass", aliases: []string{`storage-class query.Get("storage-class")`}},
		{kind: "setting", name: "user"},
	}
	if got := rows(projected); !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs:\n%+v\nwant\n%+v", got, want)
	}
	// Each spelling was asked on its own, as every word call is.
	var asked []string
	for _, item := range preset.asked["enters"] {
		if symbol, _ := item["symbol"].(string); symbol == "net/url.Values.Get" {
			asked = append(asked, callText(item))
		}
	}
	if len(asked) != 9 || !slices.Contains(asked, `query.Get("storage-class")`) {
		t.Fatalf("Get calls asked: %q", asked)
	}
	// The alias keeps its own site.
	for _, operation := range projected.Operations {
		if operation.Name == "storageClass" && (len(operation.Aliases) != 1 || operation.Aliases[0].Location.Line != operation.Location.Line+2) {
			t.Fatalf("storageClass's other spelling: %+v at %+v", operation.Aliases, operation.Location)
		}
	}

	projected, _ = read("command")
	want = []spelled{
		{kind: "command", name: "force-path-style"},
		{kind: "setting", name: "bucket"},
		{kind: "setting", name: "endpoint"},
		{kind: "setting", name: "forcePathStyle"},
		{kind: "setting", name: "password"},
		{kind: "setting", name: "region"},
		{kind: "setting", name: "storageClass", aliases: []string{`storage-class query.Get("storage-class")`}},
		{kind: "setting", name: "user"},
	}
	if got := rows(projected); !reflect.DeepEqual(got, want) {
		t.Fatalf("a spelling answered another kind:\n%+v\nwant\n%+v", got, want)
	}
}
