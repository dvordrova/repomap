// Package gocoreobject owns the immutable, target-scoped index of exact Go
// declarations captured during the ordinary typed-program lifetime.
package gocoreobject

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version = 2

	MaxPackages  = 65_536
	MaxTypes     = 65_536
	MaxCallables = 65_536
	MaxTextBytes = 16 * 1024
	// MaxIndexBytes bounds the complete canonical owned substrate. Consumers
	// that embed it in a browser-facing artifact should impose a smaller bound
	// or persist only the exact selected projection.
	MaxIndexBytes = 32 * 1024 * 1024
)

type TypeKind string

const (
	TypeNamed     TypeKind = "named"
	TypeStruct    TypeKind = "struct"
	TypeInterface TypeKind = "interface"
	TypeAlias     TypeKind = "alias"
)

func (kind TypeKind) Valid() bool {
	switch kind {
	case TypeNamed, TypeStruct, TypeInterface, TypeAlias:
		return true
	default:
		return false
	}
}

type CallableKind string

const (
	CallableFunction CallableKind = "function"
	CallableMethod   CallableKind = "method"
)

func (kind CallableKind) Valid() bool {
	return kind == CallableFunction || kind == CallableMethod
}

type Scenario struct {
	ID     string   `json:"id"`
	GOOS   string   `json:"goos"`
	GOARCH string   `json:"goarch"`
	Tags   []string `json:"tags"`
}

// Scope binds the index to the exact selected analysis target. TargetPackages
// are its executable or public-API roots; Packages below retain the complete
// admitted package closure inspected for that target.
type Scope struct {
	TargetRef        string   `json:"target_ref"`
	TargetKind       string   `json:"target_kind"`
	TargetModuleID   string   `json:"target_module_id"`
	TargetModulePath string   `json:"target_module_path"`
	TargetModuleDir  string   `json:"target_module_dir"`
	TargetPackage    string   `json:"target_package,omitempty"`
	TargetPackages   []string `json:"target_packages"`
}

const (
	ScopeExecutablePackage = "executable_package"
	ScopeModuleLibrary     = "module_library"
)

type Package struct {
	ModuleID             string `json:"module_id"`
	Module               string `json:"module"`
	ModuleDir            string `json:"module_dir"`
	Path                 string `json:"path"`
	RepresentativeSource string `json:"representative_source"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type TypeDeclaration struct {
	ID       string   `json:"id"`
	Kind     TypeKind `json:"kind"`
	Package  string   `json:"package"`
	Name     string   `json:"name"`
	Exported bool     `json:"exported"`
	Location Location `json:"location"`
}

type CallableDeclaration struct {
	ID               string       `json:"id"`
	Kind             CallableKind `json:"kind"`
	Package          string       `json:"package"`
	Name             string       `json:"name"`
	Receiver         string       `json:"receiver,omitempty"`
	Signature        string       `json:"signature"`
	Exported         bool         `json:"exported"`
	Location         Location     `json:"location"`
	DirectCallNodeID string       `json:"direct_call_node_id,omitempty"`
}

type Coverage struct {
	PackagesIndexed  int `json:"packages_indexed"`
	TypesIndexed     int `json:"types_indexed"`
	CallablesIndexed int `json:"callables_indexed"`
}

// Input is the complete exact fact projection prepared by the Go adapter. New
// canonicalizes, assigns local identities, validates, and seals it.
type Input struct {
	Scenario  Scenario
	Scope     Scope
	Packages  []Package
	Types     []TypeDeclaration
	Callables []CallableDeclaration
}

type Index struct {
	Version   int                   `json:"version"`
	Scenario  Scenario              `json:"scenario"`
	Scope     Scope                 `json:"scope"`
	Packages  []Package             `json:"packages"`
	Types     []TypeDeclaration     `json:"types"`
	Callables []CallableDeclaration `json:"callables"`
	Coverage  Coverage              `json:"coverage"`
	SHA256    string                `json:"sha256"`
}

func New(input Input) (Index, error) {
	if err := preflightInput(input); err != nil {
		return Index{}, err
	}
	index := Index{
		Version: Version,
		Scenario: Scenario{
			ID: input.Scenario.ID, GOOS: input.Scenario.GOOS, GOARCH: input.Scenario.GOARCH,
			Tags: append([]string(nil), input.Scenario.Tags...),
		},
		Scope: Scope{
			TargetRef: input.Scope.TargetRef, TargetKind: input.Scope.TargetKind,
			TargetModuleID: input.Scope.TargetModuleID, TargetModulePath: input.Scope.TargetModulePath,
			TargetModuleDir: input.Scope.TargetModuleDir, TargetPackage: input.Scope.TargetPackage,
			TargetPackages: append([]string(nil), input.Scope.TargetPackages...),
		},
		Packages:  append([]Package(nil), input.Packages...),
		Types:     append([]TypeDeclaration(nil), input.Types...),
		Callables: append([]CallableDeclaration(nil), input.Callables...),
	}
	sort.Strings(index.Scenario.Tags)
	index.Scenario.Tags = compactStrings(index.Scenario.Tags)
	sort.Strings(index.Scope.TargetPackages)
	index.Scope.TargetPackages = compactStrings(index.Scope.TargetPackages)

	packageByPath := make(map[string]Package, len(index.Packages))
	for _, pkg := range index.Packages {
		if previous, exists := packageByPath[pkg.Path]; exists && previous != pkg {
			return Index{}, fmt.Errorf("go core object index: conflicting package %q", pkg.Path)
		}
		packageByPath[pkg.Path] = pkg
	}
	index.Packages = index.Packages[:0]
	for _, pkg := range packageByPath {
		index.Packages = append(index.Packages, pkg)
	}
	sort.Slice(index.Packages, func(i, j int) bool { return packageKey(index.Packages[i]) < packageKey(index.Packages[j]) })

	for position := range index.Types {
		declaration := &index.Types[position]
		pkg, exists := packageByPath[declaration.Package]
		if !exists {
			return Index{}, fmt.Errorf("go core object index: type cites unknown package %q", declaration.Package)
		}
		if declaration.ID != "" {
			return Index{}, fmt.Errorf("go core object index: adapter supplied a type identity")
		}
		declaration.ID = stableID(
			"go-core-type", index.Scenario.ID, pkg.ModuleID, declaration.Package,
			declaration.Name, locationKey(declaration.Location),
		)
	}
	for position := range index.Callables {
		declaration := &index.Callables[position]
		pkg, exists := packageByPath[declaration.Package]
		if !exists {
			return Index{}, fmt.Errorf("go core object index: callable cites unknown package %q", declaration.Package)
		}
		if declaration.ID != "" {
			return Index{}, fmt.Errorf("go core object index: adapter supplied a callable identity")
		}
		declaration.ID = stableID(
			"go-core-callable", index.Scenario.ID, pkg.ModuleID, declaration.Package,
			string(declaration.Kind), declaration.Receiver, declaration.Name,
			locationKey(declaration.Location),
		)
	}
	sort.Slice(index.Types, func(i, j int) bool { return typeKey(index.Types[i]) < typeKey(index.Types[j]) })
	sort.Slice(index.Callables, func(i, j int) bool { return callableKey(index.Callables[i]) < callableKey(index.Callables[j]) })
	index.Coverage = Coverage{
		PackagesIndexed: len(index.Packages), TypesIndexed: len(index.Types),
		CallablesIndexed: len(index.Callables),
	}
	digest, err := indexDigest(index)
	if err != nil {
		return Index{}, err
	}
	index.SHA256 = digest
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

func preflightInput(input Input) error {
	if len(input.Packages) == 0 || len(input.Packages) > MaxPackages || len(input.Types) > MaxTypes ||
		len(input.Callables) > MaxCallables {
		return fmt.Errorf("go core object index: collection bound exceeded")
	}
	aggregate := 0
	add := func(values ...string) error {
		for _, value := range values {
			if len(value) > MaxTextBytes {
				return fmt.Errorf("go core object index: scalar bound exceeded")
			}
			aggregate += len(value)
			if aggregate > MaxIndexBytes {
				return fmt.Errorf("go core object index: aggregate scalar bound exceeded")
			}
		}
		return nil
	}
	if err := add(
		input.Scenario.ID, input.Scenario.GOOS, input.Scenario.GOARCH,
		input.Scope.TargetRef, input.Scope.TargetKind, input.Scope.TargetModuleID,
		input.Scope.TargetModulePath, input.Scope.TargetModuleDir, input.Scope.TargetPackage,
	); err != nil {
		return err
	}
	if err := add(input.Scenario.Tags...); err != nil {
		return err
	}
	if err := add(input.Scope.TargetPackages...); err != nil {
		return err
	}
	for _, pkg := range input.Packages {
		if err := add(pkg.ModuleID, pkg.Module, pkg.ModuleDir, pkg.Path, pkg.RepresentativeSource); err != nil {
			return err
		}
	}
	for _, declaration := range input.Types {
		if err := add(declaration.ID, string(declaration.Kind), declaration.Package, declaration.Name, declaration.Location.Path); err != nil {
			return err
		}
	}
	for _, declaration := range input.Callables {
		if err := add(
			declaration.ID, string(declaration.Kind), declaration.Package, declaration.Name,
			declaration.Receiver, declaration.Signature, declaration.Location.Path,
			declaration.DirectCallNodeID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (index Index) Snapshot() Index {
	result := index
	result.Scenario.Tags = append([]string(nil), index.Scenario.Tags...)
	result.Scope.TargetPackages = append([]string(nil), index.Scope.TargetPackages...)
	result.Packages = append([]Package(nil), index.Packages...)
	result.Types = append([]TypeDeclaration(nil), index.Types...)
	result.Callables = append([]CallableDeclaration(nil), index.Callables...)
	return result
}

func (index Index) Validate() error {
	if index.Version != Version || !validText(index.Scenario.ID) || !validText(index.Scenario.GOOS) ||
		!validText(index.Scenario.GOARCH) || !canonicalStrings(index.Scenario.Tags) {
		return fmt.Errorf("go core object index: invalid scenario")
	}
	if !validText(index.Scope.TargetRef) || !validText(index.Scope.TargetKind) ||
		!validText(index.Scope.TargetModuleID) || !validText(index.Scope.TargetModulePath) ||
		!validDirectory(index.Scope.TargetModuleDir) || !canonicalStrings(index.Scope.TargetPackages) ||
		len(index.Scope.TargetPackages) == 0 || index.Scope.TargetKind != ScopeExecutablePackage &&
		index.Scope.TargetKind != ScopeModuleLibrary {
		return fmt.Errorf("go core object index: invalid target scope")
	}
	if index.Scope.TargetPackage != "" && !validText(index.Scope.TargetPackage) {
		return fmt.Errorf("go core object index: invalid target package")
	}
	if len(index.Packages) == 0 || len(index.Packages) > MaxPackages || len(index.Types) > MaxTypes ||
		len(index.Callables) > MaxCallables {
		return fmt.Errorf("go core object index: collection bound exceeded")
	}
	packages := make(map[string]Package, len(index.Packages))
	for position, pkg := range index.Packages {
		if !validText(pkg.ModuleID) || !validText(pkg.Module) || !validDirectory(pkg.ModuleDir) ||
			!validText(pkg.Path) || !validSourcePath(pkg.RepresentativeSource) ||
			position > 0 && packageKey(index.Packages[position-1]) >= packageKey(pkg) {
			return fmt.Errorf("go core object index: invalid package")
		}
		if _, duplicate := packages[pkg.Path]; duplicate {
			return fmt.Errorf("go core object index: duplicate package")
		}
		packages[pkg.Path] = pkg
	}
	for _, packagePath := range index.Scope.TargetPackages {
		pkg, exists := packages[packagePath]
		if !exists || pkg.Module != index.Scope.TargetModulePath || pkg.ModuleDir != index.Scope.TargetModuleDir {
			return fmt.Errorf("go core object index: target package is outside the indexed module")
		}
	}
	if index.Scope.TargetKind == ScopeExecutablePackage &&
		(index.Scope.TargetPackage == "" || len(index.Scope.TargetPackages) != 1 ||
			index.Scope.TargetPackages[0] != index.Scope.TargetPackage) ||
		index.Scope.TargetKind == ScopeModuleLibrary && index.Scope.TargetPackage != "" {
		return fmt.Errorf("go core object index: invalid target package boundary")
	}
	typeIDs := make(map[string]struct{}, len(index.Types))
	for position, declaration := range index.Types {
		if _, exists := packages[declaration.Package]; !exists || !declaration.Kind.Valid() ||
			!validText(declaration.ID) || !validIdentifier(declaration.Name) || !validLocation(declaration.Location) ||
			position > 0 && typeKey(index.Types[position-1]) >= typeKey(declaration) {
			return fmt.Errorf("go core object index: invalid type declaration")
		}
		if _, duplicate := typeIDs[declaration.ID]; duplicate {
			return fmt.Errorf("go core object index: duplicate type declaration")
		}
		typeIDs[declaration.ID] = struct{}{}
	}
	callableIDs := make(map[string]struct{}, len(index.Callables))
	for position, declaration := range index.Callables {
		if _, exists := packages[declaration.Package]; !exists || !declaration.Kind.Valid() ||
			!validText(declaration.ID) || !validIdentifier(declaration.Name) || !validText(declaration.Signature) ||
			!validLocation(declaration.Location) || declaration.DirectCallNodeID != "" && !validText(declaration.DirectCallNodeID) ||
			position > 0 && callableKey(index.Callables[position-1]) >= callableKey(declaration) {
			return fmt.Errorf("go core object index: invalid callable declaration")
		}
		if declaration.Kind == CallableFunction && declaration.Receiver != "" ||
			declaration.Kind == CallableMethod && !validText(declaration.Receiver) {
			return fmt.Errorf("go core object index: invalid callable receiver")
		}
		if _, duplicate := callableIDs[declaration.ID]; duplicate {
			return fmt.Errorf("go core object index: duplicate callable declaration")
		}
		callableIDs[declaration.ID] = struct{}{}
	}
	if index.Coverage != (Coverage{
		PackagesIndexed: len(index.Packages), TypesIndexed: len(index.Types),
		CallablesIndexed: len(index.Callables),
	}) {
		return fmt.Errorf("go core object index: invalid coverage")
	}
	digest, err := indexDigest(index)
	if err != nil {
		return err
	}
	if len(index.SHA256) != sha256.Size*2 || index.SHA256 != digest {
		return fmt.Errorf("go core object index: sha256 mismatch")
	}
	if _, err := hex.DecodeString(index.SHA256); err != nil {
		return fmt.Errorf("go core object index: invalid sha256: %w", err)
	}
	return nil
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, value := range append([]string{prefix}, fields...) {
		digest.Write([]byte{0})
		digest.Write([]byte(value))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func indexDigest(index Index) (string, error) {
	index.SHA256 = ""
	encoded, err := json.Marshal(index)
	if err != nil {
		return "", fmt.Errorf("go core object index: encode digest material: %w", err)
	}
	if len(encoded) > MaxIndexBytes {
		return "", fmt.Errorf(
			"go core object index: canonical substrate is %d bytes, limit is %d",
			len(encoded), MaxIndexBytes,
		)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func packageKey(pkg Package) string {
	return strings.Join([]string{pkg.ModuleDir, pkg.Module, pkg.Path, pkg.ModuleID}, "\x00")
}

func typeKey(value TypeDeclaration) string {
	return strings.Join([]string{value.Package, locationKey(value.Location), value.Name, string(value.Kind), value.ID}, "\x00")
}

func callableKey(value CallableDeclaration) string {
	return strings.Join([]string{
		value.Package, locationKey(value.Location), value.Receiver, value.Name, string(value.Kind), value.ID,
	}, "\x00")
}

func locationKey(value Location) string {
	return fmt.Sprintf("%s:%09d:%09d", value.Path, value.Line, value.Column)
}

func validLocation(value Location) bool {
	return validSourcePath(value.Path) && value.Line > 0 && value.Column > 0
}

func validSourcePath(value string) bool {
	return fs.ValidPath(value) && strings.HasSuffix(value, ".go")
}

func validDirectory(value string) bool {
	return value == "." || fs.ValidPath(value)
}

func validIdentifier(value string) bool {
	return validText(value) && token.IsIdentifier(value) && value != "_"
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > MaxTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func canonicalStrings(values []string) bool {
	if !sort.StringsAreSorted(values) {
		return false
	}
	for position, value := range values {
		if !validText(value) || position > 0 && values[position-1] == value {
			return false
		}
	}
	return true
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
