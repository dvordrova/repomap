package analysistarget

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestMergeFileCandidatesOnlyUnionsHypothesesByExactFileRef(t *testing.T) {
	snapshot := hypothesisCorpus(t, "a.go", "b.go", "twenty.go")

	got, err := MergeFileCandidates(snapshot,
		[]FileCandidate{
			{FileRef: "f2", Hypotheses: []string{"contains main", "registers worker"}},
			{FileRef: "f1", Hypotheses: []string{"exports public names"}},
		},
		[]FileCandidate{
			{FileRef: "f2", Hypotheses: []string{"README says to run it", "contains main"}},
			{FileRef: "f3", Hypotheses: []string{"separate file"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileCandidate{
		{FileRef: "f1", Hypotheses: []string{"exports public names"}},
		{FileRef: "f2", Hypotheses: []string{"README says to run it", "contains main", "registers worker"}},
		{FileRef: "f3", Hypotheses: []string{"separate file"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %#v, want %#v", got, want)
	}
}

func TestMergeFileCandidatesDoesNotFuzzyMatchFileRefs(t *testing.T) {
	paths := make([]string, 20)
	for index := range paths {
		paths[index] = strings.Repeat("x", index+1) + ".go"
	}
	snapshot := hypothesisCorpus(t, paths...)

	got, err := MergeFileCandidates(snapshot,
		[]FileCandidate{{FileRef: "f2", Hypotheses: []string{"two"}}},
		[]FileCandidate{{FileRef: "f20", Hypotheses: []string{"twenty"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].FileRef != "f2" || got[1].FileRef != "f20" {
		t.Fatalf("exact refs were merged: %#v", got)
	}
}

func TestMergeFileCandidatesRejectsUnknownRefsAndBadHypotheses(t *testing.T) {
	snapshot := hypothesisCorpus(t, "main.go")
	tests := []FileCandidate{
		{FileRef: "f2", Hypotheses: []string{"unknown"}},
		{FileRef: "f1", Hypotheses: nil},
		{FileRef: "f1", Hypotheses: []string{" padded "}},
		{FileRef: "f1", Hypotheses: []string{"line\nbreak"}},
	}
	for _, candidate := range tests {
		if _, err := MergeFileCandidates(snapshot, []FileCandidate{candidate}); err == nil {
			t.Fatalf("candidate %#v was accepted", candidate)
		}
	}
}

func TestMergeFileCandidatesAllowsEmptyParallelResults(t *testing.T) {
	got, err := MergeFileCandidates(hypothesisCorpus(t, "main.go"), nil, []FileCandidate{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty result = %#v", got)
	}
}

func hypothesisCorpus(t *testing.T, paths ...string) corpus.Snapshot {
	t.Helper()
	repository := t.TempDir()
	for _, filePath := range paths {
		if err := os.WriteFile(repository+"/"+filePath, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	listing := gitfiles.Listing{Paths: paths, RegularPaths: paths}
	opened, err := corpus.New(context.Background(), repository, listing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened.Snapshot()
}
