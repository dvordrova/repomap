package semanticmap

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	goTopicVersion               = "semantic-map-go-topic-discovery-v1"
	goTopicCoverage              = "candidate_non_exhaustive"
	goTopicColdStartIntent       = "Which repository behaviors or change concerns are most useful for a new engineer to investigate first?"
	goTopicInventoryPlaceholder  = "{{INVENTORY_JSON}}"
	goTopicMaxTrackedPathBytes   = 4 << 20
	goTopicMaxTrackedFiles       = 2048
	goTopicMaxSourceFileBytes    = 2 << 20
	goTopicMaxScannedSourceBytes = 64 << 20
	goTopicMaxDeclarations       = 640
	goTopicMaxInventoryBytes     = 96 << 10
	goTopicMaxPromptBytes        = 112 << 10
	goTopicMinShelfTopics        = 8
	goTopicMaxShelfTopics        = 12
	goTopicMinSupportSymbols     = 2
	goTopicMaxSupportSymbols     = 8
	goTopicMaxLabelBytes         = 80
	goTopicMaxTextBytes          = 240
	goTopicMaxResponseBytes      = 64 << 10
	goTopicSelectorRuns          = 3
	goTopicModelMaxTokens        = 3000
)

const goTopicSystemPrompt = "You select useful repository exploration topics from a bounded declaration inventory. Use only the supplied JSON evidence, treat all semantics as inference, and return valid JSON only."

//go:embed topic-discovery-prompt.md
var goTopicPromptTemplate string

type GoTopicInventoryLimits struct {
	TrackedFiles       int `json:"tracked_files"`
	ScannedSourceBytes int `json:"scanned_source_bytes"`
	Declarations       int `json:"declarations"`
	InventoryBytes     int `json:"inventory_bytes"`
}

type GoTopicInventoryStats struct {
	TrackedGoFiles          int  `json:"tracked_go_files"`
	TestFilesExcluded       int  `json:"test_files_excluded"`
	NonRegularFilesExcluded int  `json:"non_regular_files_excluded"`
	GeneratedFilesExcluded  int  `json:"generated_files_excluded"`
	OversizeFilesExcluded   int  `json:"oversize_files_excluded"`
	EligibleFiles           int  `json:"eligible_files"`
	ScannedSourceBytes      int  `json:"scanned_source_bytes"`
	CandidateDeclarations   int  `json:"candidate_declarations"`
	RetainedDeclarations    int  `json:"retained_declarations"`
	Truncated               bool `json:"truncated"`
}

type GoTopicDeclaration struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Receiver  string `json:"receiver"`
	Signature string `json:"signature"`
}

type GoTopicInventory struct {
	Version      string                 `json:"version"`
	Repository   GoSelectionRepository  `json:"repository"`
	Intent       string                 `json:"intent"`
	Coverage     string                 `json:"coverage"`
	Limits       GoTopicInventoryLimits `json:"limits"`
	Stats        GoTopicInventoryStats  `json:"stats"`
	Declarations []GoTopicDeclaration   `json:"declarations"`
}

type GoTopic struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Why              string   `json:"why"`
	HowQuestion      string   `json:"how_question"`
	ChangeQuestion   string   `json:"change_question"`
	SupportSymbolIDs []string `json:"support_symbol_ids"`
}

type GoTopicShelf struct {
	Coverage string    `json:"coverage"`
	Topics   []GoTopic `json:"topics"`
}

type GoTopicProviderMetadata struct {
	Model                 string `json:"model"`
	MaxTokens             int    `json:"max_tokens"`
	Thinking              string `json:"thinking"`
	Attempts              int    `json:"attempts"`
	RequestBytes          int    `json:"request_bytes"`
	LatencyMillis         int64  `json:"latency_ms"`
	InputTokens           int    `json:"input_tokens,omitempty"`
	OutputTokens          int    `json:"output_tokens,omitempty"`
	PromptCacheHitTokens  int    `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int    `json:"prompt_cache_miss_tokens,omitempty"`
}

type GoTopicSelectionRun struct {
	TopicID  string                  `json:"topic_id"`
	Question string                  `json:"question"`
	Trace    GoSourceSelectionTrace  `json:"trace"`
	Packet   GoSourceSelectionPacket `json:"packet"`
}

type goTopicSelectionSpec struct {
	topicID  string
	question string
}

type GoTopicExperiment struct {
	Version              string                  `json:"version"`
	Repository           GoSelectionRepository   `json:"repository"`
	Coverage             string                  `json:"coverage"`
	InventorySHA256      string                  `json:"inventory_sha256"`
	PromptTemplateSHA256 string                  `json:"prompt_template_sha256"`
	RequestSHA256        string                  `json:"request_sha256"`
	ResponseSHA256       string                  `json:"response_sha256"`
	Inventory            GoTopicInventory        `json:"inventory"`
	Shelf                GoTopicShelf            `json:"shelf"`
	Provider             GoTopicProviderMetadata `json:"provider"`
	Selections           []GoTopicSelectionRun   `json:"selections"`
}

type goTopicDeclarationCandidate struct {
	path      string
	line      int
	kind      string
	name      string
	receiver  string
	signature string
}

func BuildGoTopicInventory(
	repositoryPath string,
	expectedRevision string,
) (GoTopicInventory, error) {
	repoPath, repository, err := validateGoSelectionInput(GoSourceSelectionOptions{
		RepositoryPath:   repositoryPath,
		ExpectedRevision: expectedRevision,
		Question:         goTopicColdStartIntent,
	})
	if err != nil {
		return GoTopicInventory{}, err
	}
	files, err := listGoTopicFiles(repoPath)
	if err != nil {
		return GoTopicInventory{}, err
	}
	stats := GoTopicInventoryStats{TrackedGoFiles: len(files)}
	reader, err := reporead.New(repoPath)
	if err != nil {
		return GoTopicInventory{}, fmt.Errorf("go topic inventory: repository reader: %w", err)
	}
	defer reader.Close()

	byTopLevel := make(map[string][]goTopicDeclarationCandidate)
	for _, sourcePath := range files {
		if strings.HasSuffix(sourcePath, "_test.go") {
			stats.TestFilesExcluded++
			continue
		}
		fileInfo, err := os.Lstat(filepath.Join(repoPath, filepath.FromSlash(sourcePath)))
		if err != nil {
			return GoTopicInventory{}, fmt.Errorf(
				"go topic inventory: inspect %s: %w",
				sourcePath,
				err,
			)
		}
		if !fileInfo.Mode().IsRegular() {
			stats.NonRegularFilesExcluded++
			continue
		}
		content, err := reader.ReadFileNoSymlinks(sourcePath, goTopicMaxSourceFileBytes)
		if err != nil {
			return GoTopicInventory{}, fmt.Errorf(
				"go topic inventory: read %s: %w",
				sourcePath,
				err,
			)
		}
		if content.Truncated {
			stats.OversizeFilesExcluded++
			stats.Truncated = true
			continue
		}
		if stats.ScannedSourceBytes+len(content.Bytes) > goTopicMaxScannedSourceBytes {
			stats.Truncated = true
			break
		}
		stats.ScannedSourceBytes += len(content.Bytes)
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(
			fileSet,
			sourcePath,
			content.Bytes,
			parser.ParseComments|parser.SkipObjectResolution,
		)
		if err != nil {
			return GoTopicInventory{}, fmt.Errorf(
				"go topic inventory: parse %s: %w",
				sourcePath,
				err,
			)
		}
		if ast.IsGenerated(file) {
			stats.GeneratedFilesExcluded++
			continue
		}
		stats.EligibleFiles++
		candidates, err := goTopicFileDeclarations(fileSet, file, sourcePath)
		if err != nil {
			return GoTopicInventory{}, err
		}
		stats.CandidateDeclarations += len(candidates)
		topLevel := goTopicTopLevel(sourcePath)
		byTopLevel[topLevel] = append(byTopLevel[topLevel], candidates...)
	}
	for group := range byTopLevel {
		byTopLevel[group] = goTopicRoundRobinDirectories(byTopLevel[group])
	}
	selected := goTopicRoundRobin(byTopLevel, goTopicMaxDeclarations)
	if len(selected) < stats.CandidateDeclarations {
		stats.Truncated = true
	}
	declarations := make([]GoTopicDeclaration, len(selected))
	for index, candidate := range selected {
		declarations[index] = GoTopicDeclaration{
			ID:        fmt.Sprintf("d%04d", index+1),
			Path:      candidate.path,
			Kind:      candidate.kind,
			Name:      candidate.name,
			Receiver:  candidate.receiver,
			Signature: candidate.signature,
		}
	}
	inventory := GoTopicInventory{
		Version:    goTopicVersion,
		Repository: repository,
		Intent:     goTopicColdStartIntent,
		Coverage:   goTopicCoverage,
		Limits: GoTopicInventoryLimits{
			TrackedFiles:       goTopicMaxTrackedFiles,
			ScannedSourceBytes: goTopicMaxScannedSourceBytes,
			Declarations:       goTopicMaxDeclarations,
			InventoryBytes:     goTopicMaxInventoryBytes,
		},
		Stats:        stats,
		Declarations: declarations,
	}
	for len(inventory.Declarations) > 0 {
		inventory.Stats.RetainedDeclarations = len(inventory.Declarations)
		encoded, err := json.Marshal(inventory)
		if err != nil {
			return GoTopicInventory{}, fmt.Errorf("go topic inventory: encode: %w", err)
		}
		if len(encoded) <= goTopicMaxInventoryBytes {
			break
		}
		inventory.Stats.Truncated = true
		inventory.Declarations = inventory.Declarations[:len(inventory.Declarations)-1]
	}
	if len(inventory.Declarations) == 0 {
		return GoTopicInventory{}, fmt.Errorf("go topic inventory: no declarations fit the budget")
	}
	if err := validateGoSelectionCheckout(repoPath, expectedRevision); err != nil {
		return GoTopicInventory{}, err
	}
	if err := ValidateGoTopicInventory(inventory); err != nil {
		return GoTopicInventory{}, err
	}
	return inventory, nil
}

func listGoTopicFiles(repoPath string) ([]string, error) {
	output, err := goTopicGitOutput(
		repoPath,
		goTopicMaxTrackedPathBytes,
		"ls-files",
		"-z",
		"--",
		":(glob)**/*.go",
	)
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, min(len(parts), goTopicMaxTrackedFiles))
	seen := make(map[string]struct{}, min(len(parts), goTopicMaxTrackedFiles))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if len(files) == goTopicMaxTrackedFiles {
			return nil, fmt.Errorf(
				"go topic inventory: tracked Go files exceed %d",
				goTopicMaxTrackedFiles,
			)
		}
		sourcePath := string(part)
		if !goSelectionCanonicalPath(sourcePath) || !strings.HasSuffix(sourcePath, ".go") {
			return nil, fmt.Errorf("go topic inventory: invalid tracked Go path")
		}
		if _, duplicate := seen[sourcePath]; duplicate {
			continue
		}
		seen[sourcePath] = struct{}{}
		files = append(files, sourcePath)
	}
	sort.Strings(files)
	return files, nil
}

func goTopicGitOutput(
	repoPath string,
	maxBytes int,
	args ...string,
) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(context.Background(), goSelectionGitTimeout)
	defer cancel()
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(commandCtx, "git", commandArgs...)
	command.Env = append(
		filterGitEnvironment(os.Environ()),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("go topic inventory: git %s: %w", strings.Join(args, " "), err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("go topic inventory: git %s: %w", strings.Join(args, " "), err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxBytes)+1))
	if len(output) > maxBytes {
		cancel()
		_ = command.Wait()
		return nil, fmt.Errorf(
			"go topic inventory: git %s output exceeds %d bytes",
			strings.Join(args, " "),
			maxBytes,
		)
	}
	waitErr := command.Wait()
	if commandCtx.Err() != nil {
		return nil, fmt.Errorf(
			"go topic inventory: git %s timed out after %s",
			strings.Join(args, " "),
			goSelectionGitTimeout,
		)
	}
	if readErr != nil {
		return nil, fmt.Errorf("go topic inventory: read git %s: %w", strings.Join(args, " "), readErr)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("go topic inventory: git %s: %w", strings.Join(args, " "), waitErr)
	}
	return output, nil
}

func goTopicFileDeclarations(
	fileSet *token.FileSet,
	file *ast.File,
	sourcePath string,
) ([]goTopicDeclarationCandidate, error) {
	result := make([]goTopicDeclarationCandidate, 0, len(file.Decls))
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Name.Name == "init" {
			continue
		}
		name := function.Name.Name
		if !goTopicScalar(name, goTopicMaxTextBytes, false) {
			return nil, fmt.Errorf("go topic inventory: invalid declaration name in %s", sourcePath)
		}
		kind := "function"
		receiver := ""
		if function.Recv != nil && len(function.Recv.List) > 0 {
			kind = "method"
			var err error
			receiver, err = goTopicFormatNode(fileSet, function.Recv.List[0].Type)
			if err != nil {
				return nil, fmt.Errorf("go topic inventory: receiver %s.%s: %w", sourcePath, name, err)
			}
		}
		signature, err := goTopicFormatNode(fileSet, function.Type)
		if err != nil {
			return nil, fmt.Errorf("go topic inventory: signature %s.%s: %w", sourcePath, name, err)
		}
		if !goTopicScalar(receiver, goTopicMaxTextBytes, true) ||
			!goTopicScalar(signature, goTopicMaxTextBytes, false) {
			continue
		}
		result = append(result, goTopicDeclarationCandidate{
			path:      sourcePath,
			line:      fileSet.Position(function.Pos()).Line,
			kind:      kind,
			name:      name,
			receiver:  receiver,
			signature: signature,
		})
	}
	return result, nil
}

func goTopicFormatNode(fileSet *token.FileSet, node any) (string, error) {
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, node); err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(output.String()), " "), nil
}

func goTopicTopLevel(sourcePath string) string {
	if separator := strings.IndexByte(sourcePath, '/'); separator >= 0 {
		return sourcePath[:separator]
	}
	return "."
}

func goTopicRoundRobin(
	byDirectory map[string][]goTopicDeclarationCandidate,
	limit int,
) []goTopicDeclarationCandidate {
	groups := make([]string, 0, len(byDirectory))
	for group := range byDirectory {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	positions := make(map[string]int, len(groups))
	result := make([]goTopicDeclarationCandidate, 0, min(limit, goTopicMaxDeclarations))
	for len(result) < limit {
		progress := false
		for _, group := range groups {
			position := positions[group]
			if position >= len(byDirectory[group]) {
				continue
			}
			result = append(result, byDirectory[group][position])
			positions[group] = position + 1
			progress = true
			if len(result) == limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return result
}

func goTopicRoundRobinDirectories(
	candidates []goTopicDeclarationCandidate,
) []goTopicDeclarationCandidate {
	byDirectory := make(map[string][]goTopicDeclarationCandidate)
	for _, candidate := range candidates {
		directory := path.Dir(candidate.path)
		byDirectory[directory] = append(byDirectory[directory], candidate)
	}
	for directory := range byDirectory {
		byDirectory[directory] = goTopicRoundRobinFiles(byDirectory[directory])
	}
	return goTopicRoundRobin(byDirectory, len(candidates))
}

func goTopicRoundRobinFiles(
	candidates []goTopicDeclarationCandidate,
) []goTopicDeclarationCandidate {
	byFile := make(map[string][]goTopicDeclarationCandidate)
	for _, candidate := range candidates {
		byFile[candidate.path] = append(byFile[candidate.path], candidate)
	}
	for file := range byFile {
		sort.Slice(byFile[file], func(i, j int) bool {
			left, right := byFile[file][i], byFile[file][j]
			if left.line != right.line {
				return left.line < right.line
			}
			if left.kind != right.kind {
				return left.kind < right.kind
			}
			return left.name < right.name
		})
	}
	return goTopicRoundRobin(byFile, len(candidates))
}

func ValidateGoTopicInventory(inventory GoTopicInventory) error {
	if inventory.Version != goTopicVersion ||
		inventory.Coverage != goTopicCoverage ||
		inventory.Intent != goTopicColdStartIntent {
		return fmt.Errorf("go topic inventory: contract metadata is invalid")
	}
	if inventory.Limits != (GoTopicInventoryLimits{
		TrackedFiles:       goTopicMaxTrackedFiles,
		ScannedSourceBytes: goTopicMaxScannedSourceBytes,
		Declarations:       goTopicMaxDeclarations,
		InventoryBytes:     goTopicMaxInventoryBytes,
	}) {
		return fmt.Errorf("go topic inventory: limits are invalid")
	}
	if len(inventory.Repository.Revision) != 40 ||
		!lowerHex(inventory.Repository.Revision) ||
		!goTopicScalar(inventory.Repository.Name, goTopicMaxTextBytes, false) {
		return fmt.Errorf("go topic inventory: repository identity is invalid")
	}
	if len(inventory.Declarations) == 0 ||
		len(inventory.Declarations) > goTopicMaxDeclarations {
		return fmt.Errorf("go topic inventory: declaration count is invalid")
	}
	for index, declaration := range inventory.Declarations {
		if declaration.ID != fmt.Sprintf("d%04d", index+1) ||
			!goSelectionCanonicalPath(declaration.Path) ||
			(declaration.Kind != "function" && declaration.Kind != "method") ||
			!goTopicScalar(declaration.Name, goTopicMaxTextBytes, false) ||
			!goTopicScalar(declaration.Receiver, goTopicMaxTextBytes, true) ||
			!goTopicScalar(declaration.Signature, goTopicMaxTextBytes, false) {
			return fmt.Errorf("go topic inventory: declarations[%d] is invalid", index)
		}
		if declaration.Kind == "function" && declaration.Receiver != "" {
			return fmt.Errorf("go topic inventory: function %q has a receiver", declaration.ID)
		}
		if declaration.Kind == "method" && declaration.Receiver == "" {
			return fmt.Errorf("go topic inventory: method %q has no receiver", declaration.ID)
		}
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		return fmt.Errorf("go topic inventory: encode validation: %w", err)
	}
	if len(encoded) > goTopicMaxInventoryBytes {
		return fmt.Errorf("go topic inventory: encoded size exceeds %d", goTopicMaxInventoryBytes)
	}
	if inventory.Stats.RetainedDeclarations != len(inventory.Declarations) ||
		inventory.Stats.CandidateDeclarations < len(inventory.Declarations) ||
		inventory.Stats.TrackedGoFiles > goTopicMaxTrackedFiles ||
		inventory.Stats.ScannedSourceBytes > goTopicMaxScannedSourceBytes {
		return fmt.Errorf("go topic inventory: statistics are inconsistent")
	}
	return nil
}

func BuildGoTopicPrompt(inventory GoTopicInventory) (string, string, error) {
	if err := ValidateGoTopicInventory(inventory); err != nil {
		return "", "", err
	}
	if strings.Count(goTopicPromptTemplate, goTopicInventoryPlaceholder) != 1 {
		return "", "", fmt.Errorf("go topic prompt: inventory placeholder must occur once")
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		return "", "", fmt.Errorf("go topic prompt: encode inventory: %w", err)
	}
	user := strings.Replace(
		goTopicPromptTemplate,
		goTopicInventoryPlaceholder,
		string(encoded),
		1,
	)
	if len(user) > goTopicMaxPromptBytes || !utf8.ValidString(user) {
		return "", "", fmt.Errorf("go topic prompt: prompt exceeds the bounded UTF-8 contract")
	}
	return goTopicSystemPrompt, user, nil
}

func goTopicSynthesisPrompt(systemPrompt, userPrompt string) componentmap.SynthesisPrompt {
	return componentmap.SynthesisPrompt{
		Version: componentmap.SynthesisPromptVersion,
		System:  systemPrompt,
		User:    userPrompt,
	}
}

func DecodeGoTopicShelf(response []byte, inventory GoTopicInventory) (GoTopicShelf, error) {
	if len(response) == 0 || len(response) > goTopicMaxResponseBytes {
		return GoTopicShelf{}, fmt.Errorf("go topic shelf: response size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(response))
	decoder.DisallowUnknownFields()
	var shelf GoTopicShelf
	if err := decoder.Decode(&shelf); err != nil {
		return GoTopicShelf{}, fmt.Errorf("go topic shelf: decode strict response: %w", err)
	}
	if err := requireGoTopicJSONEOF(decoder); err != nil {
		return GoTopicShelf{}, err
	}
	if err := ValidateGoTopicShelf(shelf, inventory); err != nil {
		return GoTopicShelf{}, err
	}
	return shelf, nil
}

func requireGoTopicJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("go topic shelf: response contains more than one JSON value")
		}
		return fmt.Errorf("go topic shelf: trailing response data: %w", err)
	}
	return nil
}

func ValidateGoTopicShelf(shelf GoTopicShelf, inventory GoTopicInventory) error {
	if err := ValidateGoTopicInventory(inventory); err != nil {
		return err
	}
	if shelf.Coverage != goTopicCoverage ||
		len(shelf.Topics) < goTopicMinShelfTopics ||
		len(shelf.Topics) > goTopicMaxShelfTopics {
		return fmt.Errorf("go topic shelf: coverage or topic count is invalid")
	}
	knownDeclarations := make(map[string]struct{}, len(inventory.Declarations))
	for _, declaration := range inventory.Declarations {
		knownDeclarations[declaration.ID] = struct{}{}
	}
	labels := make(map[string]struct{}, goTopicMaxShelfTopics)
	questions := make(map[string]struct{}, goTopicMaxShelfTopics*2)
	supportSets := make(map[string]struct{}, goTopicMaxShelfTopics)
	for index, topic := range shelf.Topics {
		if topic.ID != fmt.Sprintf("t%d", index+1) ||
			!goTopicScalar(topic.Label, goTopicMaxLabelBytes, false) ||
			!goTopicScalar(topic.Why, goTopicMaxTextBytes, false) ||
			!goTopicQuestion(topic.HowQuestion, "How ") ||
			(!goTopicQuestion(topic.ChangeQuestion, "Where ") &&
				!goTopicQuestion(topic.ChangeQuestion, "What ")) {
			return fmt.Errorf("go topic shelf: topics[%d] has an invalid scalar", index)
		}
		for _, text := range []string{
			topic.Label,
			topic.Why,
			topic.HowQuestion,
			topic.ChangeQuestion,
		} {
			if goTopicLeaksInventory(text) {
				return fmt.Errorf("go topic shelf: topics[%d] leaks inventory details", index)
			}
		}
		lowerLabel := strings.ToLower(topic.Label)
		if strings.Contains(lowerLabel, ".go") ||
			strings.HasPrefix(lowerLabel, "package ") ||
			strings.HasSuffix(lowerLabel, " package") ||
			strings.HasPrefix(lowerLabel, "imports ") {
			return fmt.Errorf("go topic shelf: topics[%d] is a package/file label", index)
		}
		labelKey := goTopicComparable(topic.Label)
		if _, duplicate := labels[labelKey]; duplicate {
			return fmt.Errorf("go topic shelf: duplicate topic label")
		}
		labels[labelKey] = struct{}{}
		for _, question := range []string{topic.HowQuestion, topic.ChangeQuestion} {
			questionKey := goTopicComparable(question)
			if _, duplicate := questions[questionKey]; duplicate {
				return fmt.Errorf("go topic shelf: duplicate topic question")
			}
			questions[questionKey] = struct{}{}
		}
		if len(topic.SupportSymbolIDs) < goTopicMinSupportSymbols ||
			len(topic.SupportSymbolIDs) > goTopicMaxSupportSymbols {
			return fmt.Errorf("go topic shelf: topics[%d] support count is invalid", index)
		}
		seenSupport := make(map[string]struct{}, goTopicMaxSupportSymbols)
		support := append([]string(nil), topic.SupportSymbolIDs...)
		for _, id := range support {
			if _, ok := knownDeclarations[id]; !ok {
				return fmt.Errorf("go topic shelf: topics[%d] references unknown declaration %q", index, id)
			}
			if _, duplicate := seenSupport[id]; duplicate {
				return fmt.Errorf("go topic shelf: topics[%d] repeats declaration %q", index, id)
			}
			seenSupport[id] = struct{}{}
		}
		sort.Strings(support)
		supportKey := strings.Join(support, "\x00")
		if _, duplicate := supportSets[supportKey]; duplicate {
			return fmt.Errorf("go topic shelf: duplicate support set")
		}
		supportSets[supportKey] = struct{}{}
	}
	return nil
}

func goTopicQuestion(value, prefix string) bool {
	return goTopicScalar(value, goTopicMaxTextBytes, false) &&
		strings.HasPrefix(value, prefix) &&
		strings.HasSuffix(value, "?")
}

func goTopicLeaksInventory(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, ".go") {
		return true
	}
	for _, field := range strings.Fields(lower) {
		candidate := strings.Trim(field, `"'()[]{}.,;:`)
		if strings.HasPrefix(candidate, "/") ||
			strings.HasPrefix(candidate, "./") ||
			strings.HasPrefix(candidate, "../") ||
			strings.Contains(candidate, `\`) {
			return true
		}
	}
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != ':'
	})
	for _, token := range tokens {
		if len(token) == 5 && token[0] == 'd' && allASCIIDigits(token[1:]) {
			return true
		}
		if colon := strings.LastIndexByte(token, ':'); colon > 0 &&
			colon < len(token)-1 &&
			allASCIIDigits(token[colon+1:]) {
			return true
		}
	}
	return false
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func goTopicScalar(value string, maxBytes int, allowEmpty bool) bool {
	if len(value) > maxBytes || (!allowEmpty && value == "") {
		return false
	}
	return utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) &&
		!strings.ContainsAny(value, "\r\n") &&
		strings.TrimSpace(value) == value
}

func goTopicComparable(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func CaptureGoTopicExperiment(
	ctx context.Context,
	repositoryPath string,
	expectedRevision string,
	goplsBinary string,
	client *deepseek.Client,
	responsePath string,
	experimentPath string,
) (GoTopicExperiment, error) {
	if client == nil {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: provider client is required")
	}
	responsePath, experimentPath, err := goTopicOutputPaths(responsePath, experimentPath)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	inventory, err := BuildGoTopicInventory(repositoryPath, expectedRevision)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	systemPrompt, userPrompt, err := BuildGoTopicPrompt(inventory)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	client.MaxTokens = goTopicModelMaxTokens
	config := client.EffectiveConfig()
	if config.Endpoint != "https://api.deepseek.com/chat/completions" ||
		config.Model != "deepseek-v4-flash" {
		return GoTopicExperiment{}, fmt.Errorf(
			"go topic experiment: live capture requires the reference DeepSeek endpoint and model",
		)
	}
	prompt := goTopicSynthesisPrompt(systemPrompt, userPrompt)
	request, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: build request: %w", err)
	}
	if !bytes.Contains(request, []byte(`"thinking":{"type":"disabled"}`)) {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: provider request must disable thinking")
	}
	started := time.Now()
	result, err := client.SynthesizeComponentLandscapeMeasured(ctx, prompt)
	latency := time.Since(started)
	if err != nil {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: provider call: %w", err)
	}
	if result.Attempts != 1 {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: provider attempts = %d, want 1", result.Attempts)
	}
	if len(result.Content) == 0 || len(result.Content) > goTopicMaxResponseBytes {
		return GoTopicExperiment{}, fmt.Errorf(
			"go topic experiment: response exceeds the persistence budget",
		)
	}
	if kind, found := secretscan.Detect(string(result.Content)); found {
		return GoTopicExperiment{}, fmt.Errorf(
			"go topic experiment: response rejected before persistence: %s detected",
			kind,
		)
	}
	if err := writeGoTopicExclusive(responsePath, result.Content); err != nil {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: persist response: %w", err)
	}
	shelf, err := DecodeGoTopicShelf(result.Content, inventory)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	selections, err := selectGoTopicSelections(
		ctx,
		repositoryPath,
		expectedRevision,
		goplsBinary,
		inventory,
		shelf,
	)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	inventoryJSON, err := json.Marshal(inventory)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	experiment := GoTopicExperiment{
		Version:              goTopicVersion,
		Repository:           inventory.Repository,
		Coverage:             goTopicCoverage,
		InventorySHA256:      goTopicSHA256(inventoryJSON),
		PromptTemplateSHA256: goTopicSHA256([]byte(goTopicPromptTemplate)),
		RequestSHA256:        goTopicSHA256(request),
		ResponseSHA256:       goTopicSHA256(result.Content),
		Inventory:            inventory,
		Shelf:                shelf,
		Provider: GoTopicProviderMetadata{
			Model:                 client.Model,
			MaxTokens:             goTopicModelMaxTokens,
			Thinking:              "disabled",
			Attempts:              result.Attempts,
			RequestBytes:          len(request),
			LatencyMillis:         latency.Milliseconds(),
			InputTokens:           result.InputTokens,
			OutputTokens:          result.OutputTokens,
			PromptCacheHitTokens:  result.PromptCacheHitTokens,
			PromptCacheMissTokens: result.PromptCacheMissTokens,
		},
		Selections: selections,
	}
	if err := ValidateGoTopicExperiment(experiment); err != nil {
		return GoTopicExperiment{}, err
	}
	encoded, err := EncodeGoTopicExperiment(experiment)
	if err != nil {
		return GoTopicExperiment{}, err
	}
	if kind, found := secretscan.Detect(string(encoded)); found {
		return GoTopicExperiment{}, fmt.Errorf(
			"go topic experiment: artifact rejected before persistence: %s detected",
			kind,
		)
	}
	if err := writeGoTopicExclusive(experimentPath, encoded); err != nil {
		return GoTopicExperiment{}, fmt.Errorf("go topic experiment: persist experiment: %w", err)
	}
	return experiment, nil
}

func ReplayGoTopicResponse(
	ctx context.Context,
	repositoryPath string,
	expectedRevision string,
	goplsBinary string,
	response []byte,
) (GoTopicInventory, GoTopicShelf, []GoTopicSelectionRun, error) {
	inventory, err := BuildGoTopicInventory(repositoryPath, expectedRevision)
	if err != nil {
		return GoTopicInventory{}, GoTopicShelf{}, nil, err
	}
	shelf, err := DecodeGoTopicShelf(response, inventory)
	if err != nil {
		return GoTopicInventory{}, GoTopicShelf{}, nil, err
	}
	selections, err := selectGoTopicSelections(
		ctx,
		repositoryPath,
		expectedRevision,
		goplsBinary,
		inventory,
		shelf,
	)
	if err != nil {
		return GoTopicInventory{}, GoTopicShelf{}, nil, err
	}
	return inventory, shelf, selections, nil
}

func selectGoTopicSelections(
	ctx context.Context,
	repositoryPath string,
	expectedRevision string,
	goplsBinary string,
	inventory GoTopicInventory,
	shelf GoTopicShelf,
) ([]GoTopicSelectionRun, error) {
	selectionSpecs := goTopicSelectionSpecs(shelf)
	selections := make([]GoTopicSelectionRun, 0, len(selectionSpecs))
	for _, spec := range selectionSpecs {
		var topic GoTopic
		for _, candidate := range shelf.Topics {
			if candidate.ID == spec.topicID {
				topic = candidate
				break
			}
		}
		anchors, err := goTopicAnchorDeclarations(inventory, topic)
		if err != nil {
			return nil, err
		}
		options := GoSourceSelectionOptions{
			RepositoryPath:   repositoryPath,
			ExpectedRevision: expectedRevision,
			Question:         spec.question,
			GoplsBinary:      goplsBinary,
		}
		firstTrace, firstPacket, err := SelectGoAnchoredQuestionSources(
			ctx,
			options,
			anchors,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"go topic experiment: select %s: %w",
				spec.topicID,
				err,
			)
		}
		selections = append(selections, GoTopicSelectionRun{
			TopicID:  spec.topicID,
			Question: spec.question,
			Trace:    firstTrace,
			Packet:   firstPacket,
		})
	}
	return selections, nil
}

func goTopicAnchorDeclarations(
	inventory GoTopicInventory,
	topic GoTopic,
) ([]GoTopicDeclaration, error) {
	byID := make(map[string]GoTopicDeclaration, len(inventory.Declarations))
	for _, declaration := range inventory.Declarations {
		byID[declaration.ID] = declaration
	}
	anchors := make([]GoTopicDeclaration, 0, len(topic.SupportSymbolIDs))
	for _, id := range topic.SupportSymbolIDs {
		declaration, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("go topic experiment: unknown support declaration %q", id)
		}
		anchors = append(anchors, declaration)
	}
	if len(anchors) < goTopicMinSupportSymbols ||
		len(anchors) > goTopicMaxSupportSymbols {
		return nil, fmt.Errorf("go topic experiment: support anchor count is invalid")
	}
	return anchors, nil
}

func goTopicOutputPaths(responsePath, experimentPath string) (string, string, error) {
	absolute := make([]string, 0, 2)
	for _, outputPath := range []string{responsePath, experimentPath} {
		if outputPath == "" {
			return "", "", fmt.Errorf("go topic experiment: output path is required")
		}
		resolved, err := filepath.Abs(outputPath)
		if err != nil {
			return "", "", fmt.Errorf(
				"go topic experiment: resolve output %s: %w",
				filepath.Base(outputPath),
				err,
			)
		}
		resolved = filepath.Clean(resolved)
		if _, err := os.Lstat(resolved); err == nil {
			return "", "", fmt.Errorf(
				"go topic experiment: refusing to overwrite %s",
				filepath.Base(resolved),
			)
		} else if !os.IsNotExist(err) {
			return "", "", fmt.Errorf(
				"go topic experiment: inspect output %s: %w",
				filepath.Base(resolved),
				err,
			)
		}
		absolute = append(absolute, resolved)
	}
	if absolute[0] == absolute[1] {
		return "", "", fmt.Errorf("go topic experiment: response and experiment outputs must differ")
	}
	return absolute[0], absolute[1], nil
}

func writeGoTopicExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return closeErr
}

func goTopicSelectionSpecs(shelf GoTopicShelf) []goTopicSelectionSpec {
	return []goTopicSelectionSpec{
		{topicID: shelf.Topics[0].ID, question: shelf.Topics[0].HowQuestion},
		{topicID: shelf.Topics[0].ID, question: shelf.Topics[0].ChangeQuestion},
		{topicID: shelf.Topics[1].ID, question: shelf.Topics[1].HowQuestion},
	}
}

func ValidateGoTopicExperiment(experiment GoTopicExperiment) error {
	if experiment.Version != goTopicVersion ||
		experiment.Coverage != goTopicCoverage ||
		experiment.Repository != experiment.Inventory.Repository {
		return fmt.Errorf("go topic experiment: metadata is inconsistent")
	}
	if err := ValidateGoTopicShelf(experiment.Shelf, experiment.Inventory); err != nil {
		return err
	}
	if len(experiment.Selections) != goTopicSelectorRuns ||
		experiment.Provider.Attempts != 1 ||
		experiment.Provider.MaxTokens != goTopicModelMaxTokens ||
		experiment.Provider.Thinking != "disabled" {
		return fmt.Errorf("go topic experiment: provider or selection counts are invalid")
	}
	for index, selection := range experiment.Selections {
		spec := goTopicSelectionSpecs(experiment.Shelf)[index]
		var topic GoTopic
		for _, candidate := range experiment.Shelf.Topics {
			if candidate.ID == spec.topicID {
				topic = candidate
				break
			}
		}
		if selection.TopicID != spec.topicID ||
			selection.Question != spec.question ||
			selection.Trace.Repository != experiment.Repository ||
			selection.Packet.Repository != experiment.Repository ||
			selection.Trace.Question != selection.Question ||
			selection.Packet.Question != selection.Question ||
			selection.Trace.Coverage != "anchor_seeded_non_exhaustive" ||
			selection.Packet.Coverage != "anchor_seeded_non_exhaustive" ||
			!goTopicSameStrings(
				selection.Trace.SeedDeclarationIDs,
				topic.SupportSymbolIDs,
			) ||
			!goTopicSameStrings(
				selection.Packet.SeedDeclarationIDs,
				topic.SupportSymbolIDs,
			) ||
			len(selection.Trace.SelectedSymbols) == 0 ||
			len(selection.Packet.SourceSlices) == 0 {
			return fmt.Errorf("go topic experiment: selections[%d] is invalid", index)
		}
	}
	for _, hash := range []string{
		experiment.InventorySHA256,
		experiment.PromptTemplateSHA256,
		experiment.RequestSHA256,
		experiment.ResponseSHA256,
	} {
		if len(hash) != 64 || !lowerHex(hash) {
			return fmt.Errorf("go topic experiment: invalid SHA-256")
		}
	}
	return nil
}

func goTopicSameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func EncodeGoTopicExperiment(experiment GoTopicExperiment) ([]byte, error) {
	encoded, err := json.MarshalIndent(experiment, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("go topic experiment: encode: %w", err)
	}
	return append(encoded, '\n'), nil
}

func goTopicSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
