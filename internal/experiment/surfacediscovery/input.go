package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	ProcessEntryAnchorGoMain = "go_main_function"

	ExecutableRolePrimaryApplication = "primary_application"
	ExecutableRoleSecondaryService   = "secondary_service"
	ExecutableRoleTooling            = "tooling"
	ExecutableRoleTestOrHelper       = "test_or_helper"
	ExecutableRoleUnknown            = "unknown"

	AvailabilityAvailable   = "available"
	AvailabilityUnavailable = "unavailable"
	AvailabilityUnknown     = "unknown"

	maxPackageDiagnostics  = 128
	maxUnavailablePackages = 128
	maxDiagnosticBytes     = 512
)

// Input carries deterministic facts produced outside typed surface discovery.
// Entrypoint anchors remain authoritative even when their package is ill-typed.
type Input struct {
	RepositoryName string
	ModuleDirs     []string
	Entrypoints    []EntrypointInput
}

type EntrypointInput struct {
	Package    string
	PackageDir string
	ModuleDir  string
	Kind       string
	Anchors    []EntrypointAnchorInput
}

type EntrypointAnchorInput struct {
	Kind   string
	Path   string
	Line   int
	Column int
}

type processEntrypoint struct {
	packagePath       string
	packageDir        string
	kind              string
	anchor            EntrypointAnchorInput
	owner             string
	role              string
	availability      string
	unavailableReason string
}

func normalizeInput(root string, input Input) (Input, []processEntrypoint) {
	input.RepositoryName = strings.TrimSpace(input.RepositoryName)
	if input.RepositoryName == "" {
		input.RepositoryName = filepath.Base(root)
	}
	for _, entrypoint := range input.Entrypoints {
		input.ModuleDirs = append(input.ModuleDirs, entrypoint.ModuleDir)
	}
	input.ModuleDirs = normalizeModuleDirs(input.ModuleDirs)

	var result []processEntrypoint
	for _, entrypoint := range input.Entrypoints {
		packagePath := strings.TrimSpace(entrypoint.Package)
		packageDir := cleanRepositoryPath(entrypoint.PackageDir)
		if packagePath == "" {
			continue
		}
		for _, anchor := range entrypoint.Anchors {
			anchor.Path = cleanRepositoryPath(anchor.Path)
			if anchor.Kind != ProcessEntryAnchorGoMain || anchor.Line <= 0 ||
				anchor.Path == "" || path.Ext(anchor.Path) != ".go" {
				continue
			}
			owner := packageDir
			if owner == "" || owner == "." {
				owner = cleanRepositoryPath(path.Dir(anchor.Path))
			}
			if owner == "" || owner == "." {
				owner = packagePath
			}
			result = append(result, processEntrypoint{
				packagePath:  packagePath,
				packageDir:   packageDir,
				kind:         strings.TrimSpace(entrypoint.Kind),
				anchor:       anchor,
				owner:        owner,
				availability: AvailabilityUnknown,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		return left.packagePath+"\x00"+left.anchor.Path+"\x00"+strconv.Itoa(left.anchor.Line) <
			right.packagePath+"\x00"+right.anchor.Path+"\x00"+strconv.Itoa(right.anchor.Line)
	})
	result = compactProcessEntrypoints(result)
	for index := range result {
		result[index].role = classifyExecutableRole(input.RepositoryName, result[index])
	}
	return input, result
}

func normalizeModuleDirs(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = cleanRepositoryPath(value)
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
	}
	if len(unique) == 0 {
		return []string{"."}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func compactProcessEntrypoints(input []processEntrypoint) []processEntrypoint {
	result := input[:0]
	previous := ""
	for _, entrypoint := range input {
		key := entrypoint.packagePath + "\x00" + entrypoint.anchor.Path + "\x00" + strconv.Itoa(entrypoint.anchor.Line)
		if key == previous {
			continue
		}
		previous = key
		result = append(result, entrypoint)
	}
	return result
}

func classifyExecutableRole(repositoryName string, entrypoint processEntrypoint) string {
	segments := repositoryPathSegments(entrypoint.packageDir, path.Dir(entrypoint.anchor.Path))
	joined := strings.Join(segments, "/")
	if hasAnySegment(segments, "dev", "tool", "tools", "hack", "script", "scripts", "build", "release", "generator", "generators") ||
		(hasAnySegment(segments, "helper", "helpers") && (strings.Contains(joined, "build") || strings.Contains(joined, "release"))) ||
		entrypoint.kind == "tool" {
		return ExecutableRoleTooling
	}
	if hasAnySegment(segments, "test", "tests", "testing", "testutil", "testdata", "helper", "helpers", "example", "examples") ||
		entrypoint.kind == "test_binary" || entrypoint.kind == "example" {
		return ExecutableRoleTestOrHelper
	}
	if entrypoint.packageDir == "." ||
		(entrypoint.packageDir == "" && path.Dir(entrypoint.anchor.Path) == ".") ||
		strings.EqualFold(path.Base(entrypoint.packageDir), strings.TrimSpace(repositoryName)) ||
		entrypoint.kind == "primary_binary" {
		return ExecutableRolePrimaryApplication
	}
	if entrypoint.owner != "" {
		return ExecutableRoleSecondaryService
	}
	return ExecutableRoleUnknown
}

func repositoryPathSegments(values ...string) []string {
	var result []string
	for _, value := range values {
		for _, segment := range strings.Split(strings.ToLower(cleanRepositoryPath(value)), "/") {
			if segment != "" && segment != "." {
				result = append(result, segment)
			}
		}
	}
	return result
}

func hasAnySegment(segments []string, candidates ...string) bool {
	for _, segment := range segments {
		for _, candidate := range candidates {
			if segment == candidate {
				return true
			}
		}
	}
	return false
}

func cleanRepositoryPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || path.IsAbs(value) {
		return ""
	}
	value = path.Clean(value)
	if value == ".." || strings.HasPrefix(value, "../") || !fs.ValidPath(value) {
		return ""
	}
	return value
}

func packageSafeForSSA(pkg *packages.Package) bool {
	return pkg != nil && pkg.PkgPath != "" && pkg.Types != nil && !pkg.IllTyped && len(pkg.Errors) == 0
}

func (a *analyzer) recordPackageLoadOutcomes(allPackages map[string]*packages.Package) {
	for index := range a.processEntrypoints {
		entrypoint := &a.processEntrypoints[index]
		pkg := allPackages[entrypoint.packagePath]
		if packageSafeForSSA(pkg) {
			entrypoint.availability = AvailabilityAvailable
			continue
		}
		entrypoint.availability = AvailabilityUnavailable
		if pkg == nil {
			entrypoint.unavailableReason = "typed package was not loaded under the recorded module scope"
		} else {
			entrypoint.unavailableReason = "package or dependency closure is ill-typed under the recorded build scenario"
		}
	}

	owners := a.packageExecutableOwners(allPackages)
	diagnosticsByPackage := make(map[string][]string)
	var diagnostics []PackageDiagnostic
	var unavailable []PackageAvailability
	seenDiagnostics := make(map[string]struct{})
	seenUnavailable := make(map[string]struct{})
	diagnosticCount := 0
	unavailableCount := 0
	ordered := make([]*packages.Package, 0, len(allPackages))
	for _, pkg := range allPackages {
		if isRepositoryPackage(a.root, pkg) {
			ordered = append(ordered, pkg)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return packageIdentity(ordered[i]) < packageIdentity(ordered[j]) })
	for _, pkg := range ordered {
		packageID := packageIdentity(pkg)
		owner, role := uniquePackageOwner(owners[packageID])
		packageDiagnostics := append([]packages.Error(nil), pkg.Errors...)
		sort.Slice(packageDiagnostics, func(i, j int) bool {
			return packageDiagnostics[i].Error() < packageDiagnostics[j].Error()
		})
		for _, packageError := range packageDiagnostics {
			location := a.packageErrorLocation(pkg, packageError.Pos)
			message := boundedDiagnosticMessage(packageError.Msg, a.root)
			id := stableDiagnosticID(packageID, packageErrorKind(packageError.Kind), message, location)
			if _, duplicate := seenDiagnostics[id]; duplicate {
				continue
			}
			seenDiagnostics[id] = struct{}{}
			diagnosticCount++
			if len(diagnostics) >= maxPackageDiagnostics {
				continue
			}
			diagnostics = append(diagnostics, PackageDiagnostic{
				ID: id, Kind: packageErrorKind(packageError.Kind), Message: message,
				Package: packageID, PackageName: pkg.Name,
				OwningExecutable: owner, ExecutableRole: role,
				Availability: AvailabilityUnavailable, Location: location,
			})
			diagnosticsByPackage[packageID] = append(diagnosticsByPackage[packageID], id)
		}
		if !packageSafeForSSA(pkg) {
			seenUnavailable[packageID] = struct{}{}
			unavailableCount++
			reason := "unsafe_dependency_closure"
			if len(pkg.Errors) > 0 {
				reason = "package_errors"
			}
			if len(unavailable) < maxUnavailablePackages {
				unavailable = append(unavailable, PackageAvailability{
					Package: packageID, PackageName: pkg.Name,
					OwningExecutable: owner, ExecutableRole: role,
					Availability: AvailabilityUnavailable, Reason: reason,
					DiagnosticIDs: append([]string(nil), diagnosticsByPackage[packageID]...),
				})
			}
		}
	}
	for _, entrypoint := range a.processEntrypoints {
		if entrypoint.availability != AvailabilityUnavailable || allPackages[entrypoint.packagePath] != nil {
			continue
		}
		if _, duplicate := seenUnavailable[entrypoint.packagePath]; duplicate {
			continue
		}
		seenUnavailable[entrypoint.packagePath] = struct{}{}
		unavailableCount++
		if len(unavailable) < maxUnavailablePackages {
			unavailable = append(unavailable, PackageAvailability{
				Package: entrypoint.packagePath, OwningExecutable: entrypoint.owner,
				ExecutableRole: entrypoint.role, Availability: AvailabilityUnavailable,
				Reason: "typed_package_not_loaded", DiagnosticIDs: []string{},
			})
		}
	}
	a.result.Coverage.PackageDiagnosticCount = diagnosticCount
	a.result.Coverage.UnavailablePackageCount = unavailableCount
	a.result.Coverage.PackageDiagnostics = diagnostics
	a.result.Coverage.UnavailablePackages = unavailable
}

func (a *analyzer) packageExecutableOwners(allPackages map[string]*packages.Package) map[string][]processEntrypoint {
	result := make(map[string][]processEntrypoint)
	for _, entrypoint := range a.processEntrypoints {
		pkg := allPackages[entrypoint.packagePath]
		if pkg == nil {
			result[entrypoint.packagePath] = append(result[entrypoint.packagePath], entrypoint)
			continue
		}
		visited := make(map[string]bool)
		var visit func(*packages.Package)
		visit = func(current *packages.Package) {
			if current == nil || visited[packageIdentity(current)] {
				return
			}
			visited[packageIdentity(current)] = true
			if !isRepositoryPackage(a.root, current) {
				return
			}
			result[packageIdentity(current)] = append(result[packageIdentity(current)], entrypoint)
			for _, imported := range current.Imports {
				visit(imported)
			}
		}
		visit(pkg)
	}
	return result
}

func uniquePackageOwner(owners []processEntrypoint) (string, string) {
	unique := make(map[string]string)
	for _, owner := range owners {
		if owner.owner != "" {
			unique[owner.owner] = owner.role
		}
	}
	if len(unique) != 1 {
		return "", ExecutableRoleUnknown
	}
	for owner, role := range unique {
		return owner, role
	}
	return "", ExecutableRoleUnknown
}

func isRepositoryPackage(root string, pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}
	if pkg.Module != nil && pkg.Module.Main {
		return true
	}
	for _, filename := range append(append([]string(nil), pkg.CompiledGoFiles...), pkg.GoFiles...) {
		if relative, err := filepath.Rel(root, filename); err == nil && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func packageIdentity(pkg *packages.Package) string {
	if pkg == nil {
		return ""
	}
	if pkg.PkgPath != "" {
		return pkg.PkgPath
	}
	return pkg.ID
}

func (a *analyzer) packageErrorLocation(pkg *packages.Package, position string) *Location {
	filename, line, column := parsePackageErrorPosition(position)
	if filename == "" || line <= 0 {
		return nil
	}
	if !filepath.IsAbs(filename) {
		filename = loadedPackageFilename(pkg, filename)
		if filename == "" {
			return nil
		}
	}
	relative, err := filepath.Rel(a.root, filename)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	relative = filepath.ToSlash(relative)
	if cleanRepositoryPath(relative) == "" {
		return nil
	}
	return &Location{Path: relative, Line: line, Column: column}
}

func loadedPackageFilename(pkg *packages.Package, filename string) string {
	if pkg == nil {
		return ""
	}
	filename = filepath.Clean(filename)
	files := make([]string, 0, len(pkg.GoFiles)+len(pkg.CompiledGoFiles)+len(pkg.OtherFiles))
	files = append(files, pkg.GoFiles...)
	files = append(files, pkg.CompiledGoFiles...)
	files = append(files, pkg.OtherFiles...)
	sort.Strings(files)
	for _, candidate := range files {
		candidate = filepath.Clean(candidate)
		if !filepath.IsAbs(candidate) && pkg.Dir != "" {
			candidate = filepath.Join(pkg.Dir, candidate)
		}
		if packageFilenameMatches(pkg, candidate, filename) {
			return candidate
		}
	}
	return ""
}

func packageFilenameMatches(pkg *packages.Package, candidate, filename string) bool {
	for _, root := range []string{pkg.Dir, packageModuleDir(pkg)} {
		if root == "" {
			continue
		}
		relative, err := filepath.Rel(root, candidate)
		if err == nil && filepath.Clean(relative) == filename {
			return true
		}
	}
	return false
}

func packageModuleDir(pkg *packages.Package) string {
	if pkg == nil || pkg.Module == nil {
		return ""
	}
	return pkg.Module.Dir
}

func parsePackageErrorPosition(position string) (string, int, int) {
	position = strings.TrimSpace(position)
	if position == "" || position == "-" {
		return "", 0, 0
	}
	last := strings.LastIndex(position, ":")
	if last < 0 {
		return "", 0, 0
	}
	lastValue, err := strconv.Atoi(position[last+1:])
	if err != nil {
		return "", 0, 0
	}
	prefix := position[:last]
	second := strings.LastIndex(prefix, ":")
	if second < 0 {
		return prefix, lastValue, 0
	}
	line, err := strconv.Atoi(prefix[second+1:])
	if err != nil {
		return prefix, lastValue, 0
	}
	return prefix[:second], line, lastValue
}

func packageErrorKind(kind packages.ErrorKind) string {
	switch kind {
	case packages.ListError:
		return "list"
	case packages.ParseError:
		return "parse"
	case packages.TypeError:
		return "type"
	default:
		return "unknown"
	}
}

func boundedDiagnosticMessage(message, root string) string {
	message = strings.Join(strings.Fields(message), " ")
	root = filepath.Clean(root)
	message = strings.ReplaceAll(message, root, ".")
	message = strings.ReplaceAll(message, filepath.ToSlash(root), ".")
	if len(message) <= maxDiagnosticBytes {
		return message
	}
	return message[:maxDiagnosticBytes]
}

func stableDiagnosticID(packagePath, kind, message string, location *Location) string {
	locationKey := ""
	if location != nil {
		locationKey = fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{packagePath, kind, message, locationKey}, "\x00")))
	return "package-diagnostic-" + hex.EncodeToString(digest[:12])
}

func (a *analyzer) recordProcessEntrypoints() {
	for _, entrypoint := range a.processEntrypoints {
		location := Location{
			Path: entrypoint.anchor.Path, Line: entrypoint.anchor.Line, Column: entrypoint.anchor.Column,
		}
		symbol := Symbol{
			ID: entrypoint.packagePath + ".main", Package: entrypoint.packagePath,
			Name: "main", Location: location,
		}
		record := TriggerRecord{
			Kind:      "process_entry",
			Identity:  Identity{Name: "main", Path: knownValue("declaration", location.Path)},
			Transport: "process", Framework: "go",
			ProcessEntrypoint: symbol,
			Dispatcher:        knownValue("not_applicable", "process entry"),
			RegistrationSite:  location,
			Handler:           knownValue("declaration", symbol.ID),
			Middleware:        []Value{},
			WrapperChain:      []Wrapper{},
			FinalSeed:         "gofacts-go-main",
			DiscoveryBasis:    "build_selected_entrypoint_anchor",
			Certainty:         "static",
			Resolution:        "exact",
			ScenarioID:        a.scenario.ID,
			Evidence: []Evidence{{
				ID: "process-entry:" + locationKey(location), Kind: "process_entry_declaration",
				Location: location, Detail: "exact build-selected top-level func main declaration",
			}},
			Provenance: []Provenance{{
				Provider: "gofacts", Version: "entrypoint-anchor-v1",
				Operation: "build_selected_main_declaration",
			}},
			DynamicFrontier:     []Frontier{},
			Status:              "confirmed_process_entry",
			OwningExecutable:    entrypoint.owner,
			ExecutableRole:      entrypoint.role,
			Availability:        entrypoint.availability,
			UnavailableReason:   entrypoint.unavailableReason,
			TerminalSourceScope: "repository",
			ApplicationClass:    ApplicationSurface,
			PromotionBasis:      PromotionRepositoryRegistration,
		}
		record.ID = stableTriggerID(record)
		a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
		limitation := "Exact build-selected main declaration; process execution and downstream typed reachability are not observed."
		if entrypoint.availability == AvailabilityUnavailable {
			limitation = "Exact build-selected main declaration; deeper typed analysis is unavailable under the recorded build scenario."
		}
		a.recordArchitectureAnchorMembersWithProvenance(
			"process_entry",
			"process entry "+symbol.ID,
			location,
			[]Symbol{symbol},
			limitation,
			Provenance{
				Provider: "gofacts", Version: "entrypoint-anchor-v1",
				Operation: "classify_exact_process_entry",
			},
		)
	}
}

func (a *analyzer) annotateTriggerOwnership(trigger *TriggerRecord) {
	if trigger == nil {
		return
	}
	for _, entrypoint := range a.processEntrypoints {
		if entrypoint.packagePath != trigger.ProcessEntrypoint.Package {
			continue
		}
		trigger.OwningExecutable = entrypoint.owner
		trigger.ExecutableRole = entrypoint.role
		trigger.Availability = entrypoint.availability
		trigger.UnavailableReason = entrypoint.unavailableReason
		return
	}
	if trigger.Availability == "" {
		trigger.Availability = AvailabilityAvailable
	}
	if trigger.ProcessEntrypoint.ID == "" &&
		trigger.ProcessEntrypoint.Package == "" &&
		trigger.ProcessEntrypoint.Location.Path == "" {
		// A shallow descriptor or binding is repository-owned source evidence,
		// not executable ownership evidence. In particular, classifying the
		// zero process location would treat path.Dir("") as the repository
		// root and incorrectly promote every such record to primary_application.
		if trigger.ExecutableRole == "" {
			trigger.ExecutableRole = ExecutableRoleUnknown
		}
		return
	}
	if trigger.OwningExecutable == "" {
		locationDir := cleanRepositoryPath(path.Dir(trigger.ProcessEntrypoint.Location.Path))
		if locationDir == "" || locationDir == "." {
			trigger.OwningExecutable = trigger.ProcessEntrypoint.Package
		} else {
			trigger.OwningExecutable = locationDir
		}
	}
	if trigger.ExecutableRole == "" {
		trigger.ExecutableRole = classifyExecutableRole(a.input.RepositoryName, processEntrypoint{
			packagePath: trigger.ProcessEntrypoint.Package,
			packageDir:  cleanRepositoryPath(path.Dir(trigger.ProcessEntrypoint.Location.Path)),
			anchor: EntrypointAnchorInput{
				Kind: ProcessEntryAnchorGoMain, Path: trigger.ProcessEntrypoint.Location.Path,
				Line: trigger.ProcessEntrypoint.Location.Line,
			},
			owner: trigger.OwningExecutable,
		})
	}
}
