package programindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactPersistenceAndInventoryHelpers(t *testing.T) {
	index, err := newMeasuredProgramIndex(shapeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	set, err := BuildArtifactSet(index)
	if err != nil {
		t.Fatalf("BuildArtifactSet: %v", err)
	}
	if len(set.Entries) != 1 || set.DefaultTargetID != index.Target.ID ||
		set.Entries[0].TargetID != index.Target.ID ||
		set.Entries[0].Filename != ArtifactFilename ||
		set.Entries[0].IndexSHA256 != index.SHA256 {
		t.Fatalf("page-local ProgramIndex set = %#v", set)
	}

	runDir := t.TempDir()
	if err := Persist(runDir, "", index); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if err := PersistArtifactSet(runDir, set); err != nil {
		t.Fatalf("PersistArtifactSet: %v", err)
	}
	indexBytes, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		t.Fatalf("read persisted index: %v", err)
	}
	restoredIndex, err := Decode(indexBytes)
	if err != nil {
		t.Fatalf("decode persisted index: %v", err)
	}
	if restoredIndex.SHA256 != index.SHA256 || restoredIndex.Target.ID != index.Target.ID {
		t.Fatal("persisted index changed exact authority")
	}
	setBytes, err := os.ReadFile(filepath.Join(runDir, ArtifactSetFilename))
	if err != nil {
		t.Fatalf("read persisted artifact set: %v", err)
	}
	restoredSet, err := DecodeArtifactSet(setBytes)
	if err != nil {
		t.Fatalf("decode persisted artifact set: %v", err)
	}
	if restoredSet.SHA256 != set.SHA256 || restoredSet.DefaultTargetID != set.DefaultTargetID {
		t.Fatal("persisted artifact set changed exact authority")
	}
}

func TestArtifactSetSealsOnePageLocalBinding(t *testing.T) {
	set, err := NewArtifactSet("python-target-api", []ArtifactSetEntry{
		{TargetID: "python-target-api", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := set.Entries[0].TargetID, "python-target-api"; got != want {
		t.Fatalf("page-local target = %q, want %q", got, want)
	}
	if got := len(set.SHA256); got != 64 {
		t.Fatalf("sha256 length = %d", got)
	}

	snapshot := set.Snapshot()
	snapshot.Entries[0].Filename = "changed.json"
	if set.Entries[0].Filename == "changed.json" {
		t.Fatal("Snapshot aliases entry storage")
	}
	if ArtifactSetFilename != "program-index-set.json" || ArtifactSetVersion != 1 {
		t.Fatalf("artifact identity drift: filename=%q version=%d", ArtifactSetFilename, ArtifactSetVersion)
	}
}
