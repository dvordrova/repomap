package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestModelResearchDetailsRemainCollapsedAndTyped(t *testing.T) {
	for _, required := range []string{
		"Model research",
		"Provider calls:",
		"External request bytes:",
		"Local authorized files:",
		"Initial model summaries:",
		"Focused local evidence inspected:",
		"Targeted model evidence windows:",
		"rejected_findings",
		"unresolved_frontiers",
	} {
		if !strings.Contains(scriptJS, required) {
			t.Fatalf("report script does not expose %q", required)
		}
	}
	if strings.Contains(scriptJS, "details.open = true") {
		t.Fatal("model research mechanics must remain collapsed by default")
	}

	state := modelresearch.NewState(modelresearch.DefaultPolicy(), modelresearch.RepositoryContext{
		Identity: "/fixture", Revision: "abc", Scenario: "go-default",
	})
	state.Usage = modelresearch.Usage{SemanticCalls: 3, RequestBytes: 184 << 10}
	state.Coverage = modelresearch.Coverage{
		LocalAuthorizedFiles: 382, InitialModelSummaries: 118,
		FocusedLocalEvidenceInspected: 24, TargetedModelEvidenceWindows: 14,
	}
	html, err := RenderHTML(&ReportData{RepoName: "fixture", ModelResearch: &state})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `"local_authorized_files":382`) ||
		!strings.Contains(string(html), `"max_semantic_calls":4`) {
		t.Fatalf("rendered report omitted typed research state: %s", html)
	}
}
