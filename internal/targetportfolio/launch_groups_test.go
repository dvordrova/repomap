package targetportfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

type launchPortfolioProvider struct {
	exhaustivePortfolioProvider
	refuseFirst bool
}

func (p *launchPortfolioProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request Request
	if err := decodePromptRequest(string(prepared.Bytes()), "Exact bounded classification-batch JSON:\n", "\n\nEnd of quoted classification-batch JSON.", &request); err != nil {
		return llm.Completion{}, err
	}
	response := Response{DefaultFileRef: &request.Candidates[0].FileRef}
	for _, file := range request.Candidates {
		response.TargetFileRefs = append(response.TargetFileRefs, file.FileRef)
	}
	for _, row := range request.NativeTargets {
		response.NativeDecisions = append(response.NativeDecisions, NativeDecision{Ref: row.Ref, Decision: "standalone"})
	}
	for _, group := range request.LaunchGroups {
		owner := group.OwnerRefs[0]
		if group.Ref == "g1" && p.refuseFirst {
			owner = "unknown"
		}
		response.LaunchDecisions = append(response.LaunchDecisions, NativeLaunchDecision{Ref: group.Ref, Owner: owner})
	}
	wire, err := json.Marshal(response)
	return llm.Completion{Response: wire, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1, ProviderResponseBytes: len(wire)}}, err
}

func TestLaunchGroupFinalRestorationRetainsChoiceAndIndependentRefusals(t *testing.T) {
	for _, refused := range []bool{false, true} {
		execution, err := Run(t.Context(), llm.Executor{Enabled: false}, &launchPortfolioProvider{refuseFirst: refused}, launchGroupCompilation(t))
		if err != nil {
			t.Fatal(err)
		}
		for i, placement := range execution.Selection.Placements[:3] {
			if refused {
				if placement.Decision != "standalone" || !strings.Contains(placement.Reason, "launch-group owner") {
					t.Fatalf("group refusal repaired or lost: %+v", placement)
				}
			} else if placement.LaunchDecision == nil || placement.LaunchDecision.Owner != "t1" || (i > 0 && placement.Decision != "seed_of:t1") {
				t.Fatalf("positive group choice lost: %+v", placement)
			}
		}
		if execution.Selection.Placements[4].Decision != "seed_of:t4" || execution.Selection.Placements[4].Reason != "" {
			t.Fatal("independent group damaged by final restoration")
		}
	}
}

func launchGroupCompilation(t *testing.T) Compilation {
	t.Helper()
	snapshot := testSnapshot(t, []string{"main.py", "__main__.py", "worker.py", "worker_main.py", "library.py"})
	rows := []NativeCandidate{
		{Ref: "t1", FileRef: "f1", Language: "python", Kind: "executable", Name: "app"},
		{Ref: "t2", FileRef: "f2", Language: "python", Kind: "executable", Name: "python -m app"},
		{Ref: "t3", FileRef: "f1", Language: "python", Kind: "executable", Name: "direct app"},
		{Ref: "t4", FileRef: "f3", Language: "python", Kind: "executable", Name: "worker"},
		{Ref: "t5", FileRef: "f4", Language: "python", Kind: "executable", Name: "worker form"},
		{Ref: "t6", FileRef: "f5", Language: "python", Kind: "library", Name: "library"},
	}
	for _, group := range [][]int{{0, 1, 2}, {3, 4}} {
		for _, member := range group {
			rows[member].Evidence = []Observation{{Kind: "launch_callable", Path: fmt.Sprintf("app%d.py", group[0]), Line: 12, Values: []string{"app", "main", "arguments=none"}}, {Kind: "launch_call_site", Path: fmt.Sprintf("launcher%d.py", member), Line: member + 1}}
			for _, owner := range group {
				if owner != member {
					rows[member].SeedOwners = append(rows[member].SeedOwners, NativeOwner{Ref: rows[owner].Ref, Name: rows[owner].Name, Kind: "executable", SameLaunch: true})
				}
			}
		}
	}
	var files []Candidate
	var required []corpus.FileID
	for i := 1; i <= 5; i++ {
		ref := corpus.FileID(fmt.Sprintf("f%d", i))
		files = append(files, Candidate{FileRef: ref, Hypotheses: []string{"native"}})
		required = append(required, ref)
	}
	c, err := CompileWithNativeAuthority(snapshot, files, required, rows)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLaunchGroupOwnerDecisionRestoresEveryOriginalMember(t *testing.T) {
	c := launchGroupCompilation(t)
	if len(c.Request.LaunchGroups) != 2 || !slices.Equal(c.Request.LaunchGroups[0].Members, []string{"t1", "t2", "t3"}) {
		t.Fatalf("different launch callables combined or forms lost: %+v", c.Request.LaunchGroups)
	}
	for _, tc := range []struct {
		name, choices, members string
		want                   []string
		refused                bool
	}{
		{"owner", `[{"ref":"g1","owner":"t1"}]`, `{"ref":"t2","decision":"standalone"}`, []string{"standalone", "seed_of:t1", "seed_of:t1"}, false},
		{"other advertised owner", `[{"ref":"g1","owner":"t3"}]`, `{"ref":"t2","decision":"standalone"}`, []string{"seed_of:t3", "seed_of:t3", "standalone"}, false},
		{"missing ignores independent seeds", `[]`, `{"ref":"t1","decision":"standalone"},{"ref":"t2","decision":"seed_of:t1"},{"ref":"t3","decision":"seed_of:t1"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"unknown group ignored", `[{"ref":"unknown","owner":"t1"}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"unknown owner", `[{"ref":"g1","owner":"outside"}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"other group's owner", `[{"ref":"g1","owner":"t4"}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"malformed choice", `[{"ref":"g1","owner":8}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"conflicting choice", `[{"ref":"g1","owner":"t1"},{"ref":"g1","owner":"t2"}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "standalone", "standalone"}, true},
		{"identical repeated choice", `[{"ref":"g1","owner":"t1"},{"ref":"g1","owner":"t1"}]`, `{"ref":"t1","decision":"standalone"}`, []string{"standalone", "seed_of:t1", "seed_of:t1"}, false},
		{"separate utility forms retain roles", `[{"ref":"g1","owner":"separate"}]`, `{"ref":"t1","decision":"tool"},{"ref":"t2","decision":"example"},{"ref":"t3","decision":"standalone"}`, []string{"tool", "example", "standalone"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			choices := strings.TrimSuffix(tc.choices, "]")
			if choices != "[" {
				choices += ","
			}
			choices += `{"ref":"g2","owner":"t4"}]`
			raw := fmt.Sprintf(`{"default_file_ref":"f1","target_file_refs":["f1"],"native_decisions":[%s,{"ref":"t6","decision":"shared_code"}],"launch_decisions":%s}`, tc.members, choices)
			result, err := ResolveResponse(c, []byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Placements) != 6 {
				t.Fatal("original native target lost")
			}
			for i, placement := range result.Placements[:3] {
				if placement.Decision != tc.want[i] || (placement.Reason != "") != tc.refused || len(placement.Candidate.Evidence) != 2 {
					t.Fatalf("member %d: %+v", i, placement)
				}
				if !tc.refused && placement.LaunchDecision == nil {
					t.Fatal("positive group choice not journaled")
				}
			}
			if result.Placements[3].Decision != "standalone" || result.Placements[4].Decision != "seed_of:t4" || result.Placements[5].Decision != "shared_code" || result.Placements[4].Reason != "" {
				t.Fatal("refused group damaged independent native decisions or group")
			}
		})
	}
}

func TestLaunchGroupProviderPartitionsRetainCompleteMembersAndEvidence(t *testing.T) {
	c := launchGroupCompilation(t)
	batches, err := classificationBatchesWithFit(c, func(wire []byte) (bool, error) {
		var request Request
		if err := json.Unmarshal(wire, &request); err != nil {
			return false, err
		}
		return len(request.Candidates) <= 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 {
		t.Fatalf("batches=%d", len(batches))
	}
	seen := make(map[string]int)
	for _, batch := range batches {
		request := batch.compilation.Request
		for _, row := range request.NativeTargets {
			seen[row.Ref]++
		}
		for _, group := range request.LaunchGroups {
			for _, member := range group.Members {
				if !slices.ContainsFunc(request.NativeTargets, func(row NativeCandidate) bool { return row.Ref == member }) {
					t.Fatal("group member split away")
				}
			}
			if !slices.ContainsFunc(request.Observations, func(row NamedObservation) bool { return row.Ref == group.CallableRef && row.Kind == "launch_callable" }) {
				t.Fatal("group callable anchor missing")
			}
		}
	}
	for _, row := range c.native {
		if seen[row.Ref] != 1 {
			t.Fatalf("%s processed %d times", row.Ref, seen[row.Ref])
		}
	}
	if _, err := compileSubset(c, c.candidates[:1]); err == nil {
		t.Fatal("partial launch-group authority accepted")
	}
	if _, err := classificationBatchesWithFit(c, func([]byte) (bool, error) { return false, nil }); err == nil || !strings.Contains(err.Error(), "indivisible") {
		t.Fatal("indivisible native group silently split")
	}
	c.Request.LaunchGroups[0].Members[0] = "t6"
	if validateCompilation(c) == nil {
		t.Fatal("modified group authority accepted")
	}
}
