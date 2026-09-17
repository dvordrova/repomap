package programindex

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReaderReadsSealedIndexesWithoutBuilderArtifacts(t *testing.T) {
	input := representativeInput()
	first, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	shared := ShareInput(input)
	secondTarget := input.Target
	secondTarget.Name, secondTarget.Selector = "second launch", "other launch"
	secondInput := shared.ForTarget(secondTarget)
	second, err := New(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	factsDir := filepath.Join(root, "program-facts")
	store := NewArtifactStore()
	for position, pair := range []struct {
		index Index
		input Input
	}{{first, shared.ForTarget(input.Target)}, {second, secondInput}} {
		runDir := filepath.Join(root, compactID("run", position+1))
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.Persist(runDir, pair.index, pair.input); err != nil {
			t.Fatal(err)
		}
		got, err := new(FileReader).ReadFile(filepath.Join(runDir, ArtifactFilename))
		if err != nil {
			t.Fatal(err)
		}
		wantBytes, _ := Encode(pair.index)
		gotBytes, _ := Encode(got)
		if !bytes.Equal(wantBytes, gotBytes) {
			t.Fatal("persisted ProgramIndex is not the sealed graph")
		}
	}
	if _, err := os.Stat(factsDir); !os.IsNotExist(err) {
		t.Fatalf("builder artifact directory exists: %v", err)
	}
}
