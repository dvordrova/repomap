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

func readCommandInputFixture(t *testing.T) string {
	t.Helper()
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
	return filepath.Join(source, reading.InputFilename)
}

func TestReadCommandStartsFromSavedInputAndStopsBeforeReport(t *testing.T) {
	input := readCommandInputFixture(t)
	output := filepath.Join(t.TempDir(), "reading")
	var stdout bytes.Buffer
	// Provider nil is injected for this wiring test; the command itself always
	// constructs the configured online provider and has no offline fallback.
	factoryCalls := 0
	factory := func() (llm.Provider, error) { factoryCalls++; return nil, nil }
	args := []string{input, "--through", "files", "--output", output}
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

func TestReadCommandExplicitOutputPreparesFreshSharedRoot(t *testing.T) {
	for _, noCache := range []bool{false, true} {
		name := "cache-enabled"
		if noCache {
			name = "no-cache"
		}
		t.Run(name, func(t *testing.T) {
			input := readCommandInputFixture(t)
			cacheRoot := filepath.Join(t.TempDir(), "fresh", "shared")
			if _, err := os.Stat(cacheRoot); !os.IsNotExist(err) {
				t.Fatalf("fixture cache root must not exist: %v", err)
			}
			outputParent := t.TempDir()
			provider := &terminologyRuntimeProvider{}
			factory := func() (llm.Provider, error) { return provider, nil }
			run := func(outputName string) {
				t.Helper()
				output := filepath.Join(outputParent, outputName)
				args := []string{input, "--through", "files", "--output", output, "--debug-dir", cacheRoot}
				if noCache {
					args = append(args, "--no-cache")
				}
				var stdout bytes.Buffer
				if err := runReadConfigured(context.Background(), args, &stdout, factory, true); err != nil {
					t.Fatalf("fresh shared root with explicit output: %v", err)
				}
				for _, artifact := range []string{"reading-result.json", "tables.md"} {
					if _, err := os.Stat(filepath.Join(output, artifact)); err != nil {
						t.Fatalf("missing reading artifact %s: %v", artifact, err)
					}
				}
				if _, err := os.Stat(filepath.Join(output, "report.html")); !os.IsNotExist(err) {
					t.Fatalf("development reading unexpectedly rendered HTML: %v", err)
				}
			}
			run("first")
			coldCalls := provider.calls
			if coldCalls == 0 {
				t.Fatal("fixture did not exercise the configured mock provider")
			}
			run("second")
			wantCalls := coldCalls
			if noCache {
				wantCalls *= 2
			}
			if provider.calls != wantCalls {
				t.Fatalf("cache policy changed: calls=%d want=%d", provider.calls, wantCalls)
			}
			payloads, err := os.ReadDir(filepath.Join(cacheRoot, llm.CacheDirectoryName, "payloads"))
			if err != nil || len(payloads) == 0 {
				t.Fatalf("required exact reading payloads missing: %v", err)
			}
			if noCache {
				records, err := filepath.Glob(filepath.Join(cacheRoot, llm.CacheDirectoryName, "*.json"))
				if err != nil || len(records) != 0 {
					t.Fatalf("--no-cache wrote response or memo records: %v / %v", records, err)
				}
			}
		})
	}
}
