package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestNarrativeCompressionAttachesSourceLessCanonicalDetailOnce(t *testing.T) {
	t.Parallel()

	data := &ReportData{OpenablePaths: []string{"ticker.go", "sync.go", "logger.go"}}
	statements := []semanticdiscovery.Statement{
		{ID: "ticker", Text: "The monitor creates its ticker.", Basis: semanticdiscovery.ClaimDirect},
		{ID: "trigger", Text: "The monitor calls sync once.", Basis: semanticdiscovery.ClaimDirect},
		{ID: "logger", Text: "The monitor records the result.", Basis: semanticdiscovery.ClaimDirect},
		{ID: "internals", Text: "The replica sync function calls sync once and returns its error.", Basis: semanticdiscovery.ClaimDirect, AspectIDs: []string{"effect"}},
	}
	artifact := semanticdiscovery.Artifact{
		ID: "monitor", Kind: semanticdiscovery.ArtifactMechanism,
		Verdict: semanticdiscovery.VerdictSupported, Question: "How does the monitor synchronize?",
		Statements: statements, CoveredAspectIDs: []string{"effect"},
		Steps: []semanticdiscovery.Step{
			{Title: "Ticker creation", StatementIDs: []string{"ticker"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "ticker.go", Line: 11}}},
			{Title: "Sync trigger", StatementIDs: []string{"trigger"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "sync.go", Line: 402}}},
			{Title: "Result logging", StatementIDs: []string{"logger"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "logger.go", Line: 31}}},
			{Title: "Sync execution internals", StatementIDs: []string{"internals"}, Evidence: []semanticdiscovery.EvidenceRef{{Path: "sync.go", Line: 145}, {Path: "sync.go", Line: 146}}},
		},
	}
	probe := userMechanismProbe(
		userMechanismSource{"ticker.go", "monitor", 10, 8},
		userMechanismSource{"sync.go", "monitorSync", 400, 8},
		userMechanismSource{"logger.go", "record", 30, 8},
	)
	mechanism, ok := projectUserMechanism(data, artifact, probe)
	if !ok {
		t.Fatal("source-backed mechanism was not projected")
	}
	if len(mechanism.Steps) != 3 || len(mechanism.unplacedImplementationDetails) != 1 {
		t.Fatalf("projection = %d narrative steps, %d private details", len(mechanism.Steps), len(mechanism.unplacedImplementationDetails))
	}

	compression := NarrativeCompression{
		ArtifactID: mechanism.ArtifactID, OrderingBasis: NarrativeOrderingEditorial,
		Phases: []NarrativeCompressionPhase{
			{Title: "Ticker creation", MemberStatementIDs: []string{"ticker", "internals"}},
			{Title: "Sync trigger", MemberStatementIDs: []string{"trigger"}},
			{Title: "Result logging", MemberStatementIDs: []string{"logger"}},
		},
	}
	phases, ok := ProjectNarrativeCompression(mechanism, compression)
	if !ok {
		t.Fatal("valid compression with a source-less canonical detail was rejected")
	}
	retained := make(map[string]int)
	for _, step := range mechanism.Steps {
		retained[step.Explanation]++
	}
	detailCount := 0
	for phaseIndex, phase := range phases {
		if len(phase.Sources) == 0 {
			t.Fatalf("phase %d is not code-first: %#v", phaseIndex, phase)
		}
		for _, detail := range phase.ImplementationDetails {
			detailCount++
			retained[detail.Explanation]++
			if phaseIndex != 1 || detail.Title != "Sync execution internals" || len(detail.Sources) != 0 ||
				len(detail.Locations) != 2 || detail.Locations[0].Line != 145 {
				t.Fatalf("detail attached to the wrong phase or lost exact references: phase=%d detail=%#v", phaseIndex, detail)
			}
		}
	}
	if detailCount != 1 {
		t.Fatalf("source-less canonical detail retained %d times, want once", detailCount)
	}
	for _, statement := range statements {
		if retained[statement.Text] != 1 {
			t.Fatalf("canonical statement %q retained %d times, want once", statement.Text, retained[statement.Text])
		}
	}
	mechanism.Phases = phases
	raw, err := json.Marshal(mechanism)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `"implementation_details"`) != 1 ||
		strings.Count(string(raw), `"Sync execution internals"`) != 1 {
		t.Fatalf("serialized phase detail is missing or duplicated: %s", raw)
	}

	sourceLessPhase := compression
	sourceLessPhase.Phases = []NarrativeCompressionPhase{
		{Title: "Ticker creation", MemberStatementIDs: []string{"ticker"}},
		{Title: "Sync trigger", MemberStatementIDs: []string{"trigger"}},
		{Title: "Result logging", MemberStatementIDs: []string{"logger"}},
		{Title: "Sync execution internals", MemberStatementIDs: []string{"internals"}},
	}
	if _, ok := ProjectNarrativeCompression(mechanism, sourceLessPhase); ok {
		t.Fatal("source-less canonical detail was promoted to a top-level phase")
	}
}
