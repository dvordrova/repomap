package pythontarget

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistCatalogWritesCanonicalExactAuthority(t *testing.T) {
	repository := fixture(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"example\"\n",
		"example.py":     "if __name__ == '__main__':\n    pass\n",
	})
	catalog, err := Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want, err := catalog.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	runDir := t.TempDir()
	if err := PersistCatalog(runDir, catalog); err != nil {
		t.Fatalf("PersistCatalog: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		t.Fatalf("read persisted catalog: %v", err)
	}
	if !bytes.Equal(saved, want) {
		t.Fatal("persisted catalog is not its canonical encoding")
	}
	restored, err := DecodeCatalog(saved)
	if err != nil {
		t.Fatalf("DecodeCatalog: %v", err)
	}
	if restored.Ref != catalog.Ref {
		t.Fatalf("persisted catalog ref = %q, want %q", restored.Ref, catalog.Ref)
	}
}
