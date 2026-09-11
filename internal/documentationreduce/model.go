// Package documentationreduce condenses repository-authored README and
// AGENTS.md documents into a compact, source-bound product-context handoff:
// one overview and the product or domain concepts of each document. It does
// not inspect or classify program elements.
package documentationreduce

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

const Version = 2

// MaxConceptsPerSource bounds the vocabulary one document contributes. The
// decoder keeps the first MaxConceptsPerSource distinct concepts of a
// response row in the model's order and drops the rest; it never refuses
// the row for exceeding the ceiling.
const MaxConceptsPerSource = 12

// Source is compact model-authored context restored to one exact guidance
// document. Concepts is a set of at most MaxConceptsPerSource entries; its
// canonical order is local.
type Source struct {
	Path     string                         `json:"path"`
	Kind     readmetargetscout.GuidanceKind `json:"kind"`
	Concepts []string                       `json:"concepts"`
}

// Result is the sealed, in-memory reduced_documentation handoff. The guidance
// digest binds it to the exact documentation_collect snapshot; the reduction
// digest binds the compact semantic result. No request-local d* ref survives.
type Result struct {
	GuidanceSHA256  string   `json:"guidance_sha256"`
	ReductionSHA256 string   `json:"reduction_sha256"`
	Overview        string   `json:"overview"`
	Sources         []Source `json:"sources"`
}

// Snapshot validates the result and returns an independently owned copy.
func (result Result) Snapshot() (Result, error) {
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return cloneResult(result), nil
}

// Validate checks the closed in-memory shape and its reduction seal. Use
// ValidateAgainst when the source GuidanceSnapshot is available.
func (result Result) Validate() error {
	if result.Sources == nil {
		return fmt.Errorf("documentation reduce: sources are missing")
	}
	if result.GuidanceSHA256 == "" {
		if result.Overview != "" || len(result.Sources) != 0 {
			return fmt.Errorf("documentation reduce: empty guidance has semantic output")
		}
		if !validSHA256(result.ReductionSHA256) {
			return fmt.Errorf("documentation reduce: digest is invalid")
		}
		digest, err := resultDigest("", "", []Source{})
		if err != nil {
			return err
		}
		if digest != result.ReductionSHA256 {
			return fmt.Errorf("documentation reduce: reduction digest mismatch")
		}
		return nil
	}
	if !validSHA256(result.GuidanceSHA256) || !validSHA256(result.ReductionSHA256) {
		return fmt.Errorf("documentation reduce: digest is invalid")
	}
	if result.Overview != "" && !validText(result.Overview) {
		return fmt.Errorf("documentation reduce: overview is invalid")
	}
	if result.Overview != "" && len(result.Sources) == 0 {
		return fmt.Errorf("documentation reduce: overview has no source-bound evidence")
	}
	for position, source := range result.Sources {
		if !validRepositoryPath(source.Path) || !validGuidanceKind(source.Kind) ||
			len(source.Concepts) == 0 || len(source.Concepts) > MaxConceptsPerSource ||
			!canonicalTextSet(source.Concepts) {
			return fmt.Errorf("documentation reduce: source %d is invalid", position)
		}
		if position > 0 && result.Sources[position-1].Path >= source.Path {
			return fmt.Errorf("documentation reduce: sources are not canonical")
		}
	}
	digest, err := resultDigest(result.GuidanceSHA256, result.Overview, result.Sources)
	if err != nil {
		return err
	}
	if digest != result.ReductionSHA256 {
		return fmt.Errorf("documentation reduce: reduction digest mismatch")
	}
	return nil
}

// ValidateAgainst proves that every retained source was restored from the
// exact documentation_collect snapshot. Sparse output is legitimate.
func (result Result) ValidateAgainst(guidance readmetargetscout.GuidanceSnapshot) error {
	if err := result.Validate(); err != nil {
		return err
	}
	snapshot, err := guidance.Snapshot()
	if err != nil {
		return fmt.Errorf("documentation reduce: guidance snapshot: %w", err)
	}
	if result.GuidanceSHA256 != snapshot.SHA256 {
		return fmt.Errorf("documentation reduce: guidance digest mismatch")
	}
	authority := make(map[string]readmetargetscout.GuidanceKind, len(snapshot.Documents))
	for _, document := range snapshot.Documents {
		authority[document.Path] = document.Kind
	}
	for _, source := range result.Sources {
		kind, known := authority[source.Path]
		if !known || kind != source.Kind {
			return fmt.Errorf("documentation reduce: source %q is outside guidance authority", source.Path)
		}
	}
	return nil
}

func sealResult(
	guidance readmetargetscout.GuidanceSnapshot,
	overview string,
	sources []Source,
) (Result, error) {
	if len(guidance.Documents) == 0 {
		digest, err := resultDigest("", "", []Source{})
		if err != nil {
			return Result{}, err
		}
		result := Result{ReductionSHA256: digest, Sources: []Source{}}
		if err := result.ValidateAgainst(guidance); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	canonical, err := canonicalSources(sources)
	if err != nil {
		return Result{}, err
	}
	digest, err := resultDigest(guidance.SHA256, overview, canonical)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		GuidanceSHA256: guidance.SHA256, ReductionSHA256: digest,
		Overview: overview, Sources: canonical,
	}
	if err := result.ValidateAgainst(guidance); err != nil {
		return Result{}, err
	}
	return result, nil
}

func canonicalSources(values []Source) ([]Source, error) {
	byPath := make(map[string]Source, len(values))
	for _, value := range values {
		if !validRepositoryPath(value.Path) || !validGuidanceKind(value.Kind) {
			return nil, fmt.Errorf("documentation reduce: invalid restored source")
		}
		for _, concept := range value.Concepts {
			if !validText(concept) {
				return nil, fmt.Errorf("documentation reduce: source %q concepts: invalid compact text", value.Path)
			}
		}
		if len(value.Concepts) == 0 {
			continue
		}
		current, exists := byPath[value.Path]
		if exists && current.Kind != value.Kind {
			return nil, fmt.Errorf("documentation reduce: conflicting kinds for %q", value.Path)
		}
		current.Path = value.Path
		current.Kind = value.Kind
		current.Concepts = append(current.Concepts, value.Concepts...)
		byPath[value.Path] = current
	}
	result := make([]Source, 0, len(byPath))
	for _, source := range byPath {
		source.Concepts = capConcepts(source.Concepts)
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func resultDigest(guidanceSHA, overview string, sources []Source) (string, error) {
	wire, err := json.Marshal(struct {
		Version        int      `json:"version"`
		GuidanceSHA256 string   `json:"guidance_sha256"`
		Overview       string   `json:"overview"`
		Sources        []Source `json:"sources"`
	}{Version: Version, GuidanceSHA256: guidanceSHA, Overview: overview, Sources: sources})
	if err != nil {
		return "", fmt.Errorf("documentation reduce: encode reduction seal: %w", err)
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func cloneResult(result Result) Result {
	cloned := result
	cloned.Sources = make([]Source, len(result.Sources))
	for position, source := range result.Sources {
		cloned.Sources[position] = Source{
			Path: source.Path, Kind: source.Kind,
			Concepts: append([]string(nil), source.Concepts...),
		}
	}
	return cloned
}

// distinctConcepts keeps the first occurrence of every concept in the order
// the values were supplied, so a ceiling keeps the model's leading choices.
func distinctConcepts(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// capConcepts keeps the first MaxConceptsPerSource distinct concepts in their
// supplied order and returns them in canonical order.
func capConcepts(values []string) []string {
	kept := distinctConcepts(values)
	if len(kept) > MaxConceptsPerSource {
		kept = kept[:MaxConceptsPerSource]
	}
	sort.Strings(kept)
	return kept
}

func canonicalTextSet(values []string) bool {
	for position, value := range values {
		if !validText(value) || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
}

func validRepositoryPath(value string) bool {
	return validText(value) && !strings.HasPrefix(value, "/") &&
		!strings.Contains(value, "\\") && path.Clean(value) == value &&
		value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func validGuidanceKind(kind readmetargetscout.GuidanceKind) bool {
	return kind == readmetargetscout.GuidanceReadme || kind == readmetargetscout.GuidanceAgents
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
