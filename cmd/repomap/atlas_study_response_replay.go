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

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const atlasStudyReplayMetadataMaxBytes = 1 << 20

var atlasStudyReplaySHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func runAtlasStudyResponseReplayCLI(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("atlas-study-response-replay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runDirFlag := flags.String("run-dir", "", "explicit copied Atlas Study run")
	requestSHAFlag := flags.String("request-sha256", "", "exact canonical v5 request SHA-256")
	responseFlag := flags.String("response", "", "exact saved provider response")
	responseSHAFlag := flags.String("response-sha256", "", "exact response SHA-256")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *runDirFlag == "" || *responseFlag == "" ||
		!atlasStudyReplaySHA256Pattern.MatchString(*requestSHAFlag) ||
		!atlasStudyReplaySHA256Pattern.MatchString(*responseSHAFlag) {
		return fmt.Errorf("usage: repomap dev atlas-study-response-replay --run-dir <copied-run> --request-sha256 <sha> --response <file> --response-sha256 <sha>")
	}
	if stdout == nil {
		return fmt.Errorf("atlas study response replay: stdout is required")
	}

	runDir, root, err := openAtlasStudyReplayCopiedRun(*runDirFlag)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range []string{atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename} {
		if err := requireAtlasStudyReplayOutputAbsent(root, name); err != nil {
			return err
		}
	}

	requestRaw, err := readAtlasStudyReplayRootFile(
		root, atlasstudy.RequestArtifactFilename, atlasstudy.MaxRequestArtifactBytes,
	)
	if err != nil {
		return fmt.Errorf("atlas study response replay: read canonical v5 request: %w", err)
	}
	requestSHA := atlasStudyReplaySHA256(requestRaw)
	if requestSHA != *requestSHAFlag {
		return fmt.Errorf("atlas study response replay: request SHA-256 mismatch")
	}
	request, err := atlasstudy.DecodeRequestRecord(requestRaw)
	if err != nil {
		return fmt.Errorf("atlas study response replay: decode canonical v5 request: %w", err)
	}

	responseRaw, err := readAtlasStudyReplayResponse(*responseFlag, runDir)
	if err != nil {
		return err
	}
	responseSHA := atlasStudyReplaySHA256(responseRaw)
	if responseSHA != *responseSHAFlag {
		return fmt.Errorf("atlas study response replay: response SHA-256 mismatch")
	}
	if kind, unsafe := secretscan.DetectAlways(string(responseRaw)); unsafe {
		return fmt.Errorf(
			"atlas study response replay: response contains credential-like content (%s)",
			secretscan.ClosedKind(kind),
		)
	}

	result, status, _, err := atlasstudy.ReplayResponseRecord(request, responseRaw)
	if err != nil {
		return fmt.Errorf("atlas study response replay: resolve exact response: %w", err)
	}
	resultRaw, err := atlasstudy.EncodeResultRecord(result)
	if err != nil {
		return fmt.Errorf("atlas study response replay: encode v6 result: %w", err)
	}
	statusRaw, err := atlasstudy.EncodeStatus(status)
	if err != nil {
		return fmt.Errorf("atlas study response replay: encode v6 status: %w", err)
	}
	for _, artifact := range []struct {
		label string
		data  []byte
	}{{"result", resultRaw}, {"status", statusRaw}} {
		if kind, unsafe := secretscan.DetectAlways(string(artifact.data)); unsafe {
			return fmt.Errorf(
				"atlas study response replay: %s contains credential-like content (%s)",
				artifact.label, secretscan.ClosedKind(kind),
			)
		}
	}

	if err := writeAtlasStudyReplayExclusive(
		root,
		atlasstudy.ResultArtifactFilename,
		resultRaw,
		func(saved []byte) error {
			decoded, err := atlasstudy.DecodeResultRecord(saved)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("atlas study replay result changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("atlas study response replay: publish v6 result: %w", err)
	}
	if err := writeAtlasStudyReplayExclusive(
		root,
		atlasstudy.StatusArtifactFilename,
		statusRaw,
		func(saved []byte) error {
			decoded, err := atlasstudy.DecodeStatus(saved)
			if err != nil {
				return err
			}
			if decoded != status {
				return fmt.Errorf("atlas study replay status changed before publication")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("atlas study response replay: publish v6 status: %w", err)
	}

	fmt.Fprintf(
		stdout,
		"request_sha256: %s\nresponse_sha256: %s\nresult_sha256: %s\nstatus_sha256: %s\ndirections: %d\nprovider_calls: 0\n",
		requestSHA,
		responseSHA,
		atlasStudyReplaySHA256(resultRaw),
		atlasStudyReplaySHA256(statusRaw),
		len(result.Directions),
	)
	return nil
}

func writeAtlasStudyReplayExclusive(
	root *os.Root,
	name string,
	data []byte,
	validate func([]byte) error,
) error {
	if root == nil || validate == nil {
		return fmt.Errorf("exclusive Atlas Study replay writer is not configured")
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
		return "", nil, fmt.Errorf("atlas study response replay: resolve run dir: %w", err)
	}
	info, err := os.Lstat(absRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("atlas study response replay: inspect run dir: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("atlas study response replay: --run-dir must be an explicit existing real directory")
	}
	realRunDir, err := filepath.EvalSymlinks(absRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("atlas study response replay: resolve real run dir: %w", err)
	}
	root, err := os.OpenRoot(realRunDir)
	if err != nil {
		return "", nil, fmt.Errorf("atlas study response replay: open run dir: %w", err)
	}
	metadataRaw, err := readAtlasStudyReplayRootFile(root, "metadata.json", atlasStudyReplayMetadataMaxBytes)
	if err != nil {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: read canonical run metadata: %w", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: decode canonical run metadata")
	}
	canonicalMetadata, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: encode canonical run metadata")
	}
	canonicalMetadata = append(canonicalMetadata, '\n')
	if !bytes.Equal(metadataRaw, canonicalMetadata) {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: run metadata is not canonical")
	}
	if metadata.Command != "atlas-first" || metadata.RunID == "" || metadata.RunID == "." ||
		!filepath.IsLocal(metadata.RunID) || filepath.Clean(metadata.RunID) != metadata.RunID ||
		filepath.Base(metadata.RunID) != metadata.RunID {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: run metadata is not an Atlas-first canonical run identity")
	}
	if filepath.Base(realRunDir) == metadata.RunID {
		root.Close()
		return "", nil, fmt.Errorf("atlas study response replay: refusing to mutate the original canonical run; pass an explicit copy")
	}
	return realRunDir, root, nil
}

func readAtlasStudyReplayResponse(responsePath, realRunDir string) ([]byte, error) {
	absResponse, err := filepath.Abs(responsePath)
	if err != nil {
		return nil, fmt.Errorf("atlas study response replay: resolve response path: %w", err)
	}
	info, err := os.Lstat(absResponse)
	if err != nil {
		return nil, fmt.Errorf("atlas study response replay: inspect response: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("atlas study response replay: response must be a bounded regular non-symlink file")
	}
	realResponse, err := filepath.EvalSymlinks(absResponse)
	if err != nil {
		return nil, fmt.Errorf("atlas study response replay: resolve real response path: %w", err)
	}
	if atlasStudyReplayPathWithin(realRunDir, realResponse) {
		return nil, fmt.Errorf("atlas study response replay: response must be outside the target run directory")
	}
	responseRaw, err := readAtlasStudyReplayRegularFile(realResponse, atlasstudy.DefaultLimits().MaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("atlas study response replay: read response: %w", err)
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
		return fmt.Errorf("atlas study response replay: refusing to overwrite existing %s", name)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("atlas study response replay: inspect output %s: %w", name, err)
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
