package guidedtour

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestPlanLeafTasksDeterministicAndBounded(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	tasks, err := PlanLeafTasks(bundle, MaxLeafTasks)
	if err != nil {
		t.Fatalf("PlanLeafTasks() error = %v", err)
	}
	if len(tasks) != 5 {
		t.Fatalf("PlanLeafTasks() count = %d, want 5", len(tasks))
	}
	if tasks[0].Kind != LeafFlow || tasks[1].Kind != LeafMechanism {
		t.Fatalf("PlanLeafTasks() candidate kinds = %q, %q", tasks[0].Kind, tasks[1].Kind)
	}
	for index, task := range tasks[2:] {
		if task.Kind != LeafComponent {
			t.Fatalf("PlanLeafTasks() task[%d] kind = %q, want component", index+2, task.Kind)
		}
		if task.FocusComponentID == "" || len(task.Candidate.Beats) == 0 || len(task.Candidate.Gaps) != 0 {
			t.Fatalf("PlanLeafTasks() component task[%d] = %#v", index+2, task)
		}
		for _, beat := range task.Candidate.Beats {
			if !containsString(beat.ComponentIDs, task.FocusComponentID) {
				t.Fatalf("PlanLeafTasks() component task includes unrelated beat %q", beat.ID)
			}
		}
		if err := task.Validate(); err != nil {
			t.Fatalf("PlanLeafTasks() task[%d] validation error = %v", index+2, err)
		}
	}

	prefix, err := PlanLeafTasks(bundle, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix, tasks[:3]) {
		t.Fatalf("PlanLeafTasks() maxTasks does not return a deterministic prefix")
	}
	overLimit, err := PlanLeafTasks(bundle, MaxLeafTasks+10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(overLimit, tasks) {
		t.Fatalf("PlanLeafTasks() did not enforce package task bound")
	}
	if _, err := PlanLeafTasks(bundle, 0); err == nil {
		t.Fatalf("PlanLeafTasks() accepted a zero task bound")
	}

	reordered := cloneFanoutBundle(t, bundle)
	reverseFanoutCandidates(reordered.Candidates)
	reverseFanoutComponents(reordered.Components)
	for index := range reordered.Candidates {
		reverseFanoutBeats(reordered.Candidates[index].Beats)
		for beatIndex := range reordered.Candidates[index].Beats {
			reverseFanoutStrings(reordered.Candidates[index].Beats[beatIndex].ComponentIDs)
			reverseFanoutEvidence(reordered.Candidates[index].Beats[beatIndex].Evidence)
		}
	}
	reorderedTasks, err := PlanLeafTasks(reordered, MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reorderedTasks, tasks) {
		t.Fatalf("PlanLeafTasks() changed after equivalent input reordering")
	}
}

func TestLeafTaskHashCanonical(t *testing.T) {
	t.Parallel()

	tasks, err := PlanLeafTasks(fanoutTestBundle(), MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	task := tasks[0]
	reordered := cloneLeafTask(t, task)
	reverseFanoutComponents(reordered.Components)
	reverseFanoutBeats(reordered.Candidate.Beats)
	for index := range reordered.Candidate.Beats {
		reverseFanoutStrings(reordered.Candidate.Beats[index].ComponentIDs)
		reverseFanoutEvidence(reordered.Candidate.Beats[index].Evidence)
	}
	firstHash, firstJSON, err := LeafTaskHash(task)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, secondJSON, err := LeafTaskHash(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("LeafTaskHash() is not canonical")
	}

	changed := cloneLeafTask(t, task)
	changed.Candidate.Beats[0].Detail = "A different supplied local detail"
	changedHash, _, err := LeafTaskHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == firstHash {
		t.Fatalf("LeafTaskHash() ignored changed local facts")
	}
}

func TestBuildLeafPromptContainsDirectLocalEvidence(t *testing.T) {
	t.Parallel()

	tasks, err := PlanLeafTasks(fanoutTestBundle(), MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildLeafPrompt(tasks[0])
	if err != nil {
		t.Fatalf("BuildLeafPrompt() error = %v", err)
	}
	if prompt.Version != LeafPromptVersion {
		t.Fatalf("BuildLeafPrompt() version = %q", prompt.Version)
	}
	for _, expected := range []string{
		tasks[0].ID,
		"evidence-flow-1",
		"cmd/server.go",
		`"observations"`,
		`"support_ids"`,
		`"candidate_connection"`,
		`"target_id"`,
		`"relation": "needs_combination"`,
		`"missing_evidence"`,
		"global supported or insufficient verdict",
	} {
		if !strings.Contains(prompt.User, expected) {
			t.Errorf("BuildLeafPrompt() missing %q", expected)
		}
	}

	second, err := BuildLeafPrompt(tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	firstPrefix, firstSuffix, found := strings.Cut(prompt.User, leafPromptTaskMarker)
	if !found {
		t.Fatalf("BuildLeafPrompt() user prompt has no task marker")
	}
	secondPrefix, secondSuffix, found := strings.Cut(second.User, leafPromptTaskMarker)
	if !found {
		t.Fatalf("BuildLeafPrompt() second user prompt has no task marker")
	}
	if prompt.System != second.System || firstPrefix != secondPrefix || firstPrefix != leafPromptUserPrefix {
		t.Fatalf("BuildLeafPrompt() changed its stable common prefix between tasks")
	}
	if firstSuffix == secondSuffix {
		t.Fatalf("BuildLeafPrompt() task JSON suffix did not vary between tasks")
	}
}

func TestFanoutPromptsExcludeModelProducedSemanticSummaries(t *testing.T) {
	t.Parallel()

	const (
		candidatePoison  = "MODEL_CANDIDATE_SUMMARY_POISON"
		componentPoison  = "MODEL_COMPONENT_SUMMARY_POISON"
		fileReasonPoison = "MODEL_FILE_REASON_POISON"
		gapReasonPoison  = "MODEL_GAP_REASON_POISON"
	)
	bundle := cloneFanoutBundle(t, fanoutTestBundle())
	for index := range bundle.Components {
		bundle.Components[index].Name = componentPoison
		bundle.Components[index].Description = componentPoison
	}
	for index := range bundle.Candidates {
		bundle.Candidates[index].Name = candidatePoison
		bundle.Candidates[index].Trigger = candidatePoison
		bundle.Candidates[index].Summary = candidatePoison
		if bundle.Candidates[index].Kind == CandidateSuggestedDirection {
			bundle.Candidates[index].Beats[0].Kind = "file"
			bundle.Candidates[index].Beats[0].Label = "main.go"
			bundle.Candidates[index].Beats[0].Detail = fileReasonPoison
			bundle.Candidates[index].Beats[0].Evidence[0].Label = "cmd/main.go"
			bundle.Candidates[index].Beats[0].Evidence[0].Location = &evidence.Location{
				Path: "cmd/main.go",
				Line: 12,
			}
			bundle.Candidates[index].Gaps[0].Label = gapReasonPoison
			bundle.Candidates[index].Gaps[0].Detail = gapReasonPoison
		}
	}

	tasks, err := PlanLeafTasks(bundle, MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LeafResult, 0, len(tasks))
	checkedDirectionFile := false
	for _, task := range tasks {
		prompt, promptErr := BuildLeafPrompt(task)
		if promptErr != nil {
			t.Fatal(promptErr)
		}
		for _, poison := range []string{
			candidatePoison,
			componentPoison,
			fileReasonPoison,
			gapReasonPoison,
		} {
			if strings.Contains(prompt.User, poison) {
				t.Fatalf("BuildLeafPrompt() leaked model-produced semantic text %q", poison)
			}
		}
		if task.Kind == LeafMechanism {
			for _, beat := range task.Candidate.Beats {
				if beat.Kind != "file" {
					continue
				}
				checkedDirectionFile = true
				if beat.Label != "main.go" || beat.Detail != neutralDirectionFileDetail ||
					len(beat.ComponentIDs) == 0 || len(beat.Evidence) == 0 ||
					beat.Evidence[0].Location == nil ||
					beat.Evidence[0].Location.Path != "cmd/main.go" {
					t.Fatalf("PlanLeafTasks() did not preserve exact file evidence with neutral prose: %#v", beat)
				}
			}
			if len(task.Candidate.Gaps) == 0 ||
				task.Candidate.Gaps[0].Label != neutralDirectionGapLabel ||
				task.Candidate.Gaps[0].Detail != neutralDirectionGapDetail {
				t.Fatalf("PlanLeafTasks() did not neutralize suggested-direction gap prose")
			}
		}
		if task.Kind == LeafFlow || task.Kind == LeafMechanism {
			results = append(results, LeafResult{Task: task, Artifact: fanoutTestArtifact(task)})
		}
	}
	if !checkedDirectionFile {
		t.Fatalf("PlanLeafTasks() produced no suggested-direction file beat")
	}

	prompt, err := BuildFanInPrompt(bundle, results)
	if err != nil {
		t.Fatal(err)
	}
	for _, poison := range []string{
		candidatePoison,
		componentPoison,
		fileReasonPoison,
		gapReasonPoison,
	} {
		if strings.Contains(prompt.User, poison) {
			t.Fatalf("BuildFanInPrompt() leaked model-produced semantic text %q", poison)
		}
	}
}

func TestParseLeafArtifactStrictJSON(t *testing.T) {
	t.Parallel()

	task := fanoutCandidateTasks(t)[0]
	valid, err := json.Marshal(fanoutTestArtifact(task))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		raw     []byte
		wantErr bool
	}{
		{name: "valid", raw: valid},
		{
			name:    "unknown field",
			raw:     append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"extra":true}`)...),
			wantErr: true,
		},
		{name: "trailing json", raw: append(append([]byte{}, valid...), []byte(` {}`)...), wantErr: true},
		{name: "fenced json", raw: append(append([]byte("```json\n"), valid...), []byte("\n```")...), wantErr: true},
		{name: "empty", raw: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseLeafArtifact(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseLeafArtifact() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFanoutParsersUseSharedResponseEnvelopeAndTypedTerminalLimit(t *testing.T) {
	task := fanoutCandidateTasks(t)[0]
	leaf, err := json.Marshal(fanoutTestArtifact(task))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLeafArtifact(append(bytes.Repeat([]byte(" "), (32<<10)+1), leaf...)); err != nil {
		t.Fatalf("leaf above former stage cap rejected: %v", err)
	}
	assertGuidedTourFanoutResponseLimit(
		t,
		"guided_tour_leaf",
		maxLeafArtifactBytes,
		func(raw []byte) error {
			_, err := ParseLeafArtifact(raw)
			return err
		},
	)

	fanIn, err := json.Marshal(FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictInsufficientEvidence,
		Explanation: "The supplied facts do not establish a story", Proposal: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFanInArtifact(append(bytes.Repeat([]byte(" "), (96<<10)+1), fanIn...)); err != nil {
		t.Fatalf("fan-in above former stage cap rejected: %v", err)
	}
	assertGuidedTourFanoutResponseLimit(
		t,
		"guided_tour_fan_in",
		maxFanInArtifactBytes,
		func(raw []byte) error {
			_, err := ParseFanInArtifact(raw)
			return err
		},
	)
}

func assertGuidedTourFanoutResponseLimit(
	t *testing.T,
	stage string,
	limit int,
	parse func([]byte) error,
) {
	t.Helper()
	observed := limit + 1
	err := parse(bytes.Repeat([]byte("x"), observed))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) ||
		limitErr.Stage != stage ||
		limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != limit ||
		limitErr.Observed != observed ||
		!limitErr.ObservedKnown {
		t.Fatalf("terminal response limit = %#v", err)
	}
}

func TestValidateLeafArtifact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*LeafTask, *LeafArtifact)
		wantErr string
	}{
		{name: "valid"},
		{name: "valid missing only", mutate: func(task *LeafTask, artifact *LeafArtifact) {
			*artifact = fanoutTestMissingOnlyArtifact(*task)
		}},
		{name: "wrong task", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.TaskID = "leaf-missing"
		}, wantErr: "task id does not match"},
		{name: "wrong candidate", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateID = "missing"
		}, wantErr: "candidate id does not match"},
		{name: "no usable partial", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations = nil
			artifact.CandidateConnection.SupportIDs = nil
			artifact.MissingEvidence = nil
		}, wantErr: "no usable partial"},
		{name: "observation without support", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].SupportIDs = nil
			artifact.CandidateConnection.SupportIDs = nil
		}, wantErr: "no support ids"},
		{name: "unknown observation support", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].SupportIDs[0] = "missing"
			artifact.CandidateConnection.SupportIDs[0] = "missing"
		}, wantErr: "unknown support"},
		{name: "observation cannot hide missing evidence", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "The supplied beat does not establish runtime order"
		}, wantErr: "must be an affirmative direct fact"},
		{name: "observation cannot carry a limitation", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "Runtime order remains unresolved"
		}, wantErr: "must be an affirmative direct fact"},
		{name: "observation cannot rename failed support", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "The supplied beat fails to establish runtime order"
		}, wantErr: "must be an affirmative direct fact"},
		{name: "observation cannot rename absent support", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "Evidence is absent for runtime order"
		}, wantErr: "must be an affirmative direct fact"},
		{name: "connection candidate mismatch", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.CandidateID = "missing"
		}, wantErr: "connection candidate id does not match"},
		{name: "connection unknown target", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.TargetID = "missing"
		}, wantErr: "unknown target"},
		{name: "connection wrong relation", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.Relation = "supports"
		}, wantErr: "must be needs_combination"},
		{name: "connection support mismatch", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.SupportIDs = nil
		}, wantErr: "do not match observation support"},
		{name: "missing evidence without ids", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.MissingEvidence = []LeafMissingEvidence{{Explanation: "Evidence is not supplied"}}
		}, wantErr: "has no exact ids"},
		{name: "missing evidence unknown beat", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.MissingEvidence = []LeafMissingEvidence{{
				Explanation: "Evidence is not supplied", BeatIDs: []string{"missing"},
			}}
		}, wantErr: "unknown beat id"},
		{name: "suggested observation unqualified behavior", mutate: func(task *LeafTask, artifact *LeafArtifact) {
			*task = fanoutCandidateTasks(t)[1]
			*artifact = fanoutTestArtifact(*task)
			artifact.Observations[0].Explanation = "The entry dispatches work"
		}, wantErr: "unsupported behavioral assertion"},
		{name: "tentative connection may describe behavior needing combination", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.Explanation = "This connection dispatches work"
		}},
		{name: "tentative connection remains non-authoritative despite mixed clauses", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.CandidateConnection.Explanation = "This does not call work and dispatches another request"
		}},
		{name: "supplied path in internal observation prose", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "The supplied cmd/server.go file is one exact static anchor"
		}, wantErr: "path-like reference"},
		{name: "invented path in observation prose", mutate: func(_ *LeafTask, artifact *LeafArtifact) {
			artifact.Observations[0].Explanation = "The supplied missing.go file is one exact static anchor"
		}, wantErr: "path-like reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := fanoutCandidateTasks(t)[0]
			artifact := fanoutTestArtifact(task)
			if tt.mutate != nil {
				tt.mutate(&task, &artifact)
			}
			err := ValidateLeafArtifact(task, artifact)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateLeafArtifact() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateLeafArtifact() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLeafArtifactRejectsEpistemicObservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		explanation string
	}{
		{name: "inadequate", explanation: "The evidence is inadequate to establish runtime order"},
		{name: "weak", explanation: "The evidence is weak regarding runtime order"},
		{name: "ambiguous", explanation: "The evidence is ambiguous about runtime order"},
		{name: "inconclusive", explanation: "The evidence is inconclusive about runtime order"},
		{name: "modal may", explanation: "This beat may establish runtime order"},
		{name: "modal could", explanation: "This beat could establish runtime order"},
		{name: "modal will", explanation: "This beat will establish runtime order"},
		{name: "suggestive", explanation: "This beat suggests a runtime transition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := fanoutCandidateTasks(t)[0]
			artifact := fanoutTestArtifact(task)
			artifact.Observations[0].Explanation = tt.explanation
			err := ValidateLeafArtifact(task, artifact)
			if err == nil || !strings.Contains(err.Error(), "must be an affirmative direct fact") {
				t.Fatalf("ValidateLeafArtifact() error = %v", err)
			}
		})
	}
}

func TestNormalizeLeafArtifactRedactsRepositoryReferencesBeforeValidation(t *testing.T) {
	t.Parallel()

	task := fanoutCandidateTasks(t)[0]
	artifact := fanoutTestArtifact(task)
	artifact.Observations[0].Explanation = "README.md, go.mod, cmd/server.go, and invented/missing.go are static anchors"
	artifact.CandidateConnection.Explanation = "Makefile and cmd/server.go need combination"
	artifact.MissingEvidence = []LeafMissingEvidence{{
		Explanation: "README.md and invented/missing.go do not establish runtime order",
		BeatIDs:     []string{task.Candidate.Beats[0].ID},
	}}

	normalized := NormalizeLeafArtifact(artifact)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{".go", "invented/", "README.md", "go.mod", "Makefile"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("NormalizeLeafArtifact() retained %q: %s", forbidden, encoded)
		}
	}
	if err := ValidateLeafArtifact(task, normalized); err != nil {
		t.Fatalf("normalized artifact failed validation: %v", err)
	}
}

func TestBuildFanInPromptIncludesMissingOnlyPartialAndExactFacts(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	tasks := fanoutCandidateTasks(t)
	results := []LeafResult{
		{Task: tasks[0], Artifact: fanoutTestArtifact(tasks[0])},
		{Task: tasks[1], Artifact: fanoutTestMissingOnlyArtifact(tasks[1])},
	}
	prompt, err := BuildFanInPrompt(bundle, results)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"considered_candidate_ids":["candidate-flow","candidate-mechanism"]`,
		`"story_candidate_ids":["candidate-flow"]`,
		`"observation_support"`, `"missing_evidence"`, `"verdict"`,
		"supported or mixed or insufficient_evidence", "The exact entry accepts input",
		"evidence-flow-1", neutralComponentDescription,
	} {
		if !strings.Contains(prompt.User, expected) {
			t.Errorf("BuildFanInPrompt() missing %q", expected)
		}
	}
	_, payloadJSON, found := strings.Cut(prompt.User, fanInPromptPayloadMarker)
	if !found {
		t.Fatal("BuildFanInPrompt() has no payload marker")
	}
	var payload fanInPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.LocalFacts.Candidates) != 2 || len(payload.Leaves) != 2 {
		t.Fatalf("missing-only candidate was dropped from fan-in payload: %#v", payload)
	}
	reversedPrompt, err := BuildFanInPrompt(bundle, []LeafResult{results[1], results[0]})
	if err != nil || !reflect.DeepEqual(prompt, reversedPrompt) {
		t.Fatalf("BuildFanInPrompt() depends on arrival order: %v", err)
	}
	if _, err := BuildFanInPrompt(bundle, results[1:]); err != nil {
		t.Fatalf("BuildFanInPrompt() rejected usable missing-only partial: %v", err)
	}
}

func TestFanInArtifactVerdictAndObservationSupport(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	tasks := fanoutCandidateTasks(t)
	results := []LeafResult{
		{Task: tasks[0], Artifact: fanoutTestArtifact(tasks[0])},
		{Task: tasks[1], Artifact: fanoutTestMissingOnlyArtifact(tasks[1])},
	}
	flow, _ := findCandidate(bundle, tasks[0].CandidateID)
	proposal := fanoutTestProposal(flow)
	valid := FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictMixed,
		Explanation: "The exact observations are useful while a supplied gap remains unresolved",
		Proposal:    &proposal,
		StepSupport: fanoutTestStepSupport(tasks[0], proposal),
	}
	if err := ValidateFanInArtifact(bundle, results, valid); err != nil {
		t.Fatalf("ValidateFanInArtifact() error = %v", err)
	}
	supported := valid
	supported.Verdict = FanInVerdictSupported
	if err := ValidateFanInArtifact(bundle, results, supported); err == nil ||
		!strings.Contains(err.Error(), "cannot discard selected-candidate missing evidence") {
		t.Fatalf("supported missing-evidence verdict error = %v", err)
	}
	withoutLineage := valid
	withoutLineage.StepSupport = nil
	if err := ValidateFanInArtifact(bundle, results, withoutLineage); err == nil ||
		!strings.Contains(err.Error(), "one entry per proposal step") {
		t.Fatalf("missing step lineage error = %v", err)
	}
	crossCandidate := valid
	crossCandidate.StepSupport = append([]FanInStepSupport{}, valid.StepSupport...)
	crossCandidate.StepSupport[0].Refs = []FanInObservationRef{{
		TaskID: tasks[1].ID, ObservationIndex: 0,
	}}
	if err := ValidateFanInArtifact(bundle, results, crossCandidate); err == nil ||
		!strings.Contains(err.Error(), "belongs to another candidate") {
		t.Fatalf("cross-candidate lineage error = %v", err)
	}
	wrongObservation := valid
	wrongObservation.StepSupport = append([]FanInStepSupport{}, valid.StepSupport...)
	wrongObservation.StepSupport[0].Refs = []FanInObservationRef{{
		TaskID: tasks[0].ID, ObservationIndex: 1,
	}}
	if err := ValidateFanInArtifact(bundle, results, wrongObservation); err == nil ||
		!strings.Contains(err.Error(), "does not support proposal beat") {
		t.Fatalf("wrong per-step observation lineage error = %v", err)
	}

	missingProposal := valid
	missingProposal.Proposal = nil
	if err := ValidateFanInArtifact(bundle, results, missingProposal); err == nil ||
		!strings.Contains(err.Error(), "requires a proposal") {
		t.Fatalf("supported/mixed nil proposal error = %v", err)
	}
	insufficient := FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictInsufficientEvidence,
		Explanation: "The supplied facts do not establish an honest story", Proposal: nil,
	}
	if err := ValidateFanInArtifact(bundle, results[1:], insufficient); err != nil {
		t.Fatalf("missing-only insufficient verdict rejected: %v", err)
	}
	insufficient.StepSupport = valid.StepSupport
	if err := ValidateFanInArtifact(bundle, results, insufficient); err == nil ||
		!strings.Contains(err.Error(), "requires empty step_support") {
		t.Fatalf("insufficient lineage error = %v", err)
	}
	insufficient.StepSupport = nil
	insufficient.Proposal = &proposal
	if err := ValidateFanInArtifact(bundle, results, insufficient); err == nil ||
		!strings.Contains(err.Error(), "requires a null proposal") {
		t.Fatalf("insufficient proposal error = %v", err)
	}

	mechanism, _ := findCandidate(bundle, tasks[1].CandidateID)
	unauthorized := fanoutTestProposal(mechanism)
	if err := ValidateFanInProposal(bundle, results, unauthorized); err == nil ||
		!strings.Contains(err.Error(), "has no atomic observations") {
		t.Fatalf("missing-only candidate authorized a story: %v", err)
	}
	partial := fanoutTestArtifact(tasks[0])
	partial.Observations = partial.Observations[:2]
	partial.CandidateConnection.SupportIDs = partial.CandidateConnection.SupportIDs[:2]
	if err := ValidateFanInProposal(bundle, []LeafResult{{Task: tasks[0], Artifact: partial}}, proposal); err == nil ||
		!strings.Contains(err.Error(), "not supported by a validated atomic observation") {
		t.Fatalf("unsupported proposal beat accepted: %v", err)
	}
}

func TestFanInMixedRequiresCanonicalMissingEvidenceGap(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	for index := range bundle.Candidates {
		if bundle.Candidates[index].ID == "candidate-flow" {
			bundle.Candidates[index].Gaps = []Gap{}
		}
	}
	tasks, err := PlanLeafTasks(bundle, MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	var task LeafTask
	for _, candidateTask := range tasks {
		if candidateTask.Kind == LeafFlow {
			task = candidateTask
			break
		}
	}
	if task.ID == "" {
		t.Fatal("flow leaf task not found")
	}
	leaf := fanoutTestArtifact(task)
	leaf.MissingEvidence = []LeafMissingEvidence{{
		Explanation: "Runtime order is not established by the supplied exact evidence",
		BeatIDs:     []string{task.Candidate.Beats[0].ID},
	}}
	proposal := fanoutTestProposal(task.Candidate)
	artifact := FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictMixed,
		Explanation: "The exact observations are useful while evidence remains unresolved",
		Proposal:    &proposal,
		StepSupport: fanoutTestStepSupport(task, proposal),
	}
	err = ValidateFanInArtifact(bundle, []LeafResult{{Task: task, Artifact: leaf}}, artifact)
	if err == nil || !strings.Contains(err.Error(), "without a cited canonical gap id") {
		t.Fatalf("mixed beat-only missing-evidence error = %v", err)
	}
}

func TestFanInMixedCoversEveryCitedGap(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	for index := range bundle.Candidates {
		if bundle.Candidates[index].ID == "candidate-flow" {
			bundle.Candidates[index].Gaps = append(bundle.Candidates[index].Gaps, Gap{
				ID: "gap-flow-extra", Label: "Second unresolved outcome",
				Detail: "The saved trace also leaves a second outcome unresolved",
			})
		}
	}
	tasks, err := PlanLeafTasks(bundle, MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	var task LeafTask
	for _, candidateTask := range tasks {
		if candidateTask.Kind == LeafFlow {
			task = candidateTask
			break
		}
	}
	if task.ID == "" {
		t.Fatal("flow leaf task not found")
	}
	leaf := fanoutTestArtifact(task)
	proposal := fanoutTestProposal(task.Candidate)
	artifact := FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictMixed,
		Explanation: "The exact observations are useful while supplied gaps remain unresolved",
		Proposal:    &proposal,
		StepSupport: fanoutTestStepSupport(task, proposal),
	}
	results := []LeafResult{{Task: task, Artifact: leaf}}
	if err := ValidateFanInArtifact(bundle, results, artifact); err == nil ||
		!strings.Contains(err.Error(), "omits known candidate gap") {
		t.Fatalf("partial mixed gap coverage error = %v", err)
	}

	for _, gap := range task.Candidate.Gaps[1:] {
		proposal.GapSummary = append(proposal.GapSummary, ProposedGapSummary{
			Explanation: "The additional supplied gap remains unresolved",
			GapIDs:      []string{gap.ID},
		})
	}
	artifact.Proposal = &proposal
	if err := ValidateFanInArtifact(bundle, results, artifact); err != nil {
		t.Fatalf("complete mixed gap coverage error = %v", err)
	}
}

func TestParseFanInArtifactStrictJSON(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(FanInArtifact{
		Version: FanInArtifactVersion, Verdict: FanInVerdictInsufficientEvidence,
		Explanation: "The supplied facts do not establish a story", Proposal: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFanInArtifact(raw); err != nil {
		t.Fatal(err)
	}
	omitted := []byte(`{"version":2,"verdict":"insufficient_evidence","explanation":"Evidence is not enough","step_support":[]}`)
	if _, err := ParseFanInArtifact(omitted); err == nil || !strings.Contains(err.Error(), "omits required proposal") {
		t.Fatalf("ParseFanInArtifact() omitted proposal error = %v", err)
	}
	nullMixed := []byte(`{"version":2,"verdict":"mixed","explanation":"Evidence is mixed","proposal":null,"step_support":[]}`)
	if _, err := ParseFanInArtifact(nullMixed); err == nil || !strings.Contains(err.Error(), "only valid for insufficient_evidence") {
		t.Fatalf("ParseFanInArtifact() mixed null proposal error = %v", err)
	}
	unknown := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"extra":true}`)...)
	if _, err := ParseFanInArtifact(unknown); err == nil {
		t.Fatal("ParseFanInArtifact() accepted an unknown field")
	}
}

func TestBuildFanInPromptRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	bundle := fanoutTestBundle()
	tasks := fanoutCandidateTasks(t)
	if _, err := BuildFanInPrompt(bundle, nil); err == nil {
		t.Fatal("BuildFanInPrompt() accepted no results")
	}
	result := LeafResult{Task: tasks[0], Artifact: fanoutTestArtifact(tasks[0])}
	if _, err := BuildFanInPrompt(bundle, []LeafResult{result, result}); err == nil ||
		!strings.Contains(err.Error(), "repeats leaf task") {
		t.Fatalf("BuildFanInPrompt() duplicate error = %v", err)
	}
	changed := cloneLeafTask(t, tasks[0])
	changed.Candidate.Beats[0].Detail = "Changed after planning"
	if _, err := BuildFanInPrompt(bundle, []LeafResult{{Task: changed, Artifact: fanoutTestArtifact(changed)}}); err == nil ||
		!strings.Contains(err.Error(), "not an exact planned leaf") {
		t.Fatalf("BuildFanInPrompt() changed projection error = %v", err)
	}
}

func fanoutCandidateTasks(t *testing.T) []LeafTask {
	t.Helper()
	tasks, err := PlanLeafTasks(fanoutTestBundle(), MaxLeafTasks)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]LeafTask, 0, 2)
	for _, task := range tasks {
		if task.Kind == LeafFlow || task.Kind == LeafMechanism {
			result = append(result, task)
		}
	}
	if len(result) != 2 {
		t.Fatalf("candidate leaf count = %d, want 2", len(result))
	}
	return result
}

func fanoutTestArtifact(task LeafTask) LeafArtifact {
	observations := make([]LeafObservation, 0, len(task.Candidate.Beats))
	beatIDs := make([]string, 0, len(task.Candidate.Beats))
	for _, beat := range task.Candidate.Beats {
		beatIDs = append(beatIDs, beat.ID)
		observations = append(observations, LeafObservation{
			Explanation: "This exact beat is one bounded local fact",
			SupportIDs:  []string{beat.ID},
		})
	}
	missing := make([]LeafMissingEvidence, 0, len(task.Candidate.Gaps))
	for _, gap := range task.Candidate.Gaps {
		missing = append(missing, LeafMissingEvidence{
			Explanation: "Runtime order is not established by the supplied exact evidence",
			GapIDs:      []string{gap.ID},
		})
	}
	return LeafArtifact{
		Version: LeafArtifactVersion, TaskID: task.ID, CandidateID: task.CandidateID,
		Observations: observations,
		CandidateConnection: LeafCandidateConnection{
			CandidateID: task.CandidateID, TargetID: fanoutTestTargetID(task),
			Relation:    LeafConnectionNeedsCombination,
			Explanation: "This local fragment needs other observations",
			SupportIDs:  beatIDs,
		},
		MissingEvidence: missing,
	}
}

func fanoutTestMissingOnlyArtifact(task LeafTask) LeafArtifact {
	missing := LeafMissingEvidence{
		Explanation: "The supplied exact facts do not establish the missing connection",
	}
	if len(task.Candidate.Gaps) > 0 {
		missing.GapIDs = []string{task.Candidate.Gaps[0].ID}
	} else {
		missing.BeatIDs = []string{task.Candidate.Beats[0].ID}
	}
	return LeafArtifact{
		Version: LeafArtifactVersion, TaskID: task.ID, CandidateID: task.CandidateID,
		Observations: []LeafObservation{},
		CandidateConnection: LeafCandidateConnection{
			CandidateID: task.CandidateID, TargetID: fanoutTestTargetID(task),
			Relation:    LeafConnectionNeedsCombination,
			Explanation: "This local fragment needs other observations",
			SupportIDs:  []string{},
		},
		MissingEvidence: []LeafMissingEvidence{missing},
	}
}

func fanoutTestTargetID(task LeafTask) string {
	if task.FocusComponentID != "" {
		return task.FocusComponentID
	}
	if len(task.Components) > 0 {
		return task.Components[0].ID
	}
	return task.Candidate.Beats[0].ID
}

func fanoutTestStepSupport(task LeafTask, proposal Proposal) []FanInStepSupport {
	artifact := fanoutTestArtifact(task)
	result := make([]FanInStepSupport, 0, len(proposal.Steps))
	for stepIndex, step := range proposal.Steps {
		refs := []FanInObservationRef{}
		for observationIndex, observation := range artifact.Observations {
			for _, beatID := range step.BeatIDs {
				if containsString(observation.SupportIDs, beatID) {
					refs = append(refs, FanInObservationRef{
						TaskID: task.ID, ObservationIndex: observationIndex,
					})
					break
				}
			}
		}
		result = append(result, FanInStepSupport{StepIndex: stepIndex, Refs: refs})
	}
	return result
}

func fanoutTestProposal(candidate Candidate) Proposal {
	steps := make([]ProposedStep, 0, 3)
	for index, beat := range candidate.Beats[:3] {
		steps = append(steps, ProposedStep{
			Title:       fmt.Sprintf("Step %d", index+1),
			Explanation: "Explain only the selected exact beat",
			BeatIDs:     []string{beat.ID},
		})
	}
	gaps := []ProposedGapSummary{}
	if len(candidate.Gaps) > 0 {
		gaps = append(gaps, ProposedGapSummary{
			Explanation: "The supplied gap remains unresolved",
			GapIDs:      []string{candidate.Gaps[0].ID},
		})
	}
	return Proposal{
		Version: ProposalVersion, CandidateID: candidate.ID,
		Title: "A bounded candidate", Summary: "An explanation grounded in exact selected beats",
		Steps: steps, GapSummary: gaps,
	}
}

func fanoutTestBundle() Bundle {
	location := func(path string, line int) *evidence.Location {
		return &evidence.Location{Path: path, Line: line}
	}
	return Bundle{
		Version: BundleVersion, RepoName: "repomap", CanvasVersion: 5,
		Components: []Component{
			{ID: "component-b", Name: "Runtime", Description: "Executes bounded work"},
			{ID: "component-a", Name: "Entry", Description: "Accepts repository input"},
		},
		Candidates: []Candidate{
			{
				ID: "candidate-mechanism", Name: "Assemble a report", Kind: CandidateSuggestedDirection,
				Trigger: "Saved facts become available", Summary: "A local mechanism candidate",
				OrderingBasis: OrderingEditorial,
				Beats: []Beat{
					{
						ID: "mechanism-1", Kind: "collect", Label: "Collect facts", Detail: "Gather bounded saved facts",
						Sequence: 1, ComponentIDs: []string{"component-a"}, SurfaceIDs: []string{}, FlowID: "mechanism-flow",
						FlowStepIDs: []string{}, Evidence: []EvidenceRef{{ID: "evidence-mechanism-1", Kind: "saved_fact", Label: "Saved facts"}},
					},
					{
						ID: "mechanism-2", Kind: "group", Label: "Group facts", Detail: "Group the supplied facts for reading",
						Sequence: 2, ComponentIDs: []string{"component-a"}, SurfaceIDs: []string{}, FlowID: "mechanism-flow",
						FlowStepIDs: []string{}, Evidence: []EvidenceRef{{ID: "evidence-mechanism-2", Kind: "saved_fact", Label: "Grouping fact"}},
					},
					{
						ID: "mechanism-3", Kind: "present", Label: "Present facts", Detail: "Present the bounded architecture view",
						Sequence: 3, ComponentIDs: []string{"component-a"}, SurfaceIDs: []string{}, FlowID: "mechanism-flow",
						FlowStepIDs: []string{}, Evidence: []EvidenceRef{{ID: "evidence-mechanism-3", Kind: "saved_fact", Label: "Presentation fact"}},
					},
				},
				Gaps: []Gap{{ID: "gap-mechanism", Label: "Runtime behavior", Detail: "The reading order is not runtime order"}},
			},
			{
				ID: "candidate-flow", Name: "Serve one request", Kind: CandidateSavedTrace,
				Trigger: "A request arrives", Summary: "A bounded saved flow candidate",
				OrderingBasis: OrderingTrace,
				Beats: []Beat{
					{
						ID: "flow-1", Kind: "entry", Label: "Accept request", Detail: "The exact entry accepts input",
						Sequence: 1, ComponentIDs: []string{"component-a"}, SurfaceIDs: []string{"surface-a"}, FlowID: "saved-flow",
						FlowStepIDs: []string{"saved-step-1"}, Evidence: []EvidenceRef{{
							ID: "evidence-flow-1", Kind: "declaration", Label: "Exact entry", Location: location("cmd/server.go", 10),
						}},
					},
					{
						ID: "flow-2", Kind: "dispatch", Label: "Dispatch request", Detail: "The dispatcher selects bounded work",
						Sequence: 2, ComponentIDs: []string{"component-a", "component-b"}, SurfaceIDs: []string{"surface-a"}, FlowID: "saved-flow",
						FlowStepIDs: []string{"saved-step-2"}, Evidence: []EvidenceRef{{ID: "evidence-flow-2", Kind: "callsite", Label: "Dispatch call"}},
					},
					{
						ID: "flow-3", Kind: "work", Label: "Perform work", Detail: "The handler performs bounded work",
						Sequence: 3, ComponentIDs: []string{"component-b"}, SurfaceIDs: []string{"surface-b"}, FlowID: "saved-flow",
						FlowStepIDs: []string{"saved-step-3"}, Evidence: []EvidenceRef{{ID: "evidence-flow-3", Kind: "transition", Label: "Handler transition"}},
					},
				},
				Gaps: []Gap{{ID: "gap-flow", Label: "Observed outcome", Detail: "The saved trace stops before an observed outcome"}},
			},
		},
	}
}

func cloneFanoutBundle(t *testing.T, bundle Bundle) Bundle {
	t.Helper()
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var result Bundle
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func cloneLeafTask(t *testing.T, task LeafTask) LeafTask {
	t.Helper()
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var result LeafTask
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func reverseFanoutStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseFanoutBeats(values []Beat) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseFanoutCandidates(values []Candidate) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseFanoutComponents(values []Component) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseFanoutEvidence(values []EvidenceRef) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
