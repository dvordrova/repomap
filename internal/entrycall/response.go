package entrycall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Response struct {
	Version          int                       `json:"version"`
	RequestRef       string                    `json:"request_ref"`
	Entries          []ResponseEntry           `json:"entries"`
	SurfaceProposals []ResponseSurfaceProposal `json:"surface_proposals"`
}

type ResponseEntry struct {
	RootRef    string   `json:"root_ref"`
	FamilyRefs []string `json:"family_refs"`
}

type ResponseSurfaceProposal struct {
	CandidateRef string                   `json:"candidate_ref"`
	KindRef      string                   `json:"kind_ref"`
	Bindings     []ResponseSurfaceBinding `json:"bindings"`
}

type ResponseSurfaceBinding struct {
	SlotRef string `json:"slot_ref"`
	FactRef string `json:"fact_ref"`
}

type Result struct {
	Version                  int                       `json:"version"`
	PromptVersion            string                    `json:"prompt_version"`
	RequestRef               string                    `json:"request_ref"`
	RequestSHA256            string                    `json:"request_sha256"`
	SubstrateSHA256          string                    `json:"substrate_sha256"`
	RepositoryStateSHA256    string                    `json:"repository_state_sha256,omitempty"`
	Entries                  []ResultEntry             `json:"entries"`
	SurfaceProposals         []ResultSurfaceProposal   `json:"surface_proposals"`
	RejectedSurfaceProposals []RejectedSurfaceProposal `json:"rejected_surface_proposals"`
	SurfaceCandidateCoverage SurfaceCandidateCoverage  `json:"surface_candidate_coverage"`
}

type ResultEntry struct {
	RootRef          string                 `json:"root_ref"`
	Label            string                 `json:"label"`
	Declaration      Location               `json:"declaration"`
	Families         []ResultFamily         `json:"families"`
	RejectedFamilies []RejectedResultFamily `json:"rejected_families"`
	Frontier         []RequestFrontier      `json:"frontier"`
	Omitted          Omitted                `json:"omitted"`
}

type RejectedFamilyReason string

const (
	RejectedFamilyUnreachable RejectedFamilyReason = "unreachable_from_root"
)

type RejectedResultFamily struct {
	Ref    string               `json:"ref"`
	Reason RejectedFamilyReason `json:"reason"`
}

type ResultFamily struct {
	Ref          string     `json:"ref"`
	CallerLabel  string     `json:"caller_label"`
	CalleeLabel  string     `json:"callee_label"`
	Invocation   Invocation `json:"invocation"`
	WitnessCount int        `json:"witness_count"`
	Callsites    []Location `json:"callsites"`
}

type ResultSurfaceProposal struct {
	ID           string               `json:"id"`
	CandidateRef string               `json:"candidate_ref"`
	RootRef      string               `json:"root_ref"`
	Kind         string               `json:"kind"`
	Role         string               `json:"role"`
	Form         SurfaceCandidateForm `json:"form"`
	Site         Location             `json:"site"`
	Identity     *ResultSurfaceValue  `json:"identity,omitempty"`
	Method       *ResultSurfaceValue  `json:"method,omitempty"`
	Path         *ResultSurfaceValue  `json:"path,omitempty"`
	Handler      *ResultSurfaceValue  `json:"handler,omitempty"`
}

type ResultSurfaceValue struct {
	Kind     SurfaceFactKind `json:"kind"`
	Text     string          `json:"text"`
	Location *Location       `json:"location,omitempty"`
}

type RejectedSurfaceReason string

const (
	RejectedSurfaceIncompatibleForm    RejectedSurfaceReason = "incompatible_form"
	RejectedSurfaceMissingBinding      RejectedSurfaceReason = "missing_required_binding"
	RejectedSurfaceDuplicateBinding    RejectedSurfaceReason = "duplicate_binding"
	RejectedSurfaceDuplicateProposal   RejectedSurfaceReason = "duplicate_proposal"
	RejectedSurfaceIncompatibleBinding RejectedSurfaceReason = "incompatible_binding"
)

type RejectedSurfaceProposal struct {
	CandidateRef string                `json:"candidate_ref"`
	Reason       RejectedSurfaceReason `json:"reason"`
}

func (result Result) SelectedFamilyCount() int {
	total := 0
	for _, entry := range result.Entries {
		total += len(entry.Families)
	}
	return total
}

func (result Result) RejectedFamilyCount() int {
	total := 0
	for _, entry := range result.Entries {
		total += len(entry.RejectedFamilies)
	}
	return total
}

func (result Result) SelectedSurfaceCount() int {
	return len(result.SurfaceProposals)
}

func (result Result) RejectedSurfaceCount() int {
	return len(result.RejectedSurfaceProposals)
}

// Reduce strictly validates refs and rooted connectivity, ignores response
// order, and restores exact local callsites from private compilation authority.
func Reduce(compilation Compilation, raw []byte) (Result, error) {
	if err := validateCompilation(compilation); err != nil {
		return Result{}, err
	}
	response, err := decodeResponse(raw)
	if err != nil {
		return Result{}, err
	}
	if response.Version != ResultVersion || response.RequestRef != compilation.Request.RequestRef ||
		len(response.Entries) != len(compilation.Request.Entries) || response.SurfaceProposals == nil ||
		len(response.SurfaceProposals) > MaxSelectedSurfaceProposals {
		return Result{}, fmt.Errorf("entry call: response identity mismatch")
	}
	responseByRoot := make(map[string]ResponseEntry, len(response.Entries))
	for _, entry := range response.Entries {
		if entry.FamilyRefs == nil {
			return Result{}, fmt.Errorf("entry call: response family_refs must be an array")
		}
		if _, duplicate := responseByRoot[entry.RootRef]; duplicate {
			return Result{}, fmt.Errorf("entry call: duplicate response root ref")
		}
		if _, known := compilation.authority[entry.RootRef]; !known {
			return Result{}, fmt.Errorf("entry call: response cites unknown root ref")
		}
		responseByRoot[entry.RootRef] = entry
	}
	result := Result{
		Version: ResultVersion, PromptVersion: PromptVersion,
		RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.wireSHA,
		SubstrateSHA256: compilation.SubstrateSHA256, Entries: []ResultEntry{},
		SurfaceProposals:         []ResultSurfaceProposal{},
		RejectedSurfaceProposals: []RejectedSurfaceProposal{},
		SurfaceCandidateCoverage: compilation.surfaceCoverage,
	}
	for _, requestEntry := range compilation.Request.Entries {
		responseEntry, present := responseByRoot[requestEntry.Ref]
		if !present {
			return Result{}, fmt.Errorf("entry call: response omitted root ref")
		}
		selected, rejected, err := validateSelectedFamilies(requestEntry, responseEntry)
		if err != nil {
			return Result{}, err
		}
		authority := compilation.authority[requestEntry.Ref]
		resultEntry := ResultEntry{
			RootRef: requestEntry.Ref, Label: requestEntry.Label,
			Declaration: authority.rootNode.Declaration,
			Families:    []ResultFamily{}, RejectedFamilies: append([]RejectedResultFamily{}, rejected...),
			Frontier: append([]RequestFrontier{}, requestEntry.Frontier...),
			Omitted:  requestEntry.Omitted,
		}
		nodeByRef := make(map[string]RequestNode, len(requestEntry.Nodes))
		for _, node := range requestEntry.Nodes {
			nodeByRef[node.Ref] = node
		}
		for _, familyRef := range selected {
			requestFamily := requestFamilyByRef(requestEntry, familyRef)
			exact := authority.familyByRef[familyRef]
			callsites := append([]Location(nil), exact.Callsites...)
			sortLocations(callsites)
			resultEntry.Families = append(resultEntry.Families, ResultFamily{
				Ref:         familyRef,
				CallerLabel: nodeByRef[requestFamily.CallerRef].Label,
				CalleeLabel: nodeByRef[requestFamily.CalleeRef].Label,
				Invocation:  requestFamily.Invocation, WitnessCount: requestFamily.WitnessCount,
				Callsites: callsites,
			})
		}
		result.Entries = append(result.Entries, resultEntry)
	}
	selectedSurfaces, rejectedSurfaces, err := reduceSurfaceProposals(compilation, response.SurfaceProposals)
	if err != nil {
		return Result{}, err
	}
	result.SurfaceProposals = selectedSurfaces
	result.RejectedSurfaceProposals = rejectedSurfaces
	return result, nil
}

func reduceSurfaceProposals(
	compilation Compilation,
	proposals []ResponseSurfaceProposal,
) ([]ResultSurfaceProposal, []RejectedSurfaceProposal, error) {
	knownKinds := map[string]struct{}{SurfaceKindRefCLICommand: {}, SurfaceKindRefHTTPRoute: {}}
	knownSlots := map[string]struct{}{
		SurfaceSlotRefIdentity: {}, SurfaceSlotRefMethod: {}, SurfaceSlotRefPath: {}, SurfaceSlotRefHandler: {},
	}
	knownFacts := make(map[string]struct{})
	for _, candidate := range compilation.Request.SurfaceCatalog.Candidates {
		for _, fact := range candidate.Facts {
			knownFacts[fact.Ref] = struct{}{}
		}
	}
	proposalCountByCandidate := make(map[string]int, len(proposals))
	proposalByCandidate := make(map[string]ResponseSurfaceProposal, len(proposals))
	for _, proposal := range proposals {
		if _, known := compilation.surfaceAuthority[proposal.CandidateRef]; !known {
			return nil, nil, fmt.Errorf("entry call: response cites unknown surface candidate ref")
		}
		if _, known := knownKinds[proposal.KindRef]; !known {
			return nil, nil, fmt.Errorf("entry call: response cites unknown surface kind ref")
		}
		for _, binding := range proposal.Bindings {
			if _, known := knownSlots[binding.SlotRef]; !known {
				return nil, nil, fmt.Errorf("entry call: response cites unknown surface slot ref")
			}
			if _, known := knownFacts[binding.FactRef]; !known {
				return nil, nil, fmt.Errorf("entry call: response cites unknown surface fact ref")
			}
		}
		proposalCountByCandidate[proposal.CandidateRef]++
		proposalByCandidate[proposal.CandidateRef] = proposal
	}

	orderedCandidateRefs := make([]string, 0, len(proposalByCandidate))
	for candidateRef := range proposalByCandidate {
		orderedCandidateRefs = append(orderedCandidateRefs, candidateRef)
	}
	sort.Slice(orderedCandidateRefs, func(i, j int) bool {
		return refLess(orderedCandidateRefs[i], orderedCandidateRefs[j])
	})
	selected := make([]ResultSurfaceProposal, 0, len(orderedCandidateRefs))
	rejected := make([]RejectedSurfaceProposal, 0)
	for _, candidateRef := range orderedCandidateRefs {
		if proposalCountByCandidate[candidateRef] > 1 {
			rejected = append(rejected, RejectedSurfaceProposal{
				CandidateRef: candidateRef,
				Reason:       RejectedSurfaceDuplicateProposal,
			})
			continue
		}
		proposal := proposalByCandidate[candidateRef]
		restored, reason := restoreSurfaceProposal(compilation.surfaceAuthority[proposal.CandidateRef], proposal)
		if reason != "" {
			rejected = append(rejected, RejectedSurfaceProposal{CandidateRef: proposal.CandidateRef, Reason: reason})
			continue
		}
		selected = append(selected, restored)
	}
	return selected, rejected, nil
}

func restoreSurfaceProposal(
	authority surfaceCandidateAuthority,
	proposal ResponseSurfaceProposal,
) (ResultSurfaceProposal, RejectedSurfaceReason) {
	if proposal.Bindings == nil || len(proposal.Bindings) == 0 {
		return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
	}
	if len(proposal.Bindings) > 4 {
		return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
	}
	if proposal.KindRef == SurfaceKindRefCLICommand && authority.exact.Form != SurfaceCandidateKeyedComposite ||
		proposal.KindRef == SurfaceKindRefHTTPRoute && authority.exact.Form != SurfaceCandidateDirectCall {
		return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleForm
	}
	bySlot := make(map[string]ExactSurfaceFact, len(proposal.Bindings))
	usedFacts := make(map[string]struct{}, len(proposal.Bindings))
	for _, binding := range proposal.Bindings {
		if _, duplicate := bySlot[binding.SlotRef]; duplicate {
			return ResultSurfaceProposal{}, RejectedSurfaceDuplicateBinding
		}
		if _, duplicate := usedFacts[binding.FactRef]; duplicate {
			return ResultSurfaceProposal{}, RejectedSurfaceDuplicateBinding
		}
		exact, owned := authority.factByRef[binding.FactRef]
		if !owned {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		bySlot[binding.SlotRef] = exact
		usedFacts[binding.FactRef] = struct{}{}
	}

	restored := ResultSurfaceProposal{
		CandidateRef: proposal.CandidateRef,
		RootRef:      authority.request.RootRef,
		Form:         authority.exact.Form,
		Site:         authority.exact.Site,
	}
	switch proposal.KindRef {
	case SurfaceKindRefCLICommand:
		if len(bySlot) < 1 || len(bySlot) > 2 {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		identity, present := bySlot[SurfaceSlotRefIdentity]
		if !present {
			return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
		}
		if identity.Kind != SurfaceFactString {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if handler, present := bySlot[SurfaceSlotRefHandler]; present {
			if handler.Kind != SurfaceFactCallable {
				return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
			}
			restored.Handler = resultSurfaceValue(handler)
		} else if !handlerlessCLIHasDescriptorEvidence(authority.exact, identity) {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if _, extra := bySlot[SurfaceSlotRefMethod]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if _, extra := bySlot[SurfaceSlotRefPath]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		restored.Kind = SurfaceKindCLICommand
		restored.Role = SurfaceRoleDescriptor
		restored.Identity = resultSurfaceValue(identity)
	case SurfaceKindRefHTTPRoute:
		if len(bySlot) != 3 {
			return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
		}
		method, methodPresent := bySlot[SurfaceSlotRefMethod]
		path, pathPresent := bySlot[SurfaceSlotRefPath]
		handler, handlerPresent := bySlot[SurfaceSlotRefHandler]
		if !methodPresent || !pathPresent || !handlerPresent {
			return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
		}
		if method.Kind != SurfaceFactString && method.Kind != SurfaceFactToken ||
			path.Kind != SurfaceFactString || handler.Kind != SurfaceFactCallable {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if method.Kind == SurfaceFactToken && !standardHTTPTokenMethod(method.Value) {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if _, extra := bySlot[SurfaceSlotRefIdentity]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		restored.Kind = SurfaceKindHTTPRoute
		restored.Role = SurfaceRoleEntrySurface
		restored.Method = resultSurfaceValue(method)
		restored.Path = resultSurfaceValue(path)
		restored.Handler = resultSurfaceValue(handler)
	default:
		panic("known surface kind was not handled")
	}
	restored.ID = surfaceProposalID(authority.exact.ID, restored.Kind)
	return restored, ""
}

func standardHTTPTokenMethod(value string) bool {
	switch strings.ToUpper(value) {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE":
		return true
	default:
		return false
	}
}

func handlerlessCLIHasDescriptorEvidence(candidate ExactSurfaceCandidate, identity ExactSurfaceFact) bool {
	hasCallable := false
	hasCompanionString := false
	for _, fact := range candidate.Facts {
		switch fact.Kind {
		case SurfaceFactCallable:
			hasCallable = true
		case SurfaceFactString:
			if fact.ID != identity.ID {
				hasCompanionString = true
			}
		}
	}
	return !hasCallable || hasCompanionString
}

func resultSurfaceValue(fact ExactSurfaceFact) *ResultSurfaceValue {
	location := fact.Location
	return &ResultSurfaceValue{Kind: fact.Kind, Text: fact.Value, Location: &location}
}

func decodeResponse(raw []byte) (Response, error) {
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Response{}, fmt.Errorf("entry call: response exceeds bounded envelope")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("entry call: decode response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Response{}, err
	}
	return response, nil
}

func validateSelectedFamilies(request RequestEntry, response ResponseEntry) ([]string, []RejectedResultFamily, error) {
	if len(response.FamilyRefs) > MaxFamiliesPerRoot {
		return nil, nil, fmt.Errorf("entry call: response selection exceeds per-root resource bound")
	}
	familyByRef := make(map[string]RequestFamily, len(request.Families))
	for _, family := range request.Families {
		familyByRef[family.Ref] = family
	}
	remaining := make(map[string]RequestFamily, len(response.FamilyRefs))
	for _, ref := range response.FamilyRefs {
		family, known := familyByRef[ref]
		if !known {
			return nil, nil, fmt.Errorf("entry call: response cites unknown family ref")
		}
		if _, duplicate := remaining[ref]; duplicate {
			return nil, nil, fmt.Errorf("entry call: response repeats family ref")
		}
		remaining[ref] = family
	}
	reachable := map[string]bool{request.RootNodeRef: true}
	ordered := make([]string, 0, len(remaining))
	rejected := make([]RejectedResultFamily, 0)
	for len(remaining) > 0 {
		progress := false
		for _, family := range request.Families {
			selected, pending := remaining[family.Ref]
			if !pending || !reachable[selected.CallerRef] {
				continue
			}
			ordered = append(ordered, family.Ref)
			reachable[selected.CalleeRef] = true
			delete(remaining, family.Ref)
			progress = true
		}
		if !progress {
			for _, family := range request.Families {
				if _, pending := remaining[family.Ref]; !pending {
					continue
				}
				rejected = append(rejected, RejectedResultFamily{
					Ref: family.Ref, Reason: RejectedFamilyUnreachable,
				})
			}
			break
		}
	}
	return ordered, rejected, nil
}

func requestFamilyByRef(entry RequestEntry, ref string) RequestFamily {
	for _, family := range entry.Families {
		if family.Ref == ref {
			return family
		}
	}
	return RequestFamily{}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("entry call: response contains trailing JSON")
		}
		return fmt.Errorf("entry call: decode response tail: %w", err)
	}
	return nil
}

func validDigest(value string, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func canonicalResultFamilyLess(left, right ResultFamily) bool {
	if left.Ref != right.Ref {
		return refLess(left.Ref, right.Ref)
	}
	return left.CallerLabel+"\x00"+left.CalleeLabel < right.CallerLabel+"\x00"+right.CalleeLabel
}

func sortResult(result *Result) {
	sort.Slice(result.Entries, func(i, j int) bool { return refLess(result.Entries[i].RootRef, result.Entries[j].RootRef) })
	for index := range result.Entries {
		sort.Slice(result.Entries[index].Families, func(i, j int) bool {
			return canonicalResultFamilyLess(result.Entries[index].Families[i], result.Entries[index].Families[j])
		})
		sort.Slice(result.Entries[index].RejectedFamilies, func(i, j int) bool {
			return refLess(result.Entries[index].RejectedFamilies[i].Ref, result.Entries[index].RejectedFamilies[j].Ref)
		})
	}
	sort.Slice(result.SurfaceProposals, func(i, j int) bool {
		return refLess(result.SurfaceProposals[i].CandidateRef, result.SurfaceProposals[j].CandidateRef)
	})
	sort.Slice(result.RejectedSurfaceProposals, func(i, j int) bool {
		return refLess(result.RejectedSurfaceProposals[i].CandidateRef, result.RejectedSurfaceProposals[j].CandidateRef)
	})
}

func refLess(left, right string) bool {
	if len(left) < 2 || len(right) < 2 {
		return left < right
	}
	leftNumber, leftErr := strconv.Atoi(left[1:])
	rightNumber, rightErr := strconv.Atoi(right[1:])
	if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return left < right
}
