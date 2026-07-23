package main

import (
	"context"
	"os"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestChiSavedResponseOfflineReplayContract(t *testing.T) {
	runDir := os.Getenv("REPOMAP_CHI_DISPATCH_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_CHI_DISPATCH_RUN to the fixed chi run directory")
	}
	responseRaw, err := verifyFixedChiDispatchReplayInputs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	var saved goldenMechanismResponseAttempt
	if err := decodeChiDispatchJSON(responseRaw, &saved); err != nil {
		t.Fatal(err)
	}
	prepared, err := loadPreparedChiDispatch(context.Background(), runDir)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := semanticdiscovery.ParseFanInArtifact([]byte(saved.Content))
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticdiscovery.ValidatePartialFanInArtifact(
		prepared.Bundle,
		[]semanticdiscovery.LeafResult{prepared.Leaf},
		parsed,
	); err != nil {
		t.Fatalf("saved chi response content validation error = %v", err)
	}
	evaluated, err := evaluateGoldenMechanismResponse(
		prepared.Bundle,
		prepared.Proposal,
		prepared.Leaf,
		[]byte(saved.Content),
	)
	if err != nil {
		t.Fatalf("saved chi response evaluation error = %v; reduction = %#v", err, evaluated.Reduction)
	}
	if err := validateChiDispatchDerivedVerdict(evaluated); err != nil {
		t.Fatal(err)
	}
}

func TestParseChiRequestDispatchArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantDir  string
		wantMode chiDispatchMode
		wantErr  bool
	}{
		{name: "live", args: []string{"run"}, wantDir: "run", wantMode: chiDispatchLive},
		{name: "prepare", args: []string{"run", "--prepare"}, wantDir: "run", wantMode: chiDispatchPrepare},
		{name: "saved response replay", args: []string{"run", "--replay-response"}, wantDir: "run", wantMode: chiDispatchResponseReplay},
		{name: "replay before path", args: []string{"--replay", "run"}, wantDir: "run", wantMode: chiDispatchReplay},
		{name: "missing path", args: nil, wantErr: true},
		{name: "conflicting modes", args: []string{"run", "--prepare", "--replay"}, wantErr: true},
		{name: "extra path", args: []string{"one", "two"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir, mode, err := parseChiRequestDispatchArgs(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseChiRequestDispatchArgs() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && (dir != test.wantDir || mode != test.wantMode) {
				t.Fatalf("parseChiRequestDispatchArgs() = %q, %q; want %q, %q", dir, mode, test.wantDir, test.wantMode)
			}
		})
	}
}

func TestChiProbeFunctionWindowPreservesExactLines(t *testing.T) {
	t.Parallel()

	probe := goldenmechanism.Result{Functions: []goldenmechanism.Function{{
		ID: "gm-fn-0123456789abcdef01234567", Symbol: "Mux.routeHTTP", Path: "mux.go",
		Location: evidence.Location{Path: "mux.go", Line: 447, Column: 1, EndLine: 449, EndColumn: 2},
		Source: []goldenmechanism.SourceLine{
			{Location: evidence.Location{Path: "mux.go", Line: 447}, Text: "func (mx *Mux) routeHTTP() {"},
			{Location: evidence.Location{Path: "mux.go", Line: 448}, Text: "\tmx.tree.FindRoute()"},
			{Location: evidence.Location{Path: "mux.go", Line: 449}, Text: "}"},
		},
	}}}
	window, err := chiProbeFunctionWindow(probe, "Mux.routeHTTP")
	if err != nil {
		t.Fatal(err)
	}
	if window.Path != "mux.go" || window.StartLine != 447 || window.EndLine != 449 ||
		len(window.Lines) != 3 || window.Lines[1] != "\tmx.tree.FindRoute()" {
		t.Fatalf("window = %#v", window)
	}

	probe.Functions[0].Source[1].Location.Line = 450
	if _, err := chiProbeFunctionWindow(probe, "Mux.routeHTTP"); err == nil {
		t.Fatal("chiProbeFunctionWindow() accepted non-contiguous source")
	}
}

func TestChiObservationsCanSelectRepeatedCallByLine(t *testing.T) {
	t.Parallel()

	window, err := sourcewindowfacts.NewWindow(
		"evidence-mux",
		"mux.go",
		70,
		[]string{
			"func (mx *Mux) ServeHTTP() {",
			"\tif parent {",
			"\t\tmx.handler.ServeHTTP()",
			"\t}",
			"\tattach()",
			"\tmx.handler.ServeHTTP()",
			"}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, "Mux.ServeHTTP")
	if err != nil {
		t.Fatal(err)
	}
	observations, err := chiObservations(
		function,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.handler.ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.handler.ServeHTTP", lineAfter: 74},
	)
	if err != nil {
		t.Fatal(err)
	}
	if observations[0].Line != 72 || observations[1].Line != 75 {
		t.Fatalf("selected call lines = %d, %d", observations[0].Line, observations[1].Line)
	}
}
