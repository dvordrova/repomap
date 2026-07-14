// Package assemble adapts orientation candidates and command traces into
// executable local flow-proof sessions. The proof model itself stays unaware
// of model/provider and Go collector contracts.
package assemble

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analyzer/golang/gotypes"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
)

// Input is the explicit local proof boundary. CommandTraces must come from the
// locally authorized deterministic snapshot, never a provider-filtered copy.
// ProofBudget is independent from every provider context budget.
type Input struct {
	CommandTraces []gofacts.CommandTrace
	Surfaces      []surfacediscovery.TriggerRecord
	ProofBudget   flowproof.Budget
}

func Attach(ctx context.Context, repoPath string, flows []flowexplain.CandidateFlow, input Input) []string {
	traces := input.CommandTraces
	budget := input.ProofBudget
	if budget.MaxTasks <= 0 {
		budget = flowproof.DefaultBudget()
	}
	executor := gotypes.NewExecutor()
	usedTraces := make(map[int]struct{})
	usedUnavailableProcessEntries := make(map[string]struct{})
	var warnings []string
	for index := range flows {
		flow := &flows[index]
		flow.LocalProof = nil
		if looksLikeCLI(*flow, traces) {
			traceIndex, trace, ok := traceForFlow(*flow, traces, usedTraces)
			if ok {
				usedTraces[traceIndex] = struct{}{}
				if trace.Version != gofacts.CommandTraceVersion {
					warnings = append(warnings, fmt.Sprintf(
						"local proof %q skipped incompatible command trace version %d; need %d",
						flow.Name, trace.Version, gofacts.CommandTraceVersion,
					))
					continue
				}
				seed := seedForFlow(*flow, trace)
				proof := flowproof.BuildCLI(seed)
				session := flowproof.Start(proof, budget, seed.ScenarioID, seed.CollectorVersion)
				if err := flowproof.Run(ctx, repoPath, &session, executor); err != nil {
					warnings = append(warnings, fmt.Sprintf("local proof %q stopped: %v", flow.Name, err))
				}
				flow.LocalProof = &session
				continue
			}
		}
		if excludesProcessProof(flow.CandidateBasis) {
			continue
		}
		seed, ok := processSeedForFlow(*flow, input.Surfaces)
		if !ok {
			continue
		}
		if processEntryUnavailable(input.Surfaces, seed.SeedSurfaceID) {
			if _, used := usedUnavailableProcessEntries[seed.SeedSurfaceID]; used {
				continue
			}
			usedUnavailableProcessEntries[seed.SeedSurfaceID] = struct{}{}
		}
		proof := flowproof.BuildProcess(seed)
		session := flowproof.Start(proof, budget, seed.ScenarioID, seed.CollectorVersion)
		flow.LocalProof = &session
	}
	return warnings
}

func processEntryUnavailable(surfaces []surfacediscovery.TriggerRecord, id string) bool {
	for _, surface := range surfaces {
		if surface.ID == id {
			return surface.Availability == surfacediscovery.AvailabilityUnavailable
		}
	}
	return false
}

func excludesProcessProof(basis string) bool {
	return basis == flowexplain.CandidateBasisSourceSignalAggregate ||
		basis == flowexplain.CandidateBasisRuntimeActivity
}

func processSeedForFlow(
	flow flowexplain.CandidateFlow,
	surfaces []surfacediscovery.TriggerRecord,
) (flowproof.ProcessSeed, bool) {
	if strings.TrimSpace(flow.LikelyEntrypoint) == "" {
		return flowproof.ProcessSeed{}, false
	}
	entries := make([]surfacediscovery.TriggerRecord, 0, 1)
	for _, surface := range surfaces {
		if surface.Kind != "process_entry" || surface.SurfaceRole != surfacediscovery.SurfaceRoleEntrySurface ||
			surface.TraceReadiness != surfacediscovery.TraceReadinessPartial ||
			surface.Resolution != "exact" || surface.ProvisionalID ||
			surface.ProcessEntrypoint.Location.Path != flow.LikelyEntrypoint {
			continue
		}
		entries = append(entries, surface)
	}
	if len(entries) != 1 {
		return flowproof.ProcessSeed{}, false
	}
	entry := entries[0]
	seed := flowproof.ProcessSeed{
		FlowID: flowexplain.GenerateFlowID(flow.Name), Goal: flow.Name,
		SeedSurfaceID: entry.ID, ScenarioID: entry.ScenarioID,
		CollectorVersion: flowproof.SurfaceCollectorVersion,
		Entrypoint:       staticSurfaceFact(entry),
		CurrentFrontier:  "downstream runtime handoff from the exact process entry remains unresolved",
	}
	if entry.Availability == surfacediscovery.AvailabilityUnavailable {
		seed.CurrentFrontier = "typed downstream closure is unavailable under the recorded build scenario"
	}
	if support, ok := supportingSurfaceForFlow(flow, entry, surfaces); ok {
		fact := staticSurfaceFact(support)
		seed.Supporting = &fact
		seed.CurrentFrontier = "runtime ordering and handoff from process entry to static " +
			strings.ReplaceAll(support.Kind, "_", " ") + " remains unresolved"
	}
	return seed, true
}

func supportingSurfaceForFlow(
	flow flowexplain.CandidateFlow,
	entry surfacediscovery.TriggerRecord,
	surfaces []surfacediscovery.TriggerRecord,
) (surfacediscovery.TriggerRecord, bool) {
	candidates := make([]surfacediscovery.TriggerRecord, 0)
	for _, surface := range surfaces {
		if surface.ID == entry.ID || surface.OwningExecutable == "" ||
			surface.OwningExecutable != entry.OwningExecutable ||
			surface.Availability != surfacediscovery.AvailabilityAvailable ||
			surface.SurfaceRole != surfacediscovery.SurfaceRoleEntrySurface ||
			surface.ApplicationClass == surfacediscovery.SupportingDependencyBehavior ||
			surface.ApplicationClass == surfacediscovery.DependencyOnly {
			continue
		}
		safeRoute := surface.Kind == "http_route" && surface.TraceReadiness == surfacediscovery.TraceReadinessReady
		safeStart := surface.Kind == "http_server" && surface.TraceReadiness == surfacediscovery.TraceReadinessPartial
		if safeRoute || safeStart {
			candidates = append(candidates, surface)
		}
	}
	if len(candidates) == 0 {
		return surfacediscovery.TriggerRecord{}, false
	}
	preferredKind := "http_route"
	if flow.FlowType == flowexplain.FlowTypeOperational {
		preferredKind = "http_server"
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftPreferred := candidates[i].Kind == preferredKind
		rightPreferred := candidates[j].Kind == preferredKind
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0], true
}

func staticSurfaceFact(surface surfacediscovery.TriggerRecord) flowproof.StaticSurfaceFact {
	location := surface.RegistrationSite
	if surface.Kind == "process_entry" {
		location = surface.ProcessEntrypoint.Location
	} else if surface.Kind == "http_route_descriptor" && surface.DescriptorSite != nil {
		location = *surface.DescriptorSite
	} else if surface.Kind == "http_server" && surface.ServerStartSite != nil {
		location = *surface.ServerStartSite
	}
	label := strings.TrimSpace(surface.Identity.Method + " " + surface.Identity.Path.Text)
	if surface.Identity.Name != "" {
		label = surface.Identity.Name
	}
	if label == "" {
		label = surface.Kind
	}
	fact := flowproof.StaticSurfaceFact{
		ID: surface.ID, Kind: surface.Kind, Label: label,
		QualifiedName: surface.ProcessEntrypoint.ID, Location: surfaceEvidenceLocation(location),
		Direct: len(surface.WrapperChain) == 0 &&
			surface.Quality.Reachability == surfacediscovery.SurfaceQualityStatic,
	}
	if surface.Handler.Known {
		fact.Handler = surface.Handler.Text
		if surface.Kind != "process_entry" {
			fact.QualifiedName = surface.Handler.Text
		}
	}
	if surface.Quality.Reachability == surfacediscovery.SurfaceQualityStatic {
		for _, wrapper := range surface.WrapperChain {
			fact.Wrappers = append(fact.Wrappers, flowproof.StaticWrapperFact{
				ID: wrapper.Symbol.ID, Label: wrapper.Symbol.Name, QualifiedName: wrapper.Symbol.ID,
				Location: surfaceEvidenceLocation(wrapper.Symbol.Location),
				Callsite: surfaceEvidenceLocation(wrapper.Callsite),
			})
		}
	}
	return fact
}

func surfaceEvidenceLocation(location surfacediscovery.Location) evidence.Location {
	if filepath.IsAbs(location.Path) || strings.ContainsRune(location.Path, '\\') ||
		location.Path == "." || !fs.ValidPath(location.Path) || location.Line <= 0 {
		return evidence.Location{}
	}
	return evidence.Location{Path: location.Path, Line: location.Line, Column: location.Column}
}

// BuildDescriptorProof is a bounded unit-level adapter for exact descriptor
// records. It intentionally does not participate in automatic process matching.
func BuildDescriptorProof(
	flowID, goal string,
	surface surfacediscovery.TriggerRecord,
) (flowproof.Proof, bool) {
	if surface.Kind != "http_route_descriptor" || surface.SurfaceRole != surfacediscovery.SurfaceRoleDescriptor ||
		surface.TraceReadiness != surfacediscovery.TraceReadinessPartial ||
		surface.Availability != surfacediscovery.AvailabilityAvailable || surface.DescriptorSite == nil {
		return flowproof.Proof{}, false
	}
	frontier := "runtime descriptor consumer registration remains unresolved"
	for _, item := range surface.DynamicFrontier {
		if item.Kind == "route_provider_dispatch_candidate" && strings.TrimSpace(item.Detail) != "" {
			frontier = item.Detail
			break
		}
	}
	return flowproof.BuildDescriptor(flowproof.DescriptorSeed{
		FlowID: flowID, Goal: goal, SeedSurfaceID: surface.ID,
		Descriptor: staticSurfaceFact(surface), ConsumerFrontier: frontier,
	}), true
}

func seedForFlow(flow flowexplain.CandidateFlow, trace gofacts.CommandTrace) flowproof.CLISeed {
	seed := flowproof.CLISeed{
		FlowID:              flowexplain.GenerateFlowID(flow.Name),
		Goal:                flow.Name,
		Command:             trace.Command,
		Framework:           trace.Framework,
		CollectorVersion:    flowproof.CLICollectorVersion,
		ScenarioID:          "go-default:" + trace.EntrypointPackage,
		Steps:               make([]flowproof.CLIStep, 0, len(trace.Steps)),
		Calls:               make([]flowproof.CLICall, 0, len(trace.HandlerCalls)),
		ConcurrentLifecycle: concurrentLifecycleFact(trace.Concurrency),
	}
	seed.SeedSurfaceID, _, _ = gofacts.CommandSurfaceIdentity(trace)
	for _, step := range trace.Steps {
		seed.Steps = append(seed.Steps, flowproof.CLIStep{
			Symbol: step.Symbol, Relation: step.Relation,
			CallsiteLocation: cloneLocation(step.CallsiteLocation), TargetLocation: step.TargetLocation,
		})
	}
	for _, call := range trace.HandlerCalls {
		seed.Calls = append(seed.Calls, flowproof.CLICall{
			Symbol: call.Symbol, Path: call.Path, Line: call.Line, Relation: call.Relation,
			Condition: cloneCondition(call.Condition), Resolved: call.Resolved,
			TargetPath: call.TargetPath, TargetLine: call.TargetLine,
		})
	}
	return seed
}

func concurrentLifecycleFact(scope gofacts.ConcurrencyScope) flowproof.ConcurrentLifecycleFact {
	fact := flowproof.ConcurrentLifecycleFact{
		Provenance: []evidence.Provenance{{
			Provider: "go_syntax", Version: flowproof.CLICollectorVersion,
			Operation: "inspect_handler_concurrency", Detail: string(scope),
		}},
	}
	switch scope {
	case gofacts.ConcurrencyPresentInHandler:
		fact.Presence = flowproof.ConcurrentLifecyclePresent
	case gofacts.ConcurrencyAbsentFromHandlerScope:
		fact.Presence = flowproof.ConcurrentLifecycleAbsent
	default:
		fact.Presence = flowproof.ConcurrentLifecycleUnknown
	}
	return fact
}

func cloneCondition(condition *evidence.Condition) *evidence.Condition {
	if condition == nil {
		return nil
	}
	copy := *condition
	return &copy
}

func cloneLocation(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	copy := *location
	return &copy
}

func looksLikeCLI(flow flowexplain.CandidateFlow, traces []gofacts.CommandTrace) bool {
	text := strings.ToLower(strings.Join([]string{flow.Name, flow.Trigger, flow.WhyInteresting}, " "))
	for _, word := range []string{"cli", "command", "subcommand", "cobra"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	for _, trace := range traces {
		if len(trace.Steps) > 0 && trace.Steps[0].TargetLocation.Path == flow.LikelyEntrypoint {
			return true
		}
	}
	return false
}

func traceForFlow(flow flowexplain.CandidateFlow, traces []gofacts.CommandTrace, used map[int]struct{}) (int, gofacts.CommandTrace, bool) {
	text := strings.ToLower(strings.Join([]string{flow.Name, flow.Trigger, flow.WhyInteresting}, " "))
	for index, trace := range traces {
		if _, alreadyUsed := used[index]; alreadyUsed {
			continue
		}
		command := strings.ToLower(strings.TrimSpace(trace.Command))
		if command != "" && containsToken(text, command) {
			return index, trace, true
		}
	}
	for index, trace := range traces {
		if _, alreadyUsed := used[index]; alreadyUsed || len(trace.Steps) == 0 {
			continue
		}
		if trace.Steps[0].TargetLocation.Path == flow.LikelyEntrypoint {
			return index, trace, true
		}
	}
	return 0, gofacts.CommandTrace{}, false
}

func containsToken(text, token string) bool {
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if part == token {
			return true
		}
	}
	return false
}
