package pythonprogramindex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIndexParserProcessFailureKeepsCauseAndBoundedStderr(t *testing.T) {
	for _, test := range []struct{ name, diagnostic string }{
		{"silent_exit", ""},
		{"whitespace_stderr", " \n\t"},
		{"stderr", " \ninterpreter failed: 'quoted detail'\n "},
		{"bounded_stderr", strings.Repeat("x", maxParserStderrBytes) + "not retained"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executable := controlledIndexParserExecutable(t, fmt.Sprintf("printf '%%s' %s >&2\nexit 23\n", indexParserShellLiteral(test.diagnostic)))
			t.Setenv("PATH", filepath.Dir(executable))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, err := runParser(ctx, parserRequest{})
			var processErr *exec.ExitError
			if !errors.As(err, &processErr) || processErr.ExitCode() != 23 {
				t.Fatalf("parser error lost exit status 23: %v", err)
			}
			expected := "python program index: isolated parser failed: exit status 23"
			diagnostic := test.diagnostic
			if len(diagnostic) > maxParserStderrBytes {
				diagnostic = diagnostic[:maxParserStderrBytes]
			}
			if diagnostic = strings.TrimSpace(diagnostic); diagnostic != "" {
				expected += ": " + diagnostic
			}
			if err.Error() != expected {
				t.Fatalf("parser diagnostic differs (got %d bytes, want %d): %.160q", len(err.Error()), len(expected), err)
			}
		})
	}
}

func TestIndexParserProcessCancellationKeepsContextCause(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	executable := controlledIndexParserExecutable(t, "printf ready > "+indexParserShellLiteral(ready)+"\nexec /bin/sleep 30\n")
	t.Setenv("PATH", filepath.Dir(executable))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runParser(ctx, parserRequest{}); done <- err }()
	// Wait for an executing process, rather than canceling during executable
	// startup (which can be delayed by host launch checks).
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	startup := time.NewTimer(10 * time.Second)
	defer startup.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("parser exited before readiness: %v", err)
		case <-startup.C:
			t.Fatal("controlled parser did not start")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("parser lost cancellation: %v", err)
		}
		var processErr *exec.ExitError
		if errors.As(err, &processErr) {
			t.Fatalf("cancellation became a process failure: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled parser did not exit")
	}
}

func TestIndexParserProcessRequiresSingleJSONDocument(t *testing.T) {
	const valid = `{"python_version":"3.14.0","views":[{"objects":[],"relations":[]}]}`
	for _, test := range []struct{ name, output, errorText string }{
		{"valid_whitespace", "\n " + valid + " \n\t", ""},
		{"malformed", `{"broken":`, "decode parser output"},
		{"unknown_field", `{"unexpected":true}`, "unknown field"},
		{"second_document", valid + ` {"fatal":"must not be ignored"}`, "trailing JSON"},
		{"trailing_garbage", valid + " garbage", "trailing JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executable := controlledIndexParserExecutable(t, "printf '%s' "+indexParserShellLiteral(test.output)+"\n")
			t.Setenv("PATH", filepath.Dir(executable))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			response, err := runParser(ctx, parserRequest{})
			if test.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), test.errorText) {
					t.Fatalf("invalid parser response error = %v, want %q", err, test.errorText)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid single response rejected: %v", err)
			}
			if response.PythonVersion != "3.14.0" || len(response.Views) != 1 || response.Views[0].Objects == nil || response.Views[0].Relations == nil {
				t.Fatalf("valid parser view lost: %#v", response)
			}
		})
	}
}

func controlledIndexParserExecutable(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("controlled parser executable requires POSIX sh")
	}
	executable := filepath.Join(t.TempDir(), "python3")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return executable
}

func indexParserShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func TestIndexParserCancellationBeforeMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, expected := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(expected.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if expected == context.DeadlineExceeded {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			}
			cancel()
			_, err := runParser(ctx, parserRequest{})
			if !errors.Is(err, expected) {
				t.Fatalf("cancellation became a missing-executable failure: %v, want %v", err, expected)
			}
		})
	}
	ctx := t.Context()
	_, err := runParser(ctx, parserRequest{})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("live context lost missing-executable failure: %v", err)
	}
}
