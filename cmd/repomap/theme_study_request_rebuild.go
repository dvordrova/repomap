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
	"github.com/dvordrova/repomap/internal/themestudy"
)

// runThemeStudyScoutRequestRebuildCLI re-derives the exact canonical Theme
// Scout request artifact from a copied run that has not yet resolved a
// provider exchange. It is a provider-free dev seam: the request is compiled
// entirely from the persisted Atlas, canonical visible Architecture Canvas,
// exact saved sources and the local seed producer, then round-trip validated
// and published with the same exclusive-write safety as the theme response
// replay seam. No provider call and no network access happen here.
//
// With --repo the seam reproduces the authority-confirmed, source-covered
// ReportData used by the original run-time producer: the manifest seed is
// verified, the scoped run authority is re-confirmed against the live
// repository, and PrepareAuthorizedSourceCoverage installs the exact fresh
// sources before the request is compiled. This makes the rebuilt Scout
// request bind the same compiled substrate as render-report --repo, so the
// deterministic mock -> replay -> render pipeline stays self-consistent.
func runThemeStudyScoutRequestRebuildCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("theme-study-scout-request-rebuild", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied theme Study run")
	repoFlag := flags.String("repo", "", "authoritative repository for the copied run (optional)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" {
		return fmt.Errorf("usage: repomap dev theme-study-scout-request-rebuild --run-dir <copied-run> [--repo <repo>]")
	}
	if stdout == nil {
		return fmt.Errorf("theme scout request rebuild: stdout is required")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()
	// A request can only be rebuilt for a run that has not yet resolved a
	// provider exchange: a persisted result or status would already bind an
	// exact request identity.
	for _, name := range []string{
		themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename,
	} {
		if err := requireAtlasStudyReplayOutputAbsent(root, name); err != nil {
			return err
		}
	}
	// Read the language from the existing Scout request artifact field when
	// the caller kept one in the copy; otherwise default to canonical English.
	// The caller deletes the stale request from the copy before rebuilding;
	// the exclusive write below also refuses to overwrite one.
	language := themestudy.LanguageEnglish
	if requestRaw, err := readAtlasStudyReplayRootFile(
		root, themestudy.ScoutRequestArtifactFilename, themestudy.MaxScoutRequestArtifactBytes,
	); err == nil {
		var existing struct {
			Language themestudy.Language `json:"language"`
		}
		if jsonErr := json.Unmarshal(requestRaw, &existing); jsonErr == nil && existing.Language != "" {
			language = existing.Language
		}
	}
	if err := requireAtlasStudyReplayOutputAbsent(root, themestudy.ScoutRequestArtifactFilename); err != nil {
		return err
	}

	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: read run: %w", err)
	}
	if *repoFlag != "" {
		prepared, err := prepareAtlasStudyRebuildData(runDir, *repoFlag, data)
		if err != nil {
			return err
		}
		data = prepared
	}
	input, err := report.BuildAtlasStudyInput(data, atlasstudyLanguage(language))
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: build exact input: %w", err)
	}
	compileInput, _, err := shapeThemeStudyCompileInput(input)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: shape exact input: %w", err)
	}
	// The local compile is the exact seed producer (Decision 213); the retired
	// single-stage provider call is never invoked by this seam.
	product, err := atlasstudy.Compile(compileInput)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: compile exact substrate: %w", err)
	}
	vocabulary := themestudy.BuildFileVocabulary(
		data.OpenablePaths, 0, func(string) bool { return true },
	)
	seedSpecs := themeSeedSpecsFromInput(compileInput)
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, 0, 0, 0,
		func(path string, startLine, endLine int) ([]string, error) {
			return []string{}, nil
		},
		func(path string) (int, error) { return 0, nil },
	)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: build seed packs: %w", err)
	}
	request, err := themestudy.CompileScout(
		language, vocabulary, packs, themeScoutContext(product, data.RepoName, themeSpanAnchorRefsFromPacks(packs)), "",
	)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: compile scout request: %w", err)
	}
	encoded, err := themestudy.EncodeScoutRequest(request)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: encode scout request: %w", err)
	}
	decoded, err := themestudy.DecodeScoutRequest(encoded)
	if err != nil {
		return fmt.Errorf("theme scout request rebuild: round-trip decode scout request: %w", err)
	}
	if !reflect.DeepEqual(decoded, request) {
		return fmt.Errorf("theme scout request rebuild: round-trip request changed")
	}
	// Owner doctrine: the rebuilt request carries locally expanded repository
	// source whose credential-shaped assignments are legitimate; real
	// credential material still fails closed.
	if kind, unsafe := secretscan.DetectSourceMaterial(string(encoded)); unsafe {
		return fmt.Errorf(
			"theme scout request rebuild: request contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}

	if err := writeAtlasStudyReplayExclusive(
		root,
		themestudy.ScoutRequestArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, err := themestudy.DecodeScoutRequest(saved)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(decoded, request) {
				return fmt.Errorf("theme scout request changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("theme scout request rebuild: publish scout request: %w", err)
	}
	fmt.Fprintf(
		stdout,
		"scout_request_sha256: %s\nprovider_calls: 0\n",
		atlasStudyReplaySHA256(encoded),
	)
	return nil
}

func atlasstudyLanguage(language themestudy.Language) atlasstudy.Language {
	if language == themestudy.LanguageRussian {
		return atlasstudy.LanguageRussian
	}
	return atlasstudy.LanguageEnglish
}

// prepareAtlasStudyRebuildData re-reads the copied run against the confirmed
// scoped repository authority and installs the exact authorized source
// coverage, mirroring the run-time producer path in cmd/repomap main.go. The
// manifest seed must match the live repository exactly, and the rebuilt
// request is then compiled from the same ReportData the authorized render
// replays, so request binding is preserved across the provider-free pipeline.
func prepareAtlasStudyRebuildData(runDir, repo string, base *report.ReportData) (*report.ReportData, error) {
	if base == nil {
		return nil, fmt.Errorf("theme scout request rebuild: base run data is required")
	}
	seed, err := report.ReadRunManifestAuthoritySeed(runDir)
	if err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: read authority seed: %w", err)
	}
	analysisRoot, err := resolveAnalysisRoot(repo)
	if err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: resolve repository authority: %w", err)
	}
	ctx := context.Background()
	before, err := freshness.CaptureRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: capture repository before authority confirmation: %w", err)
	}
	after, err := freshness.CaptureRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: capture repository after authority confirmation: %w", err)
	}
	if seed.RepositoryIdentity != before.Identity || seed.AnalysisRoot != analysisRoot ||
		seed.SelectedRevision != before.Head {
		return nil, fmt.Errorf("theme scout request rebuild: copied run authority does not match --repo")
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, analysisRoot, before, after, seed.CapturedInputPaths, true,
	)
	if err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: confirm repository authority: %w", err)
	}
	data, err := report.ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil && !report.IsExactWorkspaceGraphUnavailable(err) {
		return nil, fmt.Errorf("theme scout request rebuild: read authorized run: %w", err)
	}
	if err := report.PrepareAuthorizedSourceCoverage(ctx, data, &authority); err != nil {
		return nil, fmt.Errorf("theme scout request rebuild: prepare exact source coverage: %w", err)
	}
	return data, nil
}
