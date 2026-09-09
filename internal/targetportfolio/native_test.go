package targetportfolio

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestNativeDecisionsKeepSeparateTargetsOnOneFileAndValidateOwners(t *testing.T) {
	rows := []NativeCandidate{
		{Ref: "t1", FileRef: "f1", Language: "python", Kind: "library", Name: "package"},
		{Ref: "t2", FileRef: "f1", Language: "python", Kind: "executable", Name: "demo", SeedOwners: []NativeOwner{{Ref: "t1", Name: "package", Kind: "library"}}},
		{Ref: "t3", FileRef: "f2", Language: "python", Kind: "executable", Name: "worker", SeedOwners: []NativeOwner{{Ref: "t1", Name: "package", Kind: "library"}}},
	}
	for _, test := range []struct {
		name      string
		decisions []NativeDecision
		want      []string
		refused   int
	}{
		{"positive seed", []NativeDecision{{"t1", "standalone"}, {"t2", "seed_of:t1"}, {"t3", "standalone"}}, []string{"standalone", "seed_of:t1", "standalone"}, 0},
		{"missing keeps products", nil, []string{"standalone", "standalone", "standalone"}, 3},
		{"shared owner cannot absorb", []NativeDecision{{"t1", "shared_code"}, {"t2", "seed_of:t1"}, {"t3", "tool"}}, []string{"shared_code", "standalone", "tool"}, 1},
		{"chain cannot absorb", []NativeDecision{{"t1", "seed_of:t3"}, {"t2", "seed_of:t1"}, {"t3", "standalone"}}, []string{"standalone", "standalone", "standalone"}, 2},
		{"unknown and contradictory", []NativeDecision{{"t1", "standalone"}, {"t2", "seed_of:unknown"}, {"t3", "tool"}, {"t3", "example"}, {"unknown", "tool"}}, []string{"standalone", "standalone", "standalone"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := nativeDecisions(rows, test.decisions, true)
			refused := 0
			if len(got) != len(rows) {
				t.Fatal("native target lost")
			}
			for i, p := range got {
				if p.Decision != test.want[i] {
					t.Fatalf("%s = %s", p.Candidate.Ref, p.Decision)
				}
				if p.Reason != "" {
					refused++
				}
			}
			if refused != test.refused {
				t.Fatalf("refused %d, want %d", refused, test.refused)
			}
		})
	}
}

func TestNativeEvidenceIsOwnedAndRebuiltInEachWindow(t *testing.T) {
	snapshot := testSnapshot(t, []string{"package.py", "worker.py"})
	evidence := Observation{Kind: "documented_command", Path: "README.md", Line: 5, Values: []string{"python -m acme.worker"}}
	rows := []NativeCandidate{
		{Ref: "t1", FileRef: "f1", Language: "python", Kind: "library", Name: "package", Evidence: []Observation{evidence}},
		{Ref: "t2", FileRef: "f2", Language: "python", Kind: "executable", Name: "worker", Evidence: []Observation{evidence}, SeedOwners: []NativeOwner{{Ref: "t1", Name: "package", Kind: "library"}}},
	}
	c, err := CompileWithNativeAuthority(snapshot, []Candidate{{FileRef: "f1", Hypotheses: []string{"library"}}, {FileRef: "f2", Hypotheses: []string{"guard"}}}, []corpus.FileID{"f1", "f2"}, rows)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Evidence[0].Values[0] = "changed input"
	if err := validateCompilation(c); err != nil {
		t.Fatal(err)
	}
	if len(c.Request.Observations) != 1 || c.Request.Observations[0].Values[0] != "python -m acme.worker" {
		t.Fatal("evidence copied or not owned")
	}
	child, err := compileSubset(c, c.candidates[1:])
	if err != nil {
		t.Fatal(err)
	}
	if len(child.Request.NativeTargets) != 1 || child.Request.NativeTargets[0].Ref != "t2" || len(child.Request.Observations) != 1 || child.Request.NativeTargets[0].SeedOwners[0].Ref != "t1" {
		t.Fatal("child lost its complete evidence or closed owner")
	}
	raw := []byte(`{"default_file_ref":"f2","target_file_refs":["f2"],"native_decisions":[{"ref":"t2","decision":"standalone"}]}`)
	selection, err := ResolveResponse(child, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Placements) != 1 || selection.Placements[0].Candidate.Evidence[0].Line != 5 {
		t.Fatal("decision lost its original evidence")
	}
	encoded, _ := json.Marshal(child.Request)
	if strings.Count(string(encoded), "python -m acme.worker") != 1 {
		t.Fatal("window duplicated source quotation")
	}
	child.Request.Observations[0].Values[0] = "mutated wire"
	if validateCompilation(child) == nil {
		t.Fatal("mutated source authority accepted")
	}
}
