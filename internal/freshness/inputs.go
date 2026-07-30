package freshness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func AssessInputs(
	ctx context.Context,
	captured RepositoryState,
	current RepositoryState,
	inputs []CapturedInput,
) FreshnessResult {
	result := NewFreshnessResult(FreshnessFresh)
	if captured.Identity != current.Identity {
		result.State = FreshnessUnavailable
		result.Diagnostics = []string{"repository identity changed"}
		return result
	}
	capturedDigest, capturedErr := captured.Digest()
	currentDigest, currentErr := current.Digest()
	if capturedErr == nil && currentErr == nil && capturedDigest == currentDigest {
		return result
	}
	currentInputs, committedUnrelated, err := inputsChangedSinceCapture(ctx, captured, current, inputs)
	if err != nil {
		result.State = FreshnessUnavailable
		result.Diagnostics = []string{"captured input comparison was unavailable"}
		return result
	}
	currentByPath := make(map[string]CapturedInput, len(currentInputs))
	for _, input := range currentInputs {
		currentByPath[input.Path] = input
	}
	for _, input := range inputs {
		currentInput, exists := currentByPath[input.Path]
		if !exists || currentInput.Kind != input.Kind || currentInput.Mode != input.Mode ||
			currentInput.ContentSHA256 != input.ContentSHA256 {
			result.AffectedInputIDs = append(result.AffectedInputIDs, input.ID)
			result.AffectedPaths = append(result.AffectedPaths, input.Path)
		}
	}
	result.AffectedSubmodules = changedSubmodulePaths(captured.Submodules, current.Submodules)
	differences := CompareRepository(captured, current)
	globalChanged := len(differences) > 0 || len(result.AffectedSubmodules) > 0
	if len(result.AffectedPaths) > 0 {
		result.State = FreshnessPartiallyStale
		result.AnalyzedChanges = true
		result.UnrelatedChanges = committedUnrelated || hasUnrelatedChanges(differences, result.AffectedPaths) || len(result.AffectedSubmodules) > 0
	} else if globalChanged {
		result.State = FreshnessUnrelatedChanges
		result.UnrelatedChanges = true
	}
	return result
}

func inputsChangedSinceCapture(
	ctx context.Context,
	captured RepositoryState,
	current RepositoryState,
	inputs []CapturedInput,
) ([]CapturedInput, bool, error) {
	changed := make(map[string]struct{})
	unrelated := false
	for _, difference := range CompareRepository(captured, current) {
		if difference.Reason != ReasonRepositoryDirty {
			continue
		}
		for _, path := range difference.Paths {
			changed[path] = struct{}{}
		}
	}
	if captured.Head != current.Head {
		paths, err := changedPathsAcrossCommits(ctx, captured, current)
		if err != nil {
			return nil, false, err
		}
		inputPaths := make(map[string]struct{}, len(inputs))
		for _, input := range inputs {
			inputPaths[input.Path] = struct{}{}
		}
		for _, path := range paths {
			if _, analyzed := inputPaths[path]; analyzed {
				changed[path] = struct{}{}
			} else {
				unrelated = true
			}
		}
	}
	toCapture := make([]string, 0, len(changed))
	inputByPath := make(map[string]CapturedInput, len(inputs))
	for _, input := range inputs {
		inputByPath[input.Path] = input
		if _, ok := changed[input.Path]; ok {
			toCapture = append(toCapture, input.Path)
		}
	}
	updated, err := CaptureInputs(ctx, current, toCapture)
	if err != nil {
		return nil, false, err
	}
	for _, input := range updated {
		inputByPath[input.Path] = input
	}
	result := make([]CapturedInput, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, inputByPath[input.Path])
	}
	return result, unrelated, nil
}

func changedPathsAcrossCommits(
	ctx context.Context,
	captured RepositoryState,
	current RepositoryState,
) ([]string, error) {
	args := []string{"diff", "--name-only", "-z", "--no-renames", captured.Head, current.Head}
	output, err := gitOutput(ctx, current.Identity, args...)
	if err != nil {
		return nil, err
	}
	var result []string
	for len(output) > 0 {
		end := bytes.IndexByte(output, 0)
		if end < 0 {
			return nil, fmt.Errorf("freshness: unterminated changed-input path")
		}
		path, err := normalizeGitPath(string(output[:end]))
		if err != nil {
			return nil, err
		}
		result = append(result, path)
		output = output[end+1:]
	}
	return result, nil
}

func hasUnrelatedChanges(differences []Difference, affectedPaths []string) bool {
	affected := make(map[string]struct{}, len(affectedPaths))
	for _, path := range affectedPaths {
		affected[path] = struct{}{}
	}
	for _, difference := range differences {
		if difference.Reason != ReasonRepositoryDirty {
			continue
		}
		for _, path := range difference.Paths {
			if _, analyzed := affected[path]; !analyzed {
				return true
			}
		}
	}
	return false
}

func capturedInputPaths(inputs []CapturedInput) []string {
	paths := make([]string, len(inputs))
	for index, input := range inputs {
		paths[index] = input.Path
	}
	return paths
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
			return result, nil
		}
		return nil, err
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

func changedSubmodulePaths(saved, current []SubmoduleState) []string {
	savedByPath := make(map[string]SubmoduleState, len(saved))
	currentByPath := make(map[string]SubmoduleState, len(current))
	for _, state := range saved {
		savedByPath[state.Path] = state
	}
	for _, state := range current {
		currentByPath[state.Path] = state
	}
	paths := make(map[string]struct{})
	for path, state := range savedByPath {
		if !reflect.DeepEqual(state, currentByPath[path]) {
			paths[path] = struct{}{}
		}
	}
	for path, state := range currentByPath {
		if !reflect.DeepEqual(state, savedByPath[path]) {
			paths[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, filepath.ToSlash(path))
	}
	sort.Strings(result)
	return result
}
