package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunLocalizationStageCLIRequiresRunAndOptionalResponse(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{},
		{"-run"},
		{"run", "-response"},
		{"run", "response", "extra"},
	} {
		var stdout bytes.Buffer
		err := runLocalizationStageCLIWith(
			context.Background(),
			args,
			&stdout,
			func(string) ([]byte, error) { return []byte(`{}`), nil },
			func(context.Context, string, string) ([]byte, error) {
				return []byte(`{}`), nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("runLocalizationStageCLIWith(%q) error = %v, want usage", args, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runLocalizationStageCLIWith(%q) stdout = %q", args, stdout.String())
		}
	}
}

func TestRunLocalizationStageCLIPreviewsExactPrompt(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	gotRunDir := ""
	replayCalled := false
	err := runLocalizationStageCLIWith(
		context.Background(),
		[]string{"relative-run"},
		&stdout,
		func(runDir string) ([]byte, error) {
			gotRunDir = runDir
			return []byte(`{"version":"localization-projection-json-v1"}`), nil
		},
		func(context.Context, string, string) ([]byte, error) {
			replayCalled = true
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotRunDir == "" || !strings.HasSuffix(gotRunDir, "relative-run") {
		t.Fatalf("absolute run dir = %q", gotRunDir)
	}
	if replayCalled {
		t.Fatal("prompt preview called the response replay")
	}
	if stdout.String() != "{\"version\":\"localization-projection-json-v1\"}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationStageCLIReplaysSavedResponse(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	buildCalled := false
	gotRunDir := ""
	gotResponsePath := ""
	err := runLocalizationStageCLIWith(
		context.Background(),
		[]string{"relative-run", "relative-response.json"},
		&stdout,
		func(string) ([]byte, error) {
			buildCalled = true
			return nil, nil
		},
		func(_ context.Context, runDir, responsePath string) ([]byte, error) {
			gotRunDir = runDir
			gotResponsePath = responsePath
			return []byte(`{"locale":"ru"}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if buildCalled {
		t.Fatal("response replay called the standalone prompt preview")
	}
	if !strings.HasSuffix(gotRunDir, "relative-run") ||
		!strings.HasSuffix(gotResponsePath, "relative-response.json") {
		t.Fatalf("resolved paths = %q, %q", gotRunDir, gotResponsePath)
	}
	if stdout.String() != "{\"locale\":\"ru\"}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationStageCLIRejectsEmptyResult(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := runLocalizationStageCLIWith(
		context.Background(),
		[]string{"run"},
		&stdout,
		func(string) ([]byte, error) { return nil, nil },
		func(context.Context, string, string) ([]byte, error) { return nil, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "empty result") {
		t.Fatalf("empty result error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
