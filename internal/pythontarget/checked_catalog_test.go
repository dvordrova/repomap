package pythontarget

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestCumulativeCheckedCatalogKeepsNativeMembershipWithoutAllocations(t *testing.T) {
	catalog, _ := cumulativeCheckedPythonCatalog(t)
	wire, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Catalog
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(wire, reencoded) {
		t.Fatalf("in-memory indexes changed catalog bytes or identity: %v", err)
	}
	foreign := catalog.Entries[0].Snapshot()
	foreign.DisplayName += " from a different catalog"
	other, err := NewCatalog([]Target{foreign}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]Catalog{"constructed": catalog, "snapshot": catalog.Snapshot(), "decoded": decoded} {
		t.Run(name, func(t *testing.T) {
			checked, err := value.Check()
			if err != nil {
				t.Fatal(err)
			}
			if checked.OwnsTarget(other.Entries[0]) {
				t.Fatal("valid foreign target acquired catalog membership")
			}
			for _, target := range catalog.Entries {
				if !checked.OwnsTarget(target) {
					t.Fatalf("native target %s disappeared", target.Selector)
				}
			}
			var owns bool
			allocations := testing.AllocsPerRun(100, func() { owns = checked.OwnsTarget(catalog.Entries[0]) })
			if !owns || allocations != 0 {
				t.Fatalf("native membership reprocessed catalog data: owned=%t allocations=%g", owns, allocations)
			}
		})
	}
}

func TestCumulativeCatalogDecodeChecksContentEvenWithKnownRef(t *testing.T) {
	catalog, _ := cumulativeCheckedPythonCatalog(t)
	changed := catalog.Snapshot()
	changed.Entries[0].DisplayName += " changed without resealing"
	if err := changed.Validate(); err == nil {
		t.Fatal("explicit Validate skipped changed catalog contents")
	}
	wire, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Catalog
	if err := json.Unmarshal(wire, &decoded); err == nil {
		t.Fatal("decode trusted a previously seen catalog ref without checking its contents")
	}
}

func TestCumulativeCheckedCatalogStillReconstructsResolverTargets(t *testing.T) {
	catalog, repository := cumulativeCheckedPythonCatalog(t)
	resolver, err := NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	fileRef, ok := repository.ID("src/fixture_app/models.py")
	if !ok {
		t.Fatal("cumulative model source is absent")
	}
	view, err := resolver.ResolveOne(fileRef)
	if err != nil || view.ScopeRef == "" || !catalog.OwnsTarget(view) {
		t.Fatalf("exact resolver view lost catalog ownership: %+v / %v", view, err)
	}
	restored, ok, err := catalog.ResolveSelector(view.Selector)
	if err != nil || !ok || restored.Ref != view.Ref {
		t.Fatalf("exact resolver selector changed its target: %+v / %v", restored, err)
	}
	changed := view.Snapshot()
	changed.DisplayName += " invented interpretation"
	changed, err = sealTarget(changed)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.OwnsTarget(changed) {
		t.Fatal("a valid but noncanonical module view acquired resolver ownership")
	}
}

func cumulativeCheckedPythonCatalog(t *testing.T) (Catalog, *corpus.Corpus) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "../../testdata/repositories/python")
	wire, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../testdata/contracts/python.files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Entries []struct{ Path string }
	}
	if err := json.Unmarshal(wire, &inventory); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(inventory.Entries))
	for i, entry := range inventory.Entries {
		paths[i] = entry.Path
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	catalog, err := Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	return catalog, repository
}
