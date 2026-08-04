package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/secretscan"
)

// runAtlasStudyResponseMockCLI emits one bounded provider-free response for a
// copied run whose request was already (re)built, so the deterministic replay
// and render pipeline can be exercised without a provider call. The response
// is a pure fixture: no network, no credentials, no provider.
func runAtlasStudyResponseMockCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("atlas-study-response-mock", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied Atlas Study run")
	outFlag := flags.String("out", "", "output response file (default: sibling of the run directory)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" {
		return fmt.Errorf("usage: repomap dev atlas-study-response-mock --run-dir <copied-run> [--out <file>]")
	}
	if stdout == nil {
		return fmt.Errorf("atlas study response mock: stdout is required")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()

	requestRaw, err := readAtlasStudyReplayRootFile(
		root, atlasstudy.RequestArtifactFilename, atlasstudy.MaxRequestArtifactBytes,
	)
	if err != nil {
		return fmt.Errorf("atlas study response mock: read canonical v%d request: %w", atlasstudy.Version, err)
	}
	request, err := atlasstudy.DecodeRequestRecord(requestRaw)
	if err != nil {
		return fmt.Errorf("atlas study response mock: decode canonical v%d request: %w", atlasstudy.Version, err)
	}
	responseRaw, err := atlasstudy.MockResponse(request)
	if err != nil {
		return fmt.Errorf("atlas study response mock: build fixture response: %w", err)
	}
	if kind, unsafe := secretscan.DetectAlways(string(responseRaw)); unsafe {
		return fmt.Errorf(
			"atlas study response mock: response contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}
	var envelope struct {
		Directions []json.RawMessage `json:"directions"`
	}
	if err := json.Unmarshal(responseRaw, &envelope); err != nil {
		return fmt.Errorf("atlas study response mock: decode fixture response: %w", err)
	}

	outPath := *outFlag
	if outPath == "" {
		outPath = filepath.Join(filepath.Dir(runDir), filepath.Base(runDir)+"-mock-response.json")
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("atlas study response mock: resolve output path: %w", err)
	}
	realRunDir, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return fmt.Errorf("atlas study response mock: resolve real run dir: %w", err)
	}
	if atlasStudyReplayPathWithin(realRunDir, absOut) {
		return fmt.Errorf("atlas study response mock: output must be outside the target run directory")
	}
	if info, statErr := os.Lstat(absOut); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("atlas study response mock: output must be a regular non-symlink file")
		}
		return fmt.Errorf("atlas study response mock: refusing to overwrite %s; unlink it first", absOut)
	}
	file, err := os.OpenFile(absOut, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("atlas study response mock: create exclusive output: %w", err)
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
		return fmt.Errorf("atlas study response mock: write output: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("atlas study response mock: close output: %w", closeErr)
	}

	fmt.Fprintf(
		stdout,
		"response_sha256: %s\ndirections: %d\nprovider_calls: 0\n",
		atlasStudyReplaySHA256(responseRaw),
		len(envelope.Directions),
	)
	return nil
}
