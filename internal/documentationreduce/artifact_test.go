package documentationreduce

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestArtifactRoundTripPersistsExactCanonicalReduction(t *testing.T) {
	result := artifactTestResult(t)
	encoded, err := Encode(result)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	restored, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(restored, result) {
		t.Fatalf("decoded reduction = %#v, want %#v", restored, result)
	}

	runDir := t.TempDir()
	if err := Persist(runDir, result); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	persisted, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(persisted, encoded) {
		t.Fatalf("persisted bytes = %s, want %s", persisted, encoded)
	}
	read, err := Read(runDir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(read, result) {
		t.Fatalf("read reduction = %#v, want %#v", read, result)
	}
}

func TestDecodeRejectsCorruptAndNonCanonicalArtifacts(t *testing.T) {
	result := artifactTestResult(t)
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	brokenDigest := append([]byte(nil), encoded...)
	digestOffset := bytes.Index(brokenDigest, []byte(result.ReductionSHA256))
	if digestOffset < 0 {
		t.Fatal("encoded artifact does not contain its reduction digest")
	}
	brokenDigest[digestOffset] = '0'
	if brokenDigest[digestOffset] == encoded[digestOffset] {
		brokenDigest[digestOffset] = '1'
	}

	tests := map[string][]byte{
		"empty":          nil,
		"broken seal":    brokenDigest,
		"unknown field":  append([]byte(`{"unexpected":true,`), encoded[1:]...),
		"retired claims": []byte(strings.Replace(string(encoded), `"concepts":[`, `"claims":["Retired."],"concepts":[`, 1)),
		"trailing value": append(append([]byte(nil), encoded...), []byte(`{}`)...),
		"non-canonical":  append(append([]byte(nil), encoded...), '\n'),
	}
	for name, artifact := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(artifact); err == nil {
				t.Fatalf("Decode accepted %s artifact: %s", name, artifact)
			}
		})
	}
}

func TestPersistAndReadOwnReductionMemory(t *testing.T) {
	result := artifactTestResult(t)
	runDir := t.TempDir()
	if err := Persist(runDir, result); err != nil {
		t.Fatal(err)
	}

	result.Sources[0].Concepts[0] = "mutated caller"
	first, err := Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Sources[0].Concepts[0] == "mutated caller" {
		t.Fatal("persisted reduction aliases caller memory")
	}
	first.Sources[0].Concepts[0] = "mutated read"
	second, err := Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Sources[0].Concepts[0] == "mutated read" {
		t.Fatal("Read results alias each other")
	}
}

func TestSealCapsConceptsAndValidateRejectsAnOverfullSource(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{{
		Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: "Vocabulary.\n",
	}})
	concepts := make([]string, 0, MaxConceptsPerSource+3)
	for index := range MaxConceptsPerSource + 3 {
		concepts = append(concepts, fmt.Sprintf("Concept %02d", index))
	}
	result, err := sealResult(guidance, "", []Source{{
		Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Concepts: concepts,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || !reflect.DeepEqual(result.Sources[0].Concepts, concepts[:MaxConceptsPerSource]) {
		t.Fatalf("sealed concepts = %v, want the first %d", result.Sources[0].Concepts, MaxConceptsPerSource)
	}
	overfull := result
	overfull.Sources = []Source{{Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Concepts: concepts}}
	if err := overfull.Validate(); err == nil || !strings.Contains(err.Error(), "source 0 is invalid") {
		t.Fatalf("Validate accepted %d concepts: %v", len(concepts), err)
	}
}

func artifactTestResult(t *testing.T) Result {
	t.Helper()
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{
		{
			Path: "README.md", Kind: readmetargetscout.GuidanceReadme,
			Content: "The order service reconciles merchant orders.\n",
		},
	})
	result, err := sealResult(guidance, "Order service overview.", []Source{
		{
			Path: "README.md", Kind: readmetargetscout.GuidanceReadme,
			Concepts: []string{"Order service"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
