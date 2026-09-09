package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// SharedInput owns one immutable adapter projection. Launches may share this
// value only when their complete objects, relations and parser context agree.
// Different import/package views of the same AST remain different inputs.
type SharedInput struct {
	input Input
	wire  []byte
	key   string
}

func ShareInput(input Input) *SharedInput {
	input.Target = TargetInput{}
	input.shared = nil
	return &SharedInput{input: input}
}

func (shared *SharedInput) ForTarget(target TargetInput) Input {
	input := shared.input
	input.Target = target
	input.shared = shared
	return input
}

// The persisted facts are exactly the existing common-builder input, without
// a target. They introduce no other graph or identity assignment algorithm.
type sharedFactsArtifact struct {
	Version        int             `json:"version"`
	IndexVersion   int             `json:"index_version"`
	ScenarioSHA256 string          `json:"scenario_sha256"`
	SourceSHA256   string          `json:"source_sha256"`
	Objects        []ObjectInput   `json:"objects"`
	Relations      []RelationInput `json:"relations"`
	Coverage       CoverageInput   `json:"coverage"`
}

type targetArtifact struct {
	StorageVersion int         `json:"storage_version"`
	IndexVersion   int         `json:"index_version"`
	Facts          string      `json:"facts"`
	FactsSHA256    string      `json:"facts_sha256"`
	Target         TargetInput `json:"target"`
	IndexSHA256    string      `json:"index_sha256"`
}

// ArtifactStore is run-owned and keeps only paths/digests after a project is
// written. The adapter's shared input is released after its last target.
type ArtifactStore struct {
	dir     string
	written map[string]bool
}

func NewArtifactStore(dir string) *ArtifactStore {
	return &ArtifactStore{dir: dir, written: make(map[string]bool)}
}

func (shared *SharedInput) encode() ([]byte, string, error) {
	if shared.wire == nil {
		input := shared.input
		wire, err := json.Marshal(sharedFactsArtifact{
			Version: 1, IndexVersion: Version,
			ScenarioSHA256: input.ScenarioSHA256, SourceSHA256: input.SourceSHA256,
			Objects: input.Objects, Relations: input.Relations, Coverage: input.Coverage,
		})
		if err != nil {
			return nil, "", err
		}
		shared.wire, shared.key = wire, artifactBytesDigest(wire)
	}
	return shared.wire, shared.key, nil
}

// Persist saves the input of an already sealed base index. Ordinary targets
// all use this storage path; Encode/Decode remain the standalone Index format.
func (store *ArtifactStore) Persist(runDir string, index Index, input Input) error {
	shared := input.shared
	if shared == nil {
		shared = ShareInput(input)
	}
	wire, key, err := shared.encode()
	if err != nil {
		return fmt.Errorf("program facts: encode: %w", err)
	}
	filename := key + ".json"
	if !store.written[key] {
		if err := os.MkdirAll(store.dir, 0o755); err != nil {
			return err
		}
		writer, err := debugdump.OpenWriter(store.dir)
		if err != nil {
			return err
		}
		err = writer.WriteValidatedFile(filename, wire, func(saved []byte) error {
			if artifactBytesDigest(saved) != key {
				return fmt.Errorf("program facts: saved input differs")
			}
			return nil
		})
		writer.Close()
		if err != nil {
			return err
		}
		store.written[key] = true
	}
	relative, err := filepath.Rel(runDir, filepath.Join(store.dir, filename))
	if err != nil {
		return err
	}
	view := targetArtifact{
		StorageVersion: 1, IndexVersion: Version, Facts: filepath.ToSlash(relative),
		FactsSHA256: key, Target: input.Target, IndexSHA256: index.SHA256,
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return err
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return err
	}
	defer writer.Close()
	return writer.WriteValidatedFile(ArtifactFilename, encoded, func(saved []byte) error {
		if !bytes.Equal(saved, encoded) {
			return fmt.Errorf("program index: saved target differs")
		}
		return nil
	})
}

// ReadFile restores either a standalone encoded Index or the ordinary thin
// target artifact. Shared facts are materialized by New, preserving every
// existing target/object/relation identity, coverage value and nested link.
// No parser, repository access or provider is involved.
func ReadFile(filename string) (Index, error) {
	wire, err := os.ReadFile(filename)
	if err != nil {
		return Index{}, err
	}
	var kind struct {
		StorageVersion *int `json:"storage_version"`
	}
	if err := json.Unmarshal(wire, &kind); err != nil {
		return Index{}, err
	}
	if kind.StorageVersion == nil {
		return Decode(wire)
	}
	var view targetArtifact
	if err := decodeSharedArtifact(wire, &view); err != nil {
		return Index{}, err
	}
	if view.StorageVersion != 1 || view.IndexVersion != Version ||
		!validSHA256(view.FactsSHA256) || !validSHA256(view.IndexSHA256) ||
		view.Facts == "" || filepath.IsAbs(view.Facts) {
		return Index{}, fmt.Errorf("program index: invalid shared target artifact")
	}
	factsWire, err := os.ReadFile(filepath.Join(filepath.Dir(filename), filepath.FromSlash(view.Facts)))
	if err != nil {
		return Index{}, fmt.Errorf("program index: read shared facts: %w", err)
	}
	if artifactBytesDigest(factsWire) != view.FactsSHA256 {
		return Index{}, fmt.Errorf("program index: shared facts digest mismatch")
	}
	var facts sharedFactsArtifact
	if err := decodeSharedArtifact(factsWire, &facts); err != nil {
		return Index{}, err
	}
	if facts.Version != 1 || facts.IndexVersion != Version {
		return Index{}, fmt.Errorf("program index: unsupported shared facts version")
	}
	index, err := New(Input{
		ScenarioSHA256: facts.ScenarioSHA256, SourceSHA256: facts.SourceSHA256,
		Target: view.Target, Objects: facts.Objects, Relations: facts.Relations, Coverage: facts.Coverage,
	})
	if err != nil {
		return Index{}, err
	}
	if index.SHA256 != view.IndexSHA256 {
		return Index{}, fmt.Errorf("program index: shared target binding mismatch")
	}
	return index, nil
}

func decodeSharedArtifact(wire []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("program index: decode shared artifact: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("program index: trailing shared artifact data")
	}
	return nil
}

func artifactBytesDigest(wire []byte) string {
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}
