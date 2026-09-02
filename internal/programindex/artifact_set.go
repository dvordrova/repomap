package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

const (
	ArtifactSetVersion  = 1
	ArtifactSetFilename = "program-index-set.json"

	// AdvisoryArtifactSetBytes is a former local threshold retained for
	// diagnostics only. MaxArtifactSetBytes is a zero compatibility sentinel
	// for report readers and does not impose a local cutoff.
	MaxArtifactSetFilenameBytes = 512
	AdvisoryArtifactSetBytes    = 8 * 1024 * 1024
	MaxArtifactSetBytes         = 0
)

// ArtifactSetEntry binds one page-local target to its sealed ProgramIndex.
type ArtifactSetEntry struct {
	TargetID    string `json:"target_id"`
	Filename    string `json:"filename"`
	IndexSHA256 string `json:"index_sha256"`
}

// ArtifactSet is the canonical handoff for one page-local target run. Entries
// remains an array in the persisted version-1 schema, but validation requires
// exactly one entry bound to program-index.json.
type ArtifactSet struct {
	Version         int                `json:"version"`
	DefaultTargetID string             `json:"default_target_id"`
	Entries         []ArtifactSetEntry `json:"entries"`
	SHA256          string             `json:"sha256"`
}

// BuildArtifactSet validates one page-local ProgramIndex and binds it to the
// only canonical filename accepted by an ordinary target run.
func BuildArtifactSet(index Index) (ArtifactSet, error) {
	if err := index.Validate(); err != nil {
		return ArtifactSet{}, fmt.Errorf("program index set: index: %w", err)
	}
	return NewArtifactSet(index.Target.ID, []ArtifactSetEntry{{
		TargetID: index.Target.ID, Filename: ArtifactFilename, IndexSHA256: index.SHA256,
	}})
}

// NewArtifactSet rejects every shape except one exact page-local binding and
// seals that binding.
func NewArtifactSet(defaultTargetID string, entries []ArtifactSetEntry) (ArtifactSet, error) {
	result := ArtifactSet{
		Version:         ArtifactSetVersion,
		DefaultTargetID: defaultTargetID,
		Entries:         cloneArtifactSetEntries(entries),
	}
	if err := result.validateShape(); err != nil {
		return ArtifactSet{}, err
	}
	digest, err := artifactSetDigest(result)
	if err != nil {
		return ArtifactSet{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return ArtifactSet{}, err
	}
	return result, nil
}

// EncodeArtifactSet validates and returns canonical JSON artifact bytes.
func EncodeArtifactSet(set ArtifactSet) ([]byte, error) {
	if err := set.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		return nil, fmt.Errorf("program index set: encode artifact: %w", err)
	}
	return encoded, nil
}

// DecodeArtifactSet strictly decodes, validates, and verifies one artifact
// set. Unknown fields and trailing JSON values are rejected.
func DecodeArtifactSet(encoded []byte) (ArtifactSet, error) {
	if len(encoded) == 0 {
		return ArtifactSet{}, fmt.Errorf("program index set: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var set ArtifactSet
	if err := decoder.Decode(&set); err != nil {
		return ArtifactSet{}, fmt.Errorf("program index set: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ArtifactSet{}, fmt.Errorf("program index set: trailing JSON value")
		}
		return ArtifactSet{}, fmt.Errorf("program index set: trailing data: %w", err)
	}
	if err := set.Validate(); err != nil {
		return ArtifactSet{}, err
	}
	return set, nil
}

// Snapshot returns a consumer-owned copy.
func (set ArtifactSet) Snapshot() ArtifactSet {
	result := set
	result.Entries = cloneArtifactSetEntries(set.Entries)
	return result
}

// Validate checks bounds, canonical target bindings, safe artifact basenames,
// index digests, and the complete artifact-set seal.
func (set ArtifactSet) Validate() error {
	if err := set.validateShape(); err != nil {
		return err
	}
	want, err := artifactSetDigest(set)
	if err != nil {
		return err
	}
	if !validSHA256(set.SHA256) || set.SHA256 != want {
		return fmt.Errorf("program index set: sha256 mismatch")
	}
	return nil
}

func (set ArtifactSet) validateShape() error {
	if set.Version != ArtifactSetVersion || !validText(set.DefaultTargetID) {
		return fmt.Errorf("program index set: invalid identity")
	}
	if len(set.Entries) != 1 {
		return fmt.Errorf("program index set: exactly one page-local entry is required")
	}
	entry := set.Entries[0]
	if !validText(entry.TargetID) || !validSHA256(entry.IndexSHA256) {
		return fmt.Errorf("program index set: invalid entry")
	}
	if entry.TargetID != set.DefaultTargetID {
		return fmt.Errorf("program index set: default target must match the page-local entry")
	}
	if entry.Filename != ArtifactFilename {
		return fmt.Errorf(
			"program index set: filename %q is not canonical %q",
			entry.Filename, ArtifactFilename,
		)
	}
	return nil
}

func artifactSetDigest(set ArtifactSet) (string, error) {
	payload := set.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("program index set: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func cloneArtifactSetEntries(values []ArtifactSetEntry) []ArtifactSetEntry {
	result := make([]ArtifactSetEntry, len(values))
	copy(result, values)
	return result
}
