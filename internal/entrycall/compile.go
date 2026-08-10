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

type Request struct {
	Version        int                   `json:"version"`
	RequestRef     string                `json:"request_ref"`
	Entries        []RequestEntry        `json:"entries"`
	OmittedEntries int                   `json:"omitted_entries"`
	SurfaceCatalog RequestSurfaceCatalog `json:"surface_catalog"`
}

type RequestEntry struct {
	Ref         string            `json:"ref"`
	Label       string            `json:"label"`
	RootNodeRef string            `json:"root_node_ref"`
	Nodes       []RequestNode     `json:"nodes"`
	Families    []RequestFamily   `json:"families"`
	Frontier    []RequestFrontier `json:"frontier"`
	Omitted     Omitted           `json:"omitted"`
}

type RequestNode struct {
	Ref   string `json:"ref"`
	Label string `json:"label"`
	Depth int    `json:"depth"`
}

type RequestFamily struct {
	Ref          string     `json:"ref"`
	CallerRef    string     `json:"caller_ref"`
	CalleeRef    string     `json:"callee_ref"`
	Invocation   Invocation `json:"invocation"`
	WitnessCount int        `json:"witness_count"`
}

type RequestFrontier struct {
	NodeRef      string `json:"node_ref"`
	Reason       string `json:"reason"`
	FamilyCount  int    `json:"family_count"`
	WitnessCount int    `json:"witness_count"`
}

type Omitted struct {
	Nodes     int `json:"nodes"`
	Families  int `json:"families"`
	Witnesses int `json:"witnesses"`
}

type Compilation struct {
	Request         Request
	SubstrateSHA256 string

	wire             []byte
	wireSHA          string
	substrateSHA256  string
	authority        map[string]rootAuthority
	surfaceAuthority map[string]surfaceCandidateAuthority
	surfaceCoverage  SurfaceCandidateCoverage
}

type rootAuthority struct {
	rootNode    ExactNode
	familyByRef map[string]ExactFamily
	request     RequestEntry
}

type queueNode struct {
	id    string
	depth int
}

// Compile makes a deterministic shallow projection around exact process-main
// roots. High-volume families win local bounds; every excluded witness is
// still accounted for in a typed frontier.
func Compile(substrate Substrate) (Compilation, error) {
	if err := validateSubstrate(substrate); err != nil {
		return Compilation{}, err
	}
	nodeByID := make(map[string]ExactNode, len(substrate.Nodes))
	for _, node := range substrate.Nodes {
		nodeByID[node.ID] = node
	}
	outgoing := make(map[string][]ExactFamily)
	for _, family := range substrate.Families {
		outgoing[family.CallerID] = append(outgoing[family.CallerID], family)
	}
	for callerID := range outgoing {
		sort.Slice(outgoing[callerID], func(i, j int) bool {
			if outgoing[callerID][i].WitnessCount != outgoing[callerID][j].WitnessCount {
				return outgoing[callerID][i].WitnessCount > outgoing[callerID][j].WitnessCount
			}
			left, right := firstLocation(outgoing[callerID][i].Callsites), firstLocation(outgoing[callerID][j].Callsites)
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			if left.Column != right.Column {
				return left.Column < right.Column
			}
			return outgoing[callerID][i].ID < outgoing[callerID][j].ID
		})
	}
	frontierByCaller := make(map[string]ExactFrontier, len(substrate.Frontiers))
	for _, frontier := range substrate.Frontiers {
		frontierByCaller[frontier.CallerID] = frontier
	}

	roots := append([]ExactRoot(nil), substrate.Roots...)
	sort.Slice(roots, func(i, j int) bool { return roots[i].NodeID < roots[j].NodeID })
	request := Request{
		Version: RequestVersion, Entries: []RequestEntry{},
		OmittedEntries: maxInt(0, len(roots)-MaxRoots),
		SurfaceCatalog: defaultRequestSurfaceCatalog(),
	}
	compilation := Compilation{
		authority:        make(map[string]rootAuthority),
		surfaceAuthority: make(map[string]surfaceCandidateAuthority),
	}
	compilation.SubstrateSHA256 = substrateSHA256(substrate)
	compilation.substrateSHA256 = compilation.SubstrateSHA256
	nextNodeRef, nextFamilyRef := 1, 1
	rootRefByNodeID := make(map[string]string, minInt(len(roots), MaxRoots))
	for index, root := range roots {
		if index >= MaxRoots {
			break
		}
		entry, authority := compileRoot(
			root, nodeByID, outgoing, frontierByCaller,
			index+1, &nextNodeRef, &nextFamilyRef,
		)
		request.Entries = append(request.Entries, entry)
		compilation.authority[entry.Ref] = authority
		rootRefByNodeID[root.NodeID] = entry.Ref
	}
	surfaceCatalog, surfaceAuthority, surfaceCoverage, err := compileSurfaceCatalog(substrate, rootRefByNodeID)
	if err != nil {
		return Compilation{}, err
	}
	request.SurfaceCatalog = surfaceCatalog
	compilation.surfaceAuthority = surfaceAuthority
	compilation.surfaceCoverage = surfaceCoverage
	identity, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("entry call: encode request identity: %w", err)
	}
	digest := sha256.Sum256(identity)
	request.RequestRef = "q-" + hex.EncodeToString(digest[:8])
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("entry call: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf("entry call: compiled request exceeds %d bytes", MaxRequestBytes)
	}
	compilation.Request = request
	compilation.wire = append([]byte(nil), wire...)
	compilation.wireSHA = sha256Hex(wire)
	return compilation, nil
}

func compileRoot(
	root ExactRoot,
	nodeByID map[string]ExactNode,
	outgoing map[string][]ExactFamily,
	frontierByCaller map[string]ExactFrontier,
	rootNumber int,
	nextNodeRef, nextFamilyRef *int,
) (RequestEntry, rootAuthority) {
	rootNode := nodeByID[root.NodeID]
	entry := RequestEntry{
		Ref: "r" + fmt.Sprint(rootNumber), Label: sanitizeLabel(rootNode.Label),
		Nodes: []RequestNode{}, Families: []RequestFamily{}, Frontier: []RequestFrontier{},
	}
	authority := rootAuthority{
		rootNode: rootNode, familyByRef: make(map[string]ExactFamily),
	}
	nodeRefByID := make(map[string]string)
	omittedNodeIDs := make(map[string]struct{})
	addNode := func(node ExactNode, depth int) string {
		ref := "n" + fmt.Sprint(*nextNodeRef)
		(*nextNodeRef)++
		nodeRefByID[node.ID] = ref
		entry.Nodes = append(entry.Nodes, RequestNode{Ref: ref, Label: sanitizeLabel(node.Label), Depth: depth})
		return ref
	}
	entry.RootNodeRef = addNode(rootNode, 0)
	queue := []queueNode{{id: root.NodeID, depth: 0}}
	processed := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if processed[current.id] {
			continue
		}
		processed[current.id] = true
		callerRef := nodeRefByID[current.id]
		appendExactFrontier(&entry, callerRef, frontierByCaller[current.id])
		families := outgoing[current.id]
		if current.depth >= MaxDepth {
			appendOmittedFrontier(&entry, callerRef, "depth_limit", families, omittedNodeIDs)
			continue
		}
		admitted := families
		if len(admitted) > MaxOutgoingFamiliesPerNode {
			appendOmittedFrontier(&entry, callerRef, "outgoing_limit", admitted[MaxOutgoingFamiliesPerNode:], omittedNodeIDs)
			admitted = admitted[:MaxOutgoingFamiliesPerNode]
		}
		for familyIndex, family := range admitted {
			callee, known := nodeByID[family.CalleeID]
			if !known {
				appendOmittedFrontier(&entry, callerRef, "invalid_endpoint", admitted[familyIndex:familyIndex+1], omittedNodeIDs)
				continue
			}
			calleeRef, seen := nodeRefByID[callee.ID]
			if !seen && len(entry.Nodes) >= MaxNodesPerRoot {
				appendOmittedFrontier(&entry, callerRef, "node_limit", admitted[familyIndex:], omittedNodeIDs)
				break
			}
			if len(entry.Families) >= MaxFamiliesPerRoot {
				appendOmittedFrontier(&entry, callerRef, "family_limit", admitted[familyIndex:], omittedNodeIDs)
				break
			}
			if !seen {
				calleeRef = addNode(callee, current.depth+1)
				queue = append(queue, queueNode{id: callee.ID, depth: current.depth + 1})
			}
			familyRef := "f" + fmt.Sprint(*nextFamilyRef)
			(*nextFamilyRef)++
			entry.Families = append(entry.Families, RequestFamily{
				Ref: familyRef, CallerRef: callerRef, CalleeRef: calleeRef,
				Invocation: family.Invocation, WitnessCount: family.WitnessCount,
			})
			authority.familyByRef[familyRef] = family
		}
	}
	for nodeID := range omittedNodeIDs {
		if _, advertised := nodeRefByID[nodeID]; !advertised {
			entry.Omitted.Nodes++
		}
	}
	sort.Slice(entry.Nodes, func(i, j int) bool {
		if entry.Nodes[i].Depth != entry.Nodes[j].Depth {
			return entry.Nodes[i].Depth < entry.Nodes[j].Depth
		}
		return entry.Nodes[i].Ref < entry.Nodes[j].Ref
	})
	sort.Slice(entry.Frontier, func(i, j int) bool {
		if entry.Frontier[i].NodeRef != entry.Frontier[j].NodeRef {
			return entry.Frontier[i].NodeRef < entry.Frontier[j].NodeRef
		}
		return entry.Frontier[i].Reason < entry.Frontier[j].Reason
	})
	authority.request = entry
	return entry, authority
}

func appendExactFrontier(entry *RequestEntry, nodeRef string, frontier ExactFrontier) {
	values := []struct {
		reason string
		count  int
	}{
		{"dynamic_invoke", frontier.DynamicInvokesExcluded},
		{"non_static", frontier.NonStaticCallsExcluded},
		{"unidentified_static", frontier.UnidentifiedCallsExcluded},
	}
	for _, value := range values {
		if value.count <= 0 {
			continue
		}
		entry.Frontier = append(entry.Frontier, RequestFrontier{
			NodeRef: nodeRef, Reason: value.reason,
			FamilyCount: value.count, WitnessCount: value.count,
		})
		entry.Omitted.Families += value.count
		entry.Omitted.Witnesses += value.count
	}
}

func appendOmittedFrontier(
	entry *RequestEntry,
	nodeRef, reason string,
	families []ExactFamily,
	omittedNodeIDs map[string]struct{},
) {
	if len(families) == 0 {
		return
	}
	witnesses := 0
	for _, family := range families {
		witnesses += family.WitnessCount
		omittedNodeIDs[family.CalleeID] = struct{}{}
	}
	entry.Frontier = append(entry.Frontier, RequestFrontier{
		NodeRef: nodeRef, Reason: reason, FamilyCount: len(families), WitnessCount: witnesses,
	})
	entry.Omitted.Families += len(families)
	entry.Omitted.Witnesses += witnesses
}

func validateSubstrate(substrate Substrate) error {
	if substrate.Version != SubstrateVersion || substrate.State != StateReady || substrate.ClosedReason != "" {
		return fmt.Errorf("entry call: substrate is not ready")
	}
	if len(substrate.Roots) == 0 {
		return fmt.Errorf("entry call: substrate has no roots")
	}
	nodes := make(map[string]ExactNode, len(substrate.Nodes))
	for _, node := range substrate.Nodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Label) == "" {
			return fmt.Errorf("entry call: invalid exact node")
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return fmt.Errorf("entry call: duplicate exact node")
		}
		if !node.External && !validLocation(node.Declaration) {
			return fmt.Errorf("entry call: invalid local declaration")
		}
		nodes[node.ID] = node
	}
	seenRoots := make(map[string]struct{}, len(substrate.Roots))
	for _, root := range substrate.Roots {
		node, known := nodes[root.NodeID]
		if !known || node.External {
			return fmt.Errorf("entry call: root is not an exact local node")
		}
		if _, duplicate := seenRoots[root.NodeID]; duplicate {
			return fmt.Errorf("entry call: duplicate root")
		}
		seenRoots[root.NodeID] = struct{}{}
	}
	seenFamilies := make(map[string]struct{}, len(substrate.Families))
	for _, family := range substrate.Families {
		if strings.TrimSpace(family.ID) == "" || family.WitnessCount <= 0 || !family.Invocation.Valid() {
			return fmt.Errorf("entry call: invalid exact family")
		}
		if _, duplicate := seenFamilies[family.ID]; duplicate {
			return fmt.Errorf("entry call: duplicate exact family")
		}
		seenFamilies[family.ID] = struct{}{}
		caller, callerKnown := nodes[family.CallerID]
		_, calleeKnown := nodes[family.CalleeID]
		if !callerKnown || caller.External || !calleeKnown || len(family.Callsites) == 0 ||
			len(family.Callsites) > MaxRepresentativeCallsites {
			return fmt.Errorf("entry call: invalid family endpoints or callsites")
		}
		for _, location := range family.Callsites {
			if !validLocation(location) {
				return fmt.Errorf("entry call: invalid family callsite")
			}
		}
	}
	seenFrontiers := make(map[string]struct{}, len(substrate.Frontiers))
	for _, frontier := range substrate.Frontiers {
		node, known := nodes[frontier.CallerID]
		if !known || node.External || frontier.DynamicInvokesExcluded < 0 ||
			frontier.NonStaticCallsExcluded < 0 || frontier.UnidentifiedCallsExcluded < 0 {
			return fmt.Errorf("entry call: invalid exact frontier")
		}
		if _, duplicate := seenFrontiers[frontier.CallerID]; duplicate {
			return fmt.Errorf("entry call: duplicate exact frontier")
		}
		seenFrontiers[frontier.CallerID] = struct{}{}
	}
	if err := validateSurfaceSubstrate(substrate, seenRoots); err != nil {
		return err
	}
	return nil
}

func validateSurfaceSubstrate(substrate Substrate, roots map[string]struct{}) error {
	coverage := substrate.Coverage
	counts := []int{
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
	if compilation.Request.Version != RequestVersion || len(compilation.Request.Entries) == 0 ||
		len(compilation.Request.Entries) > MaxRoots || compilation.Request.OmittedEntries < 0 ||
		!validRequestRef(compilation.Request.RequestRef) || len(compilation.SubstrateSHA256) != sha256.Size*2 ||
		compilation.SubstrateSHA256 != compilation.substrateSHA256 ||
		len(compilation.authority) != len(compilation.Request.Entries) ||
		!validSurfaceCoverage(compilation.surfaceCoverage) {
		return fmt.Errorf("entry call: invalid compilation")
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil || len(wire) > MaxRequestBytes || !bytes.Equal(wire, compilation.wire) ||
		compilation.wireSHA != sha256Hex(wire) {
		return fmt.Errorf("entry call: request wire binding mismatch")
	}
	for _, entry := range compilation.Request.Entries {
		authority, known := compilation.authority[entry.Ref]
		if !known || !requestEntriesEqual(entry, authority.request) || entry.RootNodeRef == "" ||
			len(entry.Nodes) == 0 || len(entry.Nodes) > MaxNodesPerRoot || len(entry.Families) > MaxFamiliesPerRoot {
			return fmt.Errorf("entry call: request authority mismatch")
		}
	}
	if err := validateCompiledSurfaceCatalog(compilation); err != nil {
		return err
	}
	return nil
}

// AdvertisedFamilyCount lets the runtime skip an empty semantic call without
// inspecting or reinterpreting the provider request.
func (compilation Compilation) AdvertisedFamilyCount() int {
	if validateCompilation(compilation) != nil {
		return 0
	}
	total := 0
	for _, entry := range compilation.Request.Entries {
		total += len(entry.Families)
	}
	return total
}

// AdvertisedSurfaceCandidateCount lets the runtime preserve the ordinary
// one-call path when exact candidate facts exist even if no call family was
// useful enough to advertise.
func (compilation Compilation) AdvertisedSurfaceCandidateCount() int {
	if validateCompilation(compilation) != nil {
		return 0
	}
	return len(compilation.Request.SurfaceCatalog.Candidates)
}

// SurfaceCoverage returns aggregate backend-owned accounting only. Candidate
// identity and exact values remain in private compilation authority.
func (compilation Compilation) SurfaceCoverage() SurfaceCandidateCoverage {
	if validateCompilation(compilation) != nil {
		return SurfaceCandidateCoverage{}
	}
	return compilation.surfaceCoverage
}

func (compilation Compilation) RequestSHA256() string {
	if validateCompilation(compilation) != nil {
		return ""
	}
	return compilation.wireSHA
}

func substrateSHA256(substrate Substrate) string {
	roots := append([]ExactRoot(nil), substrate.Roots...)
	nodes := append([]ExactNode(nil), substrate.Nodes...)
	families := append([]ExactFamily(nil), substrate.Families...)
	frontiers := append([]ExactFrontier(nil), substrate.Frontiers...)
	surfaceCandidates := append([]ExactSurfaceCandidate(nil), substrate.SurfaceCandidates...)
	sort.Slice(roots, func(i, j int) bool { return roots[i].NodeID < roots[j].NodeID })
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(families, func(i, j int) bool { return families[i].ID < families[j].ID })
	sort.Slice(frontiers, func(i, j int) bool { return frontiers[i].CallerID < frontiers[j].CallerID })
	for index := range surfaceCandidates {
		surfaceCandidates[index].Facts = append([]ExactSurfaceFact(nil), surfaceCandidates[index].Facts...)
		sort.Slice(surfaceCandidates[index].Facts, func(i, j int) bool {
			return surfaceCandidates[index].Facts[i].ID < surfaceCandidates[index].Facts[j].ID
		})
	}
	sort.Slice(surfaceCandidates, func(i, j int) bool { return surfaceCandidates[i].ID < surfaceCandidates[j].ID })
	for index := range families {
		families[index].Callsites = append([]Location(nil), families[index].Callsites...)
		sortLocations(families[index].Callsites)
	}
	// The Exact* JSON tags deliberately hide authority, so use a private wire
	// representation solely for this local digest.
	type digestRoot struct{ NodeID string }
	type digestNode struct {
		ID, Label   string
		Declaration Location
		External    bool
	}
	type digestFamily struct {
		ID, CallerID, CalleeID string
		Invocation             Invocation
		WitnessCount           int
		Callsites              []Location
	}
	type digestFrontier struct {
		CallerID                                                                  string
		DynamicInvokesExcluded, NonStaticCallsExcluded, UnidentifiedCallsExcluded int
	}
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
		Nodes             []digestNode
		Families          []digestFamily
		Frontiers         []digestFrontier
		SurfaceCandidates []digestSurfaceCandidate
		Coverage          Coverage
	}{Version: substrate.Version, Roots: make([]digestRoot, 0, len(roots)), Nodes: make([]digestNode, 0, len(nodes)), Families: make([]digestFamily, 0, len(families)), Frontiers: make([]digestFrontier, 0, len(frontiers)), SurfaceCandidates: make([]digestSurfaceCandidate, 0, len(surfaceCandidates)), Coverage: substrate.Coverage}
	for _, root := range roots {
		wire.Roots = append(wire.Roots, digestRoot{root.NodeID})
	}
	for _, node := range nodes {
		wire.Nodes = append(wire.Nodes, digestNode{node.ID, node.Label, node.Declaration, node.External})
	}
	for _, family := range families {
		wire.Families = append(wire.Families, digestFamily{family.ID, family.CallerID, family.CalleeID, family.Invocation, family.WitnessCount, family.Callsites})
	}
	for _, frontier := range frontiers {
		wire.Frontiers = append(wire.Frontiers, digestFrontier{frontier.CallerID, frontier.DynamicInvokesExcluded, frontier.NonStaticCallsExcluded, frontier.UnidentifiedCallsExcluded})
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

func firstLocation(locations []Location) Location {
	first := locations[0]
	for _, candidate := range locations[1:] {
		if candidate.Path < first.Path ||
			(candidate.Path == first.Path && candidate.Line < first.Line) ||
			(candidate.Path == first.Path && candidate.Line == first.Line && candidate.Column < first.Column) {
			first = candidate
		}
	}
	return first
}

func requestEntriesEqual(left, right RequestEntry) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
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
