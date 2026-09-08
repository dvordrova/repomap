package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestReadCommandStartsFromSavedInputAndStopsBeforeReport(t *testing.T) {
	source := t.TempDir()
	opts := reading.Options{OwnerRunDir: source, Repository: "example", Revision: "abc",
		Targets: []reading.TargetMeta{{ID: "t1", Language: "go", Kind: "library", Name: "example", Root: "."}},
		Graph: atlas.Graph{Version: atlas.GraphVersion, Revision: "abc", Places: []atlas.Place{
			{ID: "dir:.", Kind: atlas.PlaceDirectory, Path: ".", TargetIDs: []string{"t1"}, Given: "one file", Directory: &atlas.DirectoryFacts{Files: []string{"main.go"}, FileCount: 1}},
			{ID: "file:main.go", Kind: atlas.PlaceFile, Path: "main.go", TargetIDs: []string{"t1"}, Parent: "dir:.", Given: "one function", File: &atlas.FileFacts{}},
		}}}
	if _, err := reading.SaveInput(opts); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "reading")
	var stdout bytes.Buffer
	// Provider nil is injected for this wiring test; the command itself always
	// constructs the configured online provider and has no offline fallback.
	factoryCalls := 0
	factory := func() (llm.Provider, error) { factoryCalls++; return nil, nil }
	args := []string{filepath.Join(source, reading.InputFilename), "--through", "files", "--output", output}
	if err := runReadWithProvider(context.Background(), args, &stdout, factory); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "reading-result.json"))
	if err != nil || !strings.Contains(string(raw), `"complete": false`) {
		t.Fatalf("summary: %s %v", raw, err)
	}
	for _, name := range []string{"atlas.json", "report.json", "report.html"} {
		if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
			t.Fatalf("partial reading produced %s", name)
		}
	}
	if err := runReadWithProvider(context.Background(), args, &stdout, factory); err == nil {
		t.Fatal("existing experiment overwritten")
	}
	factoryCalls = 0
	if err := runReadWithProvider(context.Background(), []string{args[0], "--through", "route", "--question", "Where is state?"}, &stdout, factory); err == nil || !strings.Contains(err.Error(), "use --through answer") || factoryCalls != 0 {
		t.Fatalf("removed selector needs an explicit migration diagnostic before provider setup: %v", err)
	}
	if err := runReadWithProvider(context.Background(), []string{args[0], "--through", "typo"}, &stdout, factory); err == nil || factoryCalls != 0 {
		t.Fatal("bad controls reached provider")
	}
	for _, flags := range [][]string{{"--through", "question"}, {"--through", "files", "--question", "Where is state?"}} {
		if err := runReadWithProvider(context.Background(), append([]string{args[0]}, flags...), &stdout, factory); err == nil || factoryCalls != 0 {
			t.Fatal("invalid question controls reached provider")
		}
	}
	questionOutput := filepath.Join(t.TempDir(), "question")
	if err := runReadWithProvider(context.Background(), []string{args[0], "--question", "Where is state?", "--output", questionOutput}, &stdout, factory); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(questionOutput, atlas.QuestionFilename))
	if err != nil || !strings.Contains(string(raw), `"question": "Where is state?"`) || !strings.Contains(string(raw), `"unresolved_chunks": 1`) {
		t.Fatalf("question output: %s %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(questionOutput, "atlas.json")); !os.IsNotExist(err) {
		t.Fatal("question-only pass produced a complete atlas")
	}
}
