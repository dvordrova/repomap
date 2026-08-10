package targetportfolio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/secretscan"
)

type requestIdentity struct {
	Version  int      `json:"version"`
	RepoName string   `json:"repo_name"`
	Targets  []Target `json:"targets"`
}

// Compile projects the exact ordinary portfolio surface from the complete
// private catalog. Executables and module-root libraries are advertised in
// canonical catalog order; non-root library packages remain private catalog
// authority for explicit --target and --all-targets inclusion. The projection
// never ranks, truncates, or applies a path-purpose heuristic.
func Compile(repoName string, catalog analysistarget.TargetCatalog) (Compilation, error) {
	if err := catalog.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("target portfolio: catalog: %w", err)
	}
	if err := validateRepoName(repoName); err != nil {
		return Compilation{}, err
	}
	if len(catalog.Entries) == 0 {
		return Compilation{}, fmt.Errorf("target portfolio: catalog has no targets")
	}

	advertised := ordinarySurfaceEntries(catalog.Entries)
	if len(advertised) == 0 {
		return Compilation{}, fmt.Errorf("target portfolio: catalog has no executable or module-root library targets")
	}
	targets := make([]Target, 0, len(advertised))
	authority := make(map[string]analysistarget.TargetCatalogEntry, len(advertised))
	for index, entry := range advertised {
		ref := fmt.Sprintf("t%d", index+1)
		kind, err := providerKind(entry.Candidate.Target.Kind)
		if err != nil {
			return Compilation{}, err
		}
		if err := validateDisplayPath(entry.DisplayPath); err != nil {
			return Compilation{}, err
		}
		if !entry.DeclarationsScanned {
			return Compilation{}, fmt.Errorf("target portfolio: declaration labels are unavailable for %q", entry.DisplayPath)
		}
		symbols, err := compileSymbolGroups(entry.Symbols)
		if err != nil {
			return Compilation{}, fmt.Errorf("target portfolio: target %q symbols: %w", entry.DisplayPath, err)
		}
		targets = append(targets, Target{Ref: ref, DisplayPath: entry.DisplayPath, Kind: kind, Symbols: symbols})
		authority[ref] = snapshotEntry(entry)
	}

	identity := requestIdentity{Version: RequestVersion, RepoName: repoName, Targets: targets}
	requestRef, err := deriveRequestRef(catalog.Ref, identity)
	if err != nil {
		return Compilation{}, err
	}
	request := Request{
		Version: RequestVersion, RequestRef: requestRef, RepoName: repoName,
		Targets: append([]Target(nil), targets...),
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("target portfolio: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf(
			"target portfolio: complete request is %d bytes, limit is %d", len(wire), MaxRequestBytes,
		)
	}
	if _, found := secretscan.DetectAlways(string(wire)); found {
		return Compilation{}, fmt.Errorf("target portfolio: provider request contains credential-shaped content")
	}
	requestSHA := sha256Hex(wire)
	compilation := Compilation{
		Version: CompilationVersion, CatalogRef: catalog.Ref, Request: request,
		RequestSHA256: requestSHA, wire: string(wire), catalog: catalog.Snapshot(), authority: authority,
	}
	compilation.sealed = compilationSeal(compilation.CatalogRef, compilation.RequestSHA256)
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

func validateCompilation(compilation Compilation) error {
	if compilation.Version != CompilationVersion || compilation.CatalogRef == "" ||
		compilation.CatalogRef != compilation.catalog.Ref || compilation.RequestSHA256 == "" {
		return fmt.Errorf("target portfolio: invalid compilation identity")
	}
	if err := compilation.catalog.Validate(); err != nil {
		return fmt.Errorf("target portfolio: private catalog: %w", err)
	}
	if err := validateRepoName(compilation.Request.RepoName); err != nil {
		return err
	}
	advertised := ordinarySurfaceEntries(compilation.catalog.Entries)
	if compilation.Request.Version != RequestVersion || len(compilation.Request.Targets) == 0 ||
		len(compilation.Request.Targets) != len(advertised) ||
		len(compilation.authority) != len(advertised) {
		return fmt.Errorf("target portfolio: invalid request shape")
	}
	identity := requestIdentity{
		Version: compilation.Request.Version, RepoName: compilation.Request.RepoName,
		Targets: append([]Target(nil), compilation.Request.Targets...),
	}
	wantRequestRef, err := deriveRequestRef(compilation.CatalogRef, identity)
	if err != nil {
		return err
	}
	if compilation.Request.RequestRef != wantRequestRef {
		return fmt.Errorf("target portfolio: request identity mismatch")
	}
	for index, target := range compilation.Request.Targets {
		wantRef := fmt.Sprintf("t%d", index+1)
		entry := advertised[index]
		wantKind, err := providerKind(entry.Candidate.Target.Kind)
		if err != nil {
			return err
		}
		wantSymbols, symbolsErr := compileSymbolGroups(entry.Symbols)
		if symbolsErr != nil {
			return fmt.Errorf("target portfolio: target symbols: %w", symbolsErr)
		}
		if !entry.DeclarationsScanned || target.Ref != wantRef || target.DisplayPath != entry.DisplayPath || target.Kind != wantKind ||
			!reflect.DeepEqual(target.Symbols, wantSymbols) ||
			!reflect.DeepEqual(compilation.authority[wantRef], entry) {
			return fmt.Errorf("target portfolio: request authority mismatch")
		}
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil {
		return fmt.Errorf("target portfolio: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes || compilation.wire != string(wire) ||
		compilation.RequestSHA256 != sha256Hex(wire) ||
		compilation.sealed != compilationSeal(compilation.CatalogRef, compilation.RequestSHA256) {
		return fmt.Errorf("target portfolio: request wire binding mismatch")
	}
	if _, found := secretscan.DetectAlways(compilation.wire); found {
		return fmt.Errorf("target portfolio: provider request contains credential-shaped content")
	}
	return nil
}

func providerKind(kind analysistarget.Kind) (TargetKind, error) {
	switch kind {
	case analysistarget.KindExecutablePackage:
		return TargetExecutable, nil
	case analysistarget.KindLibraryPackage:
		return TargetLibrary, nil
	default:
		return "", fmt.Errorf("target portfolio: unsupported target kind")
	}
}

func compileSymbolGroups(declarations []gofacts.PackageDeclaration) ([]SymbolGroup, error) {
	if err := gofacts.ValidatePackageDeclarations(declarations); err != nil {
		return nil, err
	}
	groups := make([]SymbolGroup, 0, 5)
	for _, declaration := range declarations {
		kind := string(declaration.Kind)
		name := declaration.Label()
		if len(groups) == 0 || groups[len(groups)-1].Kind != kind {
			groups = append(groups, SymbolGroup{Kind: kind, Names: []string{name}})
			continue
		}
		groups[len(groups)-1].Names = append(groups[len(groups)-1].Names, name)
	}
	return groups, nil
}

func validateRepoName(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsAny(value, `/\\`) || containsControl(value) {
		return fmt.Errorf("target portfolio: invalid repository name")
	}
	return nil
}

func validateDisplayPath(value string) error {
	if value == "" || !utf8.ValidString(value) || containsControl(value) {
		return fmt.Errorf("target portfolio: invalid display path")
	}
	return nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func deriveRequestRef(catalogRef string, identity requestIdentity) (string, error) {
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("target portfolio: encode request identity: %w", err)
	}
	digest := sha256.Sum256(append(append([]byte(catalogRef), 0), encoded...))
	return "tpq-" + hex.EncodeToString(digest[:12]), nil
}

func compilationSeal(catalogRef, requestSHA string) string {
	return sha256Hex([]byte("target-portfolio-compilation-v3\x00" + catalogRef + "\x00" + requestSHA))
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func snapshotEntry(entry analysistarget.TargetCatalogEntry) analysistarget.TargetCatalogEntry {
	result := entry
	result.Candidate.Target = entry.Candidate.Target.Snapshot()
	result.Symbols = append([]gofacts.PackageDeclaration(nil), entry.Symbols...)
	return result
}

func ordinarySurfaceEntries(entries []analysistarget.TargetCatalogEntry) []analysistarget.TargetCatalogEntry {
	result := make([]analysistarget.TargetCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if AdvertisedForSelection(entry) {
			result = append(result, entry)
		}
	}
	return result
}
