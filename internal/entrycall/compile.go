package entrycall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Request is the versioned compiled activity catalog. The activity-surface
// cube exposes SurfaceCatalog in its sole provider request. Root refs occur
// only on candidates; exact root identities remain private.
type Request struct {
	Version        int                   `json:"version"`
	SurfaceCatalog RequestSurfaceCatalog `json:"surface_catalog"`
}

type Compilation struct {
	Request         Request
	SubstrateSHA256 string

	wire             []byte
	substrateSHA256  string
	rootNodeByRef    map[string]string
	surfaceAuthority map[string]surfaceCandidateAuthority
	surfaceCoverage  SurfaceCandidateCoverage
}

// Compile assigns request-local root and candidate refs, then emits only the
// bounded generic activity catalog. The repository call closure has already
// established each candidate's exact root before this boundary.
func Compile(substrate Substrate) (Compilation, error) {
	if err := validateSubstrate(substrate); err != nil {
		return Compilation{}, err
	}

	roots := append([]ExactRoot(nil), substrate.Roots...)
	sort.Slice(roots, func(i, j int) bool { return roots[i].NodeID < roots[j].NodeID })
	rootRefByNodeID := make(map[string]string, len(roots))
	rootNodeByRef := make(map[string]string, len(roots))
	for index, root := range roots {
		ref := fmt.Sprintf("r%d", index+1)
		rootRefByNodeID[root.NodeID] = ref
		rootNodeByRef[ref] = root.NodeID
	}

	surfaceCatalog, surfaceAuthority, surfaceCoverage, err := compileSurfaceCatalog(substrate, rootRefByNodeID)
	if err != nil {
		return Compilation{}, err
	}
	request := Request{Version: RequestVersion, SurfaceCatalog: surfaceCatalog}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("entry call: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf("entry call: compiled request exceeds %d bytes", MaxRequestBytes)
	}
	digest := substrateSHA256(substrate)
	return Compilation{
		Request: request, SubstrateSHA256: digest,
		wire: append([]byte(nil), wire...), substrateSHA256: digest,
		rootNodeByRef: rootNodeByRef, surfaceAuthority: surfaceAuthority,
		surfaceCoverage: surfaceCoverage,
	}, nil
}

func validateSubstrate(substrate Substrate) error {
	if substrate.Version != SubstrateVersion || substrate.State != StateReady || substrate.ClosedReason != "" {
		return fmt.Errorf("entry call: substrate is not ready")
	}
	if len(substrate.Roots) == 0 || substrate.Coverage.RootsConsidered != len(substrate.Roots) {
		return fmt.Errorf("entry call: substrate has invalid root coverage")
	}
	seenRoots := make(map[string]struct{}, len(substrate.Roots))
	for _, root := range substrate.Roots {
		if strings.TrimSpace(root.NodeID) == "" {
			return fmt.Errorf("entry call: invalid exact root")
		}
		if _, duplicate := seenRoots[root.NodeID]; duplicate {
			return fmt.Errorf("entry call: duplicate root")
		}
		seenRoots[root.NodeID] = struct{}{}
	}
	return validateSurfaceSubstrate(substrate, seenRoots)
}

func validateSurfaceSubstrate(substrate Substrate, roots map[string]struct{}) error {
	coverage := substrate.Coverage
	counts := []int{
		coverage.RootsConsidered,
		coverage.SurfaceCandidatesConsidered,
		coverage.SurfaceCandidatesIndexed,
		coverage.SurfaceCandidateLimitExcluded,
		coverage.SurfaceCandidateFactsConsidered,
		coverage.SurfaceCandidateFactsIndexed,
		coverage.SurfaceCandidateFactLimitExcluded,
		coverage.UnsafeSurfaceCandidateFactsExcluded,
		coverage.UnreachableSurfaceCandidatesExcluded,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("entry call: invalid surface coverage")
		}
	}
	if len(substrate.SurfaceCandidates) > MaxRawSurfaceCandidates ||
		coverage.SurfaceCandidatesIndexed != len(substrate.SurfaceCandidates) ||
		coverage.SurfaceCandidatesConsidered < coverage.SurfaceCandidatesIndexed {
		return fmt.Errorf("entry call: invalid surface candidate coverage")
	}
	indexedFacts := 0
	seenCandidates := make(map[string]struct{}, len(substrate.SurfaceCandidates))
	seenFacts := make(map[string]struct{})
	for _, candidate := range substrate.SurfaceCandidates {
		if strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.Sketch) == "" ||
			!candidate.Form.Valid() || !validLocation(candidate.Site) ||
			len(candidate.Facts) == 0 || len(candidate.Facts) > MaxRawSurfaceFactsPerCandidate {
			return fmt.Errorf("entry call: invalid exact surface candidate")
		}
		if _, duplicate := seenCandidates[candidate.ID]; duplicate {
			return fmt.Errorf("entry call: duplicate exact surface candidate")
		}
		seenCandidates[candidate.ID] = struct{}{}
		if _, knownRoot := roots[candidate.RootNodeID]; !knownRoot {
			return fmt.Errorf("entry call: surface candidate root is not an exact selected root")
		}
		for _, fact := range candidate.Facts {
			indexedFacts++
			if strings.TrimSpace(fact.ID) == "" || !fact.Kind.Valid() || fact.Position < 0 ||
				strings.TrimSpace(fact.Label) == "" || strings.TrimSpace(fact.Value) == "" ||
				!validLocation(fact.Location) {
				return fmt.Errorf("entry call: invalid exact surface fact")
			}
			if _, duplicate := seenFacts[fact.ID]; duplicate {
				return fmt.Errorf("entry call: duplicate exact surface fact")
			}
			seenFacts[fact.ID] = struct{}{}
		}
	}
	if coverage.SurfaceCandidateFactsIndexed != indexedFacts ||
		coverage.SurfaceCandidateFactsConsidered < coverage.SurfaceCandidateFactsIndexed {
		return fmt.Errorf("entry call: invalid surface fact coverage")
	}
	return nil
}

func validLocation(location Location) bool {
	return location.Line > 0 && location.Column >= 0 && fs.ValidPath(location.Path) &&
		location.Path != "." && !strings.HasPrefix(location.Path, "<external>/")
}

func validateCompilation(compilation Compilation) error {
	if compilation.Request.Version != RequestVersion || len(compilation.rootNodeByRef) == 0 ||
		len(compilation.SubstrateSHA256) != sha256.Size*2 ||
		compilation.SubstrateSHA256 != compilation.substrateSHA256 ||
		!validSurfaceCoverage(compilation.surfaceCoverage) {
		return fmt.Errorf("entry call: invalid compilation")
	}
	seenRootNodes := make(map[string]struct{}, len(compilation.rootNodeByRef))
	for ref, nodeID := range compilation.rootNodeByRef {
		if !validRef(ref, "r") || strings.TrimSpace(nodeID) == "" {
			return fmt.Errorf("entry call: invalid root authority")
		}
		if _, duplicate := seenRootNodes[nodeID]; duplicate {
			return fmt.Errorf("entry call: duplicate root authority")
		}
		seenRootNodes[nodeID] = struct{}{}
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil || len(wire) > MaxRequestBytes || !bytes.Equal(wire, compilation.wire) {
		return fmt.Errorf("entry call: request wire binding mismatch")
	}
	return validateCompiledSurfaceCatalog(compilation)
}

// RootNodeID restores a request-local root ref to exact local authority.
func (compilation Compilation) RootNodeID(ref string) (string, bool) {
	if validateCompilation(compilation) != nil {
		return "", false
	}
	nodeID, known := compilation.rootNodeByRef[ref]
	return nodeID, known
}

// SurfaceCoverage returns aggregate backend-owned accounting only. Candidate
// identity and exact values remain in private compilation authority.
func (compilation Compilation) SurfaceCoverage() SurfaceCandidateCoverage {
	if validateCompilation(compilation) != nil {
		return SurfaceCandidateCoverage{}
	}
	return compilation.surfaceCoverage
}

func substrateSHA256(substrate Substrate) string {
	roots := append([]ExactRoot(nil), substrate.Roots...)
	surfaceCandidates := append([]ExactSurfaceCandidate(nil), substrate.SurfaceCandidates...)
	sort.Slice(roots, func(i, j int) bool { return roots[i].NodeID < roots[j].NodeID })
	for index := range surfaceCandidates {
		surfaceCandidates[index].Facts = append([]ExactSurfaceFact(nil), surfaceCandidates[index].Facts...)
		sort.Slice(surfaceCandidates[index].Facts, func(i, j int) bool {
			return surfaceCandidates[index].Facts[i].ID < surfaceCandidates[index].Facts[j].ID
		})
	}
	sort.Slice(surfaceCandidates, func(i, j int) bool { return surfaceCandidates[i].ID < surfaceCandidates[j].ID })

	type digestRoot struct{ NodeID string }
	type digestSurfaceFact struct {
		ID       string
		Kind     SurfaceFactKind
		Position int
		Label    string
		Value    string
		Location Location
	}
	type digestSurfaceCandidate struct {
		ID, RootNodeID string
		Form           SurfaceCandidateForm
		Sketch         string
		Site           Location
		Facts          []digestSurfaceFact
	}
	wire := struct {
		Version           int
		Roots             []digestRoot
		SurfaceCandidates []digestSurfaceCandidate
		Coverage          Coverage
	}{
		Version: substrate.Version, Roots: make([]digestRoot, 0, len(roots)),
		SurfaceCandidates: make([]digestSurfaceCandidate, 0, len(surfaceCandidates)),
		Coverage:          substrate.Coverage,
	}
	for _, root := range roots {
		wire.Roots = append(wire.Roots, digestRoot{root.NodeID})
	}
	for _, candidate := range surfaceCandidates {
		digestCandidate := digestSurfaceCandidate{
			ID: candidate.ID, RootNodeID: candidate.RootNodeID, Form: candidate.Form,
			Sketch: candidate.Sketch, Site: candidate.Site,
			Facts: make([]digestSurfaceFact, 0, len(candidate.Facts)),
		}
		for _, fact := range candidate.Facts {
			digestCandidate.Facts = append(digestCandidate.Facts, digestSurfaceFact{
				ID: fact.ID, Kind: fact.Kind, Position: fact.Position,
				Label: fact.Label, Value: fact.Value, Location: fact.Location,
			})
		}
		wire.SurfaceCandidates = append(wire.SurfaceCandidates, digestCandidate)
	}
	encoded, _ := json.Marshal(wire)
	return sha256Hex(encoded)
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
