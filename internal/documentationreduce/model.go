// Package documentationreduce condenses repository-authored README and
// AGENTS.md documents into a compact, source-bound product-context handoff.
// It does not inspect or classify program elements.
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

const Version = 1

// Source is compact model-authored context restored to one exact guidance
// document. Claims and Concepts are sets; their canonical order is local.
type Source struct {
	Path     string                         `json:"path"`
	Kind     readmetargetscout.GuidanceKind `json:"kind"`
	Claims   []string                       `json:"claims"`
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
			source.Claims == nil || source.Concepts == nil ||
			len(source.Claims)+len(source.Concepts) == 0 ||
			!canonicalTextSet(source.Claims) || !canonicalTextSet(source.Concepts) {
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
		claims, err := canonicalizeText(value.Claims)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: source %q claims: %w", value.Path, err)
		}
		concepts, err := canonicalizeText(value.Concepts)
		if err != nil {
			return nil, fmt.Errorf("documentation reduce: source %q concepts: %w", value.Path, err)
		}
		if len(claims)+len(concepts) == 0 {
			continue
		}
		current, exists := byPath[value.Path]
		if exists && current.Kind != value.Kind {
			return nil, fmt.Errorf("documentation reduce: conflicting kinds for %q", value.Path)
		}
		current.Path = value.Path
		current.Kind = value.Kind
		current.Claims = append(current.Claims, claims...)
		current.Concepts = append(current.Concepts, concepts...)
		byPath[value.Path] = current
	}
	result := make([]Source, 0, len(byPath))
	for _, source := range byPath {
		claims, _ := canonicalizeText(source.Claims)
		concepts, _ := canonicalizeText(source.Concepts)
		source.Claims = claims
		source.Concepts = concepts
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	if result == nil {
		return []Source{}, nil
	}
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
			Claims:   append([]string(nil), source.Claims...),
			Concepts: append([]string(nil), source.Concepts...),
		}
	}
	if cloned.Sources == nil {
		cloned.Sources = []Source{}
	}
	return cloned
}

func canonicalizeText(values []string) ([]string, error) {
	result := append([]string(nil), values...)
	for _, value := range result {
		if !validText(value) {
			return nil, fmt.Errorf("invalid compact text")
		}
	}
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	if write == 0 {
		return []string{}, nil
	}
	return result[:write], nil
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
