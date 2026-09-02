package groupindex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistAndReadRoundTrip(t *testing.T) {
	program := testProgramIndex(t, "persistence")
	index, _, err := Build(program, testProposals(testSubjectIDs(t, program)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	runDir := t.TempDir()
	if err := Persist(runDir, index); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	restored, err := Read(runDir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if restored.SHA256 != index.SHA256 || restored.Target.ID != index.Target.ID {
		t.Fatalf("restored authority = %#v, want %#v", restored, index)
	}

	path := filepath.Join(runDir, ArtifactFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("\n{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(runDir); err == nil {
		t.Fatal("Read accepted trailing artifact data")
	}
}
