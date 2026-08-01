package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type orientationResponseFixture struct {
	ProjectGuess string
	Confidence   float64
	Map          []orientationMapFixture
	FirstFiles   []orientationFileFixture
	Flows        []orientationFlowFixture
	Questions    []string
	Research     []orientationResearchFixture
	Warnings     []string
}

type orientationMapFixture struct {
	Name         string
	Role         string
	EvidencePath string
	WhyItMatters string
}

type orientationFileFixture struct {
	Path   string
	Reason string
}

type orientationFlowFixture struct {
	Name           string
	FlowType       string
	Trigger        string
	EntrypointPath string
	LikelyPaths    []string
	EvidencePaths  []string
	WhyInteresting string
	Confidence     float64
}

type orientationResearchFixture struct {
	ID                 string
	Purpose            string
	Question           string
	CandidatePaths     []string
	EvidenceCategories []string
}

func orientationResponseForRequest(t *testing.T, requestBody []byte, fixture orientationResponseFixture) []byte {
	t.Helper()
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(requestBody, &request); err != nil {
		t.Fatalf("decode orientation provider request: %v", err)
	}
	if len(request.Messages) == 0 {
		t.Fatal("orientation provider request has no messages")
	}
	const marker = "Orientation facts bundle JSON:\n"
	content := request.Messages[len(request.Messages)-1].Content
	index := strings.LastIndex(content, marker)
	if index < 0 {
		t.Fatalf("orientation provider request has no wire bundle marker")
	}
	var wire struct {
		Files []struct {
			FileRef string `json:"file_ref"`
			Path    string `json:"path"`
		} `json:"file_index"`
		Candidates []struct {
			FileRef     string `json:"file_ref"`
			EvidenceRef string `json:"evidence_ref"`
		} `json:"candidate_file_index"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content[index+len(marker):])), &wire); err != nil {
		t.Fatalf("decode orientation wire bundle: %v", err)
	}
	fileRefByPath := make(map[string]string, len(wire.Files))
	for _, file := range wire.Files {
		fileRefByPath[file.Path] = file.FileRef
	}
	evidenceByFileRef := make(map[string]string, len(wire.Candidates))
	for _, candidate := range wire.Candidates {
		evidenceByFileRef[candidate.FileRef] = candidate.EvidenceRef
	}
	fileRef := func(path string) string {
		t.Helper()
		ref := fileRefByPath[path]
		if ref == "" {
			t.Fatalf("orientation wire has no file ref for %q", path)
		}
		return ref
	}
	evidenceRef := func(path string) string {
		t.Helper()
		ref := evidenceByFileRef[fileRef(path)]
		if ref == "" {
			t.Fatalf("orientation wire has no candidate evidence ref for %q", path)
		}
		return ref
	}

	highLevelMap := make([]any, 0, len(fixture.Map))
	for _, item := range fixture.Map {
		role := item.Role
		if role == "" {
			role = "unknown"
		}
		highLevelMap = append(highLevelMap, map[string]any{
			"name": item.Name, "role": role,
			"evidence_refs":  []string{evidenceRef(item.EvidencePath)},
			"why_it_matters": item.WhyItMatters,
		})
	}
	firstFiles := make([]any, 0, len(fixture.FirstFiles))
	for _, file := range fixture.FirstFiles {
		firstFiles = append(firstFiles, map[string]any{"file_ref": fileRef(file.Path), "reason": file.Reason})
	}
	flows := make([]any, 0, len(fixture.Flows))
	for _, flow := range fixture.Flows {
		flowType := flow.FlowType
		if flowType == "" {
			flowType = "request"
		}
		likelyRefs := make([]string, 0, len(flow.LikelyPaths))
		for _, path := range flow.LikelyPaths {
			likelyRefs = append(likelyRefs, fileRef(path))
		}
		evidenceRefs := make([]string, 0, len(flow.EvidencePaths))
		for _, path := range flow.EvidencePaths {
			evidenceRefs = append(evidenceRefs, evidenceRef(path))
		}
		flows = append(flows, map[string]any{
			"name": flow.Name, "flow_type": flowType, "trigger": flow.Trigger,
			"likely_entrypoint_ref": fileRef(flow.EntrypointPath), "likely_file_refs": likelyRefs,
			"why_interesting": flow.WhyInteresting, "evidence_refs": evidenceRefs,
			"confidence": flow.Confidence,
		})
	}
	research := make([]any, 0, len(fixture.Research))
	for _, question := range fixture.Research {
		candidateRefs := make([]string, 0, len(question.CandidatePaths))
		for _, path := range question.CandidatePaths {
			candidateRefs = append(candidateRefs, fileRef(path))
		}
		research = append(research, map[string]any{
			"id": question.ID, "purpose": question.Purpose, "question": question.Question,
			"candidate_file_refs": candidateRefs, "evidence_categories": question.EvidenceCategories,
		})
	}
	encoded, err := json.Marshal(map[string]any{
		"project_guess": fixture.ProjectGuess,
		"confidence":    fixture.Confidence, "high_level_map": highLevelMap,
		"first_files_to_open": firstFiles, "candidate_flows": flows,
		"important_domain_words": []any{}, "questions_for_human": fixture.Questions,
		"research_questions": research, "warnings": fixture.Warnings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
