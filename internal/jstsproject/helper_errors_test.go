package jstsproject

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

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func discoverFailureProject(t *testing.T, root string) error {
	t.Helper()
	paths := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = DiscoverSelected(t.Context(), repository, root, "jsts:package.json")
	return err
}

func writeFailureProject(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, root, "package.json", `{"name":"failure-case","devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "src/main.ts", "export const ready = true\n")
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/main.ts"]}`)
}

func TestHelperSourceFailureDoesNotInferMissingCompilerFromPath(t *testing.T) {
	root := preparedCompilerProject(t)
	writeFailureProject(t, root)
	for _, marker := range []string{
		"load prepared TypeScript compiler",
		"TypeScript compiler is not rooted in analyzed node_modules",
	} {
		t.Run(marker, func(t *testing.T) {
			writeTestFile(t, root, "tsconfig.json", fmt.Sprintf(`{"include":["src/main.ts"],"references":[{"path":"./%s"}]}`, marker))
			err := discoverFailureProject(t, root)
			if err == nil || !strings.Contains(err.Error(), "unresolved project reference") {
				t.Fatalf("expected native project reference failure, got %v", err)
			}
			if errors.Is(err, ErrTypeScriptCompilerUnavailable) {
				t.Fatalf("loaded compiler misclassified by a source path: %v", err)
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("source failure lost its process cause: %v", err)
			}
		})
	}
}

func TestHelperCompilerFailureKeepsTypedCauseDespiteDiagnosticClipping(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required to execute the embedded helper")
	}
	for _, kind := range []string{"missing", "load_error", "long_load_error"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			writeFailureProject(t, root)
			if kind != "missing" {
				writeTestFile(t, root, "node_modules/typescript/package.json", `{"name":"typescript","version":"5.9.3"}`)
				body := "throw new Error('compiler load failed')\n"
				if kind == "long_load_error" {
					body = fmt.Sprintf("process.stderr.write('x'.repeat(%d));\n", maxHelperStderrBytes+100) + body
				}
				writeTestFile(t, root, "node_modules/typescript/lib/typescript.js", body)
			}
			err := discoverFailureProject(t, root)
			var exitErr *exec.ExitError
			if !errors.Is(err, ErrTypeScriptCompilerUnavailable) || !errors.As(err, &exitErr) || exitErr.ExitCode() != helperCompilerUnavailableExitCode {
				t.Fatalf("compiler failure lost its typed/process cause: %v", err)
			}
			if kind == "long_load_error" && (!strings.Contains(err.Error(), "diagnostic truncated") || strings.Contains(err.Error(), "compiler load failed")) {
				t.Fatalf("test did not exercise clipped compiler diagnostic: %v", err)
			}
		})
	}
}

func TestHelperMissingNodeAndPriorCancellationKeepTheirCauses(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := invokeHelper(t.Context(), t.TempDir(), helperRequest{})
	var lookupErr *exec.Error
	if !errors.Is(err, ErrTypeScriptCompilerUnavailable) || !errors.As(err, &lookupErr) || lookupErr.Name != "node" {
		t.Fatalf("missing Node lost its prerequisite/lookup cause: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = invokeHelper(ctx, t.TempDir(), helperRequest{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTypeScriptCompilerUnavailable) {
		t.Fatalf("canceled request became a missing-tool failure: %v", err)
	}
}

func TestHelperUnclassifiedProcessFailureKeepsExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled executable uses a POSIX shell")
	}
	for _, diagnostic := range []string{"", "load prepared TypeScript compiler"} {
		t.Run(diagnostic, func(t *testing.T) {
			bin := t.TempDir()
			body := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s' >&2\nexit 23\n", diagnostic)
			if err := os.WriteFile(filepath.Join(bin, "node"), []byte(body), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			_, err := invokeHelper(t.Context(), t.TempDir(), helperRequest{})
			var exitErr *exec.ExitError
			if errors.Is(err, ErrTypeScriptCompilerUnavailable) || !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
				t.Fatalf("generic failure incorrectly classified or lost cause: %v", err)
			}
			if !strings.Contains(err.Error(), "exit status 23") || !strings.Contains(err.Error(), diagnostic) {
				t.Fatalf("process diagnostic missing from error: %v", err)
			}
		})
	}
}

func TestHelperRunningCancellationPrecedesFailureClassification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled executable uses a POSIX shell")
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "ready")
	// The helper has really started before cancellation; a pre-start timer can
	// accidentally pass without exercising command.Run's error path.
	body := "#!/bin/sh\nprintf ready > '" + strings.ReplaceAll(marker, "'", "'\"'\"'") + "'\nexec /bin/sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	root := t.TempDir()
	go func() {
		_, err := invokeHelper(ctx, root, helperRequest{})
		result <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case err := <-result:
			t.Fatalf("helper exited before cancellation: %v", err)
		case <-timeout.C:
			t.Fatal("helper did not signal it had started")
		case <-ticker.C:
			if _, err := os.Stat(marker); err == nil {
				cancel()
				select {
				case err := <-result:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("running cancellation lost: %v", err)
					}
				case <-timeout.C:
					t.Fatal("canceled helper did not exit")
				}
				return
			}
		}
	}
}
