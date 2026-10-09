package clojureproject

import (
	"fmt"
	"slices"
	"testing"
)

// Othello's deps.edn and shadow-cljs.edn, shortened: the rows a newcomer
// reads to run each program, with the line each is written on.
func TestManifestRowsQuoteAliasesAndBuilds(t *testing.T) {
	deps := `{:paths ["src"]
 :deps {org.clojure/clojure {:mvn/version "1.12.0"}}
 :aliases
 {:spec {:extra-paths ["spec"]
         :extra-deps {speclj/speclj {:mvn/version "3.12.2"}}
         :main-opts ["-m" "speclj.main" "-c"]}
  :dev {:extra-paths ["dev"]}
  :run {:extra-deps {quil/quil {:mvn/version "4.3.1563"}}
        :main-opts ["-m" "othello.core"]}}}
`
	shadow := `{:deps {:aliases [:web]}
 :dev-http {8080 {:root "public"}}
 :builds
 {:app {:target :browser
        :output-dir "public/js"
        :modules {:main {:init-fn othello.ui.web/init}}}
  :script {:target :node-script :main example.cli/main :output-to "out/cli.js"}
  :lib {:target :esm :modules {:core {:entries [example.core]}}}}}
`
	rows := func(name, text string) []string {
		var result []string
		for _, row := range ReadManifest(name, []byte(text)).Rows(name) {
			result = append(result, fmt.Sprintf("%d %s = %s", row.Line, row.Key, row.Value))
		}
		return result
	}
	if got, want := rows("deps.edn", deps), []string{
		"1 paths = src",
		"2 dependency.org.clojure/clojure = 1.12.0",
		"4 aliases.spec.main-opts = -m speclj.main -c",
		"4 aliases.spec.extra-paths = spec",
		"4 aliases.spec.extra-deps = speclj/speclj",
		"7 aliases.dev.extra-paths = dev",
		"8 aliases.run.main-opts = -m othello.core",
		"8 aliases.run.extra-deps = quil/quil",
	}; !slices.Equal(got, want) {
		t.Fatalf("deps.edn rows:\n have %q\n want %q", got, want)
	}
	if got, want := rows("shadow-cljs.edn", shadow), []string{
		"1 deps.aliases = web",
		"2 dev-http.8080 = public",
		"4 builds.app.target = browser",
		"4 builds.app.output-dir = public/js",
		"6 builds.app.modules.main.init-fn = othello.ui.web/init",
		"7 builds.script.target = node-script",
		"7 builds.script.main = example.cli/main",
		"8 builds.lib.target = esm",
		"8 builds.lib.modules.core.entries = example.core",
	}; !slices.Equal(got, want) {
		t.Fatalf("shadow-cljs.edn rows:\n have %q\n want %q", got, want)
	}
	// The alias that runs speclj's runner names its directory as tests; :dev
	// adds a directory and runs nothing, so dev is no test directory.
	if got := ReadManifest("deps.edn", []byte(deps)).TestDirectories("deps.edn", "."); !slices.Equal(got, []string{"spec"}) {
		t.Fatalf("deps.edn test directories: %v", got)
	}
	// Leiningen runs its :test-paths, and its own default test directory
	// when the project writes none.
	if got := ReadManifest("project.clj", []byte(`(defproject app "0.1.0" :main app.core :test-paths ["t"])`)).TestDirectories("project.clj", "svc"); !slices.Equal(got, []string{"svc/t"}) {
		t.Fatalf("project.clj test paths: %v", got)
	}
	if got := ReadManifest("project.clj", []byte(`(defproject app "0.1.0" :main app.core)`)).TestDirectories("project.clj", "."); !slices.Equal(got, []string{"test"}) {
		t.Fatalf("project.clj default test path: %v", got)
	}
}

// Trailing keyword/value pairs and a trailing map are keyword arguments; a
// repeated or auto-resolved keyword, or a keyword with no value, leaves the
// arguments positional.
func TestKeywordArguments(t *testing.T) {
	for _, test := range []struct {
		call string
		want []string
	}{
		{`(q/sketch :title "Othello" :setup setup :key-pressed on-key)`, []string{"title=\"Othello\"", "setup=setup", "key-pressed=on-key"}},
		{`(assoc state :ai-job job)`, []string{"1=state", "ai-job=job"}},
		{`(ws/websocket url {:on-message f :on-close g})`, []string{"1=url", "on-message=f", "on-close=g"}},
		{`(f :a 1 :a 2)`, []string{"1=:a", "2=1", "3=:a", "4=2"}},
		{`(f ::a 1)`, []string{"1=::a", "2=1"}},
		{`(f x :a)`, []string{"1=x", "2=:a"}},
		{`(ex-info reason {})`, []string{"1=reason", "2={}"}},
	} {
		s := newSource([]byte(test.call))
		var got []string
		for _, argument := range s.arguments(site{Filename: "f.clj", Row: 1, Col: 1, EndRow: 1, EndCol: len(test.call) + 1}) {
			key := argument.Keyword
			if key == "" {
				key = fmt.Sprint(argument.Position)
			}
			value := argument.Origin.Text
			if argument.Value != "" {
				value = fmt.Sprintf("%q", argument.Value)
			}
			got = append(got, key+"="+value)
		}
		if !slices.Equal(got, test.want) {
			t.Fatalf("%s:\n have %q\n want %q", test.call, got, test.want)
		}
	}
}

func TestShadowBuildLevelLiteralEntriesKeepOriginalLines(t *testing.T) {
	text := `{:builds {:app {:target :npm-module
                        :entries [example.web
                                  example.service
                                  "not-a-namespace" :not-a-namespace [example.nested]]}
                  :computed {:target :npm-module :entries (discover-entries)}
                  :scalar {:target :npm-module :entries example.scalar}}}`
	manifest := ReadManifest("shadow-cljs.edn", []byte(text))
	if len(manifest.Builds) != 3 || !slices.Equal(manifest.Builds[0].Entries, []Entry{
		{Key: "entries", Symbol: "example.web", Line: 2},
		{Key: "entries", Symbol: "example.service", Line: 3},
	}) || len(manifest.Builds[1].Entries) != 0 || len(manifest.Builds[2].Entries) != 0 {
		t.Fatalf("literal namespace entries: %+v", manifest.Builds)
	}
	var rows []Row
	for _, row := range manifest.Rows("shadow-cljs.edn") {
		if row.Key == "builds.app.entries" {
			rows = append(rows, row)
		}
	}
	if !slices.Equal(rows, []Row{{Key: "builds.app.entries", Value: "example.web", Line: 2}, {Key: "builds.app.entries", Value: "example.service", Line: 3}}) {
		t.Fatalf("original manifest rows: %+v", rows)
	}
}
