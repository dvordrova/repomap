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

const (
	MaxResponseBindings          = 4
	MaxCLIResponseBindings       = 2
	MaxHTTPResponseBindings      = 3
	MaxScheduledResponseBindings = 2
)

type Response struct {
	Version          int                       `json:"version"`
	SurfaceProposals []ResponseSurfaceProposal `json:"surface_proposals"`
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
	SurfaceProposals         []ResultSurfaceProposal   `json:"surface_proposals"`
	RejectedSurfaceProposals []RejectedSurfaceProposal `json:"rejected_surface_proposals"`
	SurfaceCandidateCoverage SurfaceCandidateCoverage  `json:"surface_candidate_coverage"`
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

// Reduce strictly validates request-local refs and restores exact local facts
// from private compilation authority.
func Reduce(compilation Compilation, raw []byte) (Result, error) {
	if err := validateCompilation(compilation); err != nil {
		return Result{}, err
	}
	response, err := decodeResponse(raw)
	if err != nil {
		return Result{}, err
	}
	if response.Version != ResponseVersion || response.SurfaceProposals == nil ||
		len(response.SurfaceProposals) > MaxSelectedSurfaceProposals {
		return Result{}, fmt.Errorf("entry call: response identity mismatch")
	}
	result := Result{
		SurfaceProposals:         []ResultSurfaceProposal{},
		RejectedSurfaceProposals: []RejectedSurfaceProposal{},
		SurfaceCandidateCoverage: compilation.surfaceCoverage,
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
	knownKinds := map[string]struct{}{
		SurfaceKindRefCLICommand: {}, SurfaceKindRefHTTPRoute: {}, SurfaceKindRefScheduledJob: {},
	}
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
	if len(proposal.Bindings) > MaxResponseBindings {
		return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
	}
	if proposal.KindRef == SurfaceKindRefCLICommand && authority.exact.Form != SurfaceCandidateKeyedComposite ||
		proposal.KindRef == SurfaceKindRefHTTPRoute && authority.exact.Form != SurfaceCandidateDirectCall ||
		proposal.KindRef == SurfaceKindRefScheduledJob && authority.exact.Form != SurfaceCandidateDirectCall {
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
		if len(bySlot) < 1 || len(bySlot) > MaxCLIResponseBindings {
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
		} else if !handlerlessSurfaceHasDescriptorEvidence(authority.exact, identity) {
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
		if len(bySlot) < 1 || len(bySlot) > MaxHTTPResponseBindings {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		method, methodPresent := bySlot[SurfaceSlotRefMethod]
		path, pathPresent := bySlot[SurfaceSlotRefPath]
		handler, handlerPresent := bySlot[SurfaceSlotRefHandler]
		if !pathPresent {
			return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
		}
		if path.Kind != SurfaceFactString || !strings.HasPrefix(path.Value, "/") {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if methodPresent && !validHTTPMethodFact(method) {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if handlerPresent && handler.Kind != SurfaceFactCallable {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if _, extra := bySlot[SurfaceSlotRefIdentity]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		restored.Kind = SurfaceKindHTTPRoute
		restored.Path = resultSurfaceValue(path)
		restored.Role = SurfaceRoleDescriptor
		if methodPresent {
			restored.Method = resultSurfaceValue(method)
		}
		if !handlerPresent {
			// Once the provider has classified this exact direct call as an HTTP
			// route, a sole repository-local callable is backend-owned,
			// unambiguous handler evidence. Do not require the provider to repeat
			// a choice the local candidate has already made uniquely. Zero or
			// multiple callables remain detached descriptors.
			if callable, unique := soleSurfaceCallable(authority.exact); unique {
				handler = callable
				handlerPresent = true
			}
		}
		if handlerPresent {
			restored.Handler = resultSurfaceValue(handler)
			restored.Role = SurfaceRoleEntrySurface
		}
	case SurfaceKindRefScheduledJob:
		if len(bySlot) > MaxScheduledResponseBindings {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		identity, identityPresent := bySlot[SurfaceSlotRefIdentity]
		handler, handlerPresent := bySlot[SurfaceSlotRefHandler]
		if !identityPresent {
			return ResultSurfaceProposal{}, RejectedSurfaceMissingBinding
		}
		if identity.Kind != SurfaceFactString {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if handlerPresent {
			if handler.Kind != SurfaceFactCallable {
				return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
			}
			restored.Handler = resultSurfaceValue(handler)
			restored.Role = SurfaceRoleEntrySurface
		} else {
			if !handlerlessSurfaceHasDescriptorEvidence(authority.exact, identity) {
				return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
			}
			restored.Role = SurfaceRoleDescriptor
		}
		if _, extra := bySlot[SurfaceSlotRefMethod]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		if _, extra := bySlot[SurfaceSlotRefPath]; extra {
			return ResultSurfaceProposal{}, RejectedSurfaceIncompatibleBinding
		}
		restored.Kind = SurfaceKindScheduledJob
		restored.Identity = resultSurfaceValue(identity)
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

func validHTTPMethodFact(fact ExactSurfaceFact) bool {
	switch fact.Kind {
	case SurfaceFactToken:
		return standardHTTPTokenMethod(fact.Value)
	case SurfaceFactString:
		return validHTTPMethodText(fact.Value)
	default:
		return false
	}
}

func validHTTPMethodText(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func handlerlessSurfaceHasDescriptorEvidence(candidate ExactSurfaceCandidate, identity ExactSurfaceFact) bool {
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

func soleSurfaceCallable(candidate ExactSurfaceCandidate) (ExactSurfaceFact, bool) {
	var found ExactSurfaceFact
	seen := false
	for _, fact := range candidate.Facts {
		if fact.Kind != SurfaceFactCallable {
			continue
		}
		if seen {
			return ExactSurfaceFact{}, false
		}
		found = fact
		seen = true
	}
	return found, seen
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
