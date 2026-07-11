// Package assemble adapts orientation candidates and command traces into
// executable local flow-proof sessions. The proof model itself stays unaware
// of model/provider and Go collector contracts.
package assemble

import (
	"context"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/analyzer/golang/gotypes"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func Attach(ctx context.Context, repoPath string, flows []flowexplain.CandidateFlow, traces []gofacts.CommandTrace) []string {
	executor := gotypes.NewExecutor()
	usedTraces := make(map[int]struct{})
	var warnings []string
	for index := range flows {
		flow := &flows[index]
		flow.LocalProof = nil
		if !looksLikeCLI(*flow, traces) {
			continue
		}
		traceIndex, trace, ok := traceForFlow(*flow, traces, usedTraces)
		if !ok {
			continue
		}
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
		session := flowproof.Start(proof, flowproof.DefaultBudget(), seed.ScenarioID, seed.CollectorVersion)
		if err := flowproof.Run(ctx, repoPath, &session, executor); err != nil {
			warnings = append(warnings, fmt.Sprintf("local proof %q stopped: %v", flow.Name, err))
		}
		flow.LocalProof = &session
	}
	return warnings
}

func seedForFlow(flow flowexplain.CandidateFlow, trace gofacts.CommandTrace) flowproof.CLISeed {
	seed := flowproof.CLISeed{
		FlowID:           flowexplain.GenerateFlowID(flow.Name),
		Goal:             flow.Name,
		Command:          trace.Command,
		Framework:        trace.Framework,
		CollectorVersion: flowproof.CLICollectorVersion,
		ScenarioID:       "go-default:" + trace.EntrypointPackage,
		Steps:            make([]flowproof.CLIStep, 0, len(trace.Steps)),
		Calls:            make([]flowproof.CLICall, 0, len(trace.HandlerCalls)),
	}
	if trace.Concurrency == gofacts.ConcurrencyAbsentFromHandlerScope {
		seed.NotApplicableSlots = map[flowproof.SlotKind]flowproof.ApplicabilityReason{
			flowproof.SlotConcurrency: flowproof.ApplicabilityNoConcurrentLifecycleInScope,
		}
	}
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
