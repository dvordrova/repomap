package quality

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxManifestBytes            int64 = 256 * 1024
	maxOrientationContextBytes  int64 = 512 * 1024
	maxOrientationResponseBytes int64 = 512 * 1024
	maxSourceBundleBytes        int64 = 256 * 1024
	maxSourceResponseBytes      int64 = 512 * 1024
	maxTestEvidenceBytes        int64 = 2 * 1024 * 1024
)

// LoadedTask contains hash-verified replay inputs. Artifact JSON is retained as
// bytes so the evaluator can use the existing source and test contracts rather
// than a second shadow representation.
type LoadedTask struct {
	Task                Task
	ManifestPath        string
	OrientationContext  []byte
	OrientationResponse []byte
	SourceBundle        []byte
	SourceResponse      []byte
	TestEvidence        []byte
}

func Load(manifestPath string) (LoadedTask, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return LoadedTask{}, fmt.Errorf("quality: manifest path is required")
	}
	resolvedManifest, err := resolveManifest(manifestPath)
	if err != nil {
		return LoadedTask{}, err
	}
	manifestJSON, err := readBoundedFile(resolvedManifest, maxManifestBytes, "manifest")
	if err != nil {
		return LoadedTask{}, err
	}
	var task Task
	if err := decodeStrictJSON(manifestJSON, &task); err != nil {
		return LoadedTask{}, fmt.Errorf("quality: decode manifest: %w", err)
	}
	if err := task.Validate(); err != nil {
		return LoadedTask{}, err
	}

	baseDir := filepath.Dir(resolvedManifest)
	orientationContext, err := loadArtifact(
		baseDir,
		"orientation_context",
		task.Artifacts.OrientationContext,
		maxOrientationContextBytes,
	)
	if err != nil {
		return LoadedTask{}, err
	}
	orientationResponse, err := loadArtifact(
		baseDir,
		"orientation_response",
		task.Artifacts.OrientationResponse,
		maxOrientationResponseBytes,
	)
	if err != nil {
		return LoadedTask{}, err
	}
	sourceBundle, err := loadArtifact(
		baseDir,
		"source_bundle",
		task.Artifacts.SourceBundle,
		maxSourceBundleBytes,
	)
	if err != nil {
		return LoadedTask{}, err
	}
	sourceResponse, err := loadArtifact(
		baseDir,
		"source_response",
		task.Artifacts.SourceResponse,
		maxSourceResponseBytes,
	)
	if err != nil {
		return LoadedTask{}, err
	}
	testEvidence, err := loadArtifact(
		baseDir,
		"test_evidence",
		task.Artifacts.TestEvidence,
		maxTestEvidenceBytes,
	)
	if err != nil {
		return LoadedTask{}, err
	}
	// Parse derived context only after every replay artifact has passed its
	// exact hash check. Source and test bytes are parsed by their owning cubes.
	var context OrientationGroundingContext
	if err := decodeStrictJSON(orientationContext, &context); err != nil {
		return LoadedTask{}, fmt.Errorf("quality: decode orientation grounding context: %w", err)
	}
	if err := context.Validate(); err != nil {
		return LoadedTask{}, err
	}
	if context.RepoName != task.Repository.Name {
		return LoadedTask{}, fmt.Errorf(
			"quality: orientation grounding repository %q does not match task repository %q",
			context.RepoName,
			task.Repository.Name,
		)
	}

	return LoadedTask{
		Task:                task,
		ManifestPath:        resolvedManifest,
		OrientationContext:  orientationContext,
		OrientationResponse: orientationResponse,
		SourceBundle:        sourceBundle,
		SourceResponse:      sourceResponse,
		TestEvidence:        testEvidence,
	}, nil
}

func resolveManifest(manifestPath string) (string, error) {
	absolute, err := filepath.Abs(manifestPath)
	if err != nil {
		return "", fmt.Errorf("quality: resolve manifest path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("quality: resolve manifest symlinks: %w", err)
	}
	return resolved, nil
}

func loadArtifact(baseDir, name string, ref ArtifactRef, limit int64) ([]byte, error) {
	path, err := resolveContainedArtifact(baseDir, ref.Path)
	if err != nil {
		return nil, fmt.Errorf("quality: resolve artifact %s: %w", name, err)
	}
	data, err := readBoundedFile(path, limit, "artifact "+name)
	if err != nil {
		return nil, err
	}
	actualSHA256 := fmt.Sprintf("%x", sha256.Sum256(data))
	if actualSHA256 != ref.SHA256 {
		return nil, fmt.Errorf(
			"quality: artifact %s sha256 mismatch: got %s, want %s",
			name,
			actualSHA256,
			ref.SHA256,
		)
	}
	return data, nil
}

func resolveContainedArtifact(baseDir, relativePath string) (string, error) {
	if !validRelativePath(relativePath) {
		return "", fmt.Errorf("invalid manifest-relative path %q", relativePath)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(baseDir, filepath.FromSlash(relativePath)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(baseDir, resolved)
	if err != nil {
		return "", fmt.Errorf("verify containment: %w", err)
	}
	if relative == "." || !filepath.IsLocal(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact resolves outside manifest directory")
	}
	return resolved, nil
}

func readBoundedFile(path string, limit int64, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("quality: open %s: %w", label, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("quality: stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("quality: %s is not a regular file", label)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("quality: %s is %d bytes, limit is %d", label, info.Size(), limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("quality: read %s: %w", label, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("quality: %s exceeds %d byte limit", label, limit)
	}
	return data, nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing json value")
		}
		return err
	}
	return nil
}
