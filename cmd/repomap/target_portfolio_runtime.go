package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type targetPortfolioClient interface {
	TargetPortfolioPromptJSON(targetportfolio.Prompt) ([]byte, error)
	TargetPortfolioBodyMeasured(context.Context, []byte) (modelresearch.ProviderResult, error)
}

type targetPortfolioClientFactory func() (targetPortfolioClient, error)

type targetPortfolioRunOutcome struct {
	SelectedRef         string
	SelectedPath        string
	SelectedKind        analysistarget.Kind
	SelectedTargets     int
	SelectedTargetRefs  []string
	UsedLocalDefault    bool
	FailureCode         string
	Request             []byte
	Response            []byte
	ResponseUnavailable *debugdump.SemanticUnavailable
	RequestProvenance   string
	SemanticState       string
	ValidationCode      string
	SemanticCalls       int
	TransportAttempts   int
	RequestBytes        int
	ResponseBytes       int
	LatencyMillis       int64
}

func defaultTargetPortfolioClientFactory() (targetPortfolioClient, error) {
	return deepseek.NewFromEnv()
}

// selectAllTargetsForRun makes --all-targets an inclusion control only. The
// complete catalog order is retained while default ownership follows the
// explicit override, the ordinary online portfolio selector, or the exact
// offline catalog default in that order.
func selectAllTargetsForRun(
	ctx context.Context,
	repoName string,
	catalog analysistarget.TargetCatalog,
	offline bool,
	override string,
	output *runOutput,
	clients targetPortfolioClientFactory,
) (snapshot.TargetRunSelection, targetPortfolioRunOutcome, error) {
	if err := catalog.Validate(); err != nil {
		return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, err
	}
	refs := make([]string, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		refs = append(refs, entry.Candidate.Target.Ref)
	}
	if len(refs) == 0 {
		return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, fmt.Errorf("--all-targets: no advertised Go targets")
	}

	var outcome targetPortfolioRunOutcome
	defaultRef := ""
	override = strings.TrimSpace(override)
	switch {
	case override != "":
		entry, resolveErr := resolveAllTargetsDefaultOverride(catalog, override)
		if resolveErr != nil {
			return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, resolveErr
		}
		defaultRef = entry.Candidate.Target.Ref
		outcome.SelectedRef = defaultRef
		outcome.SelectedPath = entry.DisplayPath
		outcome.SelectedKind = entry.Candidate.Target.Kind
	case offline:
		defaultRef = catalog.DefaultTargetRef
		defaultEligible := false
		for _, entry := range catalog.Entries {
			if entry.Candidate.Target.Ref == defaultRef && targetportfolio.EligibleForSelection(entry) {
				defaultEligible = true
				break
			}
		}
		if defaultRef == "" || !defaultEligible {
			return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, fmt.Errorf(
				"--all-targets: no eligible exact local default; rerun with --target TARGET; choices: %s",
				targetPortfolioChoices(catalog),
			)
		}
		for _, entry := range catalog.Entries {
			if entry.Candidate.Target.Ref == defaultRef {
				outcome.SelectedRef = defaultRef
				outcome.SelectedPath = entry.DisplayPath
				outcome.SelectedKind = entry.Candidate.Target.Kind
				break
			}
		}
	default:
		var err error
		defaultRef, outcome, err = selectTargetPortfolioForRun(
			ctx, repoName, catalog, output, clients,
		)
		if err != nil {
			return snapshot.TargetRunSelection{}, outcome, err
		}
	}
	outcome.SelectedTargets = len(refs)
	outcome.SelectedTargetRefs = append([]string(nil), refs...)
	return snapshot.TargetRunSelection{
		DefaultTargetRef: defaultRef,
		TargetRefs:       refs,
	}, outcome, nil
}

func selectTargetPortfolioForRun(
	ctx context.Context,
	repoName string,
	catalog analysistarget.TargetCatalog,
	output *runOutput,
	clients targetPortfolioClientFactory,
) (string, targetPortfolioRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if err := catalog.Validate(); err != nil {
		return "", targetPortfolioRunOutcome{}, fmt.Errorf("target portfolio: validate catalog: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", targetPortfolioRunOutcome{}, err
	}
	eligible := make([]analysistarget.TargetCatalogEntry, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if targetportfolio.EligibleForSelection(entry) {
			eligible = append(eligible, entry)
		}
	}
	if len(eligible) == 0 {
		return "", targetPortfolioRunOutcome{}, fmt.Errorf(
			"target portfolio has no executable or module Library API target eligible for an ordinary report; choose one with --target: %s",
			targetPortfolioChoices(catalog),
		)
	}
	if len(eligible) == 1 {
		entry := eligible[0]
		return entry.Candidate.Target.Ref, targetPortfolioRunOutcome{
			SelectedRef: entry.Candidate.Target.Ref, SelectedPath: entry.DisplayPath,
			SelectedKind: entry.Candidate.Target.Kind, SelectedTargets: 1,
			SelectedTargetRefs: []string{entry.Candidate.Target.Ref},
		}, nil
	}

	outcome := targetPortfolioRunOutcome{}
	compilation, err := targetportfolio.Compile(targetPortfolioRepoName(repoName), catalog)
	if err != nil {
		return targetPortfolioFallback(catalog, output, outcome, "request_build_failed")
	}
	prompt, err := targetportfolio.BuildPrompt(compilation)
	if err != nil {
		return targetPortfolioFallback(catalog, output, outcome, "request_build_failed")
	}
	providerBundle, err := targetportfolio.ProviderVisibleJSON(compilation)
	if err != nil {
		return targetPortfolioFallback(catalog, output, outcome, "request_build_failed")
	}
	outcome.Request = providerBundle
	outcome.RequestBytes = len(providerBundle)
	outcome.RequestProvenance = debugdump.SemanticRequestPrepared

	if clients == nil {
		return targetPortfolioFallback(catalog, output, outcome, "provider_configuration_failed")
	}
	client, err := clients()
	if err != nil || client == nil {
		return targetPortfolioFallback(catalog, output, outcome, "provider_configuration_failed")
	}
	envelope, err := client.TargetPortfolioPromptJSON(prompt)
	if err != nil {
		return targetPortfolioFallback(catalog, output, outcome, "request_build_failed")
	}
	outcome.Request = append([]byte(nil), envelope...)
	outcome.RequestBytes = len(envelope)
	if _, found := secretscan.DetectAlways(string(envelope)); found {
		return targetPortfolioFallback(catalog, output, outcome, "request_secret_scan")
	}
	if err := ctx.Err(); err != nil {
		return "", targetPortfolioCanceled(outcome), err
	}

	output.Stage(
		"Analysis target",
		fmt.Sprintf("asking the model to choose from %d exact Go product candidates", len(compilation.Request.Targets)),
	)
	started := time.Now()
	providerResult, callErr := client.TargetPortfolioBodyMeasured(ctx, envelope)
	outcome.LatencyMillis = time.Since(started).Milliseconds()
	outcome.TransportAttempts = providerResult.Attempts
	outcome.ResponseBytes = providerResultResponseBytes(providerResult)
	if providerResult.Attempts > 0 {
		outcome.SemanticCalls = 1
		outcome.RequestProvenance = debugdump.SemanticRequestExactSent
	}
	if callErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			outcome.Response = providerFailureContentForExchange(callErr, providerResult.Content)
			outcome = targetPortfolioCanceled(outcome)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", outcome, ctxErr
			}
			return "", outcome, callErr
		}
		outcome.Response = providerFailureContentForExchange(callErr, providerResult.Content)
		if len(outcome.Response) == 0 {
			outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
				Code: debugdump.SemanticUnavailableNoContent, OriginalBytes: outcome.ResponseBytes,
			}
		}
		if _, found := secretscan.DetectAlways(string(outcome.Response)); found {
			originalBytes := len(outcome.Response)
			outcome.Response = nil
			outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
				Code: debugdump.SemanticUnavailableOmitted, OriginalBytes: originalBytes,
			}
			return targetPortfolioFallback(catalog, output, outcome, "response_secret_scan")
		}
		return targetPortfolioFallback(catalog, output, outcome, "provider_failed")
	}

	outcome.Response = append([]byte(nil), providerResult.Content...)
	if _, found := secretscan.DetectAlways(string(outcome.Response)); found {
		originalBytes := len(outcome.Response)
		outcome.Response = nil
		outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableOmitted, OriginalBytes: originalBytes,
		}
		return targetPortfolioFallback(catalog, output, outcome, "response_secret_scan")
	}
	selection, err := targetportfolio.ResolveResponse(compilation, outcome.Response)
	if err != nil {
		return targetPortfolioFallback(catalog, output, outcome, "response_validation")
	}
	outcome.SelectedRef = selection.Default.Candidate.Target.Ref
	outcome.SelectedPath = selection.Default.DisplayPath
	outcome.SelectedKind = selection.Default.Candidate.Target.Kind
	outcome.SelectedTargets = len(selection.Targets)
	outcome.SelectedTargetRefs = make([]string, 0, len(selection.Targets))
	for _, target := range selection.Targets {
		outcome.SelectedTargetRefs = append(outcome.SelectedTargetRefs, target.Candidate.Target.Ref)
	}
	outcome.SemanticState = debugdump.SemanticStateAccepted
	outcome.ValidationCode = debugdump.SemanticValidationAccepted
	return outcome.SelectedRef, outcome, nil
}

func targetPortfolioRepoName(repositoryIdentity string) string {
	trimmed := strings.Trim(strings.TrimSpace(repositoryIdentity), "/")
	separator := strings.LastIndexByte(trimmed, '/')
	if separator < 0 {
		return trimmed
	}
	last := trimmed[separator+1:]
	if isGoMajorVersionPathSegment(last) {
		trimmed = strings.TrimSuffix(trimmed[:separator], "/")
		if previous := strings.LastIndexByte(trimmed, '/'); previous >= 0 {
			return trimmed[previous+1:]
		}
		return trimmed
	}
	return last
}

func isGoMajorVersionPathSegment(value string) bool {
	if len(value) < 2 || value[0] != 'v' || value[1] == '0' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return len(value) > 2 || value[1] >= '2'
}

func targetPortfolioFallback(
	catalog analysistarget.TargetCatalog,
	output *runOutput,
	outcome targetPortfolioRunOutcome,
	code string,
) (string, targetPortfolioRunOutcome, error) {
	outcome.FailureCode = code
	outcome.SemanticState, outcome.ValidationCode = targetPortfolioFailureSemantics(code)
	for _, entry := range catalog.Entries {
		if entry.Candidate.Target.Ref != catalog.DefaultTargetRef ||
			!targetportfolio.EligibleForSelection(entry) {
			continue
		}
		outcome.SelectedRef = entry.Candidate.Target.Ref
		outcome.SelectedPath = entry.DisplayPath
		outcome.SelectedKind = entry.Candidate.Target.Kind
		outcome.SelectedTargets = 1
		outcome.SelectedTargetRefs = []string{entry.Candidate.Target.Ref}
		outcome.UsedLocalDefault = true
		output.Warn(
			"Target portfolio selection unavailable",
			"reason: "+targetPortfolioFailureCopy(code),
			"using exact local default: "+entry.DisplayPath,
		)
		return outcome.SelectedRef, outcome, nil
	}
	return "", outcome, fmt.Errorf(
		"target portfolio selection unavailable (%s) and there is no exact local default; choose one with --target: %s",
		targetPortfolioFailureCopy(code), targetPortfolioChoices(catalog),
	)
}

func targetPortfolioCanceled(outcome targetPortfolioRunOutcome) targetPortfolioRunOutcome {
	outcome.FailureCode = "canceled"
	outcome.SemanticState = debugdump.SemanticStateCanceled
	outcome.ValidationCode = debugdump.SemanticValidationCanceled
	if len(outcome.Response) == 0 {
		outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableCanceled, OriginalBytes: outcome.ResponseBytes,
		}
	}
	return outcome
}

func targetPortfolioFailureSemantics(code string) (string, string) {
	switch code {
	case "provider_configuration_failed", "provider_failed", "request_build_failed":
		return debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider
	case "request_secret_scan", "response_secret_scan":
		return debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret
	default:
		return debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse
	}
}

func targetPortfolioFailureCopy(code string) string {
	switch code {
	case "provider_configuration_failed":
		return "provider configuration failed"
	case "provider_failed":
		return "provider call failed"
	case "request_build_failed":
		return "bounded request could not be prepared"
	case "request_secret_scan", "response_secret_scan":
		return "mandatory credential scan rejected the exchange"
	case "response_validation":
		return "model response was invalid"
	default:
		return "selection failed"
	}
}

func targetPortfolioChoices(catalog analysistarget.TargetCatalog) string {
	const limit = 12
	choices := make([]string, 0, min(len(catalog.Entries), limit)+1)
	for _, entry := range catalog.Entries {
		if len(choices) == limit {
			break
		}
		choices = append(choices, fmt.Sprintf(
			"%s (%s; %s)", entry.DisplayPath, entry.Candidate.Target.Kind, entry.Candidate.Key,
		))
	}
	if len(catalog.Entries) > len(choices) {
		choices = append(choices, fmt.Sprintf("... and %d more", len(catalog.Entries)-len(choices)))
	}
	return strings.Join(choices, ", ")
}

// resolveAllTargetsDefaultOverride applies the same fail-closed selector
// semantics as ordinary target resolution while preserving --all-targets as
// an inclusion control. Exact refs and typed candidate keys win first. Human
// path aliases are accepted only when they identify one surface: a module-root
// executable and that module's Library API deliberately share a display path,
// so an untyped alias such as "." or "server" must not silently pick one.
func resolveAllTargetsDefaultOverride(
	catalog analysistarget.TargetCatalog,
	override string,
) (analysistarget.TargetCatalogEntry, error) {
	exact := make([]analysistarget.TargetCatalogEntry, 0, 1)
	for _, entry := range catalog.Entries {
		if override == entry.Candidate.Target.Ref || override == entry.Candidate.Key {
			exact = append(exact, entry)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--all-targets: --target %q matches more than one exact target", override,
		)
	}

	aliases := make([]analysistarget.TargetCatalogEntry, 0, 1)
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		match := override == entry.DisplayPath
		if target.Kind == analysistarget.KindModuleLibrary {
			match = match || override == target.ModulePath
		} else {
			match = match || override == target.PackagePath
		}
		if match {
			aliases = append(aliases, entry)
		}
	}
	switch len(aliases) {
	case 1:
		return aliases[0], nil
	case 0:
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--all-targets: --target %q is not an advertised module surface; choose one of: %s",
			override, targetPortfolioChoices(catalog),
		)
	default:
		keys := make([]string, 0, len(aliases))
		for _, entry := range aliases {
			keys = append(keys, entry.Candidate.Key)
		}
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--all-targets: --target %q is ambiguous; use one exact target key: %s",
			override, strings.Join(keys, ", "),
		)
	}
}

func recordTargetPortfolioOutcome(
	runDir string,
	outcome targetPortfolioRunOutcome,
	output *runOutput,
) error {
	if outcome.RequestBytes == 0 || len(outcome.Request) == 0 || outcome.SemanticState == "" {
		return nil
	}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return fmt.Errorf("target portfolio: open semantic exchange writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Target portfolio semantic exchange journal unavailable",
	})
	responseUnavailable := outcome.ResponseUnavailable
	if len(outcome.Response) == 0 && responseUnavailable == nil {
		responseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableNoContent, OriginalBytes: outcome.ResponseBytes,
		}
	}
	writer.RecordSemanticExchange(debugdump.SemanticExchange{
		Stage:           debugdump.SemanticStageTargetPortfolio,
		InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: outcome.RequestProvenance,
		State:             outcome.SemanticState, ValidationCode: outcome.ValidationCode,
		SemanticCalls: outcome.SemanticCalls, TransportAttempts: outcome.TransportAttempts,
		Request: outcome.Request, Response: outcome.Response,
		ResponseUnavailable: responseUnavailable,
	})
	return recordAtlasFirstStageDiagnostic(runDir, targetPortfolioDiagnostic(outcome))
}

func targetPortfolioDiagnostic(outcome targetPortfolioRunOutcome) atlasFirstStageDiagnostic {
	state := "accepted"
	if outcome.UsedLocalDefault {
		state = "rejected"
		if outcome.SemanticState == debugdump.SemanticStateProviderFailed {
			state = "provider_failed"
		}
	}
	return atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageTargetPortfolio, State: state,
		RequestBytes: outcome.RequestBytes, SemanticCalls: outcome.SemanticCalls,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	}
}
