package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestAutomaticGoTargetAuthorityHonorsExplicitCLIAndEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		env      map[string]string
		want     bool
	}{
		{name: "host default", want: true},
		{name: "explicit flag", explicit: "darwin/amd64"},
		{name: "GOOS environment", env: map[string]string{"GOOS": "darwin"}},
		{name: "GOARCH environment", env: map[string]string{"GOARCH": "amd64"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := automaticGoTargetAllowed(test.explicit, func(name string) string {
				return test.env[name]
			})
			if got != test.want {
				t.Fatalf("automaticGoTargetAllowed() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRunDefaultAutoGoTargetPublishesLinuxWithExactProvenanceOffline(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")
	repository := mobyAutoTargetFixture(t)
	debugDir := t.TempDir()
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repository, []string{
		"--offline", "--target", "cmd/dockerd", "--no-open", "--no-serve",
		"--debug-dir", debugDir, "--depth", "1",
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		resolveGoTarget: darwinAMD64TargetResolver,
	}); err != nil {
		t.Fatalf("automatic Linux run: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Go target: auto: linux/amd64 (host darwin)") {
		t.Fatalf("automatic target provenance is absent from console:\n%s", stderr.String())
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	metadataRaw, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.EffectiveOptions.GoTarget != "linux/amd64" ||
		metadata.EffectiveOptions.GoTargetSource != snapshot.GoTargetSelectionAuto ||
		metadata.EffectiveOptions.GoTargetBaseline != "darwin/amd64" {
		t.Fatalf("automatic target metadata = %#v", metadata.EffectiveOptions)
	}
	if metadata.ProviderRequestCount != 0 {
		t.Fatalf("offline provider request count = %d", metadata.ProviderRequestCount)
	}
	for _, attempt := range metadata.RequestAttempts {
		if attempt.ProviderCallCount != 0 || attempt.TransportAttemptCount != 0 {
			t.Fatalf("offline attempt reached provider: %#v", attempt)
		}
	}
	surfaceRaw, err := os.ReadFile(filepath.Join(runDir, "trigger_catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog surfacediscovery.TriggerCatalog
	if err := json.Unmarshal(surfaceRaw, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Scenario.GOOS != "linux" || catalog.Scenario.GOARCH != "amd64" {
		t.Fatalf("surface scenario is not exact Linux authority: %#v", catalog.Scenario)
	}
}

func TestRunDefaultExplicitDarwinBlocksAutoAndFailsClosedForDockerd(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")
	repository := mobyAutoTargetFixture(t)
	var stderr bytes.Buffer
	err := runDefaultWithDeps(repository, []string{
		"--offline", "--go-target", "darwin/amd64", "--target", "cmd/dockerd",
		"--no-open", "--no-serve", "--debug-dir", t.TempDir(), "--depth", "1",
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		resolveGoTarget: darwinAMD64TargetResolver,
	})
	if err == nil {
		t.Fatalf("explicit Darwin unexpectedly recovered cmd/dockerd:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "Go target: auto:") {
		t.Fatalf("explicit target entered automatic lane:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "platform hint: linux/amd64") {
		t.Fatalf("explicit failure lost the non-mutating D251 hint:\n%s\nerror: %v", stderr.String(), err)
	}
}

func mobyAutoTargetFixture(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	for _, directory := range []string{"cmd/dockerd", "daemon"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/moby\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "cmd/dockerd/main.go"), "package main\nimport \"example.com/moby/daemon\"\nfunc main() { daemon.Run() }\n")
	writeFile(t, filepath.Join(repository, "daemon/config_linux.go"), "package daemon\nfunc Run() {}\n")
	writeFile(t, filepath.Join(repository, "daemon/network_linux.go"), "package daemon\nconst network = true\n")
	writeFile(t, filepath.Join(repository, "daemon/storage_linux.go"), "package daemon\nconst storage = true\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "cmd/dockerd/main.go",
		"daemon/config_linux.go", "daemon/network_linux.go", "daemon/storage_linux.go")
	commitTestRepository(t, repository)
	return repository
}

func darwinAMD64TargetResolver(explicit string, _ func(string) string) (gotarget.Target, error) {
	if explicit != "" {
		return gotarget.Parse(explicit)
	}
	return gotarget.Parse("darwin/amd64")
}
