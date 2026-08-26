package contracttest

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode"
)

const promptInventoryContractVersion = 1

type promptInventoryContract struct {
	Version int                            `json:"version"`
	Entries []promptInventoryContractEntry `json:"entries"`
}

type promptInventoryContractEntry struct {
	Path          string `json:"path"`
	Status        string `json:"status"`
	Owner         string `json:"owner"`
	Stage         string `json:"stage"`
	Role          string `json:"role"`
	EmbedSource   string `json:"embed_source"`
	EmbedPattern  string `json:"embed_pattern"`
	EmbedVariable string `json:"embed_variable"`
}

type discoveredPromptEmbed struct {
	Owner         string
	EmbedSource   string
	EmbedPattern  string
	EmbedVariable string
}

func TestStaticDomainPromptInventory(t *testing.T) {
	root := repositoryRoot(t)
	contractPath := filepath.Join(root, "testdata", "contracts", "prompts.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read prompt inventory contract: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var contract promptInventoryContract
	if err := decoder.Decode(&contract); err != nil {
		t.Fatalf("decode prompt inventory contract: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("decode prompt inventory contract trailing data: %v", err)
	}
	if contract.Version != promptInventoryContractVersion || contract.Entries == nil {
		t.Fatalf("invalid prompt inventory contract version or entries")
	}
	if !sort.SliceIsSorted(contract.Entries, func(i, j int) bool {
		return contract.Entries[i].Path < contract.Entries[j].Path
	}) {
		t.Fatal("prompt inventory contract is not path-sorted")
	}

	discovered, markdownPaths := discoverPromptEmbeds(t, root)
	if len(markdownPaths) != len(discovered) {
		var missing []string
		for _, promptPath := range markdownPaths {
			if _, ok := discovered[promptPath]; !ok {
				missing = append(missing, promptPath)
			}
		}
		t.Fatalf("internal prompt Markdown must have exactly one source go:embed binding; unbound paths: %v", missing)
	}
	if len(contract.Entries) != len(discovered) {
		t.Fatalf(
			"prompt inventory has %d entries, discovered %d embedded Markdown prompts; update %s",
			len(contract.Entries), len(discovered), contractPath,
		)
	}

	seen := make(map[string]struct{}, len(contract.Entries))
	for position, entry := range contract.Entries {
		if !validRepositoryPath(entry.Path) || !strings.HasSuffix(entry.Path, ".md") {
			t.Fatalf("prompt inventory entry %d has invalid path %q", position, entry.Path)
		}
		if entry.Status != "active" && entry.Status != "dormant" {
			t.Fatalf("prompt inventory entry %q has invalid status %q", entry.Path, entry.Status)
		}
		if !validInventoryName(entry.Owner) || !validInventoryName(entry.Stage) {
			t.Fatalf("prompt inventory entry %q has invalid owner/stage %q/%q", entry.Path, entry.Owner, entry.Stage)
		}
		if entry.Role != "system" && entry.Role != "user" {
			t.Fatalf("prompt inventory entry %q has invalid role %q", entry.Path, entry.Role)
		}
		if !validRepositoryPath(entry.EmbedSource) || !strings.HasSuffix(entry.EmbedSource, ".go") ||
			entry.EmbedPattern == "" || !validGoIdentifier(entry.EmbedVariable) {
			t.Fatalf("prompt inventory entry %q has invalid embed binding", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			t.Fatalf("duplicate prompt inventory path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}

		got, ok := discovered[entry.Path]
		if !ok {
			t.Fatalf("inventoried prompt %q has no source go:embed binding", entry.Path)
		}
		want := discoveredPromptEmbed{
			Owner:         entry.Owner,
			EmbedSource:   entry.EmbedSource,
			EmbedPattern:  entry.EmbedPattern,
			EmbedVariable: entry.EmbedVariable,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("prompt %q embed binding = %#v, inventory wants %#v", entry.Path, got, want)
		}
	}
	for promptPath := range discovered {
		if _, ok := seen[promptPath]; !ok {
			t.Fatalf("embedded prompt %q is missing from %s", promptPath, contractPath)
		}
	}
}

func discoverPromptEmbeds(t *testing.T, root string) (map[string]discoveredPromptEmbed, []string) {
	t.Helper()
	discovered := make(map[string]discoveredPromptEmbed)
	var markdownPaths []string
	for _, subtree := range []string{"cmd", "internal"} {
		subtreeRoot := filepath.Join(root, subtree)
		err := filepath.WalkDir(subtreeRoot, func(filename string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if strings.HasSuffix(relative, ".md") {
				markdownPaths = append(markdownPaths, relative)
			}
			if !strings.HasSuffix(relative, ".go") {
				return nil
			}
			return discoverGoFilePromptEmbeds(root, filename, relative, discovered)
		})
		if err != nil {
			t.Fatalf("discover %s prompt embeds: %v", subtree, err)
		}
	}
	sort.Strings(markdownPaths)
	return discovered, markdownPaths
}

func discoverGoFilePromptEmbeds(
	root string,
	filename string,
	relativeSource string,
	discovered map[string]discoveredPromptEmbed,
) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", relativeSource, err)
	}
	for _, declaration := range parsed.Decls {
		variables, ok := declaration.(*ast.GenDecl)
		if !ok || variables.Tok != token.VAR {
			continue
		}
		declarationPatterns, err := goEmbedPatterns(variables.Doc)
		if err != nil {
			return fmt.Errorf("parse %s go:embed directive: %w", relativeSource, err)
		}
		if len(declarationPatterns) != 0 && len(variables.Specs) != 1 {
			return fmt.Errorf("%s has a go:embed directive on an ambiguous var declaration", relativeSource)
		}
		for _, rawSpec := range variables.Specs {
			specification, ok := rawSpec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			patterns, err := goEmbedPatterns(specification.Doc)
			if err != nil {
				return fmt.Errorf("parse %s go:embed directive: %w", relativeSource, err)
			}
			patterns = append(append([]string(nil), declarationPatterns...), patterns...)
			if len(patterns) == 0 {
				continue
			}
			if len(specification.Names) != 1 {
				return fmt.Errorf("%s go:embed declaration must bind exactly one variable", relativeSource)
			}
			for _, pattern := range patterns {
				relativePrompt, prompt, err := resolvePromptEmbed(root, filepath.Dir(filename), pattern)
				if err != nil {
					return fmt.Errorf("resolve %s go:embed pattern %q: %w", relativeSource, pattern, err)
				}
				if !prompt {
					continue
				}
				binding := discoveredPromptEmbed{
					Owner:         parsed.Name.Name,
					EmbedSource:   relativeSource,
					EmbedPattern:  pattern,
					EmbedVariable: specification.Names[0].Name,
				}
				if previous, duplicate := discovered[relativePrompt]; duplicate {
					return fmt.Errorf("prompt %s has multiple go:embed bindings: %#v and %#v", relativePrompt, previous, binding)
				}
				discovered[relativePrompt] = binding
			}
		}
	}
	return nil
}

func goEmbedPatterns(comments *ast.CommentGroup) ([]string, error) {
	if comments == nil {
		return nil, nil
	}
	var result []string
	for _, comment := range comments.List {
		const prefix = "//go:embed"
		if !strings.HasPrefix(comment.Text, prefix) {
			continue
		}
		patterns := strings.Fields(strings.TrimSpace(strings.TrimPrefix(comment.Text, prefix)))
		if len(patterns) == 0 {
			return nil, fmt.Errorf("empty go:embed directive")
		}
		result = append(result, patterns...)
	}
	return result, nil
}

func resolvePromptEmbed(root, sourceDir, pattern string) (string, bool, error) {
	filesystemPattern := strings.TrimPrefix(pattern, "all:")
	if !strings.HasSuffix(filesystemPattern, ".md") {
		return "", false, nil
	}
	if filesystemPattern == "" || path.IsAbs(filesystemPattern) || path.Clean(filesystemPattern) != filesystemPattern ||
		strings.ContainsAny(filesystemPattern, "*?[\\") {
		return "", false, fmt.Errorf("prompt embeds must name one exact relative Markdown file")
	}
	filename := filepath.Join(sourceDir, filepath.FromSlash(filesystemPattern))
	info, err := os.Stat(filename)
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("prompt embed is not a regular file")
	}
	relative, err := filepath.Rel(root, filename)
	if err != nil {
		return "", false, err
	}
	return filepath.ToSlash(relative), true, nil
}

func validRepositoryPath(value string) bool {
	return value != "" && value == path.Clean(value) && value != "." &&
		!path.IsAbs(value) && value != ".." && !strings.HasPrefix(value, "../")
}

func validInventoryName(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func validGoIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for position, character := range value {
		if character == '_' || unicode.IsLetter(character) || (position > 0 && unicode.IsDigit(character)) {
			continue
		}
		return false
	}
	return true
}
