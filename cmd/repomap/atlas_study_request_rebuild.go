package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"reflect"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
)

// runAtlasStudyRequestRebuildCLI re-derives the exact canonical Atlas Study
// request artifact from a copied run that has not yet resolved a provider
// exchange. It is a provider-free dev seam: the request is compiled entirely
// from the persisted Atlas, canonical visible Architecture Canvas and exact
// saved sources, then round-trip validated and published with the same
// exclusive-write safety as atlas-study-response-replay. No provider call and
// no network access happen here.
//
// With --repo the seam reproduces the authority-confirmed, source-covered
// ReportData used by the original run-time producer: the manifest seed is
// verified, the scoped run authority is re-confirmed against the live
// repository, and PrepareAuthorizedSourceCoverage installs the exact fresh
// sources before the request is compiled. This makes the rebuilt request bind
// the same compiled product as render-report --repo, so the deterministic
// mock -> replay -> render pipeline stays self-consistent.
func runAtlasStudyRequestRebuildCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("atlas-study-request-rebuild", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied Atlas Study run")
	repoFlag := flags.String("repo", "", "authoritative repository for the copied run (optional)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" {
		return fmt.Errorf("usage: repomap dev atlas-study-request-rebuild --run-dir <copied-run> [--repo <repo>]")
	}
	if stdout == nil {
		return fmt.Errorf("atlas study request rebuild: stdout is required")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()
	// A request can only be rebuilt for a run that has not yet resolved a
	// provider exchange: a persisted result or status would already bind an
	// exact request identity.
	for _, name := range []string{atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename} {
		if err := requireAtlasStudyReplayOutputAbsent(root, name); err != nil {
			return err
		}
	}
	// Read the language from the existing request artifact field when the
	// caller kept one in the copy; otherwise default to canonical English.
	// The caller deletes the stale request from the copy before rebuilding;
	// the exclusive write below also refuses to overwrite one.
	language := atlasstudy.LanguageEnglish
	if requestRaw, err := readAtlasStudyReplayRootFile(
		root, atlasstudy.RequestArtifactFilename, atlasstudy.MaxRequestArtifactBytes,
	); err == nil {
		var existing struct {
			Language atlasstudy.Language `json:"language"`
		}
		if jsonErr := json.Unmarshal(requestRaw, &existing); jsonErr == nil && existing.Language != "" {
			language = existing.Language
		}
	}
	if err := requireAtlasStudyReplayOutputAbsent(root, atlasstudy.RequestArtifactFilename); err != nil {
		return err
	}

	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: read run: %w", err)
	}
	if *repoFlag != "" {
		prepared, err := prepareAtlasStudyRebuildData(runDir, *repoFlag, data)
		if err != nil {
			return err
		}
		data = prepared
	}
	input, err := report.BuildAtlasStudyInput(data, language)
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: build exact input: %w", err)
	}
	product, err := atlasstudy.Compile(input)
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: compile exact product: %w", err)
	}
	record, err := product.RequestRecord()
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: build request record: %w", err)
	}
	encoded, err := atlasstudy.EncodeRequestRecord(record)
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: encode request record: %w", err)
	}
	decoded, err := atlasstudy.DecodeRequestRecord(encoded)
	if err != nil {
		return fmt.Errorf("atlas study request rebuild: round-trip decode request: %w", err)
	}
	if err := product.ValidateRequestRecord(record); err != nil {
		return fmt.Errorf("atlas study request rebuild: round-trip validation: %w", err)
	}
	if !reflect.DeepEqual(decoded, record) {
		return fmt.Errorf("atlas study request rebuild: round-trip request changed")
	}
	if kind, unsafe := secretscan.DetectAlways(string(encoded)); unsafe {
		return fmt.Errorf(
			"atlas study request rebuild: request contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}

	if err := writeAtlasStudyReplayExclusive(
		root,
		atlasstudy.RequestArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, err := atlasstudy.DecodeRequestRecord(saved)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(decoded, record) {
				return fmt.Errorf("atlas study request rebuild changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("atlas study request rebuild: publish request: %w", err)
	}

	fmt.Fprintf(
		stdout,
		"request_sha256: %s\nspans: %d\nprovider_calls: 0\n",
		atlasStudyReplaySHA256(encoded),
		product.Coverage().SpansSelected,
	)
	return nil
}

// prepareAtlasStudyRebuildData re-reads the copied run against the confirmed
// scoped repository authority and installs the exact authorized source
// coverage, mirroring the run-time producer path in cmd/repomap main.go. The
// manifest seed must match the live repository exactly, and the rebuilt
// request is then compiled from the same ReportData the authorized render
// replays, so request binding is preserved across the provider-free pipeline.
func prepareAtlasStudyRebuildData(runDir, repo string, base *report.ReportData) (*report.ReportData, error) {
	if base == nil {
		return nil, fmt.Errorf("atlas study request rebuild: base run data is required")
	}
	seed, err := report.ReadRunManifestAuthoritySeed(runDir)
	if err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: read authority seed: %w", err)
	}
	analysisRoot, err := resolveAnalysisRoot(repo)
	if err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: resolve repository authority: %w", err)
	}
	ctx := context.Background()
	before, err := freshness.CaptureRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: capture repository before authority confirmation: %w", err)
	}
	after, err := freshness.CaptureRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: capture repository after authority confirmation: %w", err)
	}
	if seed.RepositoryIdentity != before.Identity || seed.AnalysisRoot != analysisRoot ||
		seed.SelectedRevision != before.Head {
		return nil, fmt.Errorf("atlas study request rebuild: copied run authority does not match --repo")
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, analysisRoot, before, after, seed.CapturedInputPaths, true,
	)
	if err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: confirm repository authority: %w", err)
	}
	data, err := report.ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil && !report.IsExactWorkspaceGraphUnavailable(err) {
		return nil, fmt.Errorf("atlas study request rebuild: read authorized run: %w", err)
	}
	if err := report.PrepareAuthorizedSourceCoverage(ctx, data, &authority); err != nil {
		return nil, fmt.Errorf("atlas study request rebuild: prepare exact source coverage: %w", err)
	}
	return data, nil
}
