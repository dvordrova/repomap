package programindex

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReaderReusesOneSharedProjectionAndPreservesTargetBindings(t *testing.T) {
	input := representativeInput()
	// This fixture retains exact/possible relations, native seeds and owned
	// declarations; the shared reader must preserve the complete sealed value.
	first, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	shared := ShareInput(input)
	firstInput := shared.ForTarget(input.Target)
	secondTarget := input.Target
	secondTarget.Name, secondTarget.Selector = "second launch", "other launch"
	secondInput := shared.ForTarget(secondTarget)
	second, err := New(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.Target.ID == second.Target.ID || first.SHA256 == second.SHA256 {
		t.Fatal("fixture does not distinguish target bindings")
	}
	root := t.TempDir()
	store := NewArtifactStore(filepath.Join(root, "facts"))
	paths := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	for i, pair := range []struct {
		index Index
		input Input
	}{{first, firstInput}, {second, secondInput}} {
		if err := os.MkdirAll(paths[i], 0755); err != nil {
			t.Fatal(err)
		}
		if err := store.Persist(paths[i], pair.index, pair.input); err != nil {
			t.Fatal(err)
		}
	}
	var reader FileReader
	defer reader.Release()
	assertSame := func(want Index, filename string) Index {
		t.Helper()
		got, err := reader.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		wantBytes, err := Encode(want)
		if err != nil {
			t.Fatal(err)
		}
		gotBytes, err := Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wantBytes, gotBytes) {
			t.Fatal("shared read changed canonical target, seed, declaration or relation")
		}
		return got
	}
	firstRead := assertSame(first, filepath.Join(paths[0], ArtifactFilename))
	loadedFacts := reader.facts
	factsPath := reader.factsPath
	// Once a projection is owned, a sibling view does not need another file
	// read. Mutating the returned target index must not mutate that input.
	firstRead.Objects[0].Name = "changed only in consumer"
	if err := os.Remove(factsPath); err != nil {
		t.Fatal(err)
	}
	assertSame(second, filepath.Join(paths[1], ArtifactFilename))
	if reader.facts != loadedFacts {
		t.Fatal("sibling target reconstructed the common input")
	}

	otherInput := representativeInput()
	otherInput.SourceSHA256 = strings.Repeat("c", 64)
	other, err := newMeasuredProgramIndex(otherInput)
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(root, "other")
	if err := os.MkdirAll(otherPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Persist(otherPath, other, otherInput); err != nil {
		t.Fatal(err)
	}
	assertSame(other, filepath.Join(otherPath, ArtifactFilename))
	if reader.facts == loadedFacts || reader.factsPath == factsPath {
		t.Fatal("new projection retained the previous project")
	}
	reader.Release()
	if reader.facts != nil {
		t.Fatal("completed consumer retained shared projection")
	}
}
