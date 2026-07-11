package quality

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsHashVerifiedTask(t *testing.T) {
	t.Parallel()

	fixture := newLoadFixture(t, validArtifactContents())
	loaded, err := Load(fixture.manifestPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Task.ID != fixture.task.ID {
		t.Fatalf("task id = %q, want %q", loaded.Task.ID, fixture.task.ID)
	}
	if !filepath.IsAbs(loaded.ManifestPath) {
		t.Fatalf("manifest path = %q, want absolute", loaded.ManifestPath)
	}
	if !bytes.Equal(loaded.OrientationContext, fixture.contents.orientationContext) ||
		!bytes.Equal(loaded.OrientationResponse, fixture.contents.orientationResponse) ||
		!bytes.Equal(loaded.SourceBundle, fixture.contents.sourceBundle) ||
		!bytes.Equal(loaded.SourceResponse, fixture.contents.sourceResponse) ||
		!bytes.Equal(loaded.TestEvidence, fixture.contents.testEvidence) {
		t.Fatal("loaded artifact bytes do not match fixture")
	}
}

func TestLoadEtcdPutFixture(t *testing.T) {
	t.Parallel()

	loaded, err := Load("testdata/etcd-put-v1/task.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Task.ID != "etcd-put-orientation-drilldown-v1" {
		t.Fatalf("task id = %q", loaded.Task.ID)
	}
	sourceCapture := loaded.Task.Captures.Source
	if len(loaded.OrientationContext) == 0 ||
		len(loaded.SourceBundle) != 3536 ||
		sourceCapture.ModelContextBytes != 3001 ||
		sourceCapture.ProviderRequestBytes != nil {
		t.Fatalf("loaded replay/capture sizes = context:%d source:%d capture:%#v",
			len(loaded.OrientationContext), len(loaded.SourceBundle), sourceCapture)
	}
	if loaded.Task.Captures.Orientation.ProviderRequestBytes != nil {
		t.Fatalf("legacy orientation provider request bytes = %v, want unknown", loaded.Task.Captures.Orientation.ProviderRequestBytes)
	}
	if sourceCapture.ProviderRequestSHA256 != nil {
		t.Fatalf("legacy source provider request sha256 = %v, want unknown", sourceCapture.ProviderRequestSHA256)
	}
}

func TestLoadChecksArtifactHashBeforeParsing(t *testing.T) {
	t.Parallel()

	contents := validArtifactContents()
	contents.orientationContext = []byte(`{"version":1,"repo_name":"etcd","allowed_paths":["server/key.go"],"extra":true}`)
	fixture := newLoadFixture(t, contents)
	fixture.task.Artifacts.OrientationContext.SHA256 = strings.Repeat("0", 64)
	fixture.writeManifest(t)
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("Load() error = %v, want sha256 mismatch before JSON error", err)
	}
}

func TestLoadRejectsInvalidHashedOrientationContext(t *testing.T) {
	t.Parallel()

	contents := validArtifactContents()
	contents.orientationContext = []byte("not-json")
	fixture := newLoadFixture(t, contents)
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "decode orientation grounding context") {
		t.Fatalf("Load() error = %v, want context decode error", err)
	}
}

func TestLoadRejectsUnknownManifestField(t *testing.T) {
	t.Parallel()

	fixture := newLoadFixture(t, validArtifactContents())
	manifestJSON, err := json.Marshal(fixture.task)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON = append(manifestJSON[:len(manifestJSON)-1], []byte(`,"unexpected":true}`)...)
	if err := os.WriteFile(fixture.manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() error = %v, want unknown-field error", err)
	}
}

func TestLoadRejectsArtifactSymlinkEscape(t *testing.T) {
	t.Parallel()

	fixture := newLoadFixture(t, validArtifactContents())
	artifactPath := filepath.Join(fixture.dir, filepath.FromSlash(fixture.task.Artifacts.SourceResponse.Path))
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "source-response.json")
	if err := os.WriteFile(outsidePath, fixture.contents.sourceResponse, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(artifactPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, artifactPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "outside manifest directory") {
		t.Fatalf("Load() error = %v, want containment error", err)
	}
}

func TestLoadRejectsOversizedArtifact(t *testing.T) {
	t.Parallel()

	contents := validArtifactContents()
	contents.sourceResponse = bytes.Repeat([]byte("x"), int(maxSourceResponseBytes)+1)
	fixture := newLoadFixture(t, contents)
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "limit is") {
		t.Fatalf("Load() error = %v, want byte limit error", err)
	}
}

func TestLoadKeepsCaptureSizesIndependentFromReplayArtifacts(t *testing.T) {
	t.Parallel()

	fixture := newLoadFixture(t, validArtifactContents())
	loaded, err := Load(fixture.manifestPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Task.Captures.Source.ModelContextBytes == len(loaded.SourceBundle) ||
		loaded.Task.Captures.Source.ProviderRequestBytes == nil ||
		*loaded.Task.Captures.Source.ProviderRequestBytes == len(loaded.SourceBundle) {
		t.Fatalf("capture sizes were conflated with replay input: capture=%#v replay=%d",
			loaded.Task.Captures.Source, len(loaded.SourceBundle))
	}
}

func TestLoadRejectsGroundingRepositoryMismatch(t *testing.T) {
	t.Parallel()

	contents := validArtifactContents()
	contents.orientationContext = mustJSON(t, OrientationGroundingContext{
		Version:      OrientationGroundingContextVersion,
		RepoName:     "not-etcd",
		AllowedPaths: []string{"server/key.go"},
	})
	fixture := newLoadFixture(t, contents)
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "does not match task repository") {
		t.Fatalf("Load() error = %v, want repository mismatch", err)
	}
}

func TestLoadRejectsUnknownGroundingContextField(t *testing.T) {
	t.Parallel()

	contents := validArtifactContents()
	contents.orientationContext = []byte(`{"version":1,"repo_name":"etcd","allowed_paths":["server/key.go"],"extra":true}`)
	fixture := newLoadFixture(t, contents)
	if _, err := Load(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() error = %v, want unknown-field error", err)
	}
}

type artifactContents struct {
	orientationContext  []byte
	orientationResponse []byte
	sourceBundle        []byte
	sourceResponse      []byte
	testEvidence        []byte
}

type loadFixture struct {
	dir          string
	manifestPath string
	task         Task
	contents     artifactContents
}

func validArtifactContents() artifactContents {
	contextJSON, err := json.Marshal(OrientationGroundingContext{
		Version:      OrientationGroundingContextVersion,
		RepoName:     "etcd",
		AllowedPaths: []string{"server/key.go", "server/txn.go"},
	})
	if err != nil {
		panic(err)
	}
	return artifactContents{
		orientationContext:  contextJSON,
		orientationResponse: []byte(`{"candidate_flows":[]}`),
		sourceBundle:        []byte(`{"version":1}`),
		sourceResponse:      []byte(`{"assessments":[]}`),
		testEvidence:        []byte(`{"version":1}`),
	}
}

func newLoadFixture(t *testing.T, contents artifactContents) *loadFixture {
	t.Helper()
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "artifacts")
	if err := os.Mkdir(artifactDir, 0o700); err != nil {
		t.Fatal(err)
	}
	task := validTask()
	task.Artifacts = Artifacts{
		OrientationContext:  writeArtifactFixture(t, dir, "artifacts/orientation-context.json", contents.orientationContext),
		OrientationResponse: writeArtifactFixture(t, dir, "artifacts/orientation-response.json", contents.orientationResponse),
		SourceBundle:        writeArtifactFixture(t, dir, "artifacts/source-bundle.json", contents.sourceBundle),
		SourceResponse:      writeArtifactFixture(t, dir, "artifacts/source-response.json", contents.sourceResponse),
		TestEvidence:        writeArtifactFixture(t, dir, "artifacts/test-evidence.json", contents.testEvidence),
	}
	fixture := &loadFixture{
		dir:          dir,
		manifestPath: filepath.Join(dir, "task.json"),
		task:         task,
		contents:     contents,
	}
	fixture.writeManifest(t)
	return fixture
}

func (f *loadFixture) writeManifest(t *testing.T) {
	t.Helper()
	manifestJSON, err := json.MarshalIndent(f.task, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeArtifactFixture(t *testing.T, root, relativePath string, data []byte) ArtifactRef {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return ArtifactRef{
		Path:   relativePath,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
