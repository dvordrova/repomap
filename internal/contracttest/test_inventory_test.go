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
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const testInventoryContractVersion = 1

type testInventoryContract struct {
	Version             int                    `json:"version"`
	DefaultProfile      testDefaultProfile     `json:"default_profile"`
	GoPackages          []testPackageInventory `json:"go_packages"`
	JavaScriptTestFiles []string               `json:"javascript_test_files"`
	Exceptions          []testException        `json:"exceptions"`
}

type testDefaultProfile struct {
	Resource string `json:"resource"`
	Network  string `json:"network"`
	Provider string `json:"provider"`
	Fixture  string `json:"fixture"`
}

type testPackageInventory struct {
	Directory string   `json:"directory"`
	Package   string   `json:"package"`
	Files     []string `json:"files"`
}

type testException struct {
	Path            string   `json:"path"`
	Characteristics []string `json:"characteristics,omitempty"`
	Resource        string   `json:"resource,omitempty"`
	ExternalTools   []string `json:"external_tools,omitempty"`
	Network         string   `json:"network,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	Fixture         string   `json:"fixture,omitempty"`
}

var automaticallyDetectedTestCharacteristics = map[string]struct{}{
	"ambient-environment": {},
	"conditional-skip":    {},
	"external-checkout":   {},
	"fixture-repository":  {},
	"javascript-runtime":  {},
	"loopback-network":    {},
	"subprocess":          {},
}

var allowedTestCharacteristics = map[string]struct{}{
	"ambient-environment":  {},
	"browser-runtime":      {},
	"compiler":             {},
	"conditional-skip":     {},
	"external-checkout":    {},
	"external-process":     {},
	"filesystem-platform":  {},
	"fixture-repository":   {},
	"interpreter":          {},
	"javascript-runtime":   {},
	"javascript-toolchain": {},
	"loopback-network":     {},
	"repository-fixture":   {},
	"subprocess":           {},
}

func TestRepositoryTestInventoryIsExactAndExceptionsAreClassified(t *testing.T) {
	root := repositoryRoot(t)
	contract := readTestInventoryContract(t, filepath.Join(root, "testdata", "contracts", "tests.json"))
	validateTestInventoryContract(t, contract)

	actualPackages, actualFiles := discoverGoTestInventory(t, root)
	if !reflect.DeepEqual(actualPackages, contract.GoPackages) {
		t.Fatalf("Go test-file/package inventory changed\nactual: %#v\ncontract: %#v\nupdate testdata/contracts/tests.json", actualPackages, contract.GoPackages)
	}
	actualJavaScript := discoverJavaScriptTestFiles(t, root)
	if !reflect.DeepEqual(actualJavaScript, contract.JavaScriptTestFiles) {
		t.Fatalf("JavaScript/browser test-file inventory changed\nactual: %#v\ncontract: %#v\nupdate testdata/contracts/tests.json", actualJavaScript, contract.JavaScriptTestFiles)
	}

	exceptions := make(map[string]testException, len(contract.Exceptions))
	for _, exception := range contract.Exceptions {
		exceptions[exception.Path] = exception
	}
	for _, path := range actualFiles {
		characteristics, tools := inspectGoTestCharacteristics(t, root, path)
		want := exceptions[path]
		wantAutomatic := stringSetIntersection(want.Characteristics, automaticallyDetectedTestCharacteristics)
		if !reflect.DeepEqual(characteristics, wantAutomatic) {
			t.Fatalf("test exception classification for %s = %v, want %v; classify the source-visible exception in testdata/contracts/tests.json", path, characteristics, wantAutomatic)
		}
		for _, tool := range tools {
			if !slicesContain(want.ExternalTools, tool) {
				t.Fatalf("test %s directly invokes external tool %q but its inventory entry does not classify it", path, tool)
			}
		}
	}
}

func readTestInventoryContract(t *testing.T, filename string) testInventoryContract {
	t.Helper()
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read test inventory: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var contract testInventoryContract
	if err := decoder.Decode(&contract); err != nil {
		t.Fatalf("decode test inventory: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("decode test inventory trailing data: %v", err)
	}
	return contract
}

func validateTestInventoryContract(t *testing.T, contract testInventoryContract) {
	t.Helper()
	if contract.Version != testInventoryContractVersion {
		t.Fatalf("test inventory version = %d, want %d", contract.Version, testInventoryContractVersion)
	}
	if contract.DefaultProfile != (testDefaultProfile{
		Resource: "ordinary", Network: "none", Provider: "stubbed-or-none", Fixture: "inline-or-none",
	}) {
		t.Fatalf("test inventory default profile = %#v", contract.DefaultProfile)
	}
	if contract.GoPackages == nil || contract.JavaScriptTestFiles == nil || contract.Exceptions == nil {
		t.Fatal("test inventory collections must be present, including legitimate empty collections")
	}
	seenFiles := make(map[string]struct{})
	previousPackageKey := ""
	for position, inventory := range contract.GoPackages {
		packageKey := inventory.Directory + "\x00" + inventory.Package
		if inventory.Directory == "" || inventory.Directory != filepath.ToSlash(filepath.Clean(inventory.Directory)) ||
			(position > 0 && packageKey <= previousPackageKey) || inventory.Package == "" || inventory.Files == nil {
			t.Fatalf("invalid Go test package inventory at %d: %#v", position, inventory)
		}
		previousPackageKey = packageKey
		if !sort.StringsAreSorted(inventory.Files) {
			t.Fatalf("Go test files for %s are not sorted", inventory.Directory)
		}
		for _, name := range inventory.Files {
			if filepath.Base(name) != name || !strings.HasSuffix(name, "_test.go") {
				t.Fatalf("invalid Go test filename %q in %s", name, inventory.Directory)
			}
			path := inventory.Directory + "/" + name
			if _, duplicate := seenFiles[path]; duplicate {
				t.Fatalf("duplicate Go test file %q", path)
			}
			seenFiles[path] = struct{}{}
		}
	}
	if !sort.StringsAreSorted(contract.JavaScriptTestFiles) || hasDuplicateStrings(contract.JavaScriptTestFiles) {
		t.Fatal("JavaScript test-file inventory must be sorted and unique")
	}
	previousPath := ""
	for position, exception := range contract.Exceptions {
		if exception.Path == "" || (position > 0 && exception.Path <= previousPath) {
			t.Fatalf("test exceptions are not path-sorted and unique at %d: %#v", position, exception)
		}
		previousPath = exception.Path
		if _, exists := seenFiles[exception.Path]; !exists && !slicesContain(contract.JavaScriptTestFiles, exception.Path) {
			t.Fatalf("test exception cites unowned test file %q", exception.Path)
		}
		if !sort.StringsAreSorted(exception.Characteristics) || hasDuplicateStrings(exception.Characteristics) ||
			!sort.StringsAreSorted(exception.ExternalTools) || hasDuplicateStrings(exception.ExternalTools) {
			t.Fatalf("test exception lists must be sorted and unique: %#v", exception)
		}
		for _, characteristic := range exception.Characteristics {
			if _, allowed := allowedTestCharacteristics[characteristic]; !allowed {
				t.Fatalf("unknown test characteristic %q for %s", characteristic, exception.Path)
			}
		}
		for _, tool := range exception.ExternalTools {
			if !slicesContain([]string{"chrome", "git", "go", "node", "npm", "python3", "test-binary"}, tool) {
				t.Fatalf("unknown external test tool %q for %s", tool, exception.Path)
			}
		}
		if slicesContain(exception.Characteristics, "subprocess") && len(exception.ExternalTools) == 0 {
			t.Fatalf("subprocess test %s must classify at least one external tool, including dynamic commands", exception.Path)
		}
		if !slicesContain([]string{"", "browser-vm", "compiler", "filesystem-platform", "interpreter", "process"}, exception.Resource) ||
			!slicesContain([]string{"", "loopback", "external"}, exception.Network) ||
			!slicesContain([]string{"", "real-provider"}, exception.Provider) ||
			!slicesContain([]string{"", "cumulative-go", "cumulative-jsts", "cumulative-python", "cumulative-python-and-jsts", "external-checkout", "temporary-repository"}, exception.Fixture) {
			t.Fatalf("unknown test exception profile: %#v", exception)
		}
		if slicesContain(exception.Characteristics, "loopback-network") && exception.Network != "loopback" {
			t.Fatalf("loopback test %s has no loopback network profile", exception.Path)
		}
		if slicesContain(exception.Characteristics, "external-checkout") && exception.Fixture != "external-checkout" {
			t.Fatalf("external-checkout test %s has no external-checkout fixture profile", exception.Path)
		}
		if exception.Provider == "real-provider" && exception.Network != "external" {
			t.Fatalf("real-provider test %s must explicitly classify external network access", exception.Path)
		}
		if len(exception.Characteristics) == 0 && exception.Resource == "" && len(exception.ExternalTools) == 0 &&
			exception.Network == "" && exception.Provider == "" && exception.Fixture == "" {
			t.Fatalf("empty test exception entry for %s", exception.Path)
		}
	}
}

func discoverGoTestInventory(t *testing.T, root string) ([]testPackageInventory, []string) {
	t.Helper()
	byPackage := make(map[string]*testPackageInventory)
	files := make([]string, 0)
	for _, sourceRoot := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, sourceRoot), func(filename string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.PackageClauseOnly)
			if err != nil {
				return fmt.Errorf("parse package clause for %s: %w", relative, err)
			}
			directory := filepath.ToSlash(filepath.Dir(relative))
			packageKey := directory + "\x00" + parsed.Name.Name
			inventory := byPackage[packageKey]
			if inventory == nil {
				inventory = &testPackageInventory{Directory: directory, Package: parsed.Name.Name, Files: []string{}}
				byPackage[packageKey] = inventory
			}
			inventory.Files = append(inventory.Files, entry.Name())
			files = append(files, relative)
			return nil
		})
		if err != nil {
			t.Fatalf("discover Go tests below %s: %v", sourceRoot, err)
		}
	}
	packageKeys := make([]string, 0, len(byPackage))
	for packageKey := range byPackage {
		packageKeys = append(packageKeys, packageKey)
	}
	sort.Strings(packageKeys)
	result := make([]testPackageInventory, 0, len(packageKeys))
	for _, packageKey := range packageKeys {
		inventory := *byPackage[packageKey]
		sort.Strings(inventory.Files)
		result = append(result, inventory)
	}
	sort.Strings(files)
	return result, files
}

func discoverJavaScriptTestFiles(t *testing.T, root string) []string {
	t.Helper()
	result := make([]string, 0)
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && filename != root {
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if relative == ".git" || relative == ".bin" || relative == "artifacts" ||
				relative == "testdata/repositories" || entry.Name() == "node_modules" ||
				entry.Name() == "dist" || entry.Name() == "build" || entry.Name() == "coverage" {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() || !isJavaScriptTestFilename(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("discover JavaScript tests: %v", err)
	}
	sort.Strings(result)
	return result
}

func isJavaScriptTestFilename(name string) bool {
	for _, suffix := range []string{
		".test.js", ".spec.js", "_test.js", "_spec.js",
		".test.mjs", ".spec.mjs", "_test.mjs", "_spec.mjs",
		".test.cjs", ".spec.cjs", "_test.cjs", "_spec.cjs",
		".test.ts", ".spec.ts", "_test.ts", "_spec.ts",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func inspectGoTestCharacteristics(t *testing.T, root, relative string) ([]string, []string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse test characteristics for %s: %v", relative, err)
	}
	aliases := make(map[string]string)
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("decode import in %s: %v", relative, err)
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = importPath
	}
	characteristics := make(map[string]struct{})
	tools := make(map[string]struct{})
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if function, ok := call.Fun.(*ast.Ident); ok && function.Name == "materializeFixtureRepository" {
			characteristics["fixture-repository"] = struct{}{}
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		owner, _ := selector.X.(*ast.Ident)
		if owner == nil {
			return true
		}
		switch aliases[owner.Name] {
		case "os":
			if selector.Sel.Name == "Getenv" || selector.Sel.Name == "LookupEnv" {
				characteristics["ambient-environment"] = struct{}{}
				if stringArgument(call, 0) == "REPOMAP_JSTS_CALT0DO_ROOT" {
					characteristics["external-checkout"] = struct{}{}
				}
			}
		case "os/exec":
			if selector.Sel.Name == "Command" || selector.Sel.Name == "CommandContext" {
				characteristics["subprocess"] = struct{}{}
				argument := 0
				if selector.Sel.Name == "CommandContext" {
					argument = 1
				}
				if tool := stringArgument(call, argument); tool != "" {
					tools[tool] = struct{}{}
				}
			}
			if selector.Sel.Name == "LookPath" {
				if tool := stringArgument(call, 0); tool != "" {
					tools[tool] = struct{}{}
					if tool == "node" {
						characteristics["javascript-runtime"] = struct{}{}
					}
				}
			}
		case "net/http/httptest":
			if selector.Sel.Name == "NewServer" || selector.Sel.Name == "NewTLSServer" || selector.Sel.Name == "NewUnstartedServer" {
				characteristics["loopback-network"] = struct{}{}
			}
		}
		if selector.Sel.Name == "Skip" || selector.Sel.Name == "Skipf" || selector.Sel.Name == "SkipNow" {
			characteristics["conditional-skip"] = struct{}{}
		}
		return true
	})
	return sortedStringSet(characteristics), sortedStringSet(tools)
}

func stringArgument(call *ast.CallExpr, position int) string {
	if position < 0 || position >= len(call.Args) {
		return ""
	}
	literal, ok := call.Args[position].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil || strings.Contains(value, "/") {
		return ""
	}
	return value
}

func stringSetIntersection(values []string, allowed map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func hasDuplicateStrings(values []string) bool {
	for position := 1; position < len(values); position++ {
		if values[position] == values[position-1] {
			return true
		}
	}
	return false
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
