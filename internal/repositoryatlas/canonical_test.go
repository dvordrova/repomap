package repositoryatlas

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCanonicalJSONIsDeterministicAndDoesNotMutateCaller(t *testing.T) {
	left := validAtlasFixture()
	left.Evidence = append(left.Evidence, Evidence{
		ID: "second-source", UnitID: "app", Location: evidence.Location{Path: "cmd/app/config.go", Line: 3},
		Provenance: evidence.Provenance{Provider: "gofacts", Operation: "typed_declaration"},
	})
	left.Observations[0].EvidenceRefs = []string{"source", "second-source"}
	left.Relations[0].EvidenceRefs = []string{"source", "second-source"}

	right := left
	right.Units = reversed(left.Units)
	right.Entities = reversed(left.Entities)
	right.Observations = reversed(left.Observations)
	right.Evidence = reversed(left.Evidence)
	right.Relations = reversed(left.Relations)
	right.Observations[1].EvidenceRefs = []string{"second-source", "source"}
	right.Relations[0].EvidenceRefs = []string{"second-source", "source"}
	before := cloneForMutationCheck(right)

	leftJSON, err := CanonicalJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := CanonicalJSON(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftJSON, rightJSON) {
		t.Fatalf("canonical JSON differs:\nleft:\n%s\nright:\n%s", leftJSON, rightJSON)
	}
	if !reflect.DeepEqual(right, before) {
		t.Fatal("CanonicalJSON mutated caller-owned slices")
	}
}

func TestCanonicalDeepCopiesProvenanceLocation(t *testing.T) {
	atlas := validAtlasFixture()
	atlas.Evidence[0].Provenance.Location = &evidence.Location{Path: "cmd/app/main.go", Line: 7}
	canonical, err := Canonical(atlas)
	if err != nil {
		t.Fatal(err)
	}
	canonical.Evidence[0].Provenance.Location.Path = "changed.go"
	if atlas.Evidence[0].Provenance.Location.Path != "cmd/app/main.go" {
		t.Fatal("Canonical retained a caller-owned provenance location pointer")
	}
}

func reversed[T any](values []T) []T {
	result := append([]T(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func cloneForMutationCheck(atlas Atlas) Atlas {
	cloned := atlas
	cloned.Units = append([]Unit(nil), atlas.Units...)
	cloned.Entities = append([]Entity(nil), atlas.Entities...)
	cloned.Observations = append([]Observation(nil), atlas.Observations...)
	for index := range cloned.Observations {
		cloned.Observations[index].EvidenceRefs = append([]string(nil), atlas.Observations[index].EvidenceRefs...)
	}
	cloned.Evidence = append([]Evidence(nil), atlas.Evidence...)
	cloned.Relations = append([]Relation(nil), atlas.Relations...)
	for index := range cloned.Relations {
		cloned.Relations[index].EvidenceRefs = append([]string(nil), atlas.Relations[index].EvidenceRefs...)
	}
	return cloned
}
