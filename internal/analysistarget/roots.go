package analysistarget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	TargetRootsVersion = 1

	// MaxTargetRoots inherits the already-paid DirectCallIndex resource ceiling
	// instead of inventing a smaller product/relevance cap. Roots below it stay
	// in deterministic source order; any future producer expansion beyond that
	// ceiling remains explicitly accounted rather than silently truncated.
	MaxTargetRoots = surfacediscovery.MaxDirectCallIndexNodes
)

// TargetRoot is one exact build-selected declaration owned by the selected
// package. Symbol is the producer-owned full symbol identity, not a display
// label or a consumer reconstruction.
type TargetRoot struct {
	NodeID  string `json:"-"`
	Path    string `json:"-"`
	Line    int    `json:"-"`
	Symbol  string `json:"-"`
	Package string `json:"-"`
}

// TargetRoots is a live-run-only authority envelope. Its fields cannot enter
// provider or report JSON accidentally. The self-seal binds the exact target,
// complete DirectCallIndex identity and build scenario to the retained roots
// and their explicit omission count.
type TargetRoots struct {
	Version               int                       `json:"-"`
	TargetRef             string                    `json:"-"`
	DirectCallIndexSHA256 string                    `json:"-"`
	Scenario              surfacediscovery.Scenario `json:"-"`
	Roots                 []TargetRoot              `json:"-"`
	OmittedRoots          int                       `json:"-"`
	SHA256                string                    `json:"-"`
}

// Snapshot returns an independently owned copy for an in-memory handoff.
func (roots TargetRoots) Snapshot() TargetRoots {
	result := roots
	result.Scenario.Tags = append([]string(nil), roots.Scenario.Tags...)
	result.Roots = append([]TargetRoot(nil), roots.Roots...)
	return result
}

// BindExactRoots derives roots exclusively from an already-built exact
// DirectCallIndex. It performs no package load, SSA build, provider request or
// symbol-name repair.
func BindExactRoots(target Target, index *surfacediscovery.DirectCallIndex) (TargetRoots, error) {
	if err := validateExactRootsInputs(target, index); err != nil {
		return TargetRoots{}, err
	}
	candidates, err := exactRootCandidates(target, index)
	if err != nil {
		return TargetRoots{}, err
	}
	retained, omitted := boundExactRootCandidates(candidates)
	result := TargetRoots{
		Version: TargetRootsVersion, TargetRef: target.Ref,
		DirectCallIndexSHA256: index.SHA256, Scenario: copyTargetRootScenario(index.Scenario),
		Roots: retained, OmittedRoots: omitted,
	}
	result.SHA256, err = targetRootsSHA256(result)
	if err != nil {
		return TargetRoots{}, err
	}
	return result, nil
}

// ValidateExactRoots rejects reuse under a different target, index or build
// scenario and re-derives every declaration from current producer authority.
func ValidateExactRoots(
	target Target,
	index *surfacediscovery.DirectCallIndex,
	roots TargetRoots,
) error {
	if err := validateExactRootsInputs(target, index); err != nil {
		return err
	}
	if roots.Version != TargetRootsVersion || roots.TargetRef != target.Ref ||
		roots.DirectCallIndexSHA256 != index.SHA256 ||
		!sameTargetRootScenario(roots.Scenario, index.Scenario) {
		return fmt.Errorf("analysis target roots: identity binding mismatch")
	}
	if roots.OmittedRoots < 0 || len(roots.Roots) > MaxTargetRoots {
		return fmt.Errorf("analysis target roots: invalid resource accounting")
	}

	expected, err := BindExactRoots(target, index)
	if err != nil {
		return err
	}
	if roots.OmittedRoots != expected.OmittedRoots || !sameTargetRoots(roots.Roots, expected.Roots) {
		return fmt.Errorf("analysis target roots: exact declaration binding mismatch")
	}
	wantSHA, err := targetRootsSHA256(roots)
	if err != nil {
		return err
	}
	if roots.SHA256 != expected.SHA256 || roots.SHA256 != wantSHA || len(roots.SHA256) != sha256.Size*2 {
		return fmt.Errorf("analysis target roots: sha256 mismatch")
	}
	if _, err := hex.DecodeString(roots.SHA256); err != nil {
		return fmt.Errorf("analysis target roots: invalid sha256: %w", err)
	}
	return nil
}

func validateExactRootsInputs(target Target, index *surfacediscovery.DirectCallIndex) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("analysis target roots: validate target: %w", err)
	}
	if index == nil {
		return fmt.Errorf("analysis target roots: direct call index is nil")
	}
	if err := index.Validate(); err != nil {
		return fmt.Errorf("analysis target roots: validate direct call index: %w", err)
	}
	if index.State != surfacediscovery.DirectCallIndexReady {
		return fmt.Errorf("analysis target roots: direct call index is unavailable")
	}
	if index.Scope.TargetScoped() &&
		(index.Scope.TargetKind != string(target.Kind) || index.Scope.TargetPackage != target.PackagePath) {
		return fmt.Errorf("analysis target roots: direct call index target scope mismatch")
	}
	return nil
}

type targetRootCandidate struct {
	root   TargetRoot
	column int
}

func exactRootCandidates(
	target Target,
	index *surfacediscovery.DirectCallIndex,
) ([]targetRootCandidate, error) {
	switch target.Kind {
	case KindLibraryPackage:
		result := make([]targetRootCandidate, 0)
		for _, node := range index.Nodes {
			if node.Package != target.PackagePath || !node.Exported {
				continue
			}
			result = append(result, targetRootCandidateFromNode(node))
		}
		sortTargetRootCandidates(result)
		return result, nil
	case KindExecutablePackage:
		result := make([]targetRootCandidate, 0, len(target.Roots))
		for _, targetRoot := range target.Roots {
			matches := make([]surfacediscovery.DirectCallNode, 0, 1)
			for _, node := range index.Nodes {
				if node.Package == target.PackagePath && node.Symbol.Name == "main" &&
					node.Declaration.Path == targetRoot.Path && node.Declaration.Line == targetRoot.Line {
					matches = append(matches, node)
				}
			}
			if len(matches) != 1 {
				return nil, fmt.Errorf(
					"analysis target roots: executable root %s:%d has %d exact main declarations",
					targetRoot.Path, targetRoot.Line, len(matches),
				)
			}
			result = append(result, targetRootCandidateFromNode(matches[0]))
		}
		sortTargetRootCandidates(result)
		return result, nil
	default:
		return nil, fmt.Errorf("analysis target roots: unsupported target kind %q", target.Kind)
	}
}

func targetRootCandidateFromNode(node surfacediscovery.DirectCallNode) targetRootCandidate {
	return targetRootCandidate{
		root: TargetRoot{
			NodeID: node.ID, Path: node.Declaration.Path, Line: node.Declaration.Line,
			Symbol: node.Symbol.ID, Package: node.Package,
		},
		column: node.Declaration.Column,
	}
}

func sortTargetRootCandidates(candidates []targetRootCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.root.Path != right.root.Path {
			return left.root.Path < right.root.Path
		}
		if left.root.Line != right.root.Line {
			return left.root.Line < right.root.Line
		}
		if left.column != right.column {
			return left.column < right.column
		}
		if left.root.Symbol != right.root.Symbol {
			return left.root.Symbol < right.root.Symbol
		}
		return left.root.NodeID < right.root.NodeID
	})
}

func boundExactRootCandidates(candidates []targetRootCandidate) ([]TargetRoot, int) {
	limit := len(candidates)
	if limit > MaxTargetRoots {
		limit = MaxTargetRoots
	}
	result := make([]TargetRoot, 0, limit)
	for _, candidate := range candidates[:limit] {
		result = append(result, candidate.root)
	}
	return result, len(candidates) - limit
}

func copyTargetRootScenario(scenario surfacediscovery.Scenario) surfacediscovery.Scenario {
	result := scenario
	result.Tags = append([]string(nil), scenario.Tags...)
	return result
}

func sameTargetRootScenario(left, right surfacediscovery.Scenario) bool {
	if left.ID != right.ID || left.GOOS != right.GOOS || left.GOARCH != right.GOARCH ||
		left.GoFlags != right.GoFlags || len(left.Tags) != len(right.Tags) {
		return false
	}
	for index := range left.Tags {
		if left.Tags[index] != right.Tags[index] {
			return false
		}
	}
	return true
}

func sameTargetRoots(left, right []TargetRoot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type targetRootsDigestMaterial struct {
	Version               int                       `json:"version"`
	TargetRef             string                    `json:"target_ref"`
	DirectCallIndexSHA256 string                    `json:"direct_call_index_sha256"`
	Scenario              surfacediscovery.Scenario `json:"scenario"`
	Roots                 []targetRootDigest        `json:"roots"`
	OmittedRoots          int                       `json:"omitted_roots"`
}

type targetRootDigest struct {
	NodeID  string `json:"node_id"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Symbol  string `json:"symbol"`
	Package string `json:"package"`
}

func targetRootsSHA256(roots TargetRoots) (string, error) {
	material := targetRootsDigestMaterial{
		Version: roots.Version, TargetRef: roots.TargetRef,
		DirectCallIndexSHA256: roots.DirectCallIndexSHA256,
		Scenario:              copyTargetRootScenario(roots.Scenario), OmittedRoots: roots.OmittedRoots,
		Roots: make([]targetRootDigest, 0, len(roots.Roots)),
	}
	for _, root := range roots.Roots {
		material.Roots = append(material.Roots, targetRootDigest(root))
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("analysis target roots: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
