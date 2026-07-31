package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestRunLocalizationRecordCLIRequiresRunAndOptionalResponse(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{},
		{"-run"},
		{"run", "-response"},
		{"run", "response", "extra"},
	} {
		var stdout bytes.Buffer
		factoryCalled := false
		err := runLocalizationRecordCLIWith(
			context.Background(),
			args,
			&stdout,
			func() (*deepseek.Client, error) {
				factoryCalled = true
				return &deepseek.Client{}, nil
			},
			func(
				context.Context,
				string,
				string,
				func(localization.Prompt) (deepseek.LocalizationRequestEvidence, error),
			) ([]byte, error) {
				return []byte(`{}`), nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("runLocalizationRecordCLIWith(%q) error = %v, want usage", args, err)
		}
		if factoryCalled {
			t.Fatalf("runLocalizationRecordCLIWith(%q) configured provider before usage validation", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runLocalizationRecordCLIWith(%q) stdout = %q", args, stdout.String())
		}
	}
}

func TestRunLocalizationRecordCLILooksUpWithoutResponse(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	gotRunDir := ""
	gotResponsePath := "unset"
	err := runLocalizationRecordCLIWith(
		context.Background(),
		[]string{"relative-run"},
		&stdout,
		func() (*deepseek.Client, error) {
			return &deepseek.Client{
				Endpoint: "https://example.test/v1/chat/completions",
				Model:    "model", MaxTokens: 100,
			}, nil
		},
		func(
			_ context.Context,
			runDir,
			responsePath string,
			buildRequest func(
				localization.Prompt,
			) (deepseek.LocalizationRequestEvidence, error),
		) ([]byte, error) {
			gotRunDir = runDir
			gotResponsePath = responsePath
			evidence, err := buildRequest(localization.Prompt{
				Version: localization.PromptVersion,
				System:  "system",
				User:    "user",
			})
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if evidence.Endpoint != "https://example.test/v1/chat/completions" ||
				len(evidence.Body) == 0 {
				t.Fatalf("request evidence = %#v", evidence)
			}
			return []byte(`{"status":"miss_not_found"}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotRunDir, "relative-run") || gotResponsePath != "" {
		t.Fatalf("resolved paths = %q, %q", gotRunDir, gotResponsePath)
	}
	if stdout.String() != "{\"status\":\"miss_not_found\"}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationRecordCLIPassesOptionalResponse(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	gotResponsePath := ""
	err := runLocalizationRecordCLIWith(
		context.Background(),
		[]string{"relative-run", "relative-response.json"},
		&stdout,
		func() (*deepseek.Client, error) {
			return &deepseek.Client{
				Endpoint: "https://example.test/v1/chat/completions",
				Model:    "model", MaxTokens: 100,
			}, nil
		},
		func(
			_ context.Context,
			_,
			responsePath string,
			_ func(localization.Prompt) (deepseek.LocalizationRequestEvidence, error),
		) ([]byte, error) {
			gotResponsePath = responsePath
			return []byte(`{"status":"stored"}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotResponsePath, "relative-response.json") {
		t.Fatalf("resolved response path = %q", gotResponsePath)
	}
	if stdout.String() != "{\"status\":\"stored\"}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationRecordCLIExactHitDoesNotReadInvalidResponsePath(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	missing := t.TempDir() + "/does-not-exist/projection.json"
	err := runLocalizationRecordCLIWith(
		context.Background(),
		[]string{"relative-run", missing},
		&stdout,
		func() (*deepseek.Client, error) {
			return &deepseek.Client{
				Endpoint: "https://example.test/v1/chat/completions",
				Model:    "model", MaxTokens: 100,
			}, nil
		},
		func(
			_ context.Context,
			_,
			responsePath string,
			_ func(localization.Prompt) (deepseek.LocalizationRequestEvidence, error),
		) ([]byte, error) {
			if responsePath != missing {
				t.Fatalf("response path = %q, want %q", responsePath, missing)
			}
			// An exact record hit must return before the report layer opens
			// the optional response path.
			return []byte(`{"status":"hit_exact"}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "{\"status\":\"hit_exact\"}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationRecordCLIRejectsEmptyResult(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := runLocalizationRecordCLIWith(
		context.Background(),
		[]string{"run"},
		&stdout,
		func() (*deepseek.Client, error) {
			return &deepseek.Client{}, nil
		},
		func(
			context.Context,
			string,
			string,
			func(localization.Prompt) (deepseek.LocalizationRequestEvidence, error),
		) ([]byte, error) {
			return nil, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "empty result") {
		t.Fatalf("empty result error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLocalizationRecordCLIDoesNotEchoMalformedProviderEnvironment(t *testing.T) {
	const secret = `company-secret-value-123456789`
	t.Setenv("REPOMAP_LLM_ENDPOINT", `https://example.test/%zz-api_key="`+secret+`"`)
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	var stdout bytes.Buffer
	err := runLocalizationRecordCLI([]string{t.TempDir()}, &stdout)
	if err == nil {
		t.Fatal("runLocalizationRecordCLI() error = nil")
	}
	if strings.Contains(err.Error(), secret) ||
		strings.Contains(err.Error(), "api_key") {
		t.Fatalf("provider environment leaked through error: %v", err)
	}
	if !strings.Contains(err.Error(), "configuration was rejected") {
		t.Fatalf("error = %v, want safe configuration rejection", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
