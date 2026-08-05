package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// runThemeStudyResponseMockCLI emits one bounded provider-free response for a
// copied run whose Scout request was already (re)built, so the deterministic
// replay and render pipeline can be exercised without a provider call. The
// response is a pure fixture: no network, no credentials, no provider. With
// --stage adjudication it mocks the second semantic stage over the persisted
// Scout result, expansion and Adjudication request.
func runThemeStudyResponseMockCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("theme-study-response-mock", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied theme Study run")
	outFlag := flags.String("out", "", "output response file (default: sibling of the run directory)")
	stageFlag := flags.String("stage", "scout", "semantic stage to mock: scout (default) or adjudication")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" {
		return fmt.Errorf("usage: repomap dev theme-study-response-mock --run-dir <copied-run> [--stage scout|adjudication] [--out <file>]")
	}
	if stdout == nil {
		return fmt.Errorf("theme study response mock: stdout is required")
	}
	if *stageFlag != "scout" && *stageFlag != "adjudication" {
		return fmt.Errorf("theme study response mock: --stage must be scout or adjudication")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()

	var responseRaw []byte
	var counts struct {
		Candidates int `json:"candidates"`
		Themes     int `json:"themes"`
	}
	switch *stageFlag {
	case "scout":
		requestRaw, err := readAtlasStudyReplayRootFile(
			root, themestudy.ScoutRequestArtifactFilename, themestudy.MaxScoutRequestArtifactBytes,
		)
		if err != nil {
			return fmt.Errorf("theme scout response mock: read canonical scout request: %w", err)
		}
		request, err := themestudy.DecodeScoutRequest(requestRaw)
		if err != nil {
			return fmt.Errorf("theme scout response mock: decode canonical scout request: %w", err)
		}
		responseRaw, err = themestudy.MockScoutResponse(request)
		if err != nil {
			return fmt.Errorf("theme scout response mock: build fixture response: %w", err)
		}
		var envelope struct {
			Candidates []json.RawMessage `json:"candidates"`
		}
		if err := json.Unmarshal(responseRaw, &envelope); err != nil {
			return fmt.Errorf("theme scout response mock: decode fixture response: %w", err)
		}
		counts.Candidates = len(envelope.Candidates)
	case "adjudication":
		requestRaw, err := readAtlasStudyReplayRootFile(
			root, themestudy.AdjudicationRequestArtifactFilename, themestudy.MaxAdjRequestArtifactBytes,
		)
		if err != nil {
			return fmt.Errorf("theme adjudication response mock: read canonical adjudication request: %w", err)
		}
		request, err := themestudy.DecodeAdjudicationRequest(requestRaw)
		if err != nil {
			return fmt.Errorf("theme adjudication response mock: decode canonical adjudication request: %w", err)
		}
		responseRaw, err = themestudy.MockAdjudicationResponse(request)
		if err != nil {
			return fmt.Errorf("theme adjudication response mock: build fixture response: %w", err)
		}
		var envelope struct {
			Themes []json.RawMessage `json:"themes"`
		}
		if err := json.Unmarshal(responseRaw, &envelope); err != nil {
			return fmt.Errorf("theme adjudication response mock: decode fixture response: %w", err)
		}
		counts.Themes = len(envelope.Themes)
	}
	if kind, unsafe := secretscan.DetectAlways(string(responseRaw)); unsafe {
		return fmt.Errorf(
			"theme study response mock: response contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}

	outPath := *outFlag
	if outPath == "" {
		suffix := "-scout-mock-response.json"
		if *stageFlag == "adjudication" {
			suffix = "-adjudication-mock-response.json"
		}
		outPath = filepath.Join(filepath.Dir(runDir), filepath.Base(runDir)+suffix)
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("theme study response mock: resolve output path: %w", err)
	}
	realRunDir, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return fmt.Errorf("theme study response mock: resolve real run dir: %w", err)
	}
	if atlasStudyReplayPathWithin(realRunDir, absOut) {
		return fmt.Errorf("theme study response mock: output must be outside the target run directory")
	}
	if info, statErr := os.Lstat(absOut); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("theme study response mock: output must be a regular non-symlink file")
		}
		return fmt.Errorf("theme study response mock: refusing to overwrite %s; unlink it first", absOut)
	}
	file, err := os.OpenFile(absOut, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("theme study response mock: create exclusive output: %w", err)
	}
	written, writeErr := file.Write(responseRaw)
	if writeErr == nil && written != len(responseRaw) {
		writeErr = fmt.Errorf("short write")
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(absOut)
		return fmt.Errorf("theme study response mock: write output: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("theme study response mock: close output: %w", closeErr)
	}

	switch *stageFlag {
	case "scout":
		fmt.Fprintf(
			stdout,
			"response_sha256: %s\ncandidates: %d\nprovider_calls: 0\n",
			atlasStudyReplaySHA256(responseRaw), counts.Candidates,
		)
	default:
		fmt.Fprintf(
			stdout,
			"response_sha256: %s\nthemes: %d\nprovider_calls: 0\n",
			atlasStudyReplaySHA256(responseRaw), counts.Themes,
		)
	}
	return nil
}
