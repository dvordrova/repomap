package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ArtifactSetVersion  = 1
	ArtifactSetFilename = "program-index-set.json"

	MaxArtifactSetEntries       = 4_096
	MaxArtifactSetFilenameBytes = 512
	MaxArtifactSetBytes         = 8 * 1024 * 1024
)

// ArtifactSetEntry binds one selected target view to the sealed ProgramIndex
// artifact that represents its program scope. The referenced Index is sealed
// with the same TargetID, so every selected view owns one distinct artifact
// filename even when the language adapter shared all expensive source parsing.
type ArtifactSetEntry struct {
	TargetID    string `json:"target_id"`
	Filename    string `json:"filename"`
	IndexSHA256 string `json:"index_sha256"`
}

// ArtifactSet is the canonical handoff for runs that select one or more target
// views. DefaultTargetID selects the view used by consumers that do not ask for
// a specific target.
type ArtifactSet struct {
	Version         int                `json:"version"`
	DefaultTargetID string             `json:"default_target_id"`
	Entries         []ArtifactSetEntry `json:"entries"`
	SHA256          string             `json:"sha256"`
}

// BuildArtifactSet validates a ProgramIndex inventory and binds every index to
// its distinct artifact filename before constructing the sealed artifact set.
func BuildArtifactSet(defaultTargetID string, indexes []Index, filenames []string) (ArtifactSet, error) {
	if len(indexes) == 0 || len(indexes) != len(filenames) {
		return ArtifactSet{}, fmt.Errorf("program index set: index and filename inventories do not match")
	}
	entries := make([]ArtifactSetEntry, len(indexes))
	for position, index := range indexes {
		if err := index.Validate(); err != nil {
			return ArtifactSet{}, fmt.Errorf("program index set: index %d: %w", position, err)
		}
		entries[position] = ArtifactSetEntry{
			TargetID: index.Target.ID, Filename: filenames[position], IndexSHA256: index.SHA256,
		}
	}
	return NewArtifactSet(defaultTargetID, entries)
}

// ExactIndexByTargetID returns one consumer-owned ProgramIndex from an
// already-built inventory. It fails closed when the requested target is
// missing or repeated instead of choosing an index by position.
func ExactIndexByTargetID(indexes []Index, targetID string) (Index, error) {
	var result *Index
	for position := range indexes {
		if indexes[position].Target.ID != targetID {
			continue
		}
		if result != nil {
			return Index{}, fmt.Errorf("program index set repeats default target %q", targetID)
		}
		copyIndex := indexes[position].Snapshot()
		result = &copyIndex
	}
	if result == nil {
		return Index{}, fmt.Errorf("program index set has no exact default target %q", targetID)
	}
	return *result, nil
}

// ArtifactFilenameForTarget returns the canonical default artifact filename or
// the target-specific filename used for another selected view.
func ArtifactFilenameForTarget(targetRef string, isDefault bool) string {
	if isDefault {
		return ArtifactFilename
	}
	return "program-index." + targetRef + ".json"
}

// NewArtifactSet canonicalizes entries by TargetID, rejects ambiguous
// bindings, and seals the resulting artifact set.
func NewArtifactSet(defaultTargetID string, entries []ArtifactSetEntry) (ArtifactSet, error) {
	if len(entries) == 0 || len(entries) > MaxArtifactSetEntries {
		return ArtifactSet{}, fmt.Errorf("program index set: entry bound exceeded")
	}
	result := ArtifactSet{
		Version:         ArtifactSetVersion,
		DefaultTargetID: defaultTargetID,
		Entries:         cloneArtifactSetEntries(entries),
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].TargetID != result.Entries[j].TargetID {
			return result.Entries[i].TargetID < result.Entries[j].TargetID
		}
		if result.Entries[i].Filename != result.Entries[j].Filename {
			return result.Entries[i].Filename < result.Entries[j].Filename
		}
		return result.Entries[i].IndexSHA256 < result.Entries[j].IndexSHA256
	})
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
	if len(encoded) > MaxArtifactSetBytes {
		return nil, fmt.Errorf("program index set: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactSetBytes)
	}
	return encoded, nil
}

// DecodeArtifactSet strictly decodes, validates, and verifies one artifact
// set. Unknown fields and trailing JSON values are rejected.
func DecodeArtifactSet(encoded []byte) (ArtifactSet, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactSetBytes {
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
	if set.Entries == nil || len(set.Entries) == 0 || len(set.Entries) > MaxArtifactSetEntries {
		return fmt.Errorf("program index set: entry bound exceeded")
	}
	defaultMatches := 0
	filenames := make(map[string]struct{}, len(set.Entries))
	for position, entry := range set.Entries {
		if !validText(entry.TargetID) || !validArtifactSetFilename(entry.Filename) || !validSHA256(entry.IndexSHA256) {
			return fmt.Errorf("program index set: invalid entry")
		}
		if position > 0 && set.Entries[position-1].TargetID >= entry.TargetID {
			return fmt.Errorf("program index set: entries are not canonical")
		}
		if entry.TargetID == set.DefaultTargetID {
			defaultMatches++
		}
		if _, duplicate := filenames[entry.Filename]; duplicate {
			return fmt.Errorf("program index set: filename %q is bound to more than one target", entry.Filename)
		}
		filenames[entry.Filename] = struct{}{}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("program index set: default target must have exactly one entry")
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
	if len(encoded) > MaxArtifactSetBytes {
		return "", fmt.Errorf("program index set: canonical substrate is %d bytes, limit is %d", len(encoded), MaxArtifactSetBytes)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validArtifactSetFilename(value string) bool {
	if value == "" || len(value) > MaxArtifactSetFilenameBytes || value != strings.TrimSpace(value) ||
		!utf8.ValidString(value) || strings.ContainsAny(value, `/\\`) || value == "." || value == ".." || !fs.ValidPath(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func cloneArtifactSetEntries(values []ArtifactSetEntry) []ArtifactSetEntry {
	result := make([]ArtifactSetEntry, len(values))
	copy(result, values)
	return result
}
