package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/pavedpath"
)

func TestProjectRepositoryOperationsSuppressesIncompleteSiblingAndCopy(t *testing.T) {
	t.Parallel()

	record := publicationRecordFixture(t, true)
	data := &ReportData{
		RepoName:      "fixture",
		OpenablePaths: []string{"Makefile", "README.md"},
	}
	projected, err := projectRepositoryOperations(data, record, data.OpenablePaths)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Version != pavedpath.RecordVersion || len(projected.Paths) != 1 {
		t.Fatalf("projected operations = %#v", projected)
	}
	complete := projected.Paths[0]
	if complete.ID != record.Paths[0].ID || complete.Title != record.Paths[0].Title ||
		complete.OrderingBasis != record.Paths[0].OrderingBasis ||
		len(complete.Actions) != 1 || complete.Actions[0].CopyText != "tool run -o result.txt" {
		t.Fatalf("complete path changed = %#v, saved = %#v", complete, record.Paths[0])
	}
	if len(complete.ExpectedResults) == 0 {
		t.Fatalf("complete path has no typed result: %#v", complete)
	}
	for _, result := range complete.ExpectedResults {
		if result.AfterAction != 1 {
			t.Fatalf("result is not after final action: %#v", result)
		}
	}
	completeOnlyRecord := record
	completeOnlyRecord.Paths = append([]pavedpath.Path(nil), record.Paths[:1]...)
	completeOnly, err := projectRepositoryOperations(
		&ReportData{OpenablePaths: []string{"Makefile", "README.md"}},
		completeOnlyRecord,
		[]string{"Makefile", "README.md"},
	)
	if err != nil {
		t.Fatal(err)
	}
	mixedJSON, err := json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	completeOnlyJSON, err := json.Marshal(completeOnly.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(mixedJSON) != string(completeOnlyJSON) {
		t.Fatalf("complete sibling projection changed:\nmixed: %s\nalone: %s", mixedJSON, completeOnlyJSON)
	}

	landmarks := make(map[string]OperationalLandmark, len(projected.Landmarks))
	for _, landmark := range projected.Landmarks {
		landmarks[landmark.Reference.Source.Path+"\x00"+landmark.Command] = landmark
	}
	incompleteLandmark, ok := landmarks["README.md\x00goreman start"]
	if !ok || incompleteLandmark.CopyText != "" {
		t.Fatalf("incomplete landmark = %#v, present = %t", incompleteLandmark, ok)
	}
	completeLandmark, ok := landmarks["README.md\x00tool run -o result.txt"]
	if !ok || completeLandmark.CopyText != "tool run -o result.txt" {
		t.Fatalf("complete landmark = %#v, present = %t", completeLandmark, ok)
	}

	data.Operations = projected
	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	foundComplete := false
	for _, item := range index.Items {
		if item.Target.PavedPathID == record.Paths[1].ID {
			t.Fatalf("incomplete path reached Search: %#v", item)
		}
		if item.Target.PavedPathID == record.Paths[0].ID {
			foundComplete = true
		}
	}
	if !foundComplete {
		t.Fatalf("complete path is absent from Search: %#v", index.Items)
	}
}

func TestProjectRepositoryOperationsMakesAllIncompleteFallbackViewOnly(t *testing.T) {
	t.Parallel()

	record := publicationRecordFixture(t, false)
	data := &ReportData{OpenablePaths: []string{"Makefile", "README.md"}}
	projected, err := projectRepositoryOperations(data, record, data.OpenablePaths)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Paths) != 0 {
		t.Fatalf("incomplete paths were published: %#v", projected.Paths)
	}
	landmarks := make(map[string]OperationalLandmark, len(projected.Landmarks))
	for _, landmark := range projected.Landmarks {
		landmarks[landmark.Command] = landmark
	}
	incompleteLandmark, ok := landmarks["goreman start"]
	if !ok || incompleteLandmark.CopyText != "" {
		t.Fatalf("incomplete-only landmark = %#v, present = %t", incompleteLandmark, ok)
	}
	unrelatedLandmark, ok := landmarks["make test"]
	if !ok || unrelatedLandmark.CopyText != "make test" {
		t.Fatalf("unrelated landmark = %#v, present = %t", unrelatedLandmark, ok)
	}
}

func TestProjectRepositoryOperationsKeepsIncompleteCommandViewOnlyWhenEvidenceIsShared(t *testing.T) {
	t.Parallel()

	record := publicationRecordFixture(t, true)
	for index := range record.Bundle.Evidence {
		if record.Bundle.Evidence[index].ID != "incomplete-action" {
			continue
		}
		record.Bundle.Evidence[index].Commands = append(
			record.Bundle.Evidence[index].Commands,
			pavedpath.Command{
				Value: "tool run -o result.txt", Basis: pavedpath.CommandExact, SafeToCopy: true,
			},
		)
		record.Bundle.Evidence[index].Excerpt = []string{
			"goreman start",
			"$ tool run -o result.txt",
			"completed",
		}
		record.Bundle.Evidence[index].StartLine = 3
		record.Bundle.Evidence[index].EndLine = 5
	}
	record.Paths[0].Actions[0].EvidenceID = "incomplete-action"
	record.Paths[0].PrerequisiteEvidenceIDs = []string{"prerequisite"}
	record.Paths[0].ExpectedEvidenceIDs = nil
	record.Paths[1].Actions[0].EvidenceID = "incomplete-action"
	record.Paths[0].ID = publicationFixturePathID(record.Paths[0].Title, record.Paths[0].Actions)
	var err error
	record.BundleSHA256, err = pavedpath.BundleHash(record.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("shared-evidence record is invalid: %v", err)
	}

	data := &ReportData{OpenablePaths: []string{"Makefile", "README.md"}}
	projected, err := projectRepositoryOperations(data, record, data.OpenablePaths)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Paths) != 1 {
		t.Fatalf("complete shared-evidence path was not retained: %#v", projected.Paths)
	}
	foundIncompleteLandmark := false
	for _, landmark := range projected.Landmarks {
		if landmark.Command != "goreman start" {
			continue
		}
		foundIncompleteLandmark = true
		if landmark.CopyText != "" {
			t.Fatalf("incomplete shared-evidence command retained CopyText: %#v", landmark)
		}
	}
	if !foundIncompleteLandmark {
		t.Fatal("incomplete shared-evidence command landmark is absent")
	}
}

func TestCompletePavedPathBytesMatchPreDecision126Baseline(t *testing.T) {
	t.Parallel()

	record := completePublicationBaselineRecord(t)
	if len(record.Paths) != 1 {
		t.Fatalf("complete saved paths = %#v", record.Paths)
	}

	compact, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	assertPublicationBaseline(
		t,
		"compact saved record",
		compact,
		2002,
		"26055b59aaaa4a9037d81daa9fa80eeec9eb9c6f071e2f4af8f8c29f0f8b83ac",
	)

	persisted, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	persisted = append(persisted, '\n')
	assertPublicationBaseline(
		t,
		"persisted saved record",
		persisted,
		3104,
		"33966ebfa94737ba4367421328aa8dc3066285e2ce2e9062763897c6a7665b7d",
	)

	data := &ReportData{RepoName: "fixture", OpenablePaths: []string{"README.md"}}
	operations, err := projectRepositoryOperations(data, record, data.OpenablePaths)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := json.Marshal(operations)
	if err != nil {
		t.Fatal(err)
	}
	assertPublicationBaseline(
		t,
		"public operations projection",
		projected,
		6774,
		"161626049874c06c810ca2158433946fda196ebceb3ef2a582c69bcfb45cac7d",
	)
}

func assertPublicationBaseline(
	t *testing.T,
	name string,
	got []byte,
	wantLength int,
	wantSHA256 string,
) {
	t.Helper()

	digest := fmt.Sprintf("%x", sha256.Sum256(got))
	if len(got) != wantLength || digest != wantSHA256 {
		t.Fatalf(
			"%s changed from c03349cb53adeb334298c66bdd4e52288148333c: "+
				"length = %d, SHA-256 = %s; want length %d, SHA-256 %s\nJSON: %s",
			name,
			len(got),
			digest,
			wantLength,
			wantSHA256,
			got,
		)
	}
}

func completePublicationBaselineRecord(t *testing.T) pavedpath.Record {
	t.Helper()

	bundle := pavedpath.Bundle{
		Version:      pavedpath.BundleVersion,
		RepoName:     "fixture",
		AllowedPaths: []string{"README.md"},
		Evidence: []pavedpath.Evidence{
			{
				ID: "prerequisite", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 10, EndLine: 12, Label: "Run tool prerequisites",
				Excerpt: []string{
					"Prerequisite: install tool before running this procedure.",
					"",
					"Run tool",
				},
			},
			{
				ID: "steps", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 12, EndLine: 15, Label: "Run tool",
				Excerpt: []string{
					"$ tool prepare",
					"prepared",
					"$ tool run -o result.txt",
					"completed",
				},
				Commands: []pavedpath.Command{
					{Value: "tool prepare", Basis: pavedpath.CommandExact, SafeToCopy: true},
					{Value: "tool run -o result.txt", Basis: pavedpath.CommandExact, SafeToCopy: true},
				},
			},
			{
				ID: "expected", Role: pavedpath.RoleVerification,
				Path: "README.md", StartLine: 15, EndLine: 15, Label: "Expected completion",
				Excerpt: []string{"completed"},
			},
			{
				ID: "troubleshooting", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 17, EndLine: 17, Label: "Troubleshooting",
				Excerpt: []string{"If the command fails, inspect the local log."},
			},
		},
	}
	record, err := pavedpath.BuildRecord(
		bundle,
		pavedpath.Proposal{
			Version: pavedpath.ProposalVersion,
			Paths: []pavedpath.ProposedPath{{
				Title:                   "Prepare and run tool",
				Goal:                    "Produce the documented result.",
				PrerequisiteEvidenceIDs: []string{"prerequisite"},
				Actions: []pavedpath.ProposedAction{
					{
						EvidenceID:  "steps",
						Instruction: "Prepare the tool.",
						Command:     "tool prepare",
					},
					{
						EvidenceID:  "steps",
						Instruction: "Run the tool and write its result.",
						Command:     "tool run -o result.txt",
					},
				},
				ExpectedEvidenceIDs:        []string{"expected"},
				TroubleshootingEvidenceIDs: []string{"troubleshooting"},
				OrderingBasis:              pavedpath.OrderingDocumented,
			}},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func publicationRecordFixture(t *testing.T, includeComplete bool) pavedpath.Record {
	t.Helper()

	bundle := pavedpath.Bundle{
		Version: pavedpath.BundleVersion, RepoName: "fixture",
		AllowedPaths: []string{"Makefile", "README.md"},
		Evidence: []pavedpath.Evidence{
			{
				ID: "prerequisite", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 1, EndLine: 3, Label: "Run tool",
				Excerpt: []string{
					"First install tool before running this procedure.",
					"",
					"$ tool run -o result.txt",
				},
			},
			{
				ID: "complete-action", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 3, EndLine: 4, Label: "Run tool",
				Excerpt: []string{"$ tool run -o result.txt", "completed"},
				Commands: []pavedpath.Command{{
					Value: "tool run -o result.txt", Basis: pavedpath.CommandExact, SafeToCopy: true,
				}},
			},
			{
				ID: "incomplete-action", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 10, EndLine: 10, Label: "Run cluster",
				Excerpt: []string{"goreman start"},
				Commands: []pavedpath.Command{{
					Value: "goreman start", Basis: pavedpath.CommandExact, SafeToCopy: true,
				}},
			},
			{
				ID: "standalone", Role: pavedpath.RoleBuildTarget,
				Path: "Makefile", StartLine: 1, EndLine: 2, Label: "test target",
				Excerpt: []string{"test:", "\tgo test ./..."},
				Commands: []pavedpath.Command{{
					Value: "make test", Basis: pavedpath.CommandStructural, SafeToCopy: true,
				}},
			},
		},
	}
	paths := []pavedpath.ProposedPath{}
	if includeComplete {
		paths = append(paths, pavedpath.ProposedPath{
			Title: "Run tool", Goal: "Run the documented tool and observe its exact result.",
			PrerequisiteEvidenceIDs: []string{"prerequisite"},
			Actions: []pavedpath.ProposedAction{{
				EvidenceID: "complete-action", Instruction: "Run the documented tool.",
				Command: "tool run -o result.txt",
			}},
			ExpectedEvidenceIDs: []string{"incomplete-action"},
			OrderingBasis:       pavedpath.OrderingDocumented,
		})
	}
	paths = append(paths, pavedpath.ProposedPath{
		Title: "Run cluster", Goal: "Start the local cluster.",
		Actions: []pavedpath.ProposedAction{{
			EvidenceID: "incomplete-action", Instruction: "Start the local cluster.",
			Command: "goreman start",
		}},
		OrderingBasis: pavedpath.OrderingDocumented,
	})
	record, err := pavedpath.BuildRecord(
		bundle,
		pavedpath.Proposal{Version: pavedpath.ProposalVersion, Paths: paths},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantComplete := 0
	rejectedIndex := 0
	if includeComplete {
		wantComplete = 1
		rejectedIndex = 1
	}
	if len(record.Paths) != wantComplete ||
		len(record.Issues) != 1 ||
		record.Issues[0].PathIndex != rejectedIndex ||
		record.Issues[0].Code != pavedpath.PublicationIssueMissingPrerequisite {
		t.Fatalf("publication-gated record = %#v", record)
	}

	historical := pavedpath.Path{
		Title: "Run cluster",
		Goal:  "Start the local cluster.",
		Actions: []pavedpath.Action{{
			EvidenceID:  "incomplete-action",
			Instruction: "Start the local cluster.",
			Command:     "goreman start",
			SafeToCopy:  true,
		}},
		OrderingBasis: pavedpath.OrderingDocumented,
	}
	historical.ID = publicationFixturePathID(historical.Title, historical.Actions)
	record.Paths = append(record.Paths, historical)
	record.Issues = nil
	for index := range record.Landmarks {
		if record.Landmarks[index].EvidenceID == "incomplete-action" &&
			record.Landmarks[index].Command == "goreman start" {
			// Recreate the pre-Decision-126 saved authorization so projection
			// tests prove historical records are made view-only on replay.
			record.Landmarks[index].SafeToCopy = true
		}
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("historical fixture is invalid: %v", err)
	}
	return record
}

func publicationFixturePathID(title string, actions []pavedpath.Action) string {
	parts := make([]string, 0, len(actions)*3)
	for _, action := range actions {
		parts = append(parts, action.EvidenceID, action.Command, action.Endpoint)
	}
	digest := sha256.Sum256([]byte(title + "\x00" + strings.Join(parts, "\x00")))
	return fmt.Sprintf("operate-%x", digest[:8])
}
