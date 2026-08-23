package freshness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const maxCapturedInputBytes = 8 * 1024 * 1024

// CaptureInputs materializes exact content identities for an already captured
// repository state. Clean files are read from the captured commit; dirty files
// use the content hashes recorded during repository capture.
func CaptureInputs(ctx context.Context, state RepositoryState, paths []string) ([]CapturedInput, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	dirty := make(map[string]DirtyFile, len(state.Dirty))
	for _, file := range state.Dirty {
		dirty[file.Path] = file
		if file.FromPath != "" {
			dirty[file.FromPath] = DirtyFile{Status: "deleted", Path: file.FromPath, Kind: FileMissing}
		}
	}
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		normalized, err := normalizeGitPath(path)
		if err != nil {
			return nil, fmt.Errorf("freshness: captured input path: %w", err)
		}
		unique[normalized] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for path := range unique {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var committedPaths []string
	for _, path := range ordered {
		if _, exists := dirty[path]; !exists {
			committedPaths = append(committedPaths, path)
		}
	}
	committed, err := committedInputs(ctx, state, committedPaths)
	if err != nil {
		return nil, err
	}
	inputs := make([]CapturedInput, 0, len(ordered))
	for _, path := range ordered {
		stages := []string{"report_evidence"}
		if isGoBuildInput(path) {
			stages = append(stages, "go_build")
			sort.Strings(stages)
		}
		input := CapturedInput{
			Version: CapturedInputVersion, ID: sha256Hex([]byte("captured-input-v1\x00" + path)),
			Path: path, Stages: stages,
		}
		if file, exists := dirty[path]; exists {
			input.Kind = file.Kind
			input.Mode = file.Mode
			if input.Mode == "" {
				input.Mode = string(file.Kind)
			}
			input.ContentSHA256 = file.ContentSHA256
			inputs = append(inputs, input)
			continue
		}
		if captured, exists := committed[path]; exists {
			input.Kind = captured.kind
			input.Mode = captured.mode
			input.ContentSHA256 = captured.digest
		} else {
			input.Kind = FileMissing
		}
		inputs = append(inputs, input)
	}
	if _, err := CapturedInputsDigest(inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}

type committedInputRecord struct {
	path   string
	object string
	kind   FileKind
	mode   string
	digest string
}

func committedInputs(
	ctx context.Context,
	state RepositoryState,
	paths []string,
) (map[string]committedInputRecord, error) {
	result := make(map[string]committedInputRecord, len(paths))
	if len(paths) == 0 {
		return result, nil
	}
	args := []string{"ls-tree", "-z", state.Head, "--"}
	for _, path := range paths {
		args = append(args, ":(literal)"+path)
	}
	listing, err := gitOutput(ctx, state.Identity, args...)
	if err != nil {
		if _, treeErr := gitOutput(ctx, state.Identity, "cat-file", "-e", state.Head+"^{tree}"); treeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("freshness: list captured inputs at %s: %w", state.Head, err),
				fmt.Errorf("freshness: validate captured commit tree %s: %w", state.Head, treeErr),
			)
		}
		return nil, fmt.Errorf("freshness: list captured inputs at %s: %w", state.Head, err)
	}
	for len(listing) > 0 {
		end := bytes.IndexByte(listing, 0)
		if end < 0 {
			return nil, fmt.Errorf("freshness: unterminated captured-input tree entry")
		}
		entry := listing[:end]
		listing = listing[end+1:]
		header, pathBytes, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("freshness: malformed captured-input tree entry")
		}
		path := string(pathBytes)
		if _, duplicate := result[path]; duplicate {
			return nil, fmt.Errorf("freshness: duplicate captured-input tree entry %q", path)
		}
		if fields[1] != "blob" {
			return nil, fmt.Errorf("freshness: captured input %q is not a blob", path)
		}
		kind := FileRegular
		if fields[0] == "120000" {
			kind = FileSymlink
		}
		result[path] = committedInputRecord{
			path: path, object: fields[2], kind: kind, mode: fields[0],
		}
	}

	ordered := make([]committedInputRecord, 0, len(result))
	for _, path := range paths {
		if record, exists := result[path]; exists {
			ordered = append(ordered, record)
		}
	}
	if err := readCommittedInputDigests(ctx, state, ordered, result); err != nil {
		return nil, err
	}
	return result, nil
}

func readCommittedInputDigests(
	ctx context.Context,
	state RepositoryState,
	ordered []committedInputRecord,
	result map[string]committedInputRecord,
) error {
	if len(ordered) == 0 {
		return nil
	}
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", state.Identity,
		"cat-file", "--batch",
	}
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Env = isolatedGitEnvironment(os.Environ())
	var stdin strings.Builder
	for _, record := range ordered {
		stdin.WriteString(record.object)
		stdin.WriteByte('\n')
	}
	command.Stdin = strings.NewReader(stdin.String())
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("freshness: prepare captured-input batch: %w", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("freshness: start captured-input batch: %w", err)
	}
	reader := bufio.NewReader(stdout)
	fail := func(err error) error {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	for _, record := range ordered {
		header, err := reader.ReadString('\n')
		if err != nil {
			return fail(fmt.Errorf("freshness: read captured input %q header: %w", record.path, err))
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != record.object || fields[1] != "blob" {
			return fail(fmt.Errorf("freshness: malformed captured input %q header", record.path))
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 || size > maxCapturedInputBytes {
			return fail(fmt.Errorf("freshness: captured input %q exceeds the bounded content limit", record.path))
		}
		hasher := sha256.New()
		if _, err := io.CopyN(hasher, reader, size); err != nil {
			return fail(fmt.Errorf("freshness: read captured input %q: %w", record.path, err))
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return fail(fmt.Errorf("freshness: malformed captured input %q body", record.path))
		}
		record.digest = fmt.Sprintf("%x", hasher.Sum(nil))
		result[record.path] = record
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf(
			"freshness: captured-input batch: %w: %s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return nil
}
