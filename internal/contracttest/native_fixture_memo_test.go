package contracttest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// Only explicit read-only cumulative-fixture consumers use these process-local
// memos. Materialization and mutation/tool-invocation contrasts stay fresh.
type jstsFixtureAuthorities struct {
	result  jstsproject.Result
	index   programindex.Index
	catalog dependencies.Catalog
}

var nativeFixtureMemo = struct {
	sync.Mutex
	goResults                      map[string]goFixtureAuthorities
	jsResults                      map[string]jstsFixtureAuthorities
	goRuns, goHits, jsRuns, jsHits int
}{goResults: map[string]goFixtureAuthorities{}, jsResults: map[string]jstsFixtureAuthorities{}}

func TestMain(m *testing.M) {
	code := m.Run()
	fmt.Fprintf(os.Stderr, "native fixture reuse: go analyses=%d hits=%d; jsts analyses=%d hits=%d\n", nativeFixtureMemo.goRuns, nativeFixtureMemo.goHits, nativeFixtureMemo.jsRuns, nativeFixtureMemo.jsHits)
	fmt.Fprintf(os.Stderr, "native input reuse: C parses=%d hits=%d; Python analyses=%d hits=%d; Clojure analyses=%d hits=%d\n", nativeInputMemo.cRuns, nativeInputMemo.cHits, nativeInputMemo.pyRuns, nativeInputMemo.pyHits, nativeInputMemo.cljRuns, nativeInputMemo.cljHits)
	os.Exit(code)
}

// The corpus identity binds its entire tracked inventory; current full bytes
// bind every source/config file. Native environment and the fixture's explicit
// outside replacement module are inputs too. Temporary checkout paths are not
// native identities and no raw environment values are logged or persisted.
func nativeFixtureKey(t *testing.T, repository *corpus.Corpus, root, selector string, published bool) string {
	t.Helper()
	h := sha256.New()
	field := func(value []byte) { fmt.Fprintf(h, "%d:", len(value)); h.Write(value) }
	field([]byte(repository.SHA256()))
	field([]byte(selector))
	field([]byte(runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH))
	environment := os.Environ()
	slices.Sort(environment)
	for _, value := range environment {
		field([]byte(value))
	}
	for _, entry := range repository.Entries() {
		field([]byte(entry.Path))
		content, err := repository.ReadFileAll(entry.ID)
		if err != nil {
			t.Fatal(err)
		}
		field(content.Bytes)
	}
	if published {
		for _, path := range []string{"go.mod", "root.go", "driver/driver.go"} {
			field([]byte(path))
			value, err := os.ReadFile(filepath.Join(filepath.Dir(root), "published-root", filepath.FromSlash(path)))
			if os.IsNotExist(err) {
				field([]byte("absent"))
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			field([]byte("present"))
			field(value)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sharedGoFixtureAuthorities(t *testing.T, root string, repository *corpus.Corpus, selector, label string) goFixtureAuthorities {
	t.Helper()
	key := nativeFixtureKey(t, repository, root, selector, true)
	nativeFixtureMemo.Lock()
	defer nativeFixtureMemo.Unlock()
	if value, ok := nativeFixtureMemo.goResults[key]; ok {
		nativeFixtureMemo.goHits++
		return ownGoFixtureAuthorities(t, value)
	}
	value := analyzeGoFixture(t, root, repository, selector, label)
	nativeFixtureMemo.goRuns++
	nativeFixtureMemo.goResults[key] = ownGoFixtureAuthorities(t, value)
	return value
}

func ownGoFixtureAuthorities(t *testing.T, value goFixtureAuthorities) goFixtureAuthorities {
	t.Helper()
	value.target = value.target.Snapshot()
	value.origins = append([]gofacts.PackageOrigin(nil), value.origins...)
	value.direct = value.direct.Snapshot()
	value.external = value.external.Snapshot()
	value.core = value.core.Snapshot()
	value.dynamic = value.dynamic.Snapshot()
	value.tests = gofacts.CloneTestSources(value.tests)
	if value.dependencies != nil {
		encoded, err := json.Marshal(value.dependencies)
		if err != nil {
			t.Fatal(err)
		}
		var owned dependencies.Catalog
		if err := json.Unmarshal(encoded, &owned); err != nil {
			t.Fatal(err)
		}
		value.dependencies = &owned
	}
	return value
}

func sharedJSTSFixture(t *testing.T, repository *corpus.Corpus, root string) (jstsproject.Result, programindex.Index, dependencies.Catalog, error) {
	t.Helper()
	key := nativeFixtureKey(t, repository, root, "jsts:package.json", false)
	nativeFixtureMemo.Lock()
	defer nativeFixtureMemo.Unlock()
	value, ok := nativeFixtureMemo.jsResults[key]
	if !ok {
		result, err := jstsproject.Discover(t.Context(), repository, root)
		if err != nil {
			return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{}, err
		}
		index, catalog, err := jstsproject.BuildFromResult(result)
		if err != nil {
			return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{}, err
		}
		value = jstsFixtureAuthorities{result: result.Snapshot(), index: index.Snapshot(), catalog: ownNativeFixture(t, catalog)}
		nativeFixtureMemo.jsRuns++
		nativeFixtureMemo.jsResults[key] = value
	} else {
		nativeFixtureMemo.jsHits++
	}
	return value.result.Snapshot(), value.index.Snapshot(), ownNativeFixture(t, value.catalog), nil
}

func TestSharedNativeFixtureResultsKeepIndependentConsumersAndChangedInputs(t *testing.T) {
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		result, index, catalog, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Declarations) == 0 {
			t.Fatal("real native fixture has no declarations")
		}
		// The reused index and dependency catalog must be exactly the ordinary
		// projection of the complete sealed compiler result, not a shortcut
		// that merely retains its advertised SHA.
		rebuiltIndex, rebuiltCatalog, err := jstsproject.BuildFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if fixtureDecisionKey(t, index) != fixtureDecisionKey(t, rebuiltIndex) || fixtureDecisionKey(t, catalog) != fixtureDecisionKey(t, rebuiltCatalog) {
			t.Fatal("memo projection differs from the complete ordinary projection")
		}
		if len(catalog.Importers) == 0 || len(catalog.Dependencies) == 0 || len(catalog.Dependencies[0].ImporterRefs) == 0 {
			t.Fatal("real native fixture has no dependency ownership")
		}
		originalCatalog := fixtureDecisionKey(t, catalog)
		original := result.Declarations[0].Name
		// A prior consumer's checkout may already have been cleaned up. The
		// sealed result must carry relative source identity, not that checkout.
		oldRoot := root
		if err := os.RemoveAll(oldRoot); err != nil {
			t.Fatal(err)
		}
		root, repository = materializeFixtureRepository(t, "jsts")
		result.Declarations[0].Name = "consumer mutation"
		index.Objects[0].Name = "consumer mutation"
		catalog.Importers[0].Name = "consumer mutation"
		catalog.Dependencies[0].ImporterRefs[0] = "consumer mutation"
		runs := nativeFixtureMemo.jsRuns
		next, nextIndex, nextCatalog, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		if nativeFixtureMemo.jsRuns != runs || next.Declarations[0].Name != original || nextIndex.Objects[0].Name == "consumer mutation" || fixtureDecisionKey(t, nextCatalog) != originalCatalog || fixtureDecisionKey(t, nextIndex) != fixtureDecisionKey(t, rebuiltIndex) {
			t.Fatal("identical native input was recompiled or shared mutable consumer storage")
		}
		encoded, err := json.Marshal(next)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), oldRoot) {
			t.Fatal("JSTS result retained a deleted consumer checkout")
		}
		ownerFacts(t, repository, nextIndex).declared(t, "src/replica-options.ts")
		path := filepath.Join(root, "src", "replica-options.ts")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(contents, []byte("\nexport function fixtureMemoChangedSource() { return 1 }\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
		changed, changedIndex, _, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, declaration := range changed.Declarations {
			if declaration.Name == "fixtureMemoChangedSource" {
				found = true
			}
		}
		if nativeFixtureMemo.jsRuns != runs+1 || !found || changedIndex.SHA256 == nextIndex.SHA256 {
			t.Fatal("changed original source did not run the native compiler and produce its new declaration")
		}
		key := nativeFixtureKey(t, repository, root, "jsts:package.json", false)
		t.Setenv("NODE_OPTIONS", strings.TrimSpace(os.Getenv("NODE_OPTIONS")+" --stack-trace-limit=20"))
		if key == nativeFixtureKey(t, repository, root, "jsts:package.json", false) {
			t.Fatal("changed native environment reused an input key")
		}
		t.Logf("real JSTS source miss and immutable hit verified; native analyses=%d hits=%d", nativeFixtureMemo.jsRuns, nativeFixtureMemo.jsHits)
	})
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		first := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "memo isolation")
		if len(first.origins) == 0 || first.dependencies == nil || len(first.dependencies.Importers) == 0 {
			t.Fatal("real Go fixture lacks native authorities")
		}
		original := first.origins[0].PackagePath
		originalImporter := first.dependencies.Importers[0].Name
		oldRoot := root
		if err := os.RemoveAll(oldRoot); err != nil {
			t.Fatal(err)
		}
		root, repository = materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		first.origins[0].PackagePath = "consumer mutation"
		first.dependencies.Importers[0].Name = "consumer mutation"
		first.target.ModulePath = "consumer mutation"
		runs := nativeFixtureMemo.goRuns
		second := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "memo isolation")
		if nativeFixtureMemo.goRuns != runs || second.origins[0].PackagePath != original || second.dependencies.Importers[0].Name != originalImporter || second.target.ModulePath != goFixtureRootPackage {
			t.Fatal("identical Go native input was rebuilt or consumer mutated retained authority")
		}
		index, err := goadapter.Build(repository, second.target, second.origins, second.direct, second.external, second.core, second.dynamic, second.tests)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(index)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), oldRoot) {
			t.Fatal("Go authority retained a deleted consumer checkout")
		}
		ownerFacts(t, repository, index).declared(t, "cmd/app/main.go")
		key := nativeFixtureKey(t, repository, root, goFixtureAppPackage, true)
		path := filepath.Join(filepath.Dir(root), "published-root", "root.go")
		if err := os.WriteFile(path, []byte("package cumulativegofixture\nfunc PublishedRoot() string { return \"changed\" }\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if key == nativeFixtureKey(t, repository, root, goFixtureAppPackage, true) {
			t.Fatal("outside replacement module changed without changing the native input key")
		}
		t.Logf("real Go authority immutable hit verified; native analyses=%d hits=%d", nativeFixtureMemo.goRuns, nativeFixtureMemo.goHits)
	})
}
