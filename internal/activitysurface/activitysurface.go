// Package activitysurface classifies bounded exact syntax candidates as
// user-facing HTTP routes, CLI commands, or scheduled jobs. The model sees
// only request-local refs; exact identities and locations are restored locally.
package activitysurface

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	Version               = 1
	RequestVersion        = 1
	maxOutputTokens       = 32_000
	executionContract     = "repomap.activity-surfaces.v1"
	preparationVersion    = 2
	responseSchemaVersion = 1
)

//go:embed prompt.md
var promptText string

// Request is the complete provider-visible input. Exact candidate IDs, root
// node IDs, and repository locations remain in the private entrycall
// compilation authority.
type Request struct {
	Version int                             `json:"version"`
	Catalog entrycall.RequestSurfaceCatalog `json:"surface_catalog"`
}

// Response is refs-only. Values, paths, symbols, and explanations are never
// accepted from the model.
type Response struct {
	SurfaceProposals []entrycall.ResponseSurfaceProposal `json:"surface_proposals"`
}

// Value is an exact candidate fact restored from local authority.
type Value struct {
	Kind     entrycall.SurfaceFactKind `json:"kind"`
	Text     string                    `json:"text"`
	Location entrycall.Location        `json:"location"`
}

// Surface is one locally restored activity surface. No request-local refs are
// retained in the result.
type Surface struct {
	ID           string                         `json:"id"`
	RootNodeID   string                         `json:"root_node_id"`
	Kind         string                         `json:"kind"`
	Role         string                         `json:"role"`
	Form         entrycall.SurfaceCandidateForm `json:"form"`
	Registration entrycall.Location             `json:"registration"`
	Identity     *Value                         `json:"identity,omitempty"`
	Method       *Value                         `json:"method,omitempty"`
	Path         *Value                         `json:"path,omitempty"`
	Handler      *Value                         `json:"handler,omitempty"`
}

// RejectionCount retains item-local semantic rejection accounting without
// persisting the request-local candidate ref.
type RejectionCount struct {
	Reason entrycall.RejectedSurfaceReason `json:"reason"`
	Count  int                             `json:"count"`
}

type Coverage struct {
	Candidates  entrycall.SurfaceCandidateCoverage `json:"candidates"`
	Selected    int                                `json:"selected"`
	Rejected    int                                `json:"rejected"`
	ModelCalled bool                               `json:"model_called"`
}

// Result is the validated local output of the activity-surface cube.
type Result struct {
	Version         int                    `json:"version"`
	State           entrycall.State        `json:"state"`
	ClosedReason    entrycall.ClosedReason `json:"closed_reason,omitempty"`
	SubstrateSHA256 string                 `json:"substrate_sha256"`
	Surfaces        []Surface              `json:"surfaces"`
	Rejections      []RejectionCount       `json:"rejections"`
	Coverage        Coverage               `json:"coverage"`
}

type compilation struct {
	exact         entrycall.Compilation
	request       Request
	requestWire   []byte
	state         []byte
	rootNodeByRef map[string]string
}

// Run compiles one bounded refs-only request, executes the shared provider
// layer, and restores every accepted fact from exact local authority. A ready
// substrate with no advertised candidates returns without a provider call.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	substrate entrycall.Substrate,
) (Result, error) {
	snapshot := substrate.Snapshot()
	if snapshot.State == entrycall.StateUnavailable {
		result, err := unavailableResult(snapshot)
		if err != nil {
			return Result{}, err
		}
		if err := result.ValidateAgainst(snapshot); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	compiled, err := compile(snapshot)
	if err != nil {
		return Result{}, err
	}
	if len(compiled.request.Catalog.Candidates) == 0 {
		result := emptyResult(compiled)
		if err := result.ValidateAgainst(snapshot); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[Result]{
		State: compiled.state,
		Prompt: llm.Prompt{
			System: strings.TrimSpace(promptText), User: string(compiled.requestWire),
			ResponseFormatJSON: true,
		},
		Limits: llm.Limits{
			MaxRequestBytes:  entrycall.MaxProviderRequestBytes,
			MaxResponseBytes: entrycall.MaxResponseBytes,
			MaxOutputTokens:  maxOutputTokens,
		},
		DecodeValidate: func(raw []byte) (Result, error) {
			return reduce(compiled, raw)
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("activity surfaces: model cube: %w", err)
	}
	if err := outcome.Value.ValidateAgainst(snapshot); err != nil {
		return Result{}, err
	}
	return outcome.Value, nil
}

func compile(substrate entrycall.Substrate) (compilation, error) {
	exact, err := entrycall.Compile(substrate)
	if err != nil {
		return compilation{}, fmt.Errorf("activity surfaces: compile exact candidates: %w", err)
	}
	if err := requireCompleteCandidateCatalog(exact.SurfaceCoverage()); err != nil {
		return compilation{}, err
	}
	request := Request{Version: RequestVersion, Catalog: exact.Request.SurfaceCatalog}
	requestWire, err := json.Marshal(request)
	if err != nil {
		return compilation{}, fmt.Errorf("activity surfaces: encode request: %w", err)
	}
	if len(requestWire) > entrycall.MaxSurfaceCandidateSectionBytes+1024 {
		return compilation{}, fmt.Errorf("activity surfaces: request exceeds bounded envelope")
	}
	rootNodeByRef, err := compileRootAuthority(exact)
	if err != nil {
		return compilation{}, err
	}
	state, err := compileState(exact.SubstrateSHA256, requestWire)
	if err != nil {
		return compilation{}, err
	}
	return compilation{
		exact: exact, request: request, requestWire: requestWire,
		state: state, rootNodeByRef: rootNodeByRef,
	}, nil
}

func compileRootAuthority(exact entrycall.Compilation) (map[string]string, error) {
	result := make(map[string]string)
	for _, candidate := range exact.Request.SurfaceCatalog.Candidates {
		nodeID, known := exact.RootNodeID(candidate.RootRef)
		if !known || nodeID == "" {
			return nil, fmt.Errorf("activity surfaces: invalid exact root authority")
		}
		if existing := result[candidate.RootRef]; existing != "" && existing != nodeID {
			return nil, fmt.Errorf("activity surfaces: conflicting exact root authority")
		}
		result[candidate.RootRef] = nodeID
	}
	return result, nil
}

func compileState(substrateSHA256 string, requestWire []byte) ([]byte, error) {
	state, err := json.Marshal(struct {
		Contract       string `json:"contract"`
		Preparation    int    `json:"preparation"`
		ResponseSchema int    `json:"response_schema"`
		PromptSHA256   string `json:"prompt_sha256"`
		RequestSHA256  string `json:"request_sha256"`
		SubstrateSHA   string `json:"substrate_sha256"`
	}{
		Contract: executionContract, Preparation: preparationVersion,
		ResponseSchema: responseSchemaVersion,
		PromptSHA256:   sha256Hex([]byte(strings.TrimSpace(promptText))),
		RequestSHA256:  sha256Hex(requestWire), SubstrateSHA: substrateSHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("activity surfaces: encode cube state: %w", err)
	}
	return state, nil
}

func reduce(compiled compilation, raw []byte) (Result, error) {
	response, err := decodeResponse(raw)
	if err != nil {
		return Result{}, err
	}
	entryResponse := entrycall.Response{
		Version:          entrycall.ResponseVersion,
		SurfaceProposals: append([]entrycall.ResponseSurfaceProposal(nil), response.SurfaceProposals...),
	}
	wire, err := json.Marshal(entryResponse)
	if err != nil {
		return Result{}, fmt.Errorf("activity surfaces: encode exact reduction: %w", err)
	}
	restored, err := entrycall.Reduce(compiled.exact, wire)
	if err != nil {
		return Result{}, fmt.Errorf("activity surfaces: restore exact response: %w", err)
	}
	if len(restored.RejectedSurfaceProposals) > 0 {
		return Result{}, fmt.Errorf(
			"activity surfaces: model response contains %d invalid surface proposal(s): %s",
			len(restored.RejectedSurfaceProposals),
			restored.RejectedSurfaceProposals[0].Reason,
		)
	}
	result, err := restoreResult(compiled, restored)
	if err != nil {
		return Result{}, err
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func decodeResponse(raw []byte) (Response, error) {
	if len(raw) == 0 || len(raw) > entrycall.MaxResponseBytes {
		return Response{}, fmt.Errorf("activity surfaces: response exceeds bounded envelope")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("activity surfaces: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Response{}, fmt.Errorf("activity surfaces: response contains multiple JSON values")
		}
		return Response{}, fmt.Errorf("activity surfaces: decode response tail: %w", err)
	}
	if response.SurfaceProposals == nil || len(response.SurfaceProposals) > entrycall.MaxSelectedSurfaceProposals {
		return Response{}, fmt.Errorf("activity surfaces: invalid proposal envelope")
	}
	return response, nil
}

func restoreResult(compiled compilation, restored entrycall.Result) (Result, error) {
	result := Result{
		Version: Version, State: entrycall.StateReady,
		SubstrateSHA256: compiled.exact.SubstrateSHA256,
		Surfaces:        []Surface{}, Rejections: []RejectionCount{},
		Coverage: Coverage{
			Candidates: restored.SurfaceCandidateCoverage,
			Selected:   len(restored.SurfaceProposals), Rejected: len(restored.RejectedSurfaceProposals),
			ModelCalled: true,
		},
	}
	for _, proposal := range restored.SurfaceProposals {
		rootNodeID := compiled.rootNodeByRef[proposal.RootRef]
		if rootNodeID == "" {
			return Result{}, fmt.Errorf("activity surfaces: restored proposal cites unknown exact root")
		}
		surface, err := restoreSurface(rootNodeID, proposal)
		if err != nil {
			return Result{}, err
		}
		result.Surfaces = append(result.Surfaces, surface)
	}
	rejections := make(map[entrycall.RejectedSurfaceReason]int)
	for _, rejected := range restored.RejectedSurfaceProposals {
		rejections[rejected.Reason]++
	}
	for reason, count := range rejections {
		result.Rejections = append(result.Rejections, RejectionCount{Reason: reason, Count: count})
	}
	sortResult(&result)
	return result, nil
}

func restoreSurface(rootNodeID string, proposal entrycall.ResultSurfaceProposal) (Surface, error) {
	result := Surface{
		ID: proposal.ID, RootNodeID: rootNodeID, Kind: proposal.Kind, Role: proposal.Role,
		Form: proposal.Form, Registration: proposal.Site,
	}
	var err error
	if result.Identity, err = restoreValue(proposal.Identity); err != nil {
		return Surface{}, err
	}
	if result.Method, err = restoreValue(proposal.Method); err != nil {
		return Surface{}, err
	}
	if result.Path, err = restoreValue(proposal.Path); err != nil {
		return Surface{}, err
	}
	if result.Handler, err = restoreValue(proposal.Handler); err != nil {
		return Surface{}, err
	}
	return result, nil
}

func restoreValue(value *entrycall.ResultSurfaceValue) (*Value, error) {
	if value == nil {
		return nil, nil
	}
	if value.Location == nil {
		return nil, fmt.Errorf("activity surfaces: restored value has no exact location")
	}
	return &Value{Kind: value.Kind, Text: value.Text, Location: *value.Location}, nil
}

func emptyResult(compiled compilation) Result {
	return Result{
		Version: Version, State: entrycall.StateReady,
		SubstrateSHA256: compiled.exact.SubstrateSHA256,
		Surfaces:        []Surface{}, Rejections: []RejectionCount{},
		Coverage: Coverage{Candidates: compiled.exact.SurfaceCoverage()},
	}
}

func unavailableResult(substrate entrycall.Substrate) (Result, error) {
	if err := validateUnavailableSubstrate(substrate); err != nil {
		return Result{}, err
	}
	return Result{
		Version: Version, State: entrycall.StateUnavailable,
		ClosedReason:    substrate.ClosedReason,
		SubstrateSHA256: unavailableSubstrateSHA256(substrate),
		Surfaces:        []Surface{}, Rejections: []RejectionCount{},
	}, nil
}

// Validate checks the standalone closed result shape and accounting.
func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.SubstrateSHA256) ||
		result.Surfaces == nil || result.Rejections == nil {
		return fmt.Errorf("activity surfaces: invalid result identity")
	}
	switch result.State {
	case entrycall.StateReady:
		if result.ClosedReason != "" {
			return fmt.Errorf("activity surfaces: ready result has a closed reason")
		}
	case entrycall.StateUnavailable:
		if !validClosedReason(result.ClosedReason) || len(result.Surfaces) != 0 ||
			len(result.Rejections) != 0 || result.Coverage != (Coverage{}) {
			return fmt.Errorf("activity surfaces: invalid unavailable result")
		}
	default:
		return fmt.Errorf("activity surfaces: invalid result state")
	}
	if err := validateCoverage(result.Coverage); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(result.Surfaces))
	for index, surface := range result.Surfaces {
		if err := validateSurface(surface); err != nil {
			return fmt.Errorf("activity surfaces: surface %d: %w", index, err)
		}
		if _, duplicate := seen[surface.ID]; duplicate {
			return fmt.Errorf("activity surfaces: duplicate surface %q", surface.ID)
		}
		seen[surface.ID] = struct{}{}
		if index > 0 && !surfaceLess(result.Surfaces[index-1], surface) {
			return fmt.Errorf("activity surfaces: surfaces are not canonical")
		}
	}
	rejected := 0
	for index, rejection := range result.Rejections {
		if !validRejectionReason(rejection.Reason) || rejection.Count <= 0 {
			return fmt.Errorf("activity surfaces: invalid rejection accounting")
		}
		if index > 0 && result.Rejections[index-1].Reason >= rejection.Reason {
			return fmt.Errorf("activity surfaces: rejections are not canonical")
		}
		rejected += rejection.Count
	}
	if result.Coverage.Selected != len(result.Surfaces) || result.Coverage.Rejected != rejected {
		return fmt.Errorf("activity surfaces: result accounting mismatch")
	}
	return nil
}

// ValidateAgainst binds a result back to the exact substrate named by its
// digest and proves that every restored location and value belongs to one
// exact candidate. It performs no provider call.
func (result Result) ValidateAgainst(substrate entrycall.Substrate) error {
	if err := result.Validate(); err != nil {
		return err
	}
	snapshot := substrate.Snapshot()
	if result.State == entrycall.StateUnavailable || snapshot.State == entrycall.StateUnavailable {
		if result.State != entrycall.StateUnavailable || snapshot.State != entrycall.StateUnavailable {
			return fmt.Errorf("activity surfaces: exact substrate state mismatch")
		}
		if err := validateUnavailableSubstrate(snapshot); err != nil {
			return err
		}
		if result.ClosedReason != snapshot.ClosedReason ||
			result.SubstrateSHA256 != unavailableSubstrateSHA256(snapshot) {
			return fmt.Errorf("activity surfaces: unavailable substrate binding mismatch")
		}
		return nil
	}
	compiled, err := compile(snapshot)
	if err != nil {
		return fmt.Errorf("activity surfaces: validate exact substrate: %w", err)
	}
	if result.SubstrateSHA256 != compiled.exact.SubstrateSHA256 ||
		result.Coverage.Candidates != compiled.exact.SurfaceCoverage() {
		return fmt.Errorf("activity surfaces: exact substrate binding mismatch")
	}
	for _, surface := range result.Surfaces {
		restored, restoreErr := compiled.restores(surface)
		if restoreErr != nil {
			return restoreErr
		}
		if !restored {
			return fmt.Errorf("activity surfaces: surface %q is not uniquely restored from exact authority", surface.ID)
		}
	}
	return nil
}

func validateUnavailableSubstrate(substrate entrycall.Substrate) error {
	if substrate.Version != entrycall.SubstrateVersion || substrate.State != entrycall.StateUnavailable ||
		!validClosedReason(substrate.ClosedReason) || len(substrate.Roots) != 0 ||
		len(substrate.SurfaceCandidates) != 0 || substrate.Coverage != (entrycall.Coverage{}) {
		return fmt.Errorf("activity surfaces: invalid unavailable substrate")
	}
	return nil
}

func validClosedReason(reason entrycall.ClosedReason) bool {
	switch reason {
	case entrycall.ClosedSSAUnavailable, entrycall.ClosedIndexLimit, entrycall.ClosedNoEntrypoints:
		return true
	default:
		return false
	}
}

func unavailableSubstrateSHA256(substrate entrycall.Substrate) string {
	wire, _ := json.Marshal(struct {
		Version      int                    `json:"version"`
		State        entrycall.State        `json:"state"`
		ClosedReason entrycall.ClosedReason `json:"closed_reason"`
	}{
		Version: substrate.Version, State: substrate.State, ClosedReason: substrate.ClosedReason,
	})
	return sha256Hex(wire)
}

// restores proves that surface could have been produced only by selecting
// refs from this compilation's advertised catalog. It deliberately reuses the
// entrycall reducer instead of reimplementing its private candidate authority.
func (compiled compilation) restores(surface Surface) (bool, error) {
	kindRef, err := kindRef(surface.Kind)
	if err != nil {
		return false, err
	}
	matches := 0
	for _, candidate := range compiled.request.Catalog.Candidates {
		if candidate.Form != surface.Form || compiled.rootNodeByRef[candidate.RootRef] != surface.RootNodeID {
			continue
		}
		bindingSets := candidateBindingSets(candidate, surface)
		matchedCandidate := false
		for _, bindings := range bindingSets {
			restored, accepted, reduceErr := compiled.reduceOne(candidate.Ref, kindRef, bindings)
			if reduceErr != nil {
				return false, reduceErr
			}
			if accepted && surfacesEqual(restored, surface) {
				matchedCandidate = true
				break
			}
		}
		if matchedCandidate {
			matches++
		}
	}
	return matches == 1, nil
}

func (compiled compilation) reduceOne(
	candidateRef, kindRef string,
	bindings []entrycall.ResponseSurfaceBinding,
) (Surface, bool, error) {
	response := entrycall.Response{
		Version: entrycall.ResponseVersion,
		SurfaceProposals: []entrycall.ResponseSurfaceProposal{{
			CandidateRef: candidateRef, KindRef: kindRef,
			Bindings: append([]entrycall.ResponseSurfaceBinding(nil), bindings...),
		}},
	}
	wire, err := json.Marshal(response)
	if err != nil {
		return Surface{}, false, fmt.Errorf("activity surfaces: encode authority replay: %w", err)
	}
	reduced, err := entrycall.Reduce(compiled.exact, wire)
	if err != nil {
		return Surface{}, false, fmt.Errorf("activity surfaces: exact authority replay: %w", err)
	}
	if len(reduced.SurfaceProposals) == 0 {
		return Surface{}, false, nil
	}
	if len(reduced.SurfaceProposals) != 1 || len(reduced.RejectedSurfaceProposals) != 0 {
		return Surface{}, false, fmt.Errorf("activity surfaces: inconsistent exact authority replay")
	}
	proposal := reduced.SurfaceProposals[0]
	rootNodeID := compiled.rootNodeByRef[proposal.RootRef]
	if rootNodeID == "" {
		return Surface{}, false, fmt.Errorf("activity surfaces: authority replay cites unknown root")
	}
	restored, err := restoreSurface(rootNodeID, proposal)
	if err != nil {
		return Surface{}, false, err
	}
	return restored, true, nil
}

type bindingSlot struct {
	ref     string
	value   *Value
	mayOmit bool
}

func candidateBindingSets(
	candidate entrycall.RequestSurfaceCandidate,
	surface Surface,
) [][]entrycall.ResponseSurfaceBinding {
	slots := []bindingSlot{
		{ref: entrycall.SurfaceSlotRefIdentity, value: surface.Identity},
		{ref: entrycall.SurfaceSlotRefMethod, value: surface.Method},
		{ref: entrycall.SurfaceSlotRefPath, value: surface.Path},
		{
			ref: entrycall.SurfaceSlotRefHandler, value: surface.Handler,
			mayOmit: surface.Kind == entrycall.SurfaceKindHTTPRoute && surface.Handler != nil,
		},
	}
	choices := make([][]string, 0, len(slots))
	active := make([]bindingSlot, 0, len(slots))
	for _, slot := range slots {
		if slot.value == nil {
			continue
		}
		refs := make([]string, 0, len(candidate.Facts)+1)
		if slot.mayOmit {
			refs = append(refs, "")
		}
		for _, fact := range candidate.Facts {
			if fact.Kind == slot.value.Kind && fact.Value == slot.value.Text {
				refs = append(refs, fact.Ref)
			}
		}
		if len(refs) == 0 {
			return nil
		}
		active = append(active, slot)
		choices = append(choices, refs)
	}
	result := make([][]entrycall.ResponseSurfaceBinding, 0, 1)
	used := make(map[string]struct{})
	bindings := make([]entrycall.ResponseSurfaceBinding, 0, len(active))
	var visit func(int)
	visit = func(index int) {
		if index == len(active) {
			result = append(result, append([]entrycall.ResponseSurfaceBinding(nil), bindings...))
			return
		}
		for _, factRef := range choices[index] {
			if factRef == "" {
				visit(index + 1)
				continue
			}
			if _, duplicate := used[factRef]; duplicate {
				continue
			}
			used[factRef] = struct{}{}
			bindings = append(bindings, entrycall.ResponseSurfaceBinding{
				SlotRef: active[index].ref, FactRef: factRef,
			})
			visit(index + 1)
			bindings = bindings[:len(bindings)-1]
			delete(used, factRef)
		}
	}
	visit(0)
	return result
}

func kindRef(kind string) (string, error) {
	switch kind {
	case entrycall.SurfaceKindCLICommand:
		return entrycall.SurfaceKindRefCLICommand, nil
	case entrycall.SurfaceKindHTTPRoute:
		return entrycall.SurfaceKindRefHTTPRoute, nil
	case entrycall.SurfaceKindScheduledJob:
		return entrycall.SurfaceKindRefScheduledJob, nil
	default:
		return "", fmt.Errorf("activity surfaces: unknown activity kind %q", kind)
	}
}

func validateCoverage(coverage Coverage) error {
	values := []int{
		coverage.Candidates.ConsideredCandidates, coverage.Candidates.AdvertisedCandidates,
		coverage.Candidates.OmittedCandidates, coverage.Candidates.ConsideredFacts,
		coverage.Candidates.AdvertisedFacts, coverage.Candidates.OmittedFacts,
		coverage.Candidates.UnsafeFactsExcluded, coverage.Candidates.UnreachableCandidatesExcluded,
		coverage.Selected, coverage.Rejected,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("activity surfaces: negative coverage")
		}
	}
	if coverage.Candidates.AdvertisedCandidates > entrycall.MaxSurfaceCandidates ||
		coverage.Candidates.AdvertisedCandidates+coverage.Candidates.OmittedCandidates !=
			coverage.Candidates.ConsideredCandidates ||
		coverage.Candidates.AdvertisedFacts > entrycall.MaxSurfaceFacts ||
		coverage.Candidates.AdvertisedFacts+coverage.Candidates.OmittedFacts !=
			coverage.Candidates.ConsideredFacts ||
		coverage.Selected+coverage.Rejected > coverage.Candidates.AdvertisedCandidates {
		return fmt.Errorf("activity surfaces: invalid bounded coverage")
	}
	if err := requireCompleteCandidateCatalog(coverage.Candidates); err != nil {
		return err
	}
	if coverage.ModelCalled != (coverage.Candidates.AdvertisedCandidates > 0) {
		return fmt.Errorf("activity surfaces: model-call accounting mismatch")
	}
	return nil
}

func requireCompleteCandidateCatalog(coverage entrycall.SurfaceCandidateCoverage) error {
	if coverage.OmittedCandidates == 0 && coverage.OmittedFacts == 0 {
		return nil
	}
	return fmt.Errorf(
		"activity surfaces: bounded candidate catalog is incomplete: candidates considered=%d advertised=%d omitted=%d; facts considered=%d advertised=%d omitted=%d",
		coverage.ConsideredCandidates,
		coverage.AdvertisedCandidates,
		coverage.OmittedCandidates,
		coverage.ConsideredFacts,
		coverage.AdvertisedFacts,
		coverage.OmittedFacts,
	)
}

func validateSurface(surface Surface) error {
	if !validSurfaceID(surface.ID) || strings.TrimSpace(surface.RootNodeID) == "" ||
		!surface.Form.Valid() || !validLocation(surface.Registration) {
		return fmt.Errorf("invalid exact identity")
	}
	for _, value := range []*Value{surface.Identity, surface.Method, surface.Path, surface.Handler} {
		if value != nil && (!value.Kind.Valid() || !validSurfaceText(value.Text) || !validLocation(value.Location)) {
			return fmt.Errorf("invalid exact value")
		}
	}
	if surface.Handler != nil && surface.Handler.Kind != entrycall.SurfaceFactCallable {
		return fmt.Errorf("handler is not callable")
	}
	switch surface.Kind {
	case entrycall.SurfaceKindCLICommand:
		if surface.Form != entrycall.SurfaceCandidateKeyedComposite || surface.Role != entrycall.SurfaceRoleDescriptor ||
			surface.Identity == nil || surface.Identity.Kind != entrycall.SurfaceFactString ||
			surface.Method != nil || surface.Path != nil {
			return fmt.Errorf("invalid CLI command shape")
		}
	case entrycall.SurfaceKindHTTPRoute:
		if surface.Form != entrycall.SurfaceCandidateDirectCall || surface.Identity != nil ||
			surface.Path == nil || surface.Path.Kind != entrycall.SurfaceFactString ||
			!strings.HasPrefix(surface.Path.Text, "/") ||
			surface.Method != nil && !validHTTPMethod(*surface.Method) ||
			!validHandlerRole(surface) {
			return fmt.Errorf("invalid HTTP route shape")
		}
	case entrycall.SurfaceKindScheduledJob:
		if surface.Form != entrycall.SurfaceCandidateDirectCall ||
			surface.Identity == nil || surface.Identity.Kind != entrycall.SurfaceFactString ||
			surface.Method != nil || surface.Path != nil || !validHandlerRole(surface) {
			return fmt.Errorf("invalid scheduled job shape")
		}
	default:
		return fmt.Errorf("unknown activity kind %q", surface.Kind)
	}
	return nil
}

func validHandlerRole(surface Surface) bool {
	if surface.Handler == nil {
		return surface.Role == entrycall.SurfaceRoleDescriptor
	}
	return surface.Role == entrycall.SurfaceRoleEntrySurface
}

func validHTTPMethod(value Value) bool {
	switch value.Kind {
	case entrycall.SurfaceFactToken:
		switch strings.ToUpper(value.Text) {
		case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE":
			return true
		default:
			return false
		}
	case entrycall.SurfaceFactString:
		// Extension methods are valid when represented by exact string facts.
	default:
		return false
	}
	for _, character := range value.Text {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func validSurfaceText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !containsControl(value) &&
		utf8.RuneCountInString(value) <= entrycall.MaxSurfaceFactValueRunes
}

func validRejectionReason(reason entrycall.RejectedSurfaceReason) bool {
	switch reason {
	case entrycall.RejectedSurfaceIncompatibleForm,
		entrycall.RejectedSurfaceMissingBinding,
		entrycall.RejectedSurfaceDuplicateBinding,
		entrycall.RejectedSurfaceDuplicateProposal,
		entrycall.RejectedSurfaceIncompatibleBinding:
		return true
	default:
		return false
	}
}

func sortResult(result *Result) {
	sort.Slice(result.Surfaces, func(i, j int) bool { return surfaceLess(result.Surfaces[i], result.Surfaces[j]) })
	sort.Slice(result.Rejections, func(i, j int) bool { return result.Rejections[i].Reason < result.Rejections[j].Reason })
}

func surfaceLess(left, right Surface) bool {
	leftKey := strings.Join([]string{
		left.RootNodeID, left.Kind, left.Registration.Path,
		fmt.Sprintf("%010d", left.Registration.Line), fmt.Sprintf("%010d", left.Registration.Column), left.ID,
	}, "\x00")
	rightKey := strings.Join([]string{
		right.RootNodeID, right.Kind, right.Registration.Path,
		fmt.Sprintf("%010d", right.Registration.Line), fmt.Sprintf("%010d", right.Registration.Column), right.ID,
	}, "\x00")
	return leftKey < rightKey
}

func validSurfaceID(value string) bool {
	if !strings.HasPrefix(value, "model-surface-") || len(value) != len("model-surface-")+24 {
		return false
	}
	_, err := hex.DecodeString(value[len("model-surface-"):])
	return err == nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validLocation(location entrycall.Location) bool {
	return location.Line > 0 && location.Column >= 0 && fs.ValidPath(location.Path) &&
		location.Path != "." && !strings.HasPrefix(location.Path, "<external>/")
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func surfacesEqual(left, right Surface) bool {
	return left.ID == right.ID && left.RootNodeID == right.RootNodeID &&
		left.Kind == right.Kind && left.Role == right.Role && left.Form == right.Form &&
		left.Registration == right.Registration && valuesEqual(left.Identity, right.Identity) &&
		valuesEqual(left.Method, right.Method) && valuesEqual(left.Path, right.Path) &&
		valuesEqual(left.Handler, right.Handler)
}

func valuesEqual(left, right *Value) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
