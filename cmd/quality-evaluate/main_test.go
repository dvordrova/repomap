package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/quality"
)

func TestRunEvaluatesCommittedEtcdTaskOffline(t *testing.T) {
	t.Parallel()

	outPath := filepath.Join(t.TempDir(), "result.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{
		"--task", "../../internal/quality/testdata/etcd-put-v1/task.json",
		"--out", outPath,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, stderr = %s", err, stderr.String())
	}
	if stdout.String() != "PASS etcd-put-orientation-drilldown-v1\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var result quality.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.TaskID != "etcd-put-orientation-drilldown-v1" {
		t.Fatalf("result = %#v", result)
	}
	if result.BytesAndLatency.Orientation.ModelContextBytes != 108668 ||
		result.BytesAndLatency.Orientation.ProviderRequestBytes != nil ||
		result.BytesAndLatency.Source.ReplayInputBytes != 3536 ||
		result.BytesAndLatency.Source.ModelContextBytes != 3001 ||
		result.BytesAndLatency.Source.ProviderRequestBytes != nil {
		t.Fatalf("capture observations = %#v", result.BytesAndLatency)
	}
}

func TestRunValidatesArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: nil, want: "--task and --out are required"},
		{name: "positional", args: []string{"extra"}, want: "unexpected positional"},
		{name: "overwrite", args: []string{"--task", "task.json", "--out", "./task.json"}, want: "must not overwrite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			err := run(test.args, &bytes.Buffer{}, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRunRejectsFixtureArtifactOutput(t *testing.T) {
	t.Parallel()

	fixtureDir := copyEtcdQualityFixture(t)
	taskPath := filepath.Join(fixtureDir, "task.json")
	artifactPath := filepath.Join(fixtureDir, "source_response.json")
	want, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}

	err = run([]string{"--task", taskPath, "--out", artifactPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "source_response artifact") {
		t.Fatalf("error = %v, want protected source_response artifact error", err)
	}
	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("source_response artifact changed after rejected output")
	}
}

func TestRunRejectsSymlinkAliasOfFixtureArtifact(t *testing.T) {
	t.Parallel()

	fixtureDir := copyEtcdQualityFixture(t)
	taskPath := filepath.Join(fixtureDir, "task.json")
	artifactPath := filepath.Join(fixtureDir, "source_response.json")
	aliasPath := filepath.Join(fixtureDir, "result-alias.json")
	if err := os.Symlink(artifactPath, aliasPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	err := run([]string{"--task", taskPath, "--out", aliasPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "source_response artifact") {
		t.Fatalf("error = %v, want protected source_response artifact error", err)
	}
}

func TestRunRejectsHardlinkAliasOfFixtureArtifact(t *testing.T) {
	t.Parallel()

	fixtureDir := copyEtcdQualityFixture(t)
	taskPath := filepath.Join(fixtureDir, "task.json")
	artifactPath := filepath.Join(fixtureDir, "source_response.json")
	aliasPath := filepath.Join(fixtureDir, "result-alias.json")
	if err := os.Link(artifactPath, aliasPath); err != nil {
		t.Skipf("create hardlink: %v", err)
	}

	err := run([]string{"--task", taskPath, "--out", aliasPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "source_response artifact") {
		t.Fatalf("error = %v, want protected source_response artifact error", err)
	}
}

func TestRunDoesNotFollowPredictableTemporarySymlink(t *testing.T) {
	t.Parallel()

	fixtureDir := copyEtcdQualityFixture(t)
	taskPath := filepath.Join(fixtureDir, "task.json")
	artifactPath := filepath.Join(fixtureDir, "source_response.json")
	wantArtifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(fixtureDir, "result.json")
	if err := os.Symlink(artifactPath, outPath+".tmp"); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	err = run([]string{"--task", taskPath, "--out", outPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	gotArtifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotArtifact, wantArtifact) {
		t.Fatal("source_response artifact was overwritten through a temporary symlink")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output was not written: %v", err)
	}
}

func copyEtcdQualityFixture(t *testing.T) string {
	t.Helper()

	sourceDir := "../../internal/quality/testdata/etcd-put-v1"
	destinationDir := t.TempDir()
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destinationDir, entry.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return destinationDir
}
