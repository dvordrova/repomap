package analysistarget

import (
	"fmt"
	"path"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gofacts"
)

const (
	goMainFileHypothesis       = "contains an exact build-selected Go main function"
	goExportedFuncHypothesis   = "contains exported Go functions"
	goExportedMethodHypothesis = "contains exported Go methods"
	goExportedTypeHypothesis   = "contains exported Go types"
	goExportedVarHypothesis    = "contains exported Go variables"
	goExportedConstHypothesis  = "contains exported Go constants"
)

// DiscoverGoTargetFiles projects exact Go target facts onto the shared
// file-addressed scout contract. Target identities remain local: the result
// contains only corpus FileIDs and plain hypotheses.
func DiscoverGoTargetFiles(
	repository *corpus.Corpus,
	facts gofacts.Facts,
	catalog TargetCatalog,
) ([]FileCandidate, error) {
	projection, err := buildGoFileTargetProjection(repository, facts, catalog)
	if err != nil {
		return nil, err
	}
	return cloneGoFileCandidates(projection.candidates), nil
}

// DiscoverGoTargetFilesWithResolver performs the deterministic projection
// once and returns both of its consumer views: the language-neutral scout
// rows and the private exact-target resolver. Ordinary orchestration should
// use this form so it cannot rebuild the same projection immediately after
// discovery merely to obtain the resolver.
func DiscoverGoTargetFilesWithResolver(
	repository *corpus.Corpus,
	facts gofacts.Facts,
	catalog TargetCatalog,
) ([]FileCandidate, GoFileTargetResolver, error) {
	projection, err := buildGoFileTargetProjection(repository, facts, catalog)
	if err != nil {
		return nil, GoFileTargetResolver{}, err
	}
	return cloneGoFileCandidates(projection.candidates), projection.resolver, nil
}

// GoFileTargetResolver is the private local bridge from file-addressed
// portfolio choices back to the existing exact Go target identities. Its
// fields are deliberately unexported so target refs cannot become part of a
// provider-visible FileCandidate by accident.
type GoFileTargetResolver struct {
	initialized       bool
	corpusFiles       map[corpus.FileID]struct{}
	targetRefsByFile  map[corpus.FileID][]string
	orderedTargetRefs []string
}

// ResolvesOne reports whether fileRef can serve as an unambiguous portfolio
// representative for exactly one Go target. It lets language-neutral scouts
// retain useful cross-language evidence without advertising an unsupported or
// ambiguous file to the current Go-only portfolio.
func (resolver GoFileTargetResolver) ResolvesOne(fileRef corpus.FileID) bool {
	if !resolver.initialized {
		return false
	}
	return len(resolver.targetRefsByFile[fileRef]) == 1
}

// NewGoFileTargetResolver builds the local target-ref bridge from exactly the
// files advertised by DiscoverGoTargetFiles. Build-selected package membership
// alone is not target-entry authority: a neighboring implementation file must
// not silently restore the package's sole target.
func NewGoFileTargetResolver(
	repository *corpus.Corpus,
	facts gofacts.Facts,
	catalog TargetCatalog,
) (GoFileTargetResolver, error) {
	projection, err := buildGoFileTargetProjection(repository, facts, catalog)
	if err != nil {
		return GoFileTargetResolver{}, err
	}
	return projection.resolver, nil
}

// Resolve returns the canonical, de-duplicated target refs associated with
// every selected FileID. Input order and duplicate FileIDs carry no authority.
// A selected corpus file with no exact Go target association is rejected.
func (resolver GoFileTargetResolver) Resolve(fileRefs []corpus.FileID) ([]string, error) {
	if !resolver.initialized {
		return nil, fmt.Errorf("analysis target Go file resolver: resolver is not initialized")
	}
	if len(fileRefs) == 0 {
		return nil, fmt.Errorf("analysis target Go file resolver: selected file set is empty")
	}

	selectedTargets := make(map[string]struct{})
	for _, fileRef := range fileRefs {
		if _, known := resolver.corpusFiles[fileRef]; !known {
			return nil, fmt.Errorf(
				"analysis target Go file resolver: unknown file_ref %q", fileRef,
			)
		}
		targetRefs := resolver.targetRefsByFile[fileRef]
		if len(targetRefs) == 0 {
			return nil, fmt.Errorf(
				"analysis target Go file resolver: file_ref %q has no exact Go target", fileRef,
			)
		}
		for _, targetRef := range targetRefs {
			selectedTargets[targetRef] = struct{}{}
		}
	}

	result := make([]string, 0, len(selectedTargets))
	for _, targetRef := range resolver.orderedTargetRefs {
		if _, selected := selectedTargets[targetRef]; selected {
			result = append(result, targetRef)
		}
	}
	if len(result) != len(selectedTargets) {
		return nil, fmt.Errorf("analysis target Go file resolver: target authority mismatch")
	}
	return result, nil
}

// ResolveOne restores the one exact Go target represented by a default
// FileID. It rejects ambiguous file ownership rather than guessing.
func (resolver GoFileTargetResolver) ResolveOne(fileRef corpus.FileID) (string, error) {
	targetRefs, err := resolver.Resolve([]corpus.FileID{fileRef})
	if err != nil {
		return "", err
	}
	if len(targetRefs) != 1 {
		return "", fmt.Errorf(
			"analysis target Go file resolver: file_ref %q maps to %d Go targets",
			fileRef, len(targetRefs),
		)
	}
	return targetRefs[0], nil
}

type goFileTargetProjection struct {
	candidates []FileCandidate
	resolver   GoFileTargetResolver
}

func buildGoFileTargetProjection(
	repository *corpus.Corpus,
	facts gofacts.Facts,
	catalog TargetCatalog,
) (goFileTargetProjection, error) {
	if repository == nil {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: corpus is not initialized",
		)
	}
	snapshot, err := repository.Snapshot().Owned()
	if err != nil {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: corpus: %w", err,
		)
	}
	if err := catalog.Validate(); err != nil {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: catalog: %w", err,
		)
	}
	rebuiltCatalog, err := BuildCatalog(facts)
	if err != nil {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: Go facts: %w", err,
		)
	}
	if rebuiltCatalog.Ref != catalog.Ref {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: catalog does not match Go facts",
		)
	}

	packages := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		packages[packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)] = pkg
	}

	resolver := GoFileTargetResolver{
		initialized:       true,
		corpusFiles:       make(map[corpus.FileID]struct{}, len(snapshot.Entries)),
		targetRefsByFile:  make(map[corpus.FileID][]string),
		orderedTargetRefs: make([]string, 0, len(catalog.Entries)),
	}
	for _, entry := range snapshot.Entries {
		resolver.corpusFiles[entry.ID] = struct{}{}
	}

	rawCandidates := make([]FileCandidate, 0)
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		resolver.orderedTargetRefs = append(resolver.orderedTargetRefs, target.Ref)

		for _, targetPackage := range target.ModulePackages {
			pkg, ok := exactGoTargetPackage(packages, target, targetPackage)
			if !ok {
				return goFileTargetProjection{}, fmt.Errorf(
					"analysis target Go file discovery: target %q package %q is absent from Go facts",
					target.Ref, targetPackage.PackagePath,
				)
			}
			if err := validateGoPackageFileOwnership(pkg); err != nil {
				return goFileTargetProjection{}, err
			}
		}

		switch target.Kind {
		case KindExecutablePackage:
			pkg, ok := exactGoTargetPackage(packages, target, TargetPackage{
				PackagePath: target.PackagePath, PackageDir: target.PackageDir,
			})
			if !ok {
				return goFileTargetProjection{}, fmt.Errorf(
					"analysis target Go file discovery: executable target %q is absent from Go facts",
					target.Ref,
				)
			}
			if err := validateGoPackageFileOwnership(pkg); err != nil {
				return goFileTargetProjection{}, err
			}
			for _, root := range target.Roots {
				if path.Dir(root.Path) != target.PackageDir {
					return goFileTargetProjection{}, fmt.Errorf(
						"analysis target Go file discovery: main root %q is outside package %q",
						root.Path, target.PackageDir,
					)
				}
				if !strings.HasSuffix(root.Path, ".go") {
					return goFileTargetProjection{}, fmt.Errorf(
						"analysis target Go file discovery: main root %q is not a Go source file",
						root.Path,
					)
				}
				if !goPackageContainsFile(pkg.Files, root.Path) {
					return goFileTargetProjection{}, fmt.Errorf(
						"analysis target Go file discovery: main root %q is not a build-selected package file",
						root.Path,
					)
				}
				fileRef, resolveErr := exactGoDiscoveryFileID(repository, root.Path)
				if resolveErr != nil {
					return goFileTargetProjection{}, resolveErr
				}
				addGoTargetRef(&resolver, fileRef, target.Ref)
				rawCandidates = append(rawCandidates, FileCandidate{
					FileRef: fileRef, Hypotheses: []string{goMainFileHypothesis},
				})
			}

		case KindModuleLibrary:
			for _, publicPackage := range target.LibraryPackages {
				pkg, ok := exactGoTargetPackage(packages, target, publicPackage)
				if !ok {
					return goFileTargetProjection{}, fmt.Errorf(
						"analysis target Go file discovery: public package %q is absent from Go facts",
						publicPackage.PackagePath,
					)
				}
				declarations, declarationErr := gofacts.CanonicalPackageDeclarations(pkg.Declarations)
				if declarationErr != nil {
					return goFileTargetProjection{}, fmt.Errorf(
						"analysis target Go file discovery: package %q declarations: %w",
						pkg.CanonicalPath, declarationErr,
					)
				}
				for _, declaration := range declarations {
					if !declaration.ExportedAPI() {
						continue
					}
					if declaration.Path == "" || declaration.Line <= 0 || declaration.Column <= 0 {
						return goFileTargetProjection{}, fmt.Errorf(
							"analysis target Go file discovery: exported declaration %q in package %q has no exact source location",
							declaration.Label(), pkg.CanonicalPath,
						)
					}
					if path.Dir(declaration.Path) != publicPackage.PackageDir {
						return goFileTargetProjection{}, fmt.Errorf(
							"analysis target Go file discovery: exported declaration %q path %q is outside package %q",
							declaration.Label(), declaration.Path, publicPackage.PackageDir,
						)
					}
					if !goPackageContainsFile(pkg.Files, declaration.Path) {
						return goFileTargetProjection{}, fmt.Errorf(
							"analysis target Go file discovery: exported declaration %q path %q is not a build-selected package file",
							declaration.Label(), declaration.Path,
						)
					}
					hypothesis, hypothesisErr := goExportedDeclarationHypothesis(declaration.Kind)
					if hypothesisErr != nil {
						return goFileTargetProjection{}, hypothesisErr
					}
					fileRef, resolveErr := exactGoDiscoveryFileID(repository, declaration.Path)
					if resolveErr != nil {
						return goFileTargetProjection{}, resolveErr
					}
					addGoTargetRef(&resolver, fileRef, target.Ref)
					rawCandidates = append(rawCandidates, FileCandidate{
						FileRef: fileRef, Hypotheses: []string{hypothesis},
					})
				}
			}

		default:
			return goFileTargetProjection{}, fmt.Errorf(
				"analysis target Go file discovery: unsupported target kind %q", target.Kind,
			)
		}
	}

	candidates, err := MergeFileCandidates(snapshot, rawCandidates)
	if err != nil {
		return goFileTargetProjection{}, fmt.Errorf(
			"analysis target Go file discovery: merge candidates: %w", err,
		)
	}
	return goFileTargetProjection{candidates: candidates, resolver: resolver}, nil
}

func exactGoTargetPackage(
	packages map[string]gofacts.PackageFact,
	target Target,
	targetPackage TargetPackage,
) (gofacts.PackageFact, bool) {
	pkg, ok := packages[packageIdentityKey(target.ModuleID, targetPackage.PackagePath)]
	if !ok || pkg.ModulePath != target.ModulePath || pkg.PackageDir != targetPackage.PackageDir {
		return gofacts.PackageFact{}, false
	}
	return pkg, true
}

func goPackageContainsFile(paths []string, want string) bool {
	for _, filePath := range paths {
		if filePath == want {
			return true
		}
	}
	return false
}

func validateGoPackageFileOwnership(pkg gofacts.PackageFact) error {
	for _, filePath := range pkg.Files {
		if filePath == "" || path.IsAbs(filePath) || strings.Contains(filePath, `\`) ||
			path.Clean(filePath) != filePath || path.Dir(filePath) != pkg.PackageDir {
			return fmt.Errorf(
				"analysis target Go file discovery: package %q file %q is outside package directory %q",
				pkg.CanonicalPath, filePath, pkg.PackageDir,
			)
		}
	}
	return nil
}

func addGoTargetRef(resolver *GoFileTargetResolver, fileRef corpus.FileID, targetRef string) {
	refs := resolver.targetRefsByFile[fileRef]
	for _, existing := range refs {
		if existing == targetRef {
			return
		}
	}
	resolver.targetRefsByFile[fileRef] = append(refs, targetRef)
}

func exactGoDiscoveryFileID(repository *corpus.Corpus, filePath string) (corpus.FileID, error) {
	fileRef, ok := repository.ID(filePath)
	if !ok {
		return "", fmt.Errorf(
			"analysis target Go file discovery: exact source path %q is absent from the corpus",
			filePath,
		)
	}
	return fileRef, nil
}

func goExportedDeclarationHypothesis(kind gofacts.PackageDeclarationKind) (string, error) {
	switch kind {
	case gofacts.PackageDeclarationFunc:
		return goExportedFuncHypothesis, nil
	case gofacts.PackageDeclarationMethod:
		return goExportedMethodHypothesis, nil
	case gofacts.PackageDeclarationType:
		return goExportedTypeHypothesis, nil
	case gofacts.PackageDeclarationVar:
		return goExportedVarHypothesis, nil
	case gofacts.PackageDeclarationConst:
		return goExportedConstHypothesis, nil
	default:
		return "", fmt.Errorf(
			"analysis target Go file discovery: unsupported exported declaration kind %q",
			kind,
		)
	}
}

func cloneGoFileCandidates(values []FileCandidate) []FileCandidate {
	result := make([]FileCandidate, len(values))
	for index, value := range values {
		result[index] = FileCandidate{
			FileRef: value.FileRef, Hypotheses: append([]string(nil), value.Hypotheses...),
		}
	}
	if result == nil {
		return []FileCandidate{}
	}
	return result
}
