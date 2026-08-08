package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	mechanismStudyExperimentUsage = "usage: repomap dev mechanism-study-experiment --repo <repo> --root-path <path> --root-line <line> --root-symbol <exact-symbol> --label <label> --question <question> --out <directory> [--request-only]"

	mechanismStudyExperimentSummaryFile   = "summary.json"
	mechanismStudyExperimentBundleFile    = "provider-bundle.json"
	mechanismStudyExperimentEnvelopeFile  = "provider-envelope.json"
	mechanismStudyExperimentResponseFile  = "raw-response.json"
	mechanismStudyExperimentValidatedFile = "validated-result.json"
)

type mechanismStudyExperimentProvider interface {
	MechanismStudyPromptJSON(mechanismstudy.Prompt) ([]byte, error)
	MechanismStudyBodyMeasured(context.Context, []byte) (modelresearch.ProviderResult, error)
}

type mechanismStudyExperimentDependencies struct {
	analyzeContext    func(context.Context, surfacediscovery.Options) (surfacediscovery.Result, error)
	captureRepository func(context.Context, string) (freshness.RepositoryState, error)
	newPromptProvider func(io.Writer) (mechanismStudyExperimentProvider, error)
	newLiveProvider   func(io.Writer) (mechanismStudyExperimentProvider, error)
}

type mechanismStudyExperimentOptions struct {
	Repo        string
	RootPath    string
	RootLine    int
	RootSymbol  string
	Label       string
	Question    string
	Out         string
	RequestOnly bool
}

type mechanismStudyExperimentSummary struct {
	Version                       int                                          `json:"version"`
	Mode                          string                                       `json:"mode"`
	Outcome                       mechanismstudy.OutcomeState                  `json:"outcome"`
	RepositoryRevision            string                                       `json:"repository_revision"`
	RepositoryFreshnessSHA256     string                                       `json:"repository_freshness_sha256"`
	DirectCallIndexSHA256         string                                       `json:"direct_call_index_sha256"`
	DirectCallIndexState          surfacediscovery.DirectCallIndexState        `json:"direct_call_index_state"`
	DirectCallIndexClosedReason   surfacediscovery.DirectCallIndexClosedReason `json:"direct_call_index_closed_reason,omitempty"`
	Scenario                      surfacediscovery.Scenario                    `json:"scenario"`
	CatalogRef                    string                                       `json:"catalog_ref"`
	CatalogSHA256                 string                                       `json:"catalog_sha256"`
	Cards                         int                                          `json:"cards"`
	ResolvedReadings              int                                          `json:"resolved_readings"`
	AdvertisedNodes               int                                          `json:"advertised_nodes"`
	AdvertisedEdges               int                                          `json:"advertised_edges"`
	FrontierRecords               int                                          `json:"frontier_records"`
	RequestCount                  int                                          `json:"request_count"`
	RequestSHA256                 string                                       `json:"request_sha256,omitempty"`
	ProviderEnvelopeSHA256        string                                       `json:"provider_envelope_sha256,omitempty"`
	RawResponseSHA256             string                                       `json:"raw_response_sha256,omitempty"`
	ProviderCalls                 int                                          `json:"provider_calls"`
	ProviderAttempts              int                                          `json:"provider_attempts"`
	ProviderAttemptedRequestBytes int                                          `json:"provider_attempted_request_bytes"`
	ProviderResponseBytes         int                                          `json:"provider_response_bytes"`
	PreparedCards                 int                                          `json:"prepared_cards"`
	MechanismCards                int                                          `json:"mechanism_cards"`
}

type mechanismStudyExperimentValidated struct {
	Version        int                         `json:"version"`
	PromptVersion  string                      `json:"prompt_version"`
	CatalogRef     string                      `json:"catalog_ref"`
	CatalogSHA256  string                      `json:"catalog_sha256"`
	Cards          []mechanismstudy.CardResult `json:"cards"`
	RequestResults []mechanismstudy.Result     `json:"request_results,omitempty"`
}

func runMechanismStudyExperimentCLI(args []string, stdout, stderr io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runMechanismStudyExperimentCLIWith(
		ctx, args, stdout, stderr, defaultMechanismStudyExperimentDependencies(),
	)
}

func defaultMechanismStudyExperimentDependencies() mechanismStudyExperimentDependencies {
	return mechanismStudyExperimentDependencies{
		analyzeContext:    surfacediscovery.AnalyzeContext,
		captureRepository: freshness.CaptureRepository,
		newPromptProvider: func(io.Writer) (mechanismStudyExperimentProvider, error) {
			return deepseek.NewPromptFromEnv()
		},
		newLiveProvider: func(stderr io.Writer) (mechanismStudyExperimentProvider, error) {
			client, err := deepseek.NewFromEnv()
			if err != nil {
				return nil, err
			}
			client.OnWait = func(progress deepseek.WaitProgress) {
				fmt.Fprintf(
					stderr,
					"repomap: %s still running after %s (Ctrl-C to cancel)\n",
					progress.Stage,
					progress.Elapsed.Round(time.Second),
				)
			}
			return client, nil
		},
	}
}

func runMechanismStudyExperimentCLIWith(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	deps mechanismStudyExperimentDependencies,
) error {
	if stdout == nil || stderr == nil {
		return fmt.Errorf("mechanism study experiment: stdout and stderr are required")
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, mechanismStudyExperimentUsage)
		return nil
	}
	options, err := parseMechanismStudyExperimentOptions(args)
	if err != nil {
		return err
	}
	if ctx == nil {
		return fmt.Errorf("mechanism study experiment: context is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("mechanism study experiment: %w", err)
	}
	if deps.analyzeContext == nil || deps.captureRepository == nil ||
		deps.newPromptProvider == nil || deps.newLiveProvider == nil {
		return fmt.Errorf("mechanism study experiment: dependencies are incomplete")
	}

	repo, err := resolveAnalysisRoot(options.Repo)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: %w", err)
	}
	out, err := filepath.Abs(options.Out)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: resolve output directory: %w", err)
	}
	out = filepath.Clean(out)
	if err := requireMechanismStudyExperimentOutputAbsent(out); err != nil {
		return err
	}

	before, err := deps.captureRepository(ctx, repo)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: capture repository before analysis: %w", err)
	}
	beforeDigest, err := mechanismStudyRepositoryDigest(before)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: validate repository before analysis: %w", err)
	}
	analysis, err := deps.analyzeContext(ctx, surfacediscovery.DefaultOptions(repo))
	if err != nil {
		return fmt.Errorf("mechanism study experiment: analyze direct-call substrate: %w", err)
	}
	after, err := deps.captureRepository(ctx, repo)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: capture repository after analysis: %w", err)
	}
	afterDigest, err := mechanismStudyRepositoryDigest(after)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: validate repository after analysis: %w", err)
	}
	if before.Identity != after.Identity || before.Head != after.Head || beforeDigest != afterDigest {
		return fmt.Errorf("mechanism study experiment: repository changed during analysis")
	}

	compilation, err := mechanismstudy.CompileContexts(
		[]mechanismstudy.ExactContext{{
			Label: options.Label, Question: options.Question,
			Readings: []mechanismstudy.ExactReading{{
				Label: options.Label, Path: options.RootPath,
				Line: options.RootLine, Symbol: options.RootSymbol,
			}},
		}},
		analysis.DirectCallIndex,
		mechanismstudy.RepositoryBinding{
			RepositoryRevision: after.Head, RepositoryFreshnessSHA256: afterDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: compile exact context: %w", err)
	}
	preparedCards, err := mechanismstudy.PreparedCards(compilation)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: prepare result: %w", err)
	}
	batches, err := mechanismstudy.BuildRequestBatches(compilation)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: build request batches: %w", err)
	}
	if len(batches) > 1 {
		return fmt.Errorf("mechanism study experiment: one explicit context produced %d request batches", len(batches))
	}

	validated := mechanismStudyExperimentValidated{
		Version: mechanismstudy.ResultVersion, PromptVersion: mechanismstudy.PromptVersion,
		CatalogRef: compilation.CatalogRef, CatalogSHA256: compilation.CatalogSHA256,
		Cards: append([]mechanismstudy.CardResult(nil), preparedCards...),
	}
	summary := newMechanismStudyExperimentSummary(compilation, analysis.DirectCallIndex, after, afterDigest)
	summary.RequestCount = len(batches)
	artifacts := make(map[string][]byte, 5)

	if len(batches) == 1 {
		batch := batches[0]
		bundle, err := mechanismstudy.ProviderVisibleJSON(batch)
		if err != nil {
			return fmt.Errorf("mechanism study experiment: restore provider-visible bundle: %w", err)
		}
		if err := scanMechanismStudyExperimentBytes("provider-visible bundle", bundle); err != nil {
			return err
		}
		prompt, err := mechanismstudy.BuildPrompt(batch)
		if err != nil {
			return fmt.Errorf("mechanism study experiment: build prompt: %w", err)
		}
		providerFactory := deps.newLiveProvider
		if options.RequestOnly {
			providerFactory = deps.newPromptProvider
		}
		provider, err := providerFactory(stderr)
		if err != nil {
			return fmt.Errorf("mechanism study experiment: configure provider: %w", err)
		}
		envelope, err := provider.MechanismStudyPromptJSON(prompt)
		if err != nil {
			return fmt.Errorf("mechanism study experiment: build exact provider envelope: %w", err)
		}
		if err := scanMechanismStudyExperimentBytes("provider envelope", envelope); err != nil {
			return err
		}
		artifacts[mechanismStudyExperimentBundleFile] = bundle
		artifacts[mechanismStudyExperimentEnvelopeFile] = envelope
		summary.RequestSHA256 = batch.WireSHA256
		summary.ProviderEnvelopeSHA256 = mechanismStudyExperimentSHA256(envelope)

		if options.RequestOnly {
			summary.Mode = "request_only"
		} else {
			providerResult, err := provider.MechanismStudyBodyMeasured(ctx, envelope)
			if err != nil {
				return fmt.Errorf("mechanism study experiment: provider call: %w", err)
			}
			if err := scanMechanismStudyExperimentBytes("raw provider response", providerResult.Content); err != nil {
				return err
			}
			resolved, err := mechanismstudy.ResolveResponse(compilation, batch, providerResult.Content)
			if err != nil {
				return fmt.Errorf("mechanism study experiment: validate provider response: %w", err)
			}
			validated.RequestResults = []mechanismstudy.Result{resolved}
			mergeMechanismStudyExperimentCards(validated.Cards, resolved.Cards)
			artifacts[mechanismStudyExperimentResponseFile] = append([]byte(nil), providerResult.Content...)
			summary.Mode = "provider"
			summary.RawResponseSHA256 = mechanismStudyExperimentSHA256(providerResult.Content)
			summary.ProviderCalls = 1
			summary.ProviderAttempts = providerResult.Attempts
			summary.ProviderAttemptedRequestBytes = providerResult.RequestBytes
			summary.ProviderResponseBytes = providerResult.ResponseBytes
		}
	} else {
		summary.Mode = "provider_skipped"
	}

	setMechanismStudyExperimentOutcomes(&summary, validated.Cards)
	validatedJSON, err := marshalMechanismStudyExperimentArtifact(validated)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: encode validated result: %w", err)
	}
	artifacts[mechanismStudyExperimentValidatedFile] = validatedJSON
	summaryJSON, err := marshalMechanismStudyExperimentArtifact(summary)
	if err != nil {
		return fmt.Errorf("mechanism study experiment: encode summary: %w", err)
	}
	artifacts[mechanismStudyExperimentSummaryFile] = summaryJSON
	if err := publishMechanismStudyExperimentArtifacts(out, artifacts); err != nil {
		return err
	}

	fmt.Fprintf(
		stdout,
		"out: %s\noutcome: %s\nrequest_count: %d\nprovider_calls: %d\n",
		out, summary.Outcome, summary.RequestCount, summary.ProviderCalls,
	)
	return nil
}

func parseMechanismStudyExperimentOptions(args []string) (mechanismStudyExperimentOptions, error) {
	flags := flag.NewFlagSet("mechanism-study-experiment", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options mechanismStudyExperimentOptions
	flags.StringVar(&options.Repo, "repo", "", "repository to analyze")
	flags.StringVar(&options.RootPath, "root-path", "", "repository-relative exact root path")
	flags.IntVar(&options.RootLine, "root-line", 0, "exact declaration or containing-function line")
	flags.StringVar(&options.RootSymbol, "root-symbol", "", "exact symbol ID or equivalent ID")
	flags.StringVar(&options.Label, "label", "", "bounded context label")
	flags.StringVar(&options.Question, "question", "", "bounded mechanism question")
	flags.StringVar(&options.Out, "out", "", "new output directory")
	flags.BoolVar(&options.RequestOnly, "request-only", false, "save exact request without a provider call")
	if err := flags.Parse(args); err != nil {
		return mechanismStudyExperimentOptions{}, fmt.Errorf("%s: %w", mechanismStudyExperimentUsage, err)
	}
	options.Repo = strings.TrimSpace(options.Repo)
	options.RootPath = filepath.ToSlash(strings.TrimSpace(options.RootPath))
	options.RootSymbol = strings.TrimSpace(options.RootSymbol)
	options.Label = strings.TrimSpace(options.Label)
	options.Question = strings.TrimSpace(options.Question)
	options.Out = strings.TrimSpace(options.Out)
	if flags.NArg() != 0 || options.Repo == "" || options.RootPath == "" ||
		options.RootLine <= 0 || options.RootSymbol == "" || options.Label == "" ||
		options.Question == "" || options.Out == "" {
		return mechanismStudyExperimentOptions{}, fmt.Errorf("%s", mechanismStudyExperimentUsage)
	}
	if options.RootPath == "." || !fs.ValidPath(options.RootPath) || strings.ContainsRune(options.RootPath, '\\') {
		return mechanismStudyExperimentOptions{}, fmt.Errorf("mechanism study experiment: --root-path must be a clean repository-relative slash path")
	}
	return options, nil
}

func mechanismStudyRepositoryDigest(state freshness.RepositoryState) (string, error) {
	if err := state.Validate(); err != nil {
		return "", err
	}
	return state.Digest()
}

func newMechanismStudyExperimentSummary(
	compilation *mechanismstudy.Compilation,
	index *surfacediscovery.DirectCallIndex,
	repository freshness.RepositoryState,
	freshnessSHA256 string,
) mechanismStudyExperimentSummary {
	summary := mechanismStudyExperimentSummary{
		Version: 1, RepositoryRevision: repository.Head,
		RepositoryFreshnessSHA256: freshnessSHA256,
		CatalogRef:                compilation.CatalogRef, CatalogSHA256: compilation.CatalogSHA256,
		Scenario: compilation.Scenario, Cards: len(compilation.Cards),
		Outcome: mechanismstudy.OutcomePrepared,
	}
	if index != nil {
		summary.DirectCallIndexSHA256 = index.SHA256
		summary.DirectCallIndexState = index.State
		summary.DirectCallIndexClosedReason = index.ClosedReason
	}
	for _, card := range compilation.Cards {
		summary.AdvertisedNodes += len(card.Nodes)
		summary.AdvertisedEdges += len(card.Edges)
		summary.FrontierRecords += len(card.Frontier)
		for _, reading := range card.Readings {
			if reading.RootNodeRef != "" {
				summary.ResolvedReadings++
			}
		}
	}
	return summary
}

func mergeMechanismStudyExperimentCards(final, resolved []mechanismstudy.CardResult) {
	positions := make(map[string]int, len(final))
	for position, card := range final {
		positions[card.CardRef] = position
	}
	for _, card := range resolved {
		if position, ok := positions[card.CardRef]; ok {
			final[position] = card
		}
	}
}

func setMechanismStudyExperimentOutcomes(
	summary *mechanismStudyExperimentSummary,
	cards []mechanismstudy.CardResult,
) {
	if summary == nil {
		return
	}
	summary.Outcome = mechanismstudy.OutcomePrepared
	for _, card := range cards {
		switch card.State {
		case mechanismstudy.OutcomeMechanism:
			summary.MechanismCards++
			summary.Outcome = mechanismstudy.OutcomeMechanism
		default:
			summary.PreparedCards++
		}
	}
}

func marshalMechanismStudyExperimentArtifact(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func scanMechanismStudyExperimentBytes(label string, data []byte) error {
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf(
			"mechanism study experiment: %s failed credential scan (%s)",
			label, secretscan.ClosedKind(kind),
		)
	}
	return nil
}

func mechanismStudyExperimentSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func requireMechanismStudyExperimentOutputAbsent(out string) error {
	if info, err := os.Lstat(out); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("mechanism study experiment: output directory must not be a symlink")
		}
		return fmt.Errorf("mechanism study experiment: refusing to overwrite existing output %s", out)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("mechanism study experiment: inspect output directory: %w", err)
	}
	return nil
}

func publishMechanismStudyExperimentArtifacts(out string, artifacts map[string][]byte) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("mechanism study experiment: no artifacts to publish")
	}
	for name, data := range artifacts {
		if filepath.Base(name) != name || name == "." {
			return fmt.Errorf("mechanism study experiment: invalid artifact name")
		}
		if err := scanMechanismStudyExperimentBytes(name, data); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return fmt.Errorf("mechanism study experiment: create output parent: %w", err)
	}
	if err := os.Mkdir(out, 0o700); err != nil {
		return fmt.Errorf("mechanism study experiment: create exclusive output directory: %w", err)
	}
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(out, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("mechanism study experiment: create %s: %w", name, err)
		}
		data := artifacts[name]
		written, writeErr := file.Write(data)
		if writeErr == nil && written != len(data) {
			writeErr = io.ErrShortWrite
		}
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("mechanism study experiment: write %s: %w", name, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("mechanism study experiment: close %s: %w", name, closeErr)
		}
	}
	return nil
}
