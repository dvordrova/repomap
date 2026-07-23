package freshness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
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
		kind, mode, digest, err := committedInput(ctx, state, path)
		if err != nil {
			return nil, err
		}
		input.Kind, input.Mode, input.ContentSHA256 = kind, mode, digest
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

func committedInput(ctx context.Context, state RepositoryState, path string) (FileKind, string, string, error) {
	object := state.Head + ":" + path
	sizeOutput, err := gitOutput(ctx, state.Identity, "cat-file", "-s", object)
	if err != nil {
		return FileMissing, "", "", nil
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 0 || size > maxCapturedInputBytes {
		return "", "", "", fmt.Errorf("freshness: captured input %q exceeds the bounded content limit", path)
	}
	command := exec.CommandContext(
		ctx,
		"git",
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath="+os.DevNull,
		"-C", state.Identity,
		"cat-file", "blob", object,
	)
	command.Env = isolatedGitEnvironment(os.Environ())
	content, err := command.Output()
	if err != nil {
		return "", "", "", fmt.Errorf("freshness: read captured input %q: %w", path, err)
	}
	digest := sha256.Sum256(content)
	mode := "file"
	listing, err := gitOutput(ctx, state.Identity, "ls-tree", state.Head, "--", path)
	if err == nil {
		fields := strings.Fields(string(listing))
		if len(fields) > 0 {
			mode = fields[0]
		}
	}
	if mode == "120000" {
		return FileSymlink, mode, fmt.Sprintf("%x", digest[:]), nil
	}
	return FileRegular, mode, fmt.Sprintf("%x", digest[:]), nil
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
