package index_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/evidence"
	localindex "github.com/dvordrova/repomap/internal/index"
	"github.com/dvordrova/repomap/internal/symbol"
)

const fixtureTargetID = "method:server/key.go:90:20:kvServer.Put"

func TestPutAndNeighborhood(t *testing.T) {
	t.Parallel()

	store := localindex.New(localindex.Metadata{Repository: "fixture-repo", Revision: "abc123"})
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}

	got, err := store.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	got.Query = "mutated"
	got.AllowedPaths[0] = "mutated.go"

	again, err := store.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Query != bundle.Query || !reflect.DeepEqual(again.AllowedPaths, bundle.AllowedPaths) {
		t.Fatalf("stored bundle was mutated: %#v", again)
	}
	if store.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", store.Len())
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()

	metadata := localindex.Metadata{Repository: "fixture-repo", Revision: "abc123-dirty"}
	store := localindex.New(metadata)
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".repomap", "symbol-index.json")
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := localindex.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Metadata() != metadata {
		t.Fatalf("Metadata() = %#v, want %#v", loaded.Metadata(), metadata)
	}
	if !reflect.DeepEqual(loaded.Targets(), []string{fixtureTargetID}) {
		t.Fatalf("Targets() = %v", loaded.Targets())
	}
	got, err := loaded.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, bundle) {
		t.Fatalf("reloaded bundle differs\ngot:  %#v\nwant: %#v", got, bundle)
	}
}

func TestInvalidatePath(t *testing.T) {
	t.Parallel()

	store := localindex.New(localindex.Metadata{Repository: "fixture-repo"})
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}
	other := minimalBundle("function:server/other.go:10:other", "server/other.go")
	if err := store.Put(other); err != nil {
		t.Fatal(err)
	}

	removed := store.InvalidatePath("server/key.go")
	if !reflect.DeepEqual(removed, []string{fixtureTargetID}) {
		t.Fatalf("InvalidatePath() = %v", removed)
	}
	if _, err := store.Neighborhood(fixtureTargetID); !errors.Is(err, localindex.ErrNotFound) {
		t.Fatalf("Neighborhood(invalidated) error = %v", err)
	}
	if _, err := store.Neighborhood(other.Target.Entity.ID); err != nil {
		t.Fatalf("Neighborhood(unrelated) error = %v", err)
	}
}

func TestPutReplacementUpdatesPathIndex(t *testing.T) {
	t.Parallel()

	store := localindex.New(localindex.Metadata{Repository: "fixture-repo"})
	if err := store.Put(fixtureBundle(t)); err != nil {
		t.Fatal(err)
	}
	replacement := minimalBundle(fixtureTargetID, "server/replacement.go")
	if err := store.Put(replacement); err != nil {
		t.Fatal(err)
	}

	if removed := store.InvalidatePath("server/key.go"); len(removed) != 0 {
		t.Fatalf("InvalidatePath(old path) = %v, want none", removed)
	}
	if _, err := store.Neighborhood(fixtureTargetID); err != nil {
		t.Fatalf("Neighborhood(replacement) error = %v", err)
	}
	if removed := store.InvalidatePath("server/replacement.go"); !reflect.DeepEqual(removed, []string{fixtureTargetID}) {
		t.Fatalf("InvalidatePath(replacement path) = %v", removed)
	}
}

func TestPutRejectsUnsafePath(t *testing.T) {
	t.Parallel()

	store := localindex.New(localindex.Metadata{Repository: "fixture-repo"})
	bundle := minimalBundle("function:outside", "../outside.go")
	if err := store.Put(bundle); err == nil {
		t.Fatal("Put() error = nil, want unsafe path error")
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"metadata":{"repository":"fixture"},"symbols":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localindex.Load(path); err == nil {
		t.Fatal("Load() error = nil, want version error")
	}
}

func fixtureBundle(t *testing.T) symbol.Bundle {
	t.Helper()
	var bundle symbol.Bundle
	if err := json.Unmarshal(deepseektest.SymbolBundleJSON, &bundle); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func minimalBundle(targetID, path string) symbol.Bundle {
	return symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "fixture-repo",
		Query:    targetID,
		Target: symbol.Fact{Entity: evidence.Entity{
			ID:       targetID,
			Kind:     evidence.EntityFunction,
			Name:     targetID,
			Language: "go",
			Location: &evidence.Location{Path: path, Line: 10},
		}},
		AllowedPaths: []string{path},
	}
}
