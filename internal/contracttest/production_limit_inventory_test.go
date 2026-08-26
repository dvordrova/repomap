package contracttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	productionLimitInventoryVersion = 1
	productionLimitInventoryScope   = "named production hard bounds in Go constants under cmd/internal and JavaScript declarations under internal/jstsproject and internal/report/templates; derived runtime values, protocol/version/identity digest widths, minimum detection heuristics, and ordinary algorithmic literals are out of scope; false-positive names are explicit exclusions"
)

var javascriptDeclarationPattern = regexp.MustCompile(`(?m)^[\t ]*(?:const|let|var)[\t ]+([A-Za-z_$][A-Za-z0-9_$]*)[\t ]*=`)
var anonymousJavaScriptPrefixBoundPattern = regexp.MustCompile(`\.s(?:lice|ubstring|ubstr)\([\t ]*0[\t ]*,[\t ]*[0-9_]+[\t ]*\)`)

type productionLimitInventory struct {
	Version           int                                         `json:"version"`
	Scope             string                                      `json:"scope"`
	LiteralExclusions []productionLimitLiteralComparisonExclusion `json:"literal_exclusions"`
	Entries           []productionLimitInventoryEntry             `json:"entries"`
}

type productionLimitLiteralComparisonExclusion struct {
	File        string `json:"file"`
	Expression  string `json:"expression"`
	Occurrences int    `json:"occurrences"`
	Reason      string `json:"reason"`
}

type productionLimitInventoryEntry struct {
	File     string   `json:"file"`
	Category string   `json:"category"`
	Symbols  []string `json:"symbols"`
	Note     string   `json:"note,omitempty"`
}

type productionLimitSymbol struct {
	File   string
	Symbol string
}

func TestProductionLimitInventoryIsExactAndSourceBound(t *testing.T) {
	root := repositoryRoot(t)
	inventory := readProductionLimitInventory(t, root)
	declared := productionLimitInventorySymbols(t, inventory)
	discovered := discoverProductionLimitSymbols(t, root)

	missing := subtractProductionLimitSymbols(discovered, declared)
	stale := subtractProductionLimitSymbols(declared, discovered)
	if len(missing) != 0 || len(stale) != 0 {
		t.Fatalf(
			"production limit inventory is not exact\nmissing classifications:\n%s\nstale classifications:\n%s",
			formatProductionLimitSymbols(missing),
			formatProductionLimitSymbols(stale),
		)
	}
	assertLiteralComparisonExclusionsAreExact(t, root, inventory.LiteralExclusions)
	assertNoAnonymousProductionBoundary(t, root)
}

func assertLiteralComparisonExclusionsAreExact(
	t *testing.T,
	root string,
	exclusions []productionLimitLiteralComparisonExclusion,
) {
	t.Helper()
	type key struct {
		File       string
		Expression string
	}
	declared := make(map[key]int)
	previous := key{}
	for index, exclusion := range exclusions {
		current := key{File: exclusion.File, Expression: exclusion.Expression}
		if exclusion.File == "" || filepath.IsAbs(exclusion.File) ||
			filepath.ToSlash(filepath.Clean(exclusion.File)) != exclusion.File ||
			exclusion.Expression == "" || exclusion.Occurrences < 1 || strings.TrimSpace(exclusion.Reason) == "" {
			t.Fatalf("invalid production limit literal exclusion %d: %#v", index, exclusion)
		}
		if index > 0 && (current.File < previous.File || current.File == previous.File && current.Expression <= previous.Expression) {
			t.Fatalf("production limit literal exclusions must be sorted: %#v follows %#v", current, previous)
		}
		declared[current] = exclusion.Occurrences
		previous = current
	}

	discovered := make(map[key]int)
	fileSet := token.NewFileSet()
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				comparison, ok := node.(*ast.BinaryExpr)
				if !ok || !isOrderedComparison(comparison.Op) ||
					!numericLiteralAtLeast(comparison.X, 4) && !numericLiteralAtLeast(comparison.Y, 4) {
					return true
				}
				var buffer bytes.Buffer
				if err := format.Node(&buffer, fileSet, comparison); err == nil {
					discovered[key{File: filepath.ToSlash(relative), Expression: buffer.String()}]++
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("discover anonymous numeric comparisons: %v", err)
		}
	}

	var differences []string
	for candidate, occurrences := range discovered {
		if declared[candidate] != occurrences {
			differences = append(differences, fmt.Sprintf(
				"%s :: %s (found %d, classified %d)",
				candidate.File, candidate.Expression, occurrences, declared[candidate],
			))
		}
	}
	for candidate, occurrences := range declared {
		if discovered[candidate] != occurrences {
			differences = append(differences, fmt.Sprintf(
				"%s :: %s (found %d, classified %d)",
				candidate.File, candidate.Expression, discovered[candidate], occurrences,
			))
		}
	}
	if len(differences) != 0 {
		sort.Strings(differences)
		t.Fatalf(
			"anonymous numeric comparisons must be replaced by named limits or exactly excluded:\n  %s",
			strings.Join(differences, "\n  "),
		)
	}
}

func isOrderedComparison(operator token.Token) bool {
	return operator == token.LSS || operator == token.LEQ || operator == token.GTR || operator == token.GEQ
}

func numericLiteralAtLeast(expression ast.Expr, minimum int64) bool {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return false
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(literal.Value, "_", ""), 0, 64)
	return err == nil && value >= minimum
}

func readProductionLimitInventory(t *testing.T, root string) productionLimitInventory {
	t.Helper()
	path := filepath.Join(root, "testdata", "contracts", "production_limits.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read production limit inventory: %v", err)
	}
	var inventory productionLimitInventory
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		t.Fatalf("decode production limit inventory: %v", err)
	}
	if inventory.Version != productionLimitInventoryVersion {
		t.Fatalf("production limit inventory version = %d, want %d", inventory.Version, productionLimitInventoryVersion)
	}
	if inventory.Scope != productionLimitInventoryScope {
		t.Fatalf("production limit inventory scope = %q, want exact contract %q", inventory.Scope, productionLimitInventoryScope)
	}
	return inventory
}

func productionLimitInventorySymbols(t *testing.T, inventory productionLimitInventory) map[productionLimitSymbol]bool {
	t.Helper()
	allowedCategories := map[string]bool{
		"byte-bound":           true,
		"count-bound":          true,
		"depth-bound":          true,
		"text-bound":           true,
		"time-bound":           true,
		"token-bound":          true,
		"excluded-not-a-limit": true,
	}
	result := make(map[productionLimitSymbol]bool)
	previousEntryKey := ""
	for index, entry := range inventory.Entries {
		if !allowedCategories[entry.Category] {
			t.Fatalf("production limit inventory entry %d has unknown category %q", index, entry.Category)
		}
		if entry.Category == "excluded-not-a-limit" && strings.TrimSpace(entry.Note) == "" {
			t.Fatalf("production limit inventory exclusion %d has no reason", index)
		}
		if entry.File == "" || filepath.IsAbs(entry.File) || filepath.ToSlash(filepath.Clean(entry.File)) != entry.File {
			t.Fatalf("production limit inventory entry %d has invalid file %q", index, entry.File)
		}
		if len(entry.Symbols) == 0 || !sort.StringsAreSorted(entry.Symbols) {
			t.Fatalf("production limit inventory entry %d symbols must be non-empty and sorted", index)
		}
		entryKey := entry.File + "\x00" + entry.Category
		if index > 0 && entryKey <= previousEntryKey {
			t.Fatalf("production limit inventory entries must be sorted by file and category: %q follows %q", entryKey, previousEntryKey)
		}
		previousEntryKey = entryKey
		for _, symbol := range entry.Symbols {
			key := productionLimitSymbol{File: entry.File, Symbol: symbol}
			if symbol == "" || result[key] {
				t.Fatalf("production limit inventory has invalid or duplicate symbol %#v", key)
			}
			result[key] = true
		}
	}
	return result
}

func assertNoAnonymousProductionBoundary(t *testing.T, root string) {
	t.Helper()
	fileSet := token.NewFileSet()
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			imports := productionBoundaryImports(parsed)
			var anonymous []token.Position
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.CallExpr:
					if productionBoundaryCall(value, imports) && !expressionReferencesNamedLimit(productionBoundaryArgument(value)) {
						anonymous = append(anonymous, fileSet.Position(value.Pos()))
					}
				case *ast.KeyValueExpr:
					name, ok := value.Key.(*ast.Ident)
					if ok && productionLimitName(name.Name) && !expressionReferencesNamedLimit(value.Value) {
						if _, literal := value.Value.(*ast.BasicLit); literal || expressionContainsBasicLiteral(value.Value) {
							anonymous = append(anonymous, fileSet.Position(value.Pos()))
						}
					}
				}
				return true
			})
			if len(anonymous) != 0 {
				return fmt.Errorf("anonymous hard boundary at %v; bind it to an inventoried named constant", anonymous)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("validate named Go production boundaries: %v", err)
		}
	}

	for _, directory := range []string{"internal/jstsproject", "internal/report/templates"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || (!strings.HasSuffix(path, ".js") && !strings.HasSuffix(path, ".mjs")) {
				return walkErr
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if anonymousJavaScriptPrefixBoundPattern.Match(raw) {
				return fmt.Errorf("anonymous JavaScript prefix bound in %s; bind it to an inventoried named declaration", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("validate named JavaScript production boundaries: %v", err)
		}
	}
}

func productionBoundaryImports(file *ast.File) map[string]string {
	result := make(map[string]string)
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil || importPath == "" {
			continue
		}
		name := pathBase(importPath)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "" && name != "." && name != "_" {
			result[name] = importPath
		}
	}
	return result
}

func pathBase(value string) string {
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func productionBoundaryCall(call *ast.CallExpr, imports map[string]string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	owner, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	importPath := imports[owner.Name]
	return (importPath == "io" && selector.Sel.Name == "LimitReader") ||
		(importPath == "net/http" && selector.Sel.Name == "MaxBytesReader") ||
		(importPath == "context" && selector.Sel.Name == "WithTimeout")
}

func productionBoundaryArgument(call *ast.CallExpr) ast.Expr {
	selector := call.Fun.(*ast.SelectorExpr)
	index := 1
	if selector.Sel.Name == "MaxBytesReader" {
		index = 2
	}
	if index >= len(call.Args) {
		return nil
	}
	return call.Args[index]
}

func expressionReferencesNamedLimit(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		name, ok := node.(*ast.Ident)
		if ok && productionLimitName(name.Name) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func expressionContainsBasicLiteral(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, ok := node.(*ast.BasicLit); ok {
			found = true
			return false
		}
		return !found
	})
	return found
}

func discoverProductionLimitSymbols(t *testing.T, root string) map[productionLimitSymbol]bool {
	t.Helper()
	result := make(map[productionLimitSymbol]bool)
	for _, directory := range []string{"cmd", "internal"} {
		walkRoot := filepath.Join(root, directory)
		err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			switch {
			case strings.HasSuffix(relative, "_test.go"):
				return nil
			case strings.HasSuffix(relative, ".go"):
				return discoverGoProductionLimitSymbols(path, relative, result)
			case productionLimitJavaScriptFile(relative):
				return discoverJavaScriptProductionLimitSymbols(path, relative, result)
			default:
				return nil
			}
		})
		if err != nil {
			t.Fatalf("discover production limits under %s: %v", directory, err)
		}
	}
	return result
}

func discoverGoProductionLimitSymbols(path, relative string, result map[productionLimitSymbol]bool) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", relative, err)
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		declaration, ok := node.(*ast.GenDecl)
		if !ok || declaration.Tok != token.CONST {
			return true
		}
		for _, specification := range declaration.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				if productionLimitName(name.Name) {
					result[productionLimitSymbol{File: relative, Symbol: name.Name}] = true
				}
			}
		}
		return false
	})
	return nil
}

func discoverJavaScriptProductionLimitSymbols(path, relative string, result map[productionLimitSymbol]bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, match := range javascriptDeclarationPattern.FindAllSubmatch(raw, -1) {
		name := string(match[1])
		if productionLimitName(name) {
			result[productionLimitSymbol{File: relative, Symbol: name}] = true
		}
	}
	return nil
}

func productionLimitJavaScriptFile(relative string) bool {
	if !strings.HasSuffix(relative, ".js") && !strings.HasSuffix(relative, ".mjs") {
		return false
	}
	return strings.HasPrefix(relative, "internal/jstsproject/") ||
		strings.HasPrefix(relative, "internal/report/templates/")
}

func productionLimitName(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{"max", "limit", "timeout", "budget", "ceiling", "depth"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, suffix := range []string{"bytes", "tokens", "runes"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func subtractProductionLimitSymbols(left, right map[productionLimitSymbol]bool) []productionLimitSymbol {
	result := make([]productionLimitSymbol, 0)
	for symbol := range left {
		if !right[symbol] {
			result = append(result, symbol)
		}
	}
	sort.Slice(result, func(i, j int) bool { return compareProductionLimitSymbol(result[i], result[j]) < 0 })
	return result
}

func compareProductionLimitSymbol(left, right productionLimitSymbol) int {
	if value := strings.Compare(left.File, right.File); value != 0 {
		return value
	}
	return strings.Compare(left.Symbol, right.Symbol)
}

func formatProductionLimitSymbols(values []productionLimitSymbol) string {
	if len(values) == 0 {
		return "  (none)"
	}
	lines := make([]string, len(values))
	for index, value := range values {
		lines[index] = fmt.Sprintf("  %s :: %s", value.File, value.Symbol)
	}
	return strings.Join(lines, "\n")
}
