package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLocalizationReplayCLIRequiresRunAndProjection(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{"run"},
		{"run", "projection.json", "extra"},
		{"", "projection.json"},
		{"run", ""},
		{"--run", "projection.json"},
		{"run", "--projection"},
	} {
		var stdout bytes.Buffer
		err := runLocalizationReplayCLIWith(
			args,
			&stdout,
			func(string, string) ([]byte, error) {
				t.Fatal("replay called for invalid arguments")
				return nil, nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("runLocalizationReplayCLIWith(%q) error = %v, want usage", args, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runLocalizationReplayCLIWith(%q) stdout = %q", args, stdout.String())
		}
	}
}

func TestRunLocalizationReplayCLIWritesCompactJSONAndNewline(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	projectionPath := filepath.Join(t.TempDir(), "projection.json")
	var gotRunDir, gotProjectionPath string
	var stdout bytes.Buffer
	err := runLocalizationReplayCLIWith(
		[]string{runDir, projectionPath},
		&stdout,
		func(runDir, projectionPath string) ([]byte, error) {
			gotRunDir = runDir
			gotProjectionPath = projectionPath
			return []byte(`{"version":1,"locale":"ru"}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotRunDir != runDir {
		t.Fatalf("run dir = %q, want %q", gotRunDir, runDir)
	}
	if gotProjectionPath != projectionPath {
		t.Fatalf("projection path = %q, want %q", gotProjectionPath, projectionPath)
	}
	if got := stdout.String(); got != "{\"version\":1,\"locale\":\"ru\"}\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunLocalizationReplayCLIDoesNotWriteOnReplayFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("replay failed")
	var stdout bytes.Buffer
	err := runLocalizationReplayCLIWith(
		[]string{t.TempDir(), filepath.Join(t.TempDir(), "projection.json")},
		&stdout,
		func(string, string) ([]byte, error) {
			return nil, wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationReplayCLIRejectsEmptyReplayResult(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := runLocalizationReplayCLIWith(
		[]string{t.TempDir(), filepath.Join(t.TempDir(), "projection.json")},
		&stdout,
		func(string, string) ([]byte, error) {
			return nil, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "empty result") {
		t.Fatalf("error = %v, want empty result", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
