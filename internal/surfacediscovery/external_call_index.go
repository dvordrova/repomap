package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	ExternalCallIndexVersion = 1

	MaxExternalCallIndexPackages = 65_536
	MaxExternalCallIndexCallers  = 65_536
	MaxExternalCallIndexFamilies = 131_072

	MaxExternalCallRepresentativeCallsites = 3
)

// ExternalCallPackage is one repository package present in the exact loaded
// Go program. Keeping packages with no observed external calls lets a later
// target projection distinguish an empty result from an unexamined package.
type ExternalCallPackage struct {
	ModuleID    string `json:"module_id"`
	PackagePath string `json:"package_path"`
}

// ExternalCallTarget is a safely named static callee outside the repository
// program. It deliberately states only Go package and symbol identity; a
// later integration cube owns service or resource semantics.
type ExternalCallTarget struct {
	PackagePath string `json:"package_path"`
	Receiver    string `json:"receiver,omitempty"`
	Name        string `json:"name"`
}

// ExternalCallDispatch states what the Go type system proves about one call.
// Interface invoke preserves the exact declared interface method while making
// no claim about its runtime implementation.
type ExternalCallDispatch string

const (
	ExternalCallStatic          ExternalCallDispatch = "static"
	ExternalCallInterfaceInvoke ExternalCallDispatch = "interface_invoke"
)

func (dispatch ExternalCallDispatch) Valid() bool {
	return dispatch == ExternalCallStatic || dispatch == ExternalCallInterfaceInvoke
}

// ExternalCallWitness is the adapter input for one exact SSA call
// instruction. Caller reuses DirectCallIndex node authority, while Callsite
// is the exact repository-local instruction position.
type ExternalCallWitness struct {
	Caller     DirectCallNode
	Target     ExternalCallTarget
	Dispatch   ExternalCallDispatch
	Invocation DirectCallInvocation
	Callsite   Location
}

// ExternalCallExclusion retains unresolved call shapes per exact caller. None
// of these counts is promoted to an external target.
type ExternalCallExclusion struct {
	Caller                       DirectCallNode
	DynamicInvokesExcluded       int
	NonStaticCallsExcluded       int
	UnnamedStaticCalleesExcluded int
	InvalidCallsitesExcluded     int
}

// ExternalCallFamily compacts exact witnesses with the same local caller,
// external callee and invocation. WitnessCount stays complete while only a
// bounded canonical callsite sample is retained.
type ExternalCallFamily struct {
	ID               string               `json:"id"`
	CallerID         string               `json:"caller_id"`
	Target           ExternalCallTarget   `json:"target"`
	Dispatch         ExternalCallDispatch `json:"dispatch"`
	Invocation       DirectCallInvocation `json:"invocation"`
	WitnessCount     int                  `json:"witness_count"`
	Callsites        []Location           `json:"callsites"`
	CallsitesOmitted int                  `json:"callsites_omitted"`
}

type ExternalCallFrontier struct {
	CallerID                     string `json:"caller_id"`
	DynamicInvokesExcluded       int    `json:"dynamic_invokes_excluded"`
	NonStaticCallsExcluded       int    `json:"non_static_calls_excluded"`
	UnnamedStaticCalleesExcluded int    `json:"unnamed_static_callees_excluded"`
	InvalidCallsitesExcluded     int    `json:"invalid_callsites_excluded"`
}

// ExternalCallPackageFrontier accounts for exact external static witnesses
// whose repository caller cannot reuse DirectCallNode authority. Package init
// is synthetic in SSA, so it is counted under its exact package rather than
// assigned a guessed local symbol identity. Package ownership keeps later
// target projection exact.
type ExternalCallPackageFrontier struct {
	ModuleID                         string `json:"module_id"`
	PackagePath                      string `json:"package_path"`
	SyntheticCallerWitnessesExcluded int    `json:"synthetic_caller_witnesses_excluded"`
	InvalidCallerWitnessesExcluded   int    `json:"invalid_caller_witnesses_excluded"`
}

type ExternalCallIndexCoverage struct {
	PackagesIndexed                  int `json:"packages_indexed"`
	CallersIndexed                   int `json:"callers_indexed"`
	FamiliesIndexed                  int `json:"families_indexed"`
	ExternalStaticWitnesses          int `json:"external_static_witnesses"`
	ExternalInterfaceInvokeWitnesses int `json:"external_interface_invoke_witnesses"`
	RepresentativeCallsites          int `json:"representative_callsites"`
	RepresentativeCallsitesOmitted   int `json:"representative_callsites_omitted"`
	DynamicInvokesExcluded           int `json:"dynamic_invokes_excluded"`
	NonStaticCallsExcluded           int `json:"non_static_calls_excluded"`
	UnnamedStaticCalleesExcluded     int `json:"unnamed_static_callees_excluded"`
	InvalidCallsitesExcluded         int `json:"invalid_callsites_excluded"`
	SyntheticCallerWitnessesExcluded int `json:"synthetic_caller_witnesses_excluded"`
	InvalidCallerWitnessesExcluded   int `json:"invalid_caller_witnesses_excluded"`
}

// ExternalCallIndex is a root-independent exact fact set for one loaded Go
// package set and build scenario.
type ExternalCallIndex struct {
	Version          int                           `json:"version"`
	Scenario         Scenario                      `json:"scenario"`
	Modules          []DirectCallModule            `json:"modules"`
	Packages         []ExternalCallPackage         `json:"packages"`
	Callers          []DirectCallNode              `json:"callers"`
	Families         []ExternalCallFamily          `json:"families"`
	Frontiers        []ExternalCallFrontier        `json:"frontiers"`
	PackageFrontiers []ExternalCallPackageFrontier `json:"package_frontiers"`
	Coverage         ExternalCallIndexCoverage     `json:"coverage"`
	SHA256           string                        `json:"sha256"`
}

type ExternalCallIndexBuilder struct {
	index                ExternalCallIndex
	moduleByID           map[string]DirectCallModule
	packageByKey         map[string]ExternalCallPackage
	callerByID           map[string]DirectCallNode
	familyByID           map[string]ExternalCallFamily
	frontierByID         map[string]ExternalCallFrontier
	packageFrontierByKey map[string]ExternalCallPackageFrontier
}

func NewExternalCallIndexBuilder(
	scenario Scenario,
	modules []DirectCallModule,
	packages []ExternalCallPackage,
) (*ExternalCallIndexBuilder, error) {
	scenario.Tags = append([]string(nil), scenario.Tags...)
	sort.Strings(scenario.Tags)
	scenario.Tags = compactStrings(scenario.Tags)
	if strings.TrimSpace(scenario.ID) == "" || strings.TrimSpace(scenario.GOOS) == "" ||
		strings.TrimSpace(scenario.GOARCH) == "" {
		return nil, fmt.Errorf("external call index: invalid scenario")
	}
	if len(modules) > MaxExternalCallIndexPackages || len(packages) > MaxExternalCallIndexPackages {
		return nil, fmt.Errorf("external call index: module or package limit exceeded")
	}
	builder := &ExternalCallIndexBuilder{
		index: ExternalCallIndex{
			Version: ExternalCallIndexVersion, Scenario: scenario,
			Modules: []DirectCallModule{}, Packages: []ExternalCallPackage{},
			Callers: []DirectCallNode{}, Families: []ExternalCallFamily{},
			Frontiers: []ExternalCallFrontier{}, PackageFrontiers: []ExternalCallPackageFrontier{},
		},
		moduleByID:           make(map[string]DirectCallModule, len(modules)),
		packageByKey:         make(map[string]ExternalCallPackage, len(packages)),
		callerByID:           make(map[string]DirectCallNode),
		familyByID:           make(map[string]ExternalCallFamily),
		frontierByID:         make(map[string]ExternalCallFrontier),
		packageFrontierByKey: make(map[string]ExternalCallPackageFrontier),
	}
	for _, module := range modules {
		if err := validateExternalCallModule(module); err != nil {
			return nil, err
		}
		if previous, exists := builder.moduleByID[module.ID]; exists && previous != module {
			return nil, fmt.Errorf("external call index: conflicting module %q", module.ID)
		}
		builder.moduleByID[module.ID] = module
	}
	for _, value := range packages {
		if _, exists := builder.moduleByID[value.ModuleID]; !exists ||
			!externalCallPlain(value.PackagePath) {
			return nil, fmt.Errorf("external call index: invalid package")
		}
		key := externalCallPackageKey(value)
		builder.packageByKey[key] = value
	}
	for _, module := range builder.moduleByID {
		builder.index.Modules = append(builder.index.Modules, module)
	}
	for _, value := range builder.packageByKey {
		builder.index.Packages = append(builder.index.Packages, value)
	}
	sort.Slice(builder.index.Modules, func(i, j int) bool {
		return directCallModuleKey(builder.index.Modules[i]) < directCallModuleKey(builder.index.Modules[j])
	})
	sort.Slice(builder.index.Packages, func(i, j int) bool {
		return externalCallPackageKey(builder.index.Packages[i]) < externalCallPackageKey(builder.index.Packages[j])
	})
	return builder, nil
}

func (builder *ExternalCallIndexBuilder) AddWitness(value ExternalCallWitness) error {
	if builder == nil {
		return fmt.Errorf("external call index: builder is unavailable")
	}
	if err := builder.addCaller(value.Caller); err != nil {
		return err
	}
	if !validExternalCallTarget(value.Target) || !value.Dispatch.Valid() || !value.Invocation.Valid() ||
		!validRepositoryDirectCallLocation(value.Callsite) {
		return fmt.Errorf("external call index: invalid exact witness")
	}
	familyID := externalCallFamilyID(
		value.Caller.ID, value.Target, value.Dispatch, value.Invocation,
	)
	family, exists := builder.familyByID[familyID]
	if !exists {
		if len(builder.familyByID) >= MaxExternalCallIndexFamilies {
			return fmt.Errorf("external call index: family limit exceeded")
		}
		family = ExternalCallFamily{
			ID: familyID, CallerID: value.Caller.ID, Target: value.Target,
			Dispatch: value.Dispatch, Invocation: value.Invocation, Callsites: []Location{},
		}
	}
	family.WitnessCount++
	family.Callsites = appendExternalCallsite(family.Callsites, value.Callsite)
	family.CallsitesOmitted = family.WitnessCount - len(family.Callsites)
	builder.familyByID[familyID] = family
	return nil
}

func (builder *ExternalCallIndexBuilder) AddExclusion(value ExternalCallExclusion) error {
	if builder == nil {
		return fmt.Errorf("external call index: builder is unavailable")
	}
	if err := builder.addCaller(value.Caller); err != nil {
		return err
	}
	counts := []int{
		value.DynamicInvokesExcluded, value.NonStaticCallsExcluded,
		value.UnnamedStaticCalleesExcluded, value.InvalidCallsitesExcluded,
	}
	positive := false
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("external call index: invalid exclusion")
		}
		positive = positive || count > 0
	}
	if !positive {
		return fmt.Errorf("external call index: empty exclusion")
	}
	frontier := builder.frontierByID[value.Caller.ID]
	frontier.CallerID = value.Caller.ID
	frontier.DynamicInvokesExcluded += value.DynamicInvokesExcluded
	frontier.NonStaticCallsExcluded += value.NonStaticCallsExcluded
	frontier.UnnamedStaticCalleesExcluded += value.UnnamedStaticCalleesExcluded
	frontier.InvalidCallsitesExcluded += value.InvalidCallsitesExcluded
	if frontier.DynamicInvokesExcluded < 0 || frontier.NonStaticCallsExcluded < 0 ||
		frontier.UnnamedStaticCalleesExcluded < 0 || frontier.InvalidCallsitesExcluded < 0 {
		return fmt.Errorf("external call index: exclusion count overflow")
	}
	builder.frontierByID[value.Caller.ID] = frontier
	return nil
}

func (builder *ExternalCallIndexBuilder) addPackageExclusion(
	owner ExternalCallPackage,
	synthetic bool,
) error {
	if builder == nil {
		return fmt.Errorf("external call index: builder is unavailable")
	}
	key := externalCallPackageKey(owner)
	exact, exists := builder.packageByKey[key]
	if !exists || exact != owner {
		return fmt.Errorf("external call index: package exclusion cites unknown package")
	}
	frontier := builder.packageFrontierByKey[key]
	frontier.ModuleID = owner.ModuleID
	frontier.PackagePath = owner.PackagePath
	if synthetic {
		frontier.SyntheticCallerWitnessesExcluded++
		if frontier.SyntheticCallerWitnessesExcluded < 0 {
			return fmt.Errorf("external call index: package exclusion count overflow")
		}
	} else {
		frontier.InvalidCallerWitnessesExcluded++
		if frontier.InvalidCallerWitnessesExcluded < 0 {
			return fmt.Errorf("external call index: package exclusion count overflow")
		}
	}
	builder.packageFrontierByKey[key] = frontier
	return nil
}

func (builder *ExternalCallIndexBuilder) addCaller(caller DirectCallNode) error {
	if err := validateExternalCallCaller(
		caller, builder.index.Scenario, builder.moduleByID, builder.packageByKey,
	); err != nil {
		return err
	}
	if previous, exists := builder.callerByID[caller.ID]; exists {
		if !sameExternalCallCaller(previous, caller) {
			return fmt.Errorf("external call index: conflicting caller %q", caller.ID)
		}
		return nil
	}
	if len(builder.callerByID) >= MaxExternalCallIndexCallers {
		return fmt.Errorf("external call index: caller limit exceeded")
	}
	builder.callerByID[caller.ID] = copyDirectCallNode(caller)
	return nil
}

func (builder *ExternalCallIndexBuilder) Finish() (ExternalCallIndex, error) {
	if builder == nil {
		return ExternalCallIndex{}, fmt.Errorf("external call index: builder is unavailable")
	}
	result := builder.index
	for _, caller := range builder.callerByID {
		result.Callers = append(result.Callers, copyDirectCallNode(caller))
	}
	for _, family := range builder.familyByID {
		family.Callsites = append([]Location(nil), family.Callsites...)
		result.Families = append(result.Families, family)
	}
	for _, frontier := range builder.frontierByID {
		result.Frontiers = append(result.Frontiers, frontier)
	}
	for _, frontier := range builder.packageFrontierByKey {
		result.PackageFrontiers = append(result.PackageFrontiers, frontier)
	}
	canonicalizeExternalCallIndex(&result)
	result.Coverage = externalCallCoverage(result)
	digest, err := externalCallIndexSHA256(result)
	if err != nil {
		return ExternalCallIndex{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return ExternalCallIndex{}, err
	}
	return result, nil
}

func (index ExternalCallIndex) Snapshot() ExternalCallIndex {
	result := index
	result.Scenario.Tags = append([]string(nil), index.Scenario.Tags...)
	result.Modules = append([]DirectCallModule(nil), index.Modules...)
	result.Packages = append([]ExternalCallPackage(nil), index.Packages...)
	result.Callers = append([]DirectCallNode(nil), index.Callers...)
	for position := range result.Callers {
		result.Callers[position] = copyDirectCallNode(result.Callers[position])
	}
	result.Families = append([]ExternalCallFamily(nil), index.Families...)
	for position := range result.Families {
		result.Families[position].Callsites = append(
			[]Location(nil), index.Families[position].Callsites...,
		)
	}
	result.Frontiers = append([]ExternalCallFrontier(nil), index.Frontiers...)
	result.PackageFrontiers = append(
		[]ExternalCallPackageFrontier(nil), index.PackageFrontiers...,
	)
	return result
}

func (index ExternalCallIndex) Validate() error {
	if index.Version != ExternalCallIndexVersion || strings.TrimSpace(index.Scenario.ID) == "" ||
		strings.TrimSpace(index.Scenario.GOOS) == "" || strings.TrimSpace(index.Scenario.GOARCH) == "" ||
		!sort.StringsAreSorted(index.Scenario.Tags) || !uniqueStrings(index.Scenario.Tags) {
		return fmt.Errorf("external call index: invalid version or scenario")
	}
	if len(index.Modules) > MaxExternalCallIndexPackages ||
		len(index.Packages) > MaxExternalCallIndexPackages ||
		len(index.Callers) > MaxExternalCallIndexCallers ||
		len(index.Families) > MaxExternalCallIndexFamilies {
		return fmt.Errorf("external call index: bound exceeded")
	}
	modules := make(map[string]DirectCallModule, len(index.Modules))
	previous := ""
	for _, module := range index.Modules {
		if err := validateExternalCallModule(module); err != nil {
			return err
		}
		key := directCallModuleKey(module)
		if previous != "" && key <= previous {
			return fmt.Errorf("external call index: modules are not canonical")
		}
		previous = key
		modules[module.ID] = module
	}
	packages := make(map[string]ExternalCallPackage, len(index.Packages))
	previous = ""
	for _, value := range index.Packages {
		key := externalCallPackageKey(value)
		if _, exists := modules[value.ModuleID]; !exists || !externalCallPlain(value.PackagePath) ||
			(previous != "" && key <= previous) {
			return fmt.Errorf("external call index: packages are not valid canonical identities")
		}
		previous = key
		packages[key] = value
	}
	callers := make(map[string]DirectCallNode, len(index.Callers))
	previous = ""
	for _, caller := range index.Callers {
		if err := validateExternalCallCaller(caller, index.Scenario, modules, packages); err != nil {
			return err
		}
		key := directCallNodeKey(caller)
		if previous != "" && key <= previous {
			return fmt.Errorf("external call index: callers are not canonical")
		}
		previous = key
		callers[caller.ID] = caller
	}
	previous = ""
	for _, family := range index.Families {
		key := externalCallFamilyKey(family)
		if previous != "" && key <= previous {
			return fmt.Errorf("external call index: families are not canonical")
		}
		previous = key
		if _, exists := callers[family.CallerID]; !exists || !validExternalCallTarget(family.Target) || !family.Dispatch.Valid() ||
			!family.Invocation.Valid() || family.WitnessCount < 1 || len(family.Callsites) == 0 ||
			len(family.Callsites) > MaxExternalCallRepresentativeCallsites ||
			family.CallsitesOmitted != family.WitnessCount-len(family.Callsites) || family.CallsitesOmitted < 0 ||
			family.ID != externalCallFamilyID(family.CallerID, family.Target, family.Dispatch, family.Invocation) {
			return fmt.Errorf("external call index: invalid family")
		}
		for position, callsite := range family.Callsites {
			if !validRepositoryDirectCallLocation(callsite) ||
				(position > 0 && !directCallLocationLess(family.Callsites[position-1], callsite)) {
				return fmt.Errorf("external call index: invalid family callsites")
			}
		}
	}
	previous = ""
	for _, frontier := range index.Frontiers {
		if previous != "" && frontier.CallerID <= previous {
			return fmt.Errorf("external call index: frontiers are not canonical")
		}
		previous = frontier.CallerID
		if _, exists := callers[frontier.CallerID]; !exists || !validExternalCallFrontier(frontier) {
			return fmt.Errorf("external call index: invalid frontier")
		}
	}
	previous = ""
	for _, frontier := range index.PackageFrontiers {
		key := externalCallPackageFrontierKey(frontier)
		if previous != "" && key <= previous {
			return fmt.Errorf("external call index: package frontiers are not canonical")
		}
		previous = key
		if _, exists := packages[externalCallPackageKey(ExternalCallPackage{
			ModuleID: frontier.ModuleID, PackagePath: frontier.PackagePath,
		})]; !exists || !validExternalCallPackageFrontier(frontier) {
			return fmt.Errorf("external call index: invalid package frontier")
		}
	}
	if index.Coverage != externalCallCoverage(index) {
		return fmt.Errorf("external call index: coverage mismatch")
	}
	digest, err := externalCallIndexSHA256(index)
	if err != nil {
		return err
	}
	if index.SHA256 != digest || !validExternalCallSHA256(index.SHA256) {
		return fmt.Errorf("external call index: sha256 mismatch")
	}
	return nil
}

func validateExternalCallModule(module DirectCallModule) error {
	if module.ID == "" || module.Path == "" || !validDirectCallModuleDirectory(module.Directory) ||
		module.ID != stableDirectCallID("direct-module", module.Path, module.Directory) {
		return fmt.Errorf("external call index: invalid module")
	}
	return nil
}

func validateExternalCallCaller(
	caller DirectCallNode,
	scenario Scenario,
	modules map[string]DirectCallModule,
	packages map[string]ExternalCallPackage,
) error {
	if _, exists := modules[caller.ModuleID]; !exists || caller.ScenarioID != scenario.ID ||
		caller.Package == "" || caller.Symbol.ID == "" || caller.Symbol.Package != caller.Package ||
		!validDirectCallEquivalentIDs(caller.Symbol.EquivalentIDs) ||
		caller.Symbol.Location.Path != caller.Declaration.Path ||
		caller.Symbol.Location.Line != caller.Declaration.Line ||
		caller.Symbol.Location.Column != caller.Declaration.Column ||
		!validRepositoryDirectCallLocation(caller.Declaration) ||
		!validDirectCallBody(caller.Declaration, caller.Body) ||
		caller.ID != stableDirectCallNodeID(caller) {
		return fmt.Errorf("external call index: invalid caller")
	}
	if _, exists := packages[externalCallPackageKey(ExternalCallPackage{
		ModuleID: caller.ModuleID, PackagePath: caller.Package,
	})]; !exists {
		return fmt.Errorf("external call index: caller is outside loaded packages")
	}
	return nil
}

func sameExternalCallCaller(left, right DirectCallNode) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func appendExternalCallsite(values []Location, candidate Location) []Location {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	values = append(values, candidate)
	sort.Slice(values, func(i, j int) bool { return directCallLocationLess(values[i], values[j]) })
	if len(values) > MaxExternalCallRepresentativeCallsites {
		values = values[:MaxExternalCallRepresentativeCallsites]
	}
	return values
}

func canonicalizeExternalCallIndex(index *ExternalCallIndex) {
	sort.Slice(index.Modules, func(i, j int) bool {
		return directCallModuleKey(index.Modules[i]) < directCallModuleKey(index.Modules[j])
	})
	sort.Slice(index.Packages, func(i, j int) bool {
		return externalCallPackageKey(index.Packages[i]) < externalCallPackageKey(index.Packages[j])
	})
	sort.Slice(index.Callers, func(i, j int) bool {
		return directCallNodeKey(index.Callers[i]) < directCallNodeKey(index.Callers[j])
	})
	sort.Slice(index.Families, func(i, j int) bool {
		return externalCallFamilyKey(index.Families[i]) < externalCallFamilyKey(index.Families[j])
	})
	sort.Slice(index.Frontiers, func(i, j int) bool {
		return index.Frontiers[i].CallerID < index.Frontiers[j].CallerID
	})
	sort.Slice(index.PackageFrontiers, func(i, j int) bool {
		return externalCallPackageFrontierKey(index.PackageFrontiers[i]) <
			externalCallPackageFrontierKey(index.PackageFrontiers[j])
	})
}

func externalCallCoverage(index ExternalCallIndex) ExternalCallIndexCoverage {
	coverage := ExternalCallIndexCoverage{
		PackagesIndexed: len(index.Packages), CallersIndexed: len(index.Callers),
		FamiliesIndexed: len(index.Families),
	}
	for _, family := range index.Families {
		if family.Dispatch == ExternalCallStatic {
			coverage.ExternalStaticWitnesses += family.WitnessCount
		} else if family.Dispatch == ExternalCallInterfaceInvoke {
			coverage.ExternalInterfaceInvokeWitnesses += family.WitnessCount
		}
		coverage.RepresentativeCallsites += len(family.Callsites)
		coverage.RepresentativeCallsitesOmitted += family.CallsitesOmitted
	}
	for _, frontier := range index.Frontiers {
		coverage.DynamicInvokesExcluded += frontier.DynamicInvokesExcluded
		coverage.NonStaticCallsExcluded += frontier.NonStaticCallsExcluded
		coverage.UnnamedStaticCalleesExcluded += frontier.UnnamedStaticCalleesExcluded
		coverage.InvalidCallsitesExcluded += frontier.InvalidCallsitesExcluded
	}
	for _, frontier := range index.PackageFrontiers {
		coverage.SyntheticCallerWitnessesExcluded +=
			frontier.SyntheticCallerWitnessesExcluded
		coverage.InvalidCallerWitnessesExcluded +=
			frontier.InvalidCallerWitnessesExcluded
	}
	return coverage
}

func validExternalCallTarget(value ExternalCallTarget) bool {
	return externalCallPlain(value.PackagePath) && externalCallPlain(value.Name) &&
		(value.Receiver == "" || externalCallPlain(value.Receiver))
}

func validExternalCallFrontier(value ExternalCallFrontier) bool {
	counts := []int{
		value.DynamicInvokesExcluded, value.NonStaticCallsExcluded,
		value.UnnamedStaticCalleesExcluded, value.InvalidCallsitesExcluded,
	}
	positive := false
	for _, count := range counts {
		if count < 0 {
			return false
		}
		positive = positive || count > 0
	}
	return positive
}

func validExternalCallPackageFrontier(value ExternalCallPackageFrontier) bool {
	if value.SyntheticCallerWitnessesExcluded < 0 || value.InvalidCallerWitnessesExcluded < 0 {
		return false
	}
	return value.SyntheticCallerWitnessesExcluded > 0 || value.InvalidCallerWitnessesExcluded > 0
}

func externalCallPlain(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func externalCallPackageKey(value ExternalCallPackage) string {
	return value.ModuleID + "\x00" + value.PackagePath
}

func externalCallPackageFrontierKey(value ExternalCallPackageFrontier) string {
	return value.ModuleID + "\x00" + value.PackagePath
}

func externalCallFamilyID(
	callerID string,
	target ExternalCallTarget,
	dispatch ExternalCallDispatch,
	invocation DirectCallInvocation,
) string {
	return stableDirectCallID(
		"external-call-family", callerID, target.PackagePath, target.Receiver,
		target.Name, string(dispatch), string(invocation),
	)
}

func externalCallFamilyKey(value ExternalCallFamily) string {
	return value.CallerID + "\x00" + value.Target.PackagePath + "\x00" +
		value.Target.Receiver + "\x00" + value.Target.Name + "\x00" +
		string(value.Dispatch) + "\x00" + string(value.Invocation) + "\x00" + value.ID
}

func externalCallIndexSHA256(index ExternalCallIndex) (string, error) {
	copyIndex := index.Snapshot()
	copyIndex.SHA256 = ""
	encoded, err := json.Marshal(copyIndex)
	if err != nil {
		return "", fmt.Errorf("external call index: encode identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validExternalCallSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
