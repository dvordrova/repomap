package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestProjectUserMechanismGroupsSourceBySymbolAndBoundsRelatedCards(t *testing.T) {
	t.Parallel()

	data, artifact, probe := sourceProjectionFixture()
	mechanism, ok := projectUserMechanism(data, artifact, probe)
	if !ok {
		t.Fatal("projectUserMechanism() did not publish a source-backed mechanism")
	}
	if len(mechanism.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(mechanism.Steps))
	}

	wantPaths := []string{"internal/a_primary.go", "internal/b_secondary.go", "internal/c_third.go"}
	wantRoles := []string{"primary", "related", "related"}
	sources := mechanism.Steps[0].Sources
	if len(sources) != len(wantPaths) {
		t.Fatalf("source cards = %d, want one primary and two related: %#v", len(sources), sources)
	}
	for index, snippet := range sources {
		if snippet.Path != wantPaths[index] || snippet.Role != wantRoles[index] {
			t.Fatalf("source[%d] = %s (%s), want %s (%s)",
				index, snippet.Path, snippet.Role, wantPaths[index], wantRoles[index])
		}
		if err := snippet.Validate(); err != nil {
			t.Fatalf("source[%d] is invalid: %v", index, err)
		}
		if len(snippet.RelatedEvidenceIDs) < 2 {
			t.Fatalf("source[%d] did not group its evidence: %#v", index, snippet.RelatedEvidenceIDs)
		}
	}
	if strings.Contains(sources[0].Content, "DroppedStep") {
		t.Fatalf("fourth source group leaked into the three-card bound: %q", sources[0].Content)
	}
	for _, snippet := range sources {
		if snippet.Path == "internal/d_dropped.go" {
			t.Fatalf("fourth source group was published: %#v", snippet)
		}
	}
	if got := sourceLineByNumber(t, sources[0], 4); got.Text != "\tPrimaryStep(4)" || !got.Highlight {
		t.Fatalf("saved source line 4 = %#v, want exact highlighted source", got)
	}
	if !strings.Contains(sources[0].Content, "\tPrimaryStep(4)") {
		t.Fatalf("source content does not contain exact saved bytes: %q", sources[0].Content)
	}

	reversedProbe := probe
	reversedProbe.Functions = append([]goldenmechanism.Function(nil), probe.Functions...)
	for left, right := 0, len(reversedProbe.Functions)-1; left < right; left, right = left+1, right-1 {
		reversedProbe.Functions[left], reversedProbe.Functions[right] =
			reversedProbe.Functions[right], reversedProbe.Functions[left]
	}
	reprojected, ok := projectUserMechanism(data, artifact, reversedProbe)
	if !ok {
		t.Fatal("projectUserMechanism() failed after an irrelevant probe function reordering")
	}
	if !reflect.DeepEqual(mechanism, reprojected) {
		t.Fatalf("source projection depends on probe function order:\nfirst:  %#v\nsecond: %#v", mechanism, reprojected)
	}
}

func TestProjectStepSourceSnippetsKeepsOnlyDominantDistantClusterInline(t *testing.T) {
	t.Parallel()

	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction("internal/long.go", "LongPath", 1, 150),
	}}
	step := semanticdiscovery.Step{
		ID: "step-long",
		Evidence: []semanticdiscovery.EvidenceRef{
			{ID: "evidence-010", Path: "internal/long.go", Line: 10},
			{ID: "evidence-075", Path: "internal/long.go", Line: 75},
			{ID: "evidence-140", Path: "internal/long.go", Line: 140},
		},
	}

	sources := projectStepSourceSnippets(&ReportData{CapturedRevision: "revision-a"}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1: %#v", len(sources), sources)
	}
	snippet := sources[0]
	if len(snippet.Lines) > maxInlineSourceLines {
		t.Fatalf("source lines = %d, exceeds %d-line bound", len(snippet.Lines), maxInlineSourceLines)
	}
	if snippet.StartLine != 3 || snippet.EndLine != 13 {
		t.Fatalf("source bounds = %d-%d, want 3-13 around the stable first tie", snippet.StartLine, snippet.EndLine)
	}
	if got, want := len(snippet.Lines), 11; got != want {
		t.Fatalf("source lines = %d, want focused %d-line projection", got, want)
	}
	if got, want := snippet.HighlightRanges, []SourceHighlight{{StartLine: 10, EndLine: 10}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("highlight ranges = %#v, want %#v", got, want)
	}
	if got := sourceLineByNumber(t, snippet, 10); !got.Highlight {
		t.Fatalf("line 10 is not highlighted: %#v", got)
	}
	if strings.Contains(snippet.Content, omittedSourceLinesMarker) {
		t.Fatalf("one dominant cluster should be contiguous: %q", snippet.Content)
	}
	if got, want := snippet.RelatedEvidenceIDs, []string{
		"evidence-010", "evidence-075", "evidence-140",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all exact evidence ids = %#v, want %#v", got, want)
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("bounded snippet is invalid: %v", err)
	}
	legacy := cloneSourceSnippet(t, snippet)
	legacy.Content = strings.ReplaceAll(
		legacy.Content,
		omittedSourceLinesMarker,
		legacyOmittedLinesMarker,
	)
	legacy.PresentationSHA256 = sourceSnippetPresentationSHA(legacy)
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy omitted-line content no longer replays: %v", err)
	}
}

func TestProjectStepSourceSnippetsCoversDistantSubstantialOperations(t *testing.T) {
	t.Parallel()

	const sourcePath = "internal/listing.go"
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-declaration", Path: sourcePath, Line: 5},
		{ID: "evidence-sort-name", Path: sourcePath, Line: 30},
		{ID: "evidence-sort-size", Path: sourcePath, Line: 32},
		{ID: "evidence-page-offset", Path: sourcePath, Line: 80},
		{ID: "evidence-page-limit", Path: sourcePath, Line: 82},
	}}
	stepBefore := string(mustSourceProjectionJSON(t, step))
	probe := goldenmechanism.Result{
		Functions: []goldenmechanism.Function{
			sourceProjectionFunction(sourcePath, "applySortAndLimit", 1, 120),
		},
		Observations: []goldenmechanism.Observation{
			sourceProjectionObservation(
				"observation-declaration", "evidence-declaration", sourcePath, 5,
				semanticdiscovery.CapabilityStatic, goldenmechanism.BasisDeclaration,
				"", "",
			),
			sourceProjectionObservation(
				"observation-sort-name", "evidence-sort-name", sourcePath, 30,
				semanticdiscovery.CapabilityDataTransformation, goldenmechanism.BasisTransform,
				"sort by name", "",
			),
			sourceProjectionObservation(
				"observation-sort-size", "evidence-sort-size", sourcePath, 32,
				semanticdiscovery.CapabilityDataTransformation, goldenmechanism.BasisTransform,
				"sort by size", "",
			),
			sourceProjectionObservation(
				"observation-page-offset", "evidence-page-offset", sourcePath, 80,
				semanticdiscovery.CapabilityDataWrite, goldenmechanism.BasisAssignment,
				"items after offset", "",
			),
			sourceProjectionObservation(
				"observation-page-limit", "evidence-page-limit", sourcePath, 82,
				semanticdiscovery.CapabilityDataTransformation, goldenmechanism.BasisTransform,
				"items before limit", "",
			),
		},
	}

	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1: %#v", len(sources), sources)
	}
	snippet := sources[0]
	if snippet.StartLine != 23 || snippet.EndLine != 85 || len(snippet.Lines) != 26 {
		t.Fatalf("multi-fragment source = %d-%d (%d lines), want 23-85 (26 lines)",
			snippet.StartLine, snippet.EndLine, len(snippet.Lines))
	}
	if got, want := snippet.HighlightRanges, []SourceHighlight{
		{StartLine: 30, EndLine: 30},
		{StartLine: 32, EndLine: 32},
		{StartLine: 80, EndLine: 80},
		{StartLine: 82, EndLine: 82},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operation highlights = %#v, want %#v", got, want)
	}
	for _, line := range []int{30, 32, 80, 82} {
		if got := sourceLineByNumber(t, snippet, line); !got.Highlight {
			t.Fatalf("operation line %d is not highlighted: %#v", line, got)
		}
	}
	gaps := 0
	declarationVisible := false
	for _, line := range snippet.Lines {
		if line.GapBefore {
			gaps++
		}
		declarationVisible = declarationVisible || line.Line == 5
	}
	if gaps != 1 || !strings.Contains(snippet.Content, omittedSourceLinesMarker) {
		t.Fatalf("fragment gaps = %d, content = %q", gaps, snippet.Content)
	}
	if declarationVisible {
		t.Fatalf("context-only declaration became an inline fragment: %#v", snippet.Lines)
	}
	if got, want := snippet.RelatedEvidenceIDs, []string{
		"evidence-declaration",
		"evidence-page-limit",
		"evidence-page-offset",
		"evidence-sort-name",
		"evidence-sort-size",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained exact evidence ids = %#v, want %#v", got, want)
	}
	if got := string(mustSourceProjectionJSON(t, step)); got != stepBefore {
		t.Fatalf("presentation projection mutated the semantic step:\nbefore: %s\nafter:  %s", stepBefore, got)
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("multi-fragment snippet is invalid: %v", err)
	}
}

func TestProjectStepSourceSnippetsIncludesDistantPrimaryPathBoundary(t *testing.T) {
	t.Parallel()

	const sourcePath = "file/replica_client.go"
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{
			ID: "evidence-call", Kind: "bounded_go_window", Label: "direct_call",
			Path: sourcePath, Line: 159, Column: 11,
		},
		{
			ID: "evidence-assignment", Kind: "bounded_go_window", Label: "assignment",
			Path: sourcePath, Line: 160, Column: 3,
		},
		{
			ID: "evidence-return", Kind: "bounded_go_window", Label: "return",
			Path: sourcePath, Line: 170, Column: 3,
		},
		{
			ID: "evidence-boundary", Kind: boundedPrimaryPathSyntaxKind,
			Label: "file_write os rename", Path: sourcePath, Line: 224, Column: 12,
		},
	}}
	stepBefore := string(mustSourceProjectionJSON(t, step))
	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction(sourcePath, "ReplicaClient.WriteLTXFile", 157, 78),
	}}

	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1: %#v", len(sources), sources)
	}
	snippet := sources[0]
	if snippet.StartLine != 157 || snippet.EndLine != 227 || len(snippet.Lines) != 28 {
		t.Fatalf("multi-fragment source = %d-%d (%d lines), want 157-227 (28 lines)",
			snippet.StartLine, snippet.EndLine, len(snippet.Lines))
	}
	if len(snippet.Lines) > maxInlineSourceLines {
		t.Fatalf("compact source lines = %d, exceeds %d-line budget",
			len(snippet.Lines), maxInlineSourceLines)
	}
	if got, want := snippet.HighlightRanges, []SourceHighlight{
		{StartLine: 159, EndLine: 160},
		{StartLine: 170, EndLine: 170},
		{StartLine: 224, EndLine: 224},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operation highlights = %#v, want %#v", got, want)
	}
	if got := sourceLineByNumber(t, snippet, 224); !got.Highlight {
		t.Fatalf("distant primary-path boundary is not highlighted: %#v", got)
	}
	if got := sourceLineByNumber(t, snippet, 217); !got.GapBefore {
		t.Fatalf("distant primary-path fragment has no preceding gap: %#v", got)
	}
	if !strings.Contains(snippet.Content, omittedSourceLinesMarker) {
		t.Fatalf("multi-fragment source has no omission marker: %q", snippet.Content)
	}
	if got := string(mustSourceProjectionJSON(t, step)); got != stepBefore {
		t.Fatalf("presentation projection mutated the semantic step:\nbefore: %s\nafter:  %s", stepBefore, got)
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("multi-fragment snippet is invalid: %v", err)
	}
}

func TestProjectStepSourceSnippetsDoesNotAddContextOnlyFragments(t *testing.T) {
	t.Parallel()

	const sourcePath = "internal/handler.go"
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-context-branch", Path: sourcePath, Line: 20},
		{ID: "evidence-dispatch", Path: sourcePath, Line: 80},
		{ID: "evidence-declaration", Path: sourcePath, Line: 120},
	}}
	probe := goldenmechanism.Result{
		Functions: []goldenmechanism.Function{
			sourceProjectionFunction(sourcePath, "dispatch", 1, 140),
		},
		Observations: []goldenmechanism.Observation{
			sourceProjectionObservation(
				"observation-context-branch", "evidence-context-branch", sourcePath, 20,
				semanticdiscovery.CapabilityBranch, goldenmechanism.BasisBranch,
				"request is eligible", "",
			),
			sourceProjectionObservation(
				"observation-dispatch", "evidence-dispatch", sourcePath, 80,
				semanticdiscovery.CapabilityDirectCall, goldenmechanism.BasisDirectCall,
				"", "Endpoint.ServeHTTP",
			),
			sourceProjectionObservation(
				"observation-declaration", "evidence-declaration", sourcePath, 120,
				semanticdiscovery.CapabilityStatic, goldenmechanism.BasisDeclaration,
				"", "",
			),
		},
	}

	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1: %#v", len(sources), sources)
	}
	snippet := sources[0]
	if snippet.StartLine != 73 || snippet.EndLine != 83 || len(snippet.Lines) != 11 {
		t.Fatalf("focused source = %d-%d (%d lines), want 73-83 (11 lines)",
			snippet.StartLine, snippet.EndLine, len(snippet.Lines))
	}
	if got, want := snippet.HighlightRanges, []SourceHighlight{{StartLine: 80, EndLine: 80}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("highlights = %#v, want %#v", got, want)
	}
	if strings.Contains(snippet.Content, omittedSourceLinesMarker) {
		t.Fatalf("context-only evidence added a fragment: %q", snippet.Content)
	}
	if got, want := snippet.RelatedEvidenceIDs, []string{
		"evidence-context-branch", "evidence-declaration", "evidence-dispatch",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained exact evidence ids = %#v, want %#v", got, want)
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("focused snippet is invalid: %v", err)
	}
}

func TestProjectStepSourceSnippetsPrefersCaddyBrowseBehaviorCluster(t *testing.T) {
	t.Parallel()

	const sourcePath = "modules/caddyhttp/fileserver/staticfiles.go"
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-call", Path: sourcePath, Line: 372, Column: 11},
		{ID: "evidence-declaration", Path: sourcePath, Line: 269, Column: 25},
		{ID: "evidence-browse-branch", Path: sourcePath, Line: 371, Column: 6},
		{ID: "evidence-index-branch", Path: sourcePath, Line: 324, Column: 5},
	}}
	probe := goldenmechanism.Result{
		Functions: []goldenmechanism.Function{
			sourceProjectionFunction(sourcePath, "FileServer.ServeHTTP", 269, 120),
		},
		Observations: []goldenmechanism.Observation{
			sourceProjectionObservation(
				"observation-index-branch", "evidence-index-branch", sourcePath, 324,
				semanticdiscovery.CapabilityBranch, goldenmechanism.BasisBranch,
				"info.IsDir() && len(fsrv.IndexNames) > 0", "",
			),
			sourceProjectionObservation(
				"observation-call", "evidence-call", sourcePath, 372,
				semanticdiscovery.CapabilityDirectCall, goldenmechanism.BasisDirectCall,
				"", "FileServer.serveBrowse",
			),
			sourceProjectionObservation(
				"observation-declaration", "evidence-declaration", sourcePath, 269,
				semanticdiscovery.CapabilityEntry, goldenmechanism.BasisDeclaration,
				"", "",
			),
			sourceProjectionObservation(
				"observation-browse-branch", "evidence-browse-branch", sourcePath, 371,
				semanticdiscovery.CapabilityBranch, goldenmechanism.BasisBranch,
				"fsrv.Browse != nil && !fileHidden(filename, filesToHide)", "",
			),
		},
	}

	sources := projectStepSourceSnippets(
		&ReportData{OpenablePaths: []string{sourcePath}},
		step,
		nil,
		&probe,
	)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1: %#v", len(sources), sources)
	}
	snippet := sources[0]
	if snippet.StartLine != 364 || snippet.EndLine != 375 || len(snippet.Lines) != 12 {
		t.Fatalf("dominant source = %d-%d (%d lines), want 364-375 (12 lines)",
			snippet.StartLine, snippet.EndLine, len(snippet.Lines))
	}
	if got, want := snippet.HighlightRanges, []SourceHighlight{{StartLine: 371, EndLine: 372}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dominant highlights = %#v, want %#v", got, want)
	}
	if strings.Contains(snippet.Content, "ServeHTTPStep(269)") ||
		strings.Contains(snippet.Content, "ServeHTTPStep(324)") {
		t.Fatalf("signature or index-file cluster leaked into primary source: %q", snippet.Content)
	}
	if got, want := snippet.RelatedEvidenceIDs, []string{
		"evidence-browse-branch",
		"evidence-call",
		"evidence-declaration",
		"evidence-index-branch",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained exact evidence ids = %#v, want %#v", got, want)
	}
	locations := userCodeLocations(step.Evidence, map[string]struct{}{sourcePath: {}})
	if len(locations) != 4 {
		t.Fatalf("exact references = %#v, want 4 retained locations", locations)
	}
	if got, want := []int{
		locations[0].Line, locations[1].Line, locations[2].Line, locations[3].Line,
	}, []int{269, 324, 371, 372}; !reflect.DeepEqual(got, want) {
		t.Fatalf("exact references = %#v, want %#v", got, want)
	}

	reversedStep := step
	reversedStep.Evidence = append([]semanticdiscovery.EvidenceRef(nil), step.Evidence...)
	for left, right := 0, len(reversedStep.Evidence)-1; left < right; left, right = left+1, right-1 {
		reversedStep.Evidence[left], reversedStep.Evidence[right] =
			reversedStep.Evidence[right], reversedStep.Evidence[left]
	}
	reversedProbe := probe
	reversedProbe.Observations = append([]goldenmechanism.Observation(nil), probe.Observations...)
	for left, right := 0, len(reversedProbe.Observations)-1; left < right; left, right = left+1, right-1 {
		reversedProbe.Observations[left], reversedProbe.Observations[right] =
			reversedProbe.Observations[right], reversedProbe.Observations[left]
	}
	reprojected := projectStepSourceSnippets(
		&ReportData{OpenablePaths: []string{sourcePath}},
		reversedStep,
		nil,
		&reversedProbe,
	)
	if !reflect.DeepEqual(sources, reprojected) {
		t.Fatalf("dominant cluster depends on evidence order:\nfirst:  %#v\nsecond: %#v", sources, reprojected)
	}
}

func TestSelectInlineSourceLinesPrefersCompactEvidenceFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		startLine  int
		lineCount  int
		highlights []int
		wantLines  int
		wantStart  int
		wantEnd    int
		wantGaps   int
	}{
		{
			name:      "single evidence keeps bounded context around the operation",
			startLine: 1, lineCount: 120, highlights: []int{60},
			wantLines: 11, wantStart: 53, wantEnd: 63,
		},
		{
			name:      "far apart evidence becomes three fragments",
			startLine: 1, lineCount: 150, highlights: []int{10, 75, 140},
			wantLines: 33, wantStart: 3, wantEnd: 143, wantGaps: 2,
		},
		{
			name:      "dispersed sort evidence stays at preferred budget",
			startLine: 219, lineCount: 45,
			highlights: []int{223, 226, 228, 230, 232, 250, 259},
			wantLines:  37, wantStart: 219, wantEnd: 262, wantGaps: 1,
		},
		{
			name:      "short function still focuses the exact operation",
			startLine: 20, lineCount: 30, highlights: []int{34},
			wantLines: 11, wantStart: 27, wantEnd: 37,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lines := make([]savedSourceLine, 0, test.lineCount)
			for offset := range test.lineCount {
				line := test.startLine + offset
				lines = append(lines, savedSourceLine{line: line, text: fmt.Sprintf("line %d", line)})
			}
			got := selectInlineSourceLines(lines, test.highlights)
			if len(got) != test.wantLines || got[0].line != test.wantStart ||
				got[len(got)-1].line != test.wantEnd {
				t.Fatalf("selected %d lines at %d-%d, want %d at %d-%d",
					len(got), got[0].line, got[len(got)-1].line,
					test.wantLines, test.wantStart, test.wantEnd)
			}
			gaps := 0
			for index := 1; index < len(got); index++ {
				if got[index].line != got[index-1].line+1 {
					gaps++
				}
			}
			if gaps != test.wantGaps {
				t.Fatalf("fragment gaps = %d, want %d", gaps, test.wantGaps)
			}
			for _, highlight := range test.highlights {
				found := false
				for _, line := range got {
					found = found || line.line == highlight
				}
				if !found {
					t.Fatalf("evidence line %d was omitted from %#v", highlight, got)
				}
			}
		})
	}
}

func TestProjectStepSourceSnippetsCompactsAroundEvidenceAndRetainsBoundedFullFunction(t *testing.T) {
	t.Parallel()

	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction("internal/handler.go", "Handler", 100, 120),
	}}
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-handler", Path: "internal/handler.go", Line: 160},
	}}
	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1", len(sources))
	}
	snippet := sources[0]
	if got, want := len(snippet.Lines), 11; got != want {
		t.Fatalf("compact source lines = %d, want %d", got, want)
	}
	if snippet.StartLine != 153 || snippet.EndLine != 163 {
		t.Fatalf("compact source bounds = %d-%d, want 153-163", snippet.StartLine, snippet.EndLine)
	}
	if got, want := len(snippet.FullFunctionLines), 120; got != want {
		t.Fatalf("full function lines = %d, want %d", got, want)
	}
	if snippet.FullFunctionStartLine != 100 || snippet.FullFunctionEndLine != 219 {
		t.Fatalf("full function bounds = %d-%d, want 100-219",
			snippet.FullFunctionStartLine, snippet.FullFunctionEndLine)
	}
	if !sourceLineByNumber(t, snippet, 160).Highlight {
		t.Fatal("exact evidence line is not highlighted in compact source")
	}
	if !sourceLineByNumberIn(t, snippet.FullFunctionLines, 160).Highlight {
		t.Fatal("exact evidence line is not highlighted in full function")
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("compact/full source is invalid: %v", err)
	}

	tampered := cloneSourceSnippet(t, snippet)
	tampered.FullFunctionLines[1].Text = "tampered"
	tampered.PresentationSHA256 = sourceSnippetPresentationSHA(tampered)
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate() accepted full-function bytes that do not match content_sha256")
	}
}

func TestProjectStepSourceSnippetsOmitsOversizedFullFunctionCapability(t *testing.T) {
	t.Parallel()

	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction("internal/large.go", "Large", 1, maxFullFunctionSourceLines+1),
	}}
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-large", Path: "internal/large.go", Line: 100},
	}}
	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1", len(sources))
	}
	if len(sources[0].FullFunctionLines) != 0 || sources[0].FullFunctionStartLine != 0 ||
		sources[0].FullFunctionEndLine != 0 {
		t.Fatalf("oversized full function capability was published: %#v", sources[0])
	}
	if err := sources[0].Validate(); err != nil {
		t.Fatalf("compact source without full function is invalid: %v", err)
	}
}

func TestProjectStepSourceNoticesUsesOnlyAcceptedLineScopedObservations(t *testing.T) {
	t.Parallel()

	const (
		statementID = "statement-dispatch"
		factID      = "fact-dispatch"
		sourcePath  = "internal/dispatch.go"
	)
	statement := semanticdiscovery.Statement{
		ID:    statementID,
		Text:  "The handler checks the route; it dispatches the matched endpoint.",
		Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{factID},
	}
	step := semanticdiscovery.Step{
		ID: "step-dispatch", StatementIDs: []string{statementID},
		Evidence: []semanticdiscovery.EvidenceRef{
			{ID: "evidence-check", Path: sourcePath, Line: 42},
			{ID: "evidence-dispatch", Path: sourcePath, Line: 43},
		},
	}
	data := &ReportData{SemanticSupplementalFacts: []semanticdiscovery.Fact{{
		ID: factID,
		Capabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityBranch,
			semanticdiscovery.CapabilityDirectCall,
		},
		Evidence: []semanticdiscovery.EvidenceRef{
			{ID: "evidence-check", Path: sourcePath, Line: 42},
			{ID: "evidence-dispatch", Path: sourcePath, Line: 43},
		},
	}}}
	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction(sourcePath, "Dispatch", 30, 30),
	}, Observations: []goldenmechanism.Observation{
		sourceProjectionObservation(
			"observation-check",
			"evidence-check",
			sourcePath,
			42,
			semanticdiscovery.CapabilityBranch,
			goldenmechanism.BasisBranch,
			"route != nil",
			"",
		),
		sourceProjectionObservation(
			"observation-dispatch",
			"evidence-dispatch",
			sourcePath,
			43,
			semanticdiscovery.CapabilityDirectCall,
			goldenmechanism.BasisDirectCall,
			"",
			"Handler.ServeHTTP",
		),
	}}
	sources := projectStepSourceSnippets(data, step, map[string]semanticdiscovery.Statement{
		statementID: statement,
	}, &probe)
	notices := projectStepSourceNotices(data, step, map[string]semanticdiscovery.Statement{
		statementID: statement,
	}, sources)
	wantTexts := []string{
		"Checks route != nil.",
		"Calls Handler.ServeHTTP.",
	}
	if len(notices) != len(wantTexts) {
		t.Fatalf("notices = %#v, want %d deterministic clauses", notices, len(wantTexts))
	}
	for index, notice := range notices {
		if notice.Text != wantTexts[index] || notice.Path != sourcePath {
			t.Fatalf("notice[%d] = %#v, want text %q at %s", index, notice, wantTexts[index], sourcePath)
		}
		if notice.Text == statement.Text {
			t.Fatalf("notice[%d] repeats the full accepted statement", index)
		}
		wantRange := []SourceHighlight{{StartLine: 42 + index, EndLine: 42 + index}}
		if got := notice.SupportingRanges; !reflect.DeepEqual(got, wantRange) {
			t.Fatalf("notice[%d] ranges = %#v, want %#v", index, got, wantRange)
		}
		if err := notice.Validate(); err != nil {
			t.Fatalf("notice[%d] is invalid: %v", index, err)
		}
	}
}

func TestProjectStepSourceNoticesAbstainsWithoutSingleVisibleExactSupport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		factEvidence []semanticdiscovery.EvidenceRef
		stepEvidence []semanticdiscovery.EvidenceRef
	}{
		{
			name: "supporting evidence absent from step",
			factEvidence: []semanticdiscovery.EvidenceRef{
				{ID: "evidence-hidden", Path: "internal/a.go", Line: 5},
			},
			stepEvidence: []semanticdiscovery.EvidenceRef{
				{ID: "evidence-visible", Path: "internal/a.go", Line: 6},
			},
		},
		{
			name: "support spans source files",
			factEvidence: []semanticdiscovery.EvidenceRef{
				{ID: "evidence-a", Path: "internal/a.go", Line: 5},
				{ID: "evidence-b", Path: "internal/b.go", Line: 7},
			},
			stepEvidence: []semanticdiscovery.EvidenceRef{
				{ID: "evidence-a", Path: "internal/a.go", Line: 5},
				{ID: "evidence-b", Path: "internal/b.go", Line: 7},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			statement := semanticdiscovery.Statement{
				ID: "statement", Text: "Accepted claim.", Basis: semanticdiscovery.ClaimDirect,
				SupportIDs: []string{"fact"},
			}
			data := &ReportData{SemanticSupplementalFacts: []semanticdiscovery.Fact{{
				ID: "fact", Evidence: test.factEvidence,
			}}}
			step := semanticdiscovery.Step{
				StatementIDs: []string{"statement"}, Evidence: test.stepEvidence,
			}
			sources := []SourceSnippet{{
				Path:            "internal/a.go",
				Lines:           []SourceSnippetLine{{Line: 5, Text: "a", Highlight: true}},
				HighlightRanges: []SourceHighlight{{StartLine: 5, EndLine: 5}},
			}}
			if got := projectStepSourceNotices(data, step, map[string]semanticdiscovery.Statement{
				"statement": statement,
			}, sources); len(got) != 0 {
				t.Fatalf("unsafe notice was published: %#v", got)
			}
		})
	}
}

func TestProjectStepSourceNoticesRejectsObservationAtDifferentSourceLocation(t *testing.T) {
	t.Parallel()

	const (
		statementID = "statement-dispatch"
		factID      = "fact-dispatch"
		sourcePath  = "internal/dispatch.go"
	)
	statement := semanticdiscovery.Statement{
		ID: statementID, Text: "The selected handler is called.",
		Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{factID},
	}
	step := semanticdiscovery.Step{
		StatementIDs: []string{statementID},
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "evidence-dispatch", Path: sourcePath, Line: 43,
		}},
	}
	data := &ReportData{SemanticSupplementalFacts: []semanticdiscovery.Fact{{
		ID: factID,
		Capabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityDirectCall,
		},
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "evidence-dispatch", Path: sourcePath, Line: 43,
		}},
	}}}
	probe := goldenmechanism.Result{
		Functions: []goldenmechanism.Function{
			sourceProjectionFunction(sourcePath, "Dispatch", 30, 30),
		},
		Observations: []goldenmechanism.Observation{
			sourceProjectionObservation(
				"observation-dispatch", "evidence-dispatch", sourcePath, 44,
				semanticdiscovery.CapabilityDirectCall, goldenmechanism.BasisDirectCall,
				"", "Handler.ServeHTTP",
			),
		},
	}
	statements := map[string]semanticdiscovery.Statement{statementID: statement}
	sources := projectStepSourceSnippets(data, step, statements, &probe)
	if got := projectStepSourceNotices(data, step, statements, sources); len(got) != 0 {
		t.Fatalf("notice from mismatched observation location was published: %#v", got)
	}
}

func TestSourceNoticeValidateRejectsInvalidLineRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		notice SourceNotice
	}{
		{
			name:   "empty ranges",
			notice: SourceNotice{Text: "Accepted claim.", Path: "internal/a.go"},
		},
		{
			name: "reversed range",
			notice: SourceNotice{Text: "Accepted claim.", Path: "internal/a.go",
				SupportingRanges: []SourceHighlight{{StartLine: 8, EndLine: 7}}},
		},
		{
			name: "overlapping ranges",
			notice: SourceNotice{Text: "Accepted claim.", Path: "internal/a.go",
				SupportingRanges: []SourceHighlight{
					{StartLine: 5, EndLine: 8}, {StartLine: 8, EndLine: 9},
				}},
		},
		{
			name: "path traversal",
			notice: SourceNotice{Text: "Accepted claim.", Path: "../a.go",
				SupportingRanges: []SourceHighlight{{StartLine: 5, EndLine: 5}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.notice.Validate(); err == nil {
				t.Fatalf("Validate() accepted %#v", test.notice)
			}
		})
	}
}

func TestSourceSnippetValidateRejectsTraversalAndTampering(t *testing.T) {
	t.Parallel()

	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{
		sourceProjectionFunction("internal/source.go", "Source", 20, 12),
	}}
	step := semanticdiscovery.Step{Evidence: []semanticdiscovery.EvidenceRef{
		{ID: "evidence-source", Path: "internal/source.go", Line: 24},
	}}
	sources := projectStepSourceSnippets(&ReportData{}, step, nil, &probe)
	if len(sources) != 1 {
		t.Fatalf("source cards = %d, want 1", len(sources))
	}
	base := sources[0]
	if err := base.Validate(); err != nil {
		t.Fatalf("fixture snippet is invalid: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*SourceSnippet)
	}{
		{
			name: "parent traversal",
			mutate: func(snippet *SourceSnippet) {
				snippet.Path = "../outside.go"
				snippet.PresentationSHA256 = sourceSnippetPresentationSHA(*snippet)
			},
		},
		{
			name: "non canonical traversal",
			mutate: func(snippet *SourceSnippet) {
				snippet.Path = "internal/../outside.go"
				snippet.PresentationSHA256 = sourceSnippetPresentationSHA(*snippet)
			},
		},
		{
			name: "content changed after projection",
			mutate: func(snippet *SourceSnippet) {
				snippet.Content += "\nchanged"
			},
		},
		{
			name: "highlight escapes source bounds",
			mutate: func(snippet *SourceSnippet) {
				snippet.HighlightRanges[0].StartLine = snippet.StartLine - 1
				snippet.PresentationSHA256 = sourceSnippetPresentationSHA(*snippet)
			},
		},
		{
			name: "omitted line marker changed",
			mutate: func(snippet *SourceSnippet) {
				snippet.Lines[1].GapBefore = true
				snippet.PresentationSHA256 = sourceSnippetPresentationSHA(*snippet)
			},
		},
		{
			name: "presentation digest changed",
			mutate: func(snippet *SourceSnippet) {
				snippet.PresentationSHA256 = strings.Repeat("0", sha256.Size*2)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snippet := cloneSourceSnippet(t, base)
			test.mutate(&snippet)
			if err := snippet.Validate(); err == nil {
				t.Fatalf("Validate() accepted mutated snippet: %#v", snippet)
			}
		})
	}
}

func TestUserMechanismSourceProjectionDoesNotChangeSemanticAuthority(t *testing.T) {
	t.Parallel()

	data, artifact, probe := sourceProjectionFixture()
	data.SemanticArtifacts = []semanticdiscovery.Artifact{artifact}
	artifactBefore := mustSourceProjectionJSON(t, data.SemanticArtifacts)
	artifactSHA := sha256.Sum256(artifactBefore)

	canonical := semanticdiscovery.Mechanism{
		Version: semanticdiscovery.MechanismVersion,
		ID:      "semantic-mechanism-source-projection",
		Identity: semanticdiscovery.MechanismIdentity{
			RepositoryNamespace: "example.com/repository",
			IntentKey:           "request-dispatch",
			Scope: semanticdiscovery.MechanismScope{
				Kind:  semanticdiscovery.MechanismScopeGoPackage,
				Value: "example.com/repository/internal",
			},
		},
		Input: semanticdiscovery.MechanismInputManifest{
			Version:          semanticdiscovery.MechanismInputVersion,
			ValidatorVersion: semanticdiscovery.MechanismValidatorVersion,
		},
		Payload: semanticdiscovery.MechanismPayload{
			Version:       semanticdiscovery.MechanismPayloadVersion,
			OrderingBasis: semanticdiscovery.MechanismOrderingEditorial,
			Candidate: semanticdiscovery.OpportunityCandidate{
				ID: artifact.CandidateID, Kind: semanticdiscovery.ArtifactMechanism,
			},
		},
	}
	mechanismBefore := mustSourceProjectionJSON(t, canonical)
	mechanismSHA, err := semanticdiscovery.MechanismContentHash(canonical)
	if err != nil {
		t.Fatal(err)
	}

	projection, ok := projectUserMechanism(data, artifact, probe)
	if !ok {
		t.Fatal("projectUserMechanism() did not publish the fixture")
	}
	data.UserMechanisms = mergeUserMechanism(data.UserMechanisms, projection)

	artifactAfter := mustSourceProjectionJSON(t, data.SemanticArtifacts)
	if !bytes.Equal(artifactBefore, artifactAfter) || sha256.Sum256(artifactAfter) != artifactSHA {
		t.Fatalf("presentation projection changed canonical artifacts:\nbefore: %s\nafter:  %s",
			artifactBefore, artifactAfter)
	}
	mechanismAfter := mustSourceProjectionJSON(t, canonical)
	if !bytes.Equal(mechanismBefore, mechanismAfter) {
		t.Fatal("presentation projection changed canonical Mechanism fields or identity")
	}
	gotMechanismSHA, err := semanticdiscovery.MechanismContentHash(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if gotMechanismSHA != mechanismSHA {
		t.Fatalf("Mechanism content hash = %s after projection, want %s", gotMechanismSHA, mechanismSHA)
	}
	if len(data.UserMechanisms) != 1 || len(data.UserMechanisms[0].Steps[0].Sources) == 0 {
		t.Fatalf("presentation source was not stored separately: %#v", data.UserMechanisms)
	}
}

func TestProjectOverviewSourceSnippetsRanksLandmarksBeforeDeduplication(t *testing.T) {
	t.Parallel()

	http := overviewSourceEvidence(
		"evidence-http", "caddyconfig/httpcaddyfile/httptype.go", 1,
		"package httpcaddyfile", "", "func init() {", "}", "", "type ServerType struct{}", "",
		"func (st ServerType) Setup(", "\tinput []ServerBlock,", ") (*Config, error) {", "\treturn nil, nil", "}",
	)
	admin := overviewSourceEvidence(
		"evidence-admin", "admin.go", 200,
		"type AdminPermissions struct {", "\tOrigins []string", "}", "",
		"func (admin *AdminConfig) newAdminHandler() http.Handler {", "\tmux := http.NewServeMux()", "\treturn mux", "}",
	)
	adminDuplicate := admin
	adminDuplicate.ID = "evidence-admin-duplicate"
	parse := overviewSourceEvidence(
		"evidence-parse", "caddyconfig/caddyfile/parse.go", 101,
		"type parser struct{}", "", "func (p *parser) parseAll() ([]ServerBlock, error) {",
		"\tvar blocks []ServerBlock", "\treturn blocks, nil", "}",
	)
	data := &ReportData{
		OpenablePaths: []string{http.Location.Path, admin.Location.Path, parse.Location.Path},
		FirstFilesToOpen: []FileItem{{
			Path: admin.Location.Path, Reason: "Admin API handlers and configuration endpoint.", Priority: 1,
		}},
		ModelResearch: &modelresearch.State{Theory: modelresearch.WorkingTheory{
			GroundedFacts: []modelresearch.EvidenceItem{admin, parse, adminDuplicate, http},
			AcceptedModelInterpretations: []modelresearch.ValidatedFinding{
				{ResponsibilityName: "newAdminHandler", EvidenceIDs: []string{admin.ID, adminDuplicate.ID}},
				{ResponsibilityName: "config adaptation pipeline", EvidenceIDs: []string{http.ID, parse.ID}},
			},
		}},
	}

	got := projectOverviewSourceSnippets(data)
	if len(got) != 3 {
		t.Fatalf("overview landmarks = %d, want 3 unique paths: %#v", len(got), got)
	}
	wantPaths := []string{http.Location.Path, admin.Location.Path, parse.Location.Path}
	wantSymbols := []string{"Setup", "newAdminHandler", "parseAll"}
	wantKinds := []SourceLandmarkKind{
		SourceLandmarkPublicAPI, SourceLandmarkOrientation, SourceLandmarkCore,
	}
	for index := range wantPaths {
		if got[index].Path != wantPaths[index] || got[index].EnclosingSymbol != wantSymbols[index] ||
			got[index].LandmarkKind != wantKinds[index] {
			t.Fatalf("landmark[%d] = %s %s %s, want %s %s %s", index,
				got[index].Path, got[index].EnclosingSymbol, got[index].LandmarkKind,
				wantPaths[index], wantSymbols[index], wantKinds[index])
		}
		if got[index].LandmarkReason == "" || len(got[index].Lines) == 0 {
			t.Fatalf("landmark[%d] lacks reason or code: %#v", index, got[index])
		}
		if err := got[index].Validate(); err != nil {
			t.Fatalf("landmark[%d] is invalid: %v", index, err)
		}
	}
	if got[1].EnclosingSymbol == "AdminPermissions" {
		t.Fatalf("metadata struct became the default admin landmark: %#v", got[1])
	}
	if len(got[1].RelatedEvidenceIDs) != 2 {
		t.Fatalf("deduplicated admin landmark lost source references: %#v", got[1].RelatedEvidenceIDs)
	}

	reversed := append([]modelresearch.EvidenceItem(nil), data.ModelResearch.Theory.GroundedFacts...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	data.ModelResearch.Theory.GroundedFacts = reversed
	if reprojected := projectOverviewSourceSnippets(data); !reflect.DeepEqual(reprojected, got) {
		t.Fatalf("overview landmark order depends on grounded-fact order:\nfirst:  %#v\nsecond: %#v", got, reprojected)
	}
}

func TestProjectOverviewSourceSnippetsRanksBeforeLimit(t *testing.T) {
	t.Parallel()

	items := make([]modelresearch.EvidenceItem, 0, maxOverviewSourceSnippets+2)
	paths := make([]string, 0, cap(items))
	for index := 0; index < maxOverviewSourceSnippets+1; index++ {
		sourcePath := fmt.Sprintf("internal/core_%02d.go", index)
		items = append(items, overviewSourceEvidence(
			fmt.Sprintf("evidence-core-%02d", index), sourcePath, 1,
			fmt.Sprintf("func helper%d() {", index), "}",
		))
		paths = append(paths, sourcePath)
	}
	publicPath := "api/public.go"
	items = append(items, overviewSourceEvidence(
		"evidence-public", publicPath, 1, "func Serve() {", "}",
	))
	paths = append(paths, publicPath)
	data := &ReportData{
		OpenablePaths: paths,
		ModelResearch: &modelresearch.State{Theory: modelresearch.WorkingTheory{
			GroundedFacts: items,
		}},
	}

	got := projectOverviewSourceSnippets(data)
	if len(got) != maxOverviewSourceSnippets {
		t.Fatalf("landmark count = %d, want %d", len(got), maxOverviewSourceSnippets)
	}
	if got[0].Path != publicPath || got[0].LandmarkKind != SourceLandmarkPublicAPI {
		t.Fatalf("late public API did not outrank earlier generic windows: %#v", got[0])
	}
}

func TestBoundedSourceDeclarationClassifiesCallableLandmarks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		line string
		want SourceLandmarkKind
	}{
		{"cmd/tool/main.go", "func main() {", SourceLandmarkCLIEntrypoint},
		{"examples/basic/main.go", "func main() {", SourceLandmarkCore},
		{"server.go", "func ServeHTTP() {", SourceLandmarkPublicAPI},
		{"server.go", "func NewServer() *Server {", SourceLandmarkConstructor},
		{"server.go", "func requestHandler() {", SourceLandmarkHandler},
		{"server.go", "type AdminPermissions struct {", SourceLandmarkCore},
	}
	for _, test := range tests {
		t.Run(test.path+"/"+test.line, func(t *testing.T) {
			_, _, got, ok := boundedSourceDeclaration(test.path, test.line)
			if !ok || got != test.want {
				t.Fatalf("boundedSourceDeclaration(%q, %q) = %q, %v; want %q", test.path, test.line, got, ok, test.want)
			}
		})
	}
}

func overviewSourceEvidence(
	id string,
	sourcePath string,
	startLine int,
	lines ...string,
) modelresearch.EvidenceItem {
	return modelresearch.EvidenceItem{
		ID:       id,
		Kind:     modelresearch.EvidenceSource,
		Location: &evidence.Location{Path: sourcePath, Line: startLine},
		Window: &modelresearch.SourceWindow{
			StartLine: startLine, EndLine: startLine + len(lines) - 1,
			Lines: lines, CodeBearing: true,
		},
	}
}

func sourceProjectionFixture() (*ReportData, semanticdiscovery.Artifact, goldenmechanism.Result) {
	paths := []string{
		"internal/a_primary.go",
		"internal/b_secondary.go",
		"internal/c_third.go",
		"internal/d_dropped.go",
	}
	symbols := []string{"Primary", "Secondary", "Third", "Dropped"}
	functions := make([]goldenmechanism.Function, 0, len(paths))
	facts := make([]semanticdiscovery.Fact, 0, len(paths))
	for index := range paths {
		functions = append(functions, sourceProjectionFunction(paths[index], symbols[index], 1, 16))
		facts = append(facts, semanticdiscovery.Fact{
			ID: fmt.Sprintf("fact-%d", index),
			Source: &semanticdiscovery.FactSource{
				Path: paths[index], EnclosingSymbol: symbols[index],
			},
		})
	}
	data := &ReportData{
		OpenablePaths:             paths,
		CapturedRevision:          "revision-source-projection",
		SemanticSupplementalFacts: facts,
	}
	artifact := semanticdiscovery.Artifact{
		Version:     semanticdiscovery.ArtifactVersion,
		ID:          "artifact-source-projection",
		CandidateID: "candidate-source-projection",
		Kind:        semanticdiscovery.ArtifactMechanism,
		Verdict:     semanticdiscovery.VerdictSupported,
		Title:       "Source projection",
		Question:    "How does the request move through the implementation?",
		Statements: []semanticdiscovery.Statement{
			{
				ID: "statement-dispatch", Text: "The primary function coordinates related helpers.",
				Basis:      semanticdiscovery.ClaimCompositional,
				SupportIDs: []string{"fact-0", "fact-1", "fact-2", "fact-3"},
			},
			{
				ID: "statement-return", Text: "The primary function returns the result.",
				Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"fact-0"},
			},
		},
		Steps: []semanticdiscovery.Step{
			{
				ID: "step-dispatch", Title: "Coordinate helpers", StatementIDs: []string{"statement-dispatch"},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-dropped", Path: paths[3], Line: 4},
					{ID: "evidence-third-08", Path: paths[2], Line: 8},
					{ID: "evidence-secondary-07", Path: paths[1], Line: 7},
					{ID: "evidence-primary-12", Path: paths[0], Line: 12},
					{ID: "evidence-primary-02", Path: paths[0], Line: 2},
					{ID: "evidence-third-04", Path: paths[2], Line: 4},
					{ID: "evidence-secondary-03", Path: paths[1], Line: 3},
					{ID: "evidence-primary-10", Path: paths[0], Line: 10},
					{ID: "evidence-primary-04", Path: paths[0], Line: 4},
					{ID: "evidence-third-06", Path: paths[2], Line: 6},
					{ID: "evidence-secondary-05", Path: paths[1], Line: 5},
					{ID: "evidence-primary-08", Path: paths[0], Line: 8},
					{ID: "evidence-primary-06", Path: paths[0], Line: 6},
				},
			},
			{
				ID: "step-return", Title: "Return result", StatementIDs: []string{"statement-return"},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-primary-return", Path: paths[0], Line: 14},
				},
			},
		},
	}
	return data, artifact, goldenmechanism.Result{Functions: functions}
}

func sourceProjectionFunction(sourcePath, symbol string, startLine, lineCount int) goldenmechanism.Function {
	source := make([]goldenmechanism.SourceLine, 0, lineCount)
	for offset := range lineCount {
		line := startLine + offset
		text := fmt.Sprintf("\t%sStep(%d)", symbol, line)
		switch offset {
		case 0:
			text = fmt.Sprintf("func %s() {", symbol)
		case lineCount - 1:
			text = "}"
		}
		source = append(source, goldenmechanism.SourceLine{
			ID: fmt.Sprintf("source-%s-%03d", strings.ToLower(symbol), line),
			Location: evidence.Location{
				Path: sourcePath, Line: line, Column: 1,
				EndLine: line, EndColumn: len(text) + 1,
			},
			Text: text,
		})
	}
	return goldenmechanism.Function{
		ID:       "function-" + strings.ToLower(symbol),
		Symbol:   symbol,
		Path:     sourcePath,
		Location: evidence.Location{Path: sourcePath, Line: startLine, EndLine: startLine + lineCount - 1},
		Seed:     true,
		Source:   source,
	}
}

func sourceProjectionObservation(
	id string,
	evidenceID string,
	sourcePath string,
	line int,
	capability semanticdiscovery.Capability,
	basis goldenmechanism.SyntaxBasis,
	object string,
	targetSymbol string,
) goldenmechanism.Observation {
	return goldenmechanism.Observation{
		ID:           id,
		Capability:   capability,
		Operation:    string(capability),
		Object:       object,
		TargetSymbol: targetSymbol,
		Basis:        basis,
		Evidence: []goldenmechanism.EvidenceRef{{
			ID: evidenceID,
			Location: evidence.Location{
				Path: sourcePath,
				Line: line,
			},
		}},
	}
}

func sourceLineByNumber(t *testing.T, snippet SourceSnippet, lineNumber int) SourceSnippetLine {
	t.Helper()
	return sourceLineByNumberIn(t, snippet.Lines, lineNumber)
}

func sourceLineByNumberIn(
	t *testing.T,
	lines []SourceSnippetLine,
	lineNumber int,
) SourceSnippetLine {
	t.Helper()
	for _, line := range lines {
		if line.Line == lineNumber {
			return line
		}
	}
	t.Fatalf("source line %d is absent from %#v", lineNumber, lines)
	return SourceSnippetLine{}
}

func cloneSourceSnippet(t *testing.T, source SourceSnippet) SourceSnippet {
	t.Helper()
	raw := mustSourceProjectionJSON(t, source)
	var result SourceSnippet
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func mustSourceProjectionJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
