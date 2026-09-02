// Package godynamichandoff owns a sealed, local overlay for Go control-flow
// joints that a direct-call graph deliberately cannot claim as exact calls.
//
// The index is not a provider request or a sidecar artifact. The ordinary Go
// adapter consumes it in memory and projects its structural facts into the
// language-neutral ProgramIndex.
package godynamichandoff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Version = 3

type Scenario struct {
	ID     string   `json:"id"`
	GOOS   string   `json:"goos"`
	GOARCH string   `json:"goarch"`
	Tags   []string `json:"tags"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Function is an independently owned copy of the exact DirectCallIndex node
// facts used by this overlay. ID remains the DirectCallIndex node identity;
// SourceDirectCallSHA256 binds it to the producer that assigned the ID.
type Function struct {
	ID       string   `json:"id"`
	Package  string   `json:"package"`
	Symbol   string   `json:"symbol"`
	Location Location `json:"location"`
}

type Invocation string

const (
	InvocationSynchronous Invocation = "synchronous"
	InvocationGoroutine   Invocation = "goroutine"
	InvocationDeferred    Invocation = "deferred"
	// InvocationBinding is a callable value assignment, not an assertion that
	// the callable executes at this program point.
	InvocationBinding Invocation = "binding"
)

func (value Invocation) valid() bool {
	return value == InvocationSynchronous || value == InvocationGoroutine ||
		value == InvocationDeferred || value == InvocationBinding
}

// Kind describes a structural control-flow joint, not product semantics.
// CallbackTransfer means only that a callable value crosses a statically
// named function/method boundary or a declared interface-method boundary. A
// later cube may call that registration only when surface evidence supports
// the stronger verb.
type Kind string

const (
	InterfaceInvoke   Kind = "interface_invoke"
	FunctionValueCall Kind = "function_value_call"
	CallbackTransfer  Kind = "callback_transfer"
	CallableBinding   Kind = "callable_binding"
)

func (value Kind) valid() bool {
	return value == InterfaceInvoke || value == FunctionValueCall ||
		value == CallbackTransfer || value == CallableBinding
}

type Resolution string

const (
	ResolutionExact        Resolution = "exact"
	ResolutionAlternatives Resolution = "alternatives"
	ResolutionUnresolved   Resolution = "unresolved"
)

func (value Resolution) valid() bool {
	return value == ResolutionExact || value == ResolutionAlternatives || value == ResolutionUnresolved
}

// CandidateEvidence states exactly how the local SSA value itself identifies
// a repository callable. Compatibility-only type or naming candidates are not
// part of this closed vocabulary.
type CandidateEvidence string

const (
	EvidenceDirectFunctionValue       CandidateEvidence = "direct_function_value"
	EvidenceClosureValue              CandidateEvidence = "closure_value"
	EvidenceUniqueValueFlow           CandidateEvidence = "unique_value_flow"
	EvidenceValueFlowAlternative      CandidateEvidence = "value_flow_alternative"
	EvidenceConcreteInterfaceValue    CandidateEvidence = "concrete_interface_value"
	EvidenceInterfaceValueAlternative CandidateEvidence = "interface_value_alternative"
)

func (value CandidateEvidence) valid() bool {
	switch value {
	case EvidenceDirectFunctionValue, EvidenceClosureValue, EvidenceUniqueValueFlow,
		EvidenceValueFlowAlternative, EvidenceConcreteInterfaceValue,
		EvidenceInterfaceValueAlternative:
		return true
	default:
		return false
	}
}

func (value CandidateEvidence) exact() bool {
	return value == EvidenceDirectFunctionValue || value == EvidenceClosureValue ||
		value == EvidenceUniqueValueFlow || value == EvidenceConcreteInterfaceValue
}

type Candidate struct {
	FunctionID string            `json:"function_id"`
	Evidence   CandidateEvidence `json:"evidence"`
}

// StaticTarget names the exact statically declared recipient joint for a
// callback transfer. Local static calls cite FunctionID. External static calls
// and interface invokes cite package/name (and an optional receiver) without
// inventing a repository node or claiming a runtime interface implementation.
type StaticTarget struct {
	FunctionID string `json:"function_id,omitempty"`
	Package    string `json:"package,omitempty"`
	Receiver   string `json:"receiver,omitempty"`
	Name       string `json:"name,omitempty"`
}

// Slot is the exact declared joint at the callsite. Interface invokes use
// DeclaredType+Method+Signature; function-value calls use Signature; callback
// transfers use one-based Parameter+Signature; callable bindings use
// ContainerType+Field+DeclaredType and may retain the resolved callable
// signature when SSA exposes it.
type Slot struct {
	ContainerType string `json:"container_type,omitempty"`
	DeclaredType  string `json:"declared_type,omitempty"`
	Field         string `json:"field,omitempty"`
	Method        string `json:"method,omitempty"`
	Signature     string `json:"signature,omitempty"`
	Parameter     int    `json:"parameter,omitempty"`
}

type Handoff struct {
	ID                   string       `json:"id"`
	Kind                 Kind         `json:"kind"`
	CallerID             string       `json:"caller_id"`
	Invocation           Invocation   `json:"invocation"`
	Callsite             Location     `json:"callsite"`
	StaticTarget         StaticTarget `json:"static_target,omitempty"`
	Slot                 Slot         `json:"slot"`
	Resolution           Resolution   `json:"resolution"`
	Candidates           []Candidate  `json:"candidates"`
	CandidatesConsidered int          `json:"candidates_considered"`
	CandidatesOmitted    int          `json:"candidates_omitted"`
}

// Coverage separates missing relation rows from unresolved candidate
// frontiers. HandoffsOmitted counts only observed structural joints that
// produced no Handoff row; CandidatesOmitted counts open value-flow candidate
// positions for which no exact local function identity was available.
type Coverage struct {
	HandoffsObserved         int `json:"handoffs_observed"`
	HandoffsIndexed          int `json:"handoffs_indexed"`
	HandoffsOmitted          int `json:"handoffs_omitted"`
	UnsupportedCallers       int `json:"unsupported_callers"`
	InvalidCallsites         int `json:"invalid_callsites"`
	UnsupportedStaticTargets int `json:"unsupported_static_targets"`
	InterfaceInvokes         int `json:"interface_invokes"`
	FunctionValueCalls       int `json:"function_value_calls"`
	CallbackTransfers        int `json:"callback_transfers"`
	CallableBindings         int `json:"callable_bindings"`
	ExactResolutions         int `json:"exact_resolutions"`
	AlternativeResolutions   int `json:"alternative_resolutions"`
	Unresolved               int `json:"unresolved"`
	CandidatesConsidered     int `json:"candidates_considered"`
	CandidatesIndexed        int `json:"candidates_indexed"`
	CandidatesOmitted        int `json:"candidates_omitted"`
}

// CoverageInput accounts for structural handoffs that the SSA pass observed
// but could not bind to the closed Function/Location/StaticTarget vocabulary.
// Those observations never disappear into an apparently complete empty set.
type CoverageInput struct {
	UnsupportedCallers       int
	InvalidCallsites         int
	UnsupportedStaticTargets int
}

type Input struct {
	Scenario               Scenario
	SourceDirectCallSHA256 string
	Functions              []Function
	Handoffs               []Handoff
	Coverage               CoverageInput
}

type Index struct {
	Version                int        `json:"version"`
	Scenario               Scenario   `json:"scenario"`
	SourceDirectCallSHA256 string     `json:"source_direct_call_sha256"`
	Functions              []Function `json:"functions"`
	Handoffs               []Handoff  `json:"handoffs"`
	Coverage               Coverage   `json:"coverage"`
	SHA256                 string     `json:"sha256"`
}

func New(input Input) (Index, error) {
	index := Index{
		Version: Version,
		Scenario: Scenario{
			ID: input.Scenario.ID, GOOS: input.Scenario.GOOS, GOARCH: input.Scenario.GOARCH,
			Tags: append([]string(nil), input.Scenario.Tags...),
		},
		SourceDirectCallSHA256: input.SourceDirectCallSHA256,
		Functions:              append([]Function(nil), input.Functions...),
		Handoffs:               append([]Handoff(nil), input.Handoffs...),
	}
	sort.Strings(index.Scenario.Tags)
	index.Scenario.Tags = compactStrings(index.Scenario.Tags)

	functionByID := make(map[string]Function, len(index.Functions))
	for _, function := range index.Functions {
		if previous, exists := functionByID[function.ID]; exists && previous != function {
			return Index{}, fmt.Errorf("Go dynamic handoff index: conflicting function %q", function.ID)
		}
		functionByID[function.ID] = function
	}
	index.Functions = index.Functions[:0]
	for _, function := range functionByID {
		index.Functions = append(index.Functions, function)
	}
	sort.Slice(index.Functions, func(i, j int) bool { return functionKey(index.Functions[i]) < functionKey(index.Functions[j]) })

	canonical := make(map[string]Handoff, len(index.Handoffs))
	for _, handoff := range index.Handoffs {
		if handoff.ID != "" {
			return Index{}, fmt.Errorf("Go dynamic handoff index: adapter supplied a handoff identity")
		}
		if _, exists := functionByID[handoff.CallerID]; !exists {
			return Index{}, fmt.Errorf("Go dynamic handoff index: handoff cites unknown caller %q", handoff.CallerID)
		}
		if handoff.StaticTarget.FunctionID != "" {
			if _, exists := functionByID[handoff.StaticTarget.FunctionID]; !exists {
				return Index{}, fmt.Errorf("Go dynamic handoff index: transfer cites unknown static target %q", handoff.StaticTarget.FunctionID)
			}
		}
		handoff.Candidates = canonicalCandidates(handoff.Candidates)
		for _, candidate := range handoff.Candidates {
			if _, exists := functionByID[candidate.FunctionID]; !exists {
				return Index{}, fmt.Errorf("Go dynamic handoff index: handoff cites unknown candidate %q", candidate.FunctionID)
			}
		}
		if handoff.CandidatesConsidered == 0 {
			handoff.CandidatesConsidered = len(handoff.Candidates)
		}
		if handoff.CandidatesConsidered < len(handoff.Candidates) {
			return Index{}, fmt.Errorf("Go dynamic handoff index: invalid candidate accounting")
		}
		handoff.CandidatesOmitted = handoff.CandidatesConsidered - len(handoff.Candidates)
		handoff.ID = stableID(
			"go-dynamic-handoff", index.Scenario.ID, string(handoff.Kind), handoff.CallerID,
			locationKey(handoff.Callsite), staticTargetKey(handoff.StaticTarget), slotKey(handoff.Slot),
			string(handoff.Invocation),
		)
		if previous, exists := canonical[handoff.ID]; exists && handoffKey(previous) != handoffKey(handoff) {
			return Index{}, fmt.Errorf("Go dynamic handoff index: conflicting handoff %q", handoff.ID)
		}
		canonical[handoff.ID] = handoff
	}
	index.Handoffs = index.Handoffs[:0]
	for _, handoff := range canonical {
		index.Handoffs = append(index.Handoffs, handoff)
	}
	sort.Slice(index.Handoffs, func(i, j int) bool { return handoffKey(index.Handoffs[i]) < handoffKey(index.Handoffs[j]) })
	coverage, err := compileCoverage(index.Handoffs, input.Coverage)
	if err != nil {
		return Index{}, err
	}
	index.Coverage = coverage
	digest, err := indexDigest(index)
	if err != nil {
		return Index{}, err
	}
	index.SHA256 = digest
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

func (index Index) Snapshot() Index {
	result := index
	result.Scenario.Tags = append([]string(nil), index.Scenario.Tags...)
	result.Functions = append([]Function(nil), index.Functions...)
	result.Handoffs = append([]Handoff(nil), index.Handoffs...)
	for position := range result.Handoffs {
		result.Handoffs[position].Candidates = append([]Candidate(nil), index.Handoffs[position].Candidates...)
	}
	return result
}

func (index Index) Validate() error {
	if index.Version != Version || !validText(index.Scenario.ID) || !validText(index.Scenario.GOOS) ||
		!validText(index.Scenario.GOARCH) || !canonicalStrings(index.Scenario.Tags) ||
		!validSHA256(index.SourceDirectCallSHA256) {
		return fmt.Errorf("Go dynamic handoff index: invalid producer identity")
	}
	functions := make(map[string]struct{}, len(index.Functions))
	for position, function := range index.Functions {
		if !validText(function.ID) || !validText(function.Package) || !validText(function.Symbol) ||
			!validLocation(function.Location) || position > 0 && functionKey(index.Functions[position-1]) >= functionKey(function) {
			return fmt.Errorf("Go dynamic handoff index: invalid function")
		}
		if _, duplicate := functions[function.ID]; duplicate {
			return fmt.Errorf("Go dynamic handoff index: duplicate function")
		}
		functions[function.ID] = struct{}{}
	}
	for position, handoff := range index.Handoffs {
		if err := validateHandoff(handoff, functions); err != nil {
			return err
		}
		if position > 0 && handoffKey(index.Handoffs[position-1]) >= handoffKey(handoff) {
			return fmt.Errorf("Go dynamic handoff index: handoffs are not canonical")
		}
	}
	wantCoverage, err := compileCoverage(index.Handoffs, CoverageInput{
		UnsupportedCallers:       index.Coverage.UnsupportedCallers,
		InvalidCallsites:         index.Coverage.InvalidCallsites,
		UnsupportedStaticTargets: index.Coverage.UnsupportedStaticTargets,
	})
	if err != nil || index.Coverage != wantCoverage {
		return fmt.Errorf("Go dynamic handoff index: invalid coverage")
	}
	want, err := indexDigest(index)
	if err != nil {
		return err
	}
	if index.SHA256 != want {
		return fmt.Errorf("Go dynamic handoff index: digest mismatch")
	}
	return nil
}

func validateHandoff(handoff Handoff, functions map[string]struct{}) error {
	if !handoff.Kind.valid() || !handoff.Invocation.valid() || !handoff.Resolution.valid() ||
		!validText(handoff.ID) || !validLocation(handoff.Callsite) {
		return fmt.Errorf("Go dynamic handoff index: invalid handoff")
	}
	if _, exists := functions[handoff.CallerID]; !exists {
		return fmt.Errorf("Go dynamic handoff index: unknown caller")
	}
	if err := validateKindShape(handoff, functions); err != nil {
		return err
	}
	if handoff.CandidatesConsidered < len(handoff.Candidates) ||
		handoff.CandidatesOmitted != handoff.CandidatesConsidered-len(handoff.Candidates) {
		return fmt.Errorf("Go dynamic handoff index: invalid candidate accounting")
	}
	previous := ""
	for _, candidate := range handoff.Candidates {
		key := candidateKey(candidate)
		if _, exists := functions[candidate.FunctionID]; !exists || !candidate.Evidence.valid() || previous != "" && key <= previous {
			return fmt.Errorf("Go dynamic handoff index: invalid candidate")
		}
		previous = key
	}
	switch handoff.Resolution {
	case ResolutionExact:
		if len(handoff.Candidates) != 1 || handoff.CandidatesOmitted != 0 || !handoff.Candidates[0].Evidence.exact() {
			return fmt.Errorf("Go dynamic handoff index: invalid exact resolution")
		}
	case ResolutionAlternatives:
		if len(handoff.Candidates) < 1 || len(handoff.Candidates) == 1 && handoff.CandidatesOmitted == 0 {
			return fmt.Errorf("Go dynamic handoff index: alternatives require multiple known candidates or an open frontier")
		}
	case ResolutionUnresolved:
		if len(handoff.Candidates) != 0 {
			return fmt.Errorf("Go dynamic handoff index: unresolved handoff retained candidates")
		}
	}
	return nil
}

func validateKindShape(handoff Handoff, functions map[string]struct{}) error {
	target := handoff.StaticTarget
	switch handoff.Kind {
	case InterfaceInvoke:
		if !validText(handoff.Slot.DeclaredType) || !validIdentifier(handoff.Slot.Method) ||
			!validText(handoff.Slot.Signature) || handoff.Slot.ContainerType != "" ||
			handoff.Slot.Field != "" || handoff.Slot.Parameter != 0 || handoff.Invocation == InvocationBinding ||
			target != (StaticTarget{}) {
			return fmt.Errorf("Go dynamic handoff index: invalid interface invoke slot")
		}
		for _, candidate := range handoff.Candidates {
			if candidate.Evidence != EvidenceConcreteInterfaceValue &&
				candidate.Evidence != EvidenceInterfaceValueAlternative {
				return fmt.Errorf("Go dynamic handoff index: interface candidate lacks concrete SSA value flow")
			}
		}
	case FunctionValueCall:
		if !validText(handoff.Slot.Signature) || handoff.Slot.ContainerType != "" ||
			handoff.Slot.DeclaredType != "" || handoff.Slot.Field != "" ||
			handoff.Slot.Method != "" || handoff.Slot.Parameter != 0 ||
			handoff.Invocation == InvocationBinding || target != (StaticTarget{}) {
			return fmt.Errorf("Go dynamic handoff index: invalid function-value slot")
		}
	case CallbackTransfer:
		if !validText(handoff.Slot.Signature) || handoff.Slot.ContainerType != "" ||
			handoff.Slot.DeclaredType != "" || handoff.Slot.Field != "" ||
			handoff.Slot.Method != "" || handoff.Slot.Parameter < 1 ||
			handoff.Invocation == InvocationBinding || !validStaticTarget(target, functions) {
			return fmt.Errorf("Go dynamic handoff index: invalid callback transfer")
		}
	case CallableBinding:
		if handoff.Invocation != InvocationBinding || !validText(handoff.Slot.ContainerType) ||
			!validIdentifier(handoff.Slot.Field) || !validText(handoff.Slot.DeclaredType) ||
			handoff.Slot.Method != "" || handoff.Slot.Parameter != 0 || target != (StaticTarget{}) ||
			handoff.Slot.Signature != "" && !validText(handoff.Slot.Signature) {
			return fmt.Errorf("Go dynamic handoff index: invalid callable binding")
		}
	}
	return nil
}

func validStaticTarget(target StaticTarget, functions map[string]struct{}) bool {
	if target.FunctionID != "" {
		_, exists := functions[target.FunctionID]
		return exists && target.Package == "" && target.Receiver == "" && target.Name == ""
	}
	return validText(target.Package) && validIdentifier(target.Name) && (target.Receiver == "" || validText(target.Receiver))
}

func canonicalCandidates(values []Candidate) []Candidate {
	byFunction := make(map[string]Candidate, len(values))
	for _, candidate := range values {
		previous, exists := byFunction[candidate.FunctionID]
		if !exists || evidenceRank(candidate.Evidence) < evidenceRank(previous.Evidence) {
			byFunction[candidate.FunctionID] = candidate
		}
	}
	result := make([]Candidate, 0, len(byFunction))
	for _, candidate := range byFunction {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return candidateKey(result[i]) < candidateKey(result[j]) })
	return result
}

func evidenceRank(value CandidateEvidence) int {
	switch value {
	case EvidenceDirectFunctionValue:
		return 0
	case EvidenceClosureValue:
		return 1
	case EvidenceUniqueValueFlow:
		return 2
	case EvidenceValueFlowAlternative:
		return 3
	case EvidenceConcreteInterfaceValue:
		return 4
	case EvidenceInterfaceValueAlternative:
		return 5
	default:
		return 99
	}
}

func compileCoverage(handoffs []Handoff, input CoverageInput) (Coverage, error) {
	coverage := Coverage{
		HandoffsIndexed:          len(handoffs),
		UnsupportedCallers:       input.UnsupportedCallers,
		InvalidCallsites:         input.InvalidCallsites,
		UnsupportedStaticTargets: input.UnsupportedStaticTargets,
	}
	counts := []int{
		coverage.UnsupportedCallers,
		coverage.InvalidCallsites,
		coverage.UnsupportedStaticTargets,
	}
	omitted := 0
	for _, count := range counts {
		var ok bool
		omitted, ok = addNonnegativeCount(omitted, count)
		if !ok {
			return Coverage{}, fmt.Errorf("Go dynamic handoff index: coverage count overflow")
		}
	}
	observed, ok := addNonnegativeCount(len(handoffs), omitted)
	if !ok {
		return Coverage{}, fmt.Errorf("Go dynamic handoff index: coverage count overflow")
	}
	coverage.HandoffsOmitted = omitted
	coverage.HandoffsObserved = observed
	for _, handoff := range handoffs {
		switch handoff.Kind {
		case InterfaceInvoke:
			coverage.InterfaceInvokes++
		case FunctionValueCall:
			coverage.FunctionValueCalls++
		case CallbackTransfer:
			coverage.CallbackTransfers++
		case CallableBinding:
			coverage.CallableBindings++
		}
		switch handoff.Resolution {
		case ResolutionExact:
			coverage.ExactResolutions++
		case ResolutionAlternatives:
			coverage.AlternativeResolutions++
		case ResolutionUnresolved:
			coverage.Unresolved++
		}
		coverage.CandidatesConsidered, ok = addNonnegativeCount(
			coverage.CandidatesConsidered, handoff.CandidatesConsidered,
		)
		if !ok {
			return Coverage{}, fmt.Errorf("Go dynamic handoff index: candidate count overflow")
		}
		coverage.CandidatesIndexed, ok = addNonnegativeCount(
			coverage.CandidatesIndexed, len(handoff.Candidates),
		)
		if !ok {
			return Coverage{}, fmt.Errorf("Go dynamic handoff index: candidate count overflow")
		}
		coverage.CandidatesOmitted, ok = addNonnegativeCount(
			coverage.CandidatesOmitted, handoff.CandidatesOmitted,
		)
		if !ok {
			return Coverage{}, fmt.Errorf("Go dynamic handoff index: candidate count overflow")
		}
	}
	return coverage, nil
}

func addNonnegativeCount(left, right int) (int, bool) {
	if left < 0 || right < 0 || left > int(^uint(0)>>1)-right {
		return 0, false
	}
	return left + right, true
}

func (index Index) digestPayload() Index {
	result := index.Snapshot()
	result.SHA256 = ""
	return result
}

func indexDigest(index Index) (string, error) {
	encoded, err := json.Marshal(index.digestPayload())
	if err != nil {
		return "", fmt.Errorf("Go dynamic handoff index: encode digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		digest.Write([]byte(strconv.Itoa(len(field))))
		digest.Write([]byte{0})
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func functionKey(value Function) string {
	return value.ID + "\x00" + value.Package + "\x00" + value.Symbol + "\x00" + locationKey(value.Location)
}

func handoffKey(value Handoff) string {
	encoded, _ := json.Marshal(value)
	return locationKey(value.Callsite) + "\x00" + string(value.Kind) + "\x00" + value.CallerID + "\x00" + string(encoded)
}

func candidateKey(value Candidate) string {
	return value.FunctionID + "\x00" + string(value.Evidence)
}

func staticTargetKey(value StaticTarget) string {
	return value.FunctionID + "\x00" + value.Package + "\x00" + value.Receiver + "\x00" + value.Name
}

func slotKey(value Slot) string {
	return value.ContainerType + "\x00" + value.DeclaredType + "\x00" + value.Field + "\x00" +
		value.Method + "\x00" + value.Signature + "\x00" + strconv.Itoa(value.Parameter)
}

func locationKey(value Location) string {
	return value.Path + ":" + strconv.Itoa(value.Line) + ":" + strconv.Itoa(value.Column)
}

func validLocation(value Location) bool {
	return validPath(value.Path) && value.Line > 0 && value.Column > 0
}

func validPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") ||
		!fs.ValidPath(value) || value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	if !validText(value) {
		return false
	}
	for index, character := range value {
		if character == '_' || unicode.IsLetter(character) || index > 0 && unicode.IsDigit(character) {
			continue
		}
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func canonicalStrings(values []string) bool {
	if !sort.StringsAreSorted(values) {
		return false
	}
	for position, value := range values {
		if !validText(value) || position > 0 && values[position-1] == value {
			return false
		}
	}
	return true
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
