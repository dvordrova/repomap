package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/themestudy"
)

var atlasStudyReplaySHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const atlasStudyReplayMetadataMaxBytes = 1 << 20

// runThemeStudyResponseReplayCLI resolves one exact saved provider response
// against the canonical theme request artifact of a copied run and publishes
// the accepted result + status with exclusive-write safety. The seam performs
// no I/O to the provider: it re-validates the saved response with the exact
// same item-local semantics as the live run. --stage scout replays the Theme
// Scout response (publishes scout result + status); --stage adjudication
// replays the Source Review response (publishes adjudication result + status).
func runThemeStudyResponseReplayCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("theme-study-response-replay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied theme Study run")
	requestSHAFlag := flags.String("request-sha256", "", "exact canonical theme request SHA-256")
	responseFlag := flags.String("response", "", "exact saved provider response")
	responseSHAFlag := flags.String("response-sha256", "", "exact response SHA-256")
	stageFlag := flags.String("stage", "scout", "semantic stage to replay: scout (default) or adjudication")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" || *responseFlag == "" ||
		!atlasStudyReplaySHA256Pattern.MatchString(*requestSHAFlag) ||
		!atlasStudyReplaySHA256Pattern.MatchString(*responseSHAFlag) {
		return fmt.Errorf("usage: repomap dev theme-study-response-replay --run-dir <copied-run> --request-sha256 <sha> --response <file> --response-sha256 <sha> [--stage scout|adjudication]")
	}
	if stdout == nil {
		return fmt.Errorf("theme study response replay: stdout is required")
	}
	if *stageFlag != "scout" && *stageFlag != "adjudication" {
		return fmt.Errorf("theme study response replay: --stage must be scout or adjudication")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()
	if *stageFlag == "scout" {
		for _, name := range []string{themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename} {
			if err := requireAtlasStudyReplayOutputAbsent(root, name); err != nil {
				return err
			}
		}
	} else {
		for _, name := range []string{themestudy.AdjudicationResultArtifactFilename, themestudy.AdjudicationStatusArtifactFilename} {
			if err := requireAtlasStudyReplayOutputAbsent(root, name); err != nil {
				return err
			}
		}
	}

	responseRaw, err := readAtlasStudyReplayResponse(*responseFlag, runDir)
	if err != nil {
		return err
	}
	responseSHA := atlasStudyReplaySHA256(responseRaw)
	if responseSHA != *responseSHAFlag {
		return fmt.Errorf("theme study response replay: response SHA-256 mismatch")
	}
	if kind, unsafe := secretscan.DetectAlways(string(responseRaw)); unsafe {
		return fmt.Errorf(
			"theme study response replay: response contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}

	requestName := themestudy.ScoutRequestArtifactFilename
	requestLimit := themestudy.MaxScoutRequestArtifactBytes
	if *stageFlag == "adjudication" {
		requestName = themestudy.AdjudicationRequestArtifactFilename
		requestLimit = themestudy.MaxAdjRequestArtifactBytes
	}
	requestRaw, err := readAtlasStudyReplayRootFile(root, requestName, requestLimit)
	if err != nil {
		return fmt.Errorf("theme study response replay: read canonical %s: %w", requestName, err)
	}
	requestSHA := atlasStudyReplaySHA256(requestRaw)
	if requestSHA != *requestSHAFlag {
		return fmt.Errorf("theme study response replay: request SHA-256 mismatch")
	}

	if *stageFlag == "scout" {
		request, err := themestudy.DecodeScoutRequest(requestRaw)
		if err != nil {
			return fmt.Errorf("theme scout response replay: decode canonical request: %w", err)
		}
		result, status, err := themestudy.ReplayScoutResponse(request, responseRaw)
		if err != nil {
			return fmt.Errorf("theme scout response replay: resolve exact response: %w", err)
		}
		resultRaw, err := themestudy.EncodeScoutResult(result)
		if err != nil {
			return fmt.Errorf("theme scout response replay: encode result: %w", err)
		}
		statusRaw, err := themestudy.EncodeScoutStatus(status)
		if err != nil {
			return fmt.Errorf("theme scout response replay: encode status: %w", err)
		}
		if err := themeReplayPublishAll(
			root, resultRaw, statusRaw,
			themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename,
			func(saved []byte) (any, error) { return themestudy.DecodeScoutResult(saved) },
			func(saved []byte) (any, error) { return themestudy.DecodeScoutStatus(saved) },
			result, status,
		); err != nil {
			return err
		}
		fmt.Fprintf(
			stdout,
			"request_sha256: %s\nresponse_sha256: %s\nresult_sha256: %s\nstatus_sha256: %s\ncandidates: %d\nprovider_calls: 0\n",
			requestSHA, responseSHA,
			atlasStudyReplaySHA256(resultRaw), atlasStudyReplaySHA256(statusRaw),
			len(result.Candidates),
		)
		return nil
	}

	request, err := themestudy.DecodeAdjudicationRequest(requestRaw)
	if err != nil {
		return fmt.Errorf("theme adjudication response replay: decode canonical request: %w", err)
	}
	result, status, err := themestudy.ReplayAdjudicationResponse(request, responseRaw)
	if err != nil {
		return fmt.Errorf("theme adjudication response replay: resolve exact response: %w", err)
	}
	resultRaw, err := themestudy.EncodeAdjudicationResult(result)
	if err != nil {
		return fmt.Errorf("theme adjudication response replay: encode result: %w", err)
	}
	statusRaw, err := themestudy.EncodeAdjudicationStatus(status)
	if err != nil {
		return fmt.Errorf("theme adjudication response replay: encode status: %w", err)
	}
	if err := themeReplayPublishAll(
		root, resultRaw, statusRaw,
		themestudy.AdjudicationResultArtifactFilename, themestudy.AdjudicationStatusArtifactFilename,
		func(saved []byte) (any, error) { return themestudy.DecodeAdjudicationResult(saved) },
		func(saved []byte) (any, error) { return themestudy.DecodeAdjudicationStatus(saved) },
		result, status,
	); err != nil {
		return err
	}
	fmt.Fprintf(
		stdout,
		"request_sha256: %s\nresponse_sha256: %s\nresult_sha256: %s\nstatus_sha256: %s\nthemes: %d\nprovider_calls: 0\n",
		requestSHA, responseSHA,
		atlasStudyReplaySHA256(resultRaw), atlasStudyReplaySHA256(statusRaw),
		len(result.Themes),
	)
	return nil
}

// themeReplayPublishAll publishes the replayed result + status with the
// shared exclusive-write safety, verifying neither changed before
// publication. The decoder functions restore the exact stage artifact types.
func themeReplayPublishAll(
	root *os.Root,
	resultRaw []byte,
	statusRaw []byte,
	resultName string,
	statusName string,
	decodeResult func([]byte) (any, error),
	decodeStatus func([]byte) (any, error),
	result any,
	status any,
) error {
	if kind, unsafe := secretscan.DetectAlways(string(resultRaw)); unsafe {
		return fmt.Errorf(
			"theme study response replay: result contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}
	if kind, unsafe := secretscan.DetectAlways(string(statusRaw)); unsafe {
		return fmt.Errorf(
			"theme study response replay: status contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}
	if err := writeAtlasStudyReplayExclusive(
		root,
		resultName,
		resultRaw,
		func(saved []byte) error {
			decoded, err := decodeResult(saved)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("theme replay result changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("theme study response replay: publish %s: %w", resultName, err)
	}
	if err := writeAtlasStudyReplayExclusive(
		root,
		statusName,
		statusRaw,
		func(saved []byte) error {
			decoded, err := decodeStatus(saved)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(decoded, status) {
				return fmt.Errorf("theme replay status changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("theme study response replay: publish %s: %w", statusName, err)
	}
	return nil
}

func writeAtlasStudyReplayExclusive(
	root *os.Root,
	name string,
	data []byte,
	validate func([]byte) error,
) error {
	if root == nil || validate == nil {
		return fmt.Errorf("exclusive theme replay writer is not configured")
	}
	if err := validate(append([]byte(nil), data...)); err != nil {
		return fmt.Errorf("validate %s: %w", name, err)
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create exclusive %s: %w", name, err)
	}
	createdInfo, statErr := file.Stat()
	published := false
	defer func() {
		_ = file.Close()
		if published || statErr != nil {
			return
		}
		current, currentErr := root.Lstat(name)
		if currentErr == nil && os.SameFile(createdInfo, current) {
			_ = root.Remove(name)
		}
	}()
	if statErr != nil {
		return fmt.Errorf("stat exclusive %s: %w", name, statErr)
	}
	written, err := file.Write(data)
	if err != nil {
		return fmt.Errorf("write exclusive %s: %w", name, err)
	}
	if written != len(data) {
		return fmt.Errorf("write exclusive %s: short write", name)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync exclusive %s: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close exclusive %s: %w", name, err)
	}
	published = true
	return nil
}

func openAtlasStudyReplayCopiedRun(runDir string) (string, *os.Root, error) {
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return "", nil, fmt.Errorf("theme study response replay: resolve run dir: %w", err)
	}
	info, err := os.Lstat(absRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("theme study response replay: inspect run dir: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("theme study response replay: --run-dir must be an explicit existing real directory")
	}
	realRunDir, err := filepath.EvalSymlinks(absRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("theme study response replay: resolve real run dir: %w", err)
	}
	root, err := os.OpenRoot(realRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("theme study response replay: open run dir: %w", err)
	}
	metadataRaw, err := readAtlasStudyReplayRootFile(root, "metadata.json", atlasStudyReplayMetadataMaxBytes)
	if err != nil {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: read canonical run metadata: %w", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: decode canonical run metadata")
	}
	canonicalMetadata, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: encode canonical run metadata")
	}
	canonicalMetadata = append(canonicalMetadata, '\n')
	if !bytes.Equal(metadataRaw, canonicalMetadata) {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: run metadata is not canonical")
	}
	if metadata.Command != "atlas-first" || metadata.RunID == "" || metadata.RunID == "." ||
		!filepath.IsLocal(metadata.RunID) || filepath.Clean(metadata.RunID) != metadata.RunID ||
		filepath.Base(metadata.RunID) != metadata.RunID {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: run metadata is not an Atlas-first canonical run identity")
	}
	if filepath.Base(realRunDir) == metadata.RunID {
		root.Close()
		return "", nil, fmt.Errorf("theme study response replay: refusing to mutate the original canonical run; pass an explicit copy")
	}
	return realRunDir, root, nil
}

func readAtlasStudyReplayResponse(responsePath, realRunDir string) ([]byte, error) {
	absResponse, err := filepath.Abs(responsePath)
	if err != nil {
		return nil, fmt.Errorf("theme study response replay: resolve response path: %w", err)
	}
	info, err := os.Lstat(absResponse)
	if err != nil {
		return nil, fmt.Errorf("theme study response replay: inspect response: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("theme study response replay: response must be a bounded regular non-symlink file")
	}
	realResponse, err := filepath.EvalSymlinks(absResponse)
	if err != nil {
		return nil, fmt.Errorf("theme study response replay: resolve real response path: %w", err)
	}
	if atlasStudyReplayPathWithin(realRunDir, realResponse) {
		return nil, fmt.Errorf("theme study response replay: response must be outside the target run directory")
	}
	responseRaw, err := readAtlasStudyReplayRegularFile(realResponse, themestudy.MaxScoutResultArtifactBytes+themestudy.MaxAdjResultArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("theme study response replay: read response: %w", err)
	}
	return responseRaw, nil
}

func readAtlasStudyReplayRootFile(root *os.Root, name string, limit int) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a bounded regular non-symlink file", name)
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", name, limit)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%s changed before it could be read", name)
	}
	return readAtlasStudyReplayOpenedFile(file, name, limit)
}

func readAtlasStudyReplayRegularFile(name string, limit int) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file must be a bounded regular non-symlink file")
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, fmt.Errorf("file exceeds the %d-byte limit", limit)
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("file changed before it could be read")
	}
	return readAtlasStudyReplayOpenedFile(file, "file", limit)
}

func readAtlasStudyReplayOpenedFile(file *os.File, label string, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", label)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", label, limit)
	}
	return data, nil
}

func requireAtlasStudyReplayOutputAbsent(root *os.Root, name string) error {
	_, err := root.Lstat(name)
	if err == nil {
		return fmt.Errorf("theme study response replay: refusing to overwrite existing %s", name)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("theme study response replay: inspect output %s: %w", name, err)
	}
	return nil
}

func atlasStudyReplayPathWithin(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}
	return relative == "." ||
		(relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func atlasStudyReplaySHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
