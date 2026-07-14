package freshness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCaptureGoFactContextUsesDefaultBinariesAndCapturesBuildInputs(t *testing.T) {
	repo := t.TempDir()
	writeGoTestFile(t, filepath.Join(repo, ".freshness-test-root"), "marker")
	binDir := t.TempDir()
	writeGoTestExecutable(t, binDir, "go", `
if [ ! -f .freshness-test-root ]; then
	printf '%s\n' 'go command ran outside repository' >&2
	exit 21
fi
if [ "$#" -ne 8 ] || [ "$1" != "env" ] || [ "$2" != "-json" ] || [ "$3" != "GOVERSION" ] || [ "$4" != "GOOS" ] || [ "$5" != "GOARCH" ] || [ "$6" != "GOFLAGS" ] || [ "$7" != "GOWORK" ] || [ "$8" != "CGO_ENABLED" ]; then
	printf '%s\n' 'unexpected go arguments' >&2
	exit 22
fi
printf '%s\n' '{"GOVERSION":"go1.24.3","GOOS":"linux","GOARCH":"amd64","GOFLAGS":"-mod=readonly -tags=unit,integration -tags smoke,integration","GOWORK":"/repo/go.work","CGO_ENABLED":"1"}'
`)
	writeGoTestExecutable(t, binDir, "gopls", `
if [ ! -f .freshness-test-root ]; then
	printf '%s\n' 'gopls command ran outside repository' >&2
	exit 23
fi
if [ "$#" -ne 1 ] || [ "$1" != "version" ]; then
	printf '%s\n' 'unexpected gopls arguments' >&2
	exit 24
fi
printf '\n%s\n\n' 'golang.org/x/tools/gopls v0.23.1'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	repository := goTestRepositoryState(repo)
	got, err := CaptureGoFactContext(context.Background(), repository, GoOptions{
		Collector:        "symbol-neighborhood",
		CollectorVersion: "v2",
		AnalyzerOptions:  `{"max_call_roots":1}`,
		CollectorOptions: `{"max_candidates":12}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputJSON, err := json.Marshal(struct {
		GOFLAGS          string `json:"go_flags"`
		GOWORK           string `json:"go_work"`
		CGO              string `json:"cgo_enabled"`
		AnalyzerOptions  string `json:"analyzer_options"`
		CollectorOptions string `json:"collector_options"`
	}{
		GOFLAGS:          "-mod=readonly -tags=unit,integration -tags smoke,integration",
		GOWORK:           "/repo/go.work",
		CGO:              "1",
		AnalyzerOptions:  `{"max_call_roots":1}`,
		CollectorOptions: `{"max_candidates":12}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := FactContext{
		Version:          FactContextVersion,
		Repository:       repository,
		GoVersion:        "go1.24.3",
		Analyzer:         "gopls",
		AnalyzerVersion:  "v0.23.1",
		Collector:        "symbol-neighborhood",
		CollectorVersion: "v2",
		InputsSHA256:     sha256Hex(inputJSON),
		Build: evidence.BuildContext{
			GOOS:      "linux",
			GOARCH:    "amd64",
			BuildTags: []string{"integration", "smoke", "unit"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CaptureGoFactContext() = %#v, want %#v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("captured context is invalid: %v", err)
	}
}

func TestCaptureGoFactContextReportsCommandFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		failGo    bool
		wantStage string
		wantText  string
	}{
		{name: "go env", failGo: true, wantStage: "go env", wantText: "go exploded"},
		{name: "gopls version", wantStage: "gopls version", wantText: "gopls exploded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repo := t.TempDir()
			binDir := t.TempDir()
			goBody := `printf '%s\n' '{"GOVERSION":"go1.24.3","GOOS":"linux","GOARCH":"amd64","GOFLAGS":""}'`
			goplsBody := `printf '%s\n' 'golang.org/x/tools/gopls v0.23.1'`
			if test.failGo {
				goBody = `printf '%s\n' 'go exploded' >&2; exit 31`
			} else {
				goplsBody = `printf '%s\n' 'gopls exploded' >&2; exit 32`
			}
			goBinary := writeGoTestExecutable(t, binDir, "fake-go", goBody)
			goplsBinary := writeGoTestExecutable(t, binDir, "fake-gopls", goplsBody)

			_, err := CaptureGoFactContext(context.Background(), goTestRepositoryState(repo), GoOptions{
				GoBinary:         goBinary,
				GoplsBinary:      goplsBinary,
				Collector:        "symbol-neighborhood",
				CollectorVersion: "v2",
				AnalyzerOptions:  "fixture-analyzer",
				CollectorOptions: "fixture-collector",
			})
			if err == nil {
				t.Fatal("CaptureGoFactContext() error = nil")
			}
			if !strings.Contains(err.Error(), test.wantStage) || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("CaptureGoFactContext() error = %q, want stage %q and output %q", err, test.wantStage, test.wantText)
			}
		})
	}
}

func TestCaptureGoFactContextRejectsMissingGoplsVersion(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	binDir := t.TempDir()
	goBinary := writeGoTestExecutable(t, binDir, "fake-go", `printf '%s\n' '{"GOVERSION":"go1.24.3","GOOS":"linux","GOARCH":"amd64","GOFLAGS":"-tags=unit"}'`)
	goplsBinary := writeGoTestExecutable(t, binDir, "fake-gopls", `exit 0`)

	_, err := CaptureGoFactContext(context.Background(), goTestRepositoryState(repo), GoOptions{
		GoBinary:         goBinary,
		GoplsBinary:      goplsBinary,
		Collector:        "symbol-neighborhood",
		CollectorVersion: "v2",
		AnalyzerOptions:  "fixture-analyzer",
		CollectorOptions: "fixture-collector",
	})
	if err == nil || !strings.Contains(err.Error(), "gopls version") {
		t.Fatalf("CaptureGoFactContext() error = %v, want missing gopls version", err)
	}
}

func TestCaptureGoFactContextRequiresCollectorIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options GoOptions
		want    string
	}{
		{
			name:    "collector",
			options: GoOptions{CollectorVersion: "v1", AnalyzerOptions: "fixture", CollectorOptions: "fixture"},
			want:    "collector is required",
		},
		{
			name:    "collector version",
			options: GoOptions{Collector: "symbol-neighborhood", AnalyzerOptions: "fixture", CollectorOptions: "fixture"},
			want:    "collector version is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := CaptureGoFactContext(context.Background(), goTestRepositoryState(t.TempDir()), test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CaptureGoFactContext() error = %v, want %q", err, test.want)
			}
		})
	}
}

func goTestRepositoryState(identity string) RepositoryState {
	return RepositoryState{
		Version:  RepositoryStateVersion,
		Identity: identity,
		Head:     strings.Repeat("a", 40),
		Dirty:    []DirtyFile{},
	}
}

func writeGoTestExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	writeGoTestFile(t, path, "#!/bin/sh\nset -eu\n"+body+"\n")
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeGoTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
