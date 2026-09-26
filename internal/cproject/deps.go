package cproject

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	p "github.com/dvordrova/repomap/internal/programindex"
)

// emitImports turns each #include clang entered from a corpus file into one
// imports relation, and one dependency of the including file: a repository
// header is a workspace dependency, a platform header the standard library,
// any other header a package. A directive an include guard or an inactive
// branch kept from entering anything is not an include of this build.
func (b *builder) emitImports() {
	for _, unit := range b.parsed.Units {
		for _, include := range unit.Includes {
			if include.Line <= 0 || include.Spelled == "" {
				continue
			}
			parent := unit.Path
			if include.Parent >= 0 {
				if unit.Includes[include.Parent].Class != FileCorpus {
					continue
				}
				parent = unit.Includes[include.Parent].Path
			}
			spelled := path.Clean(include.Spelled)
			key := fmt.Sprintf("%s:%d", parent, include.Line)
			if b.imported[key] {
				continue
			}
			b.imported[key] = true
			from := b.module(parent)
			at := &p.Location{Path: parent, Line: include.Line, Column: 1}
			var to string
			dependency := dependencies.Dependency{Language: "c", Name: spelled, PackagePath: spelled}
			switch include.Class {
			case FileCorpus:
				to = b.module(include.Path)
				dependency.Kind, dependency.Name, dependency.PackagePath = dependencies.KindWorkspace, include.Path, include.Path
				dependency.ModulePath, dependency.RepositoryPath = b.parsed.Program.Name, path.Dir(include.Path)
			case FilePlatform:
				to = b.header(spelled, p.ExternalAuthorityPlatform)
				dependency.Kind = dependencies.KindStdlib
			default:
				to = b.header(spelled, p.ExternalAuthorityPackage)
				dependency.Kind, dependency.ModulePath = dependencies.KindExternal, spelled
			}
			b.relation(p.RelationInput{SourceRef: "c:include:" + key, Kind: p.RelationImports, FromRef: from, ToRefs: []string{to}, Resolution: p.ResolutionExact,
				Location: at, Witnesses: []p.Witness{{Kind: "c_include", Detail: "#include " + quoted(include.Spelled, include.Class), Location: at}}})
			dependency.ImporterRefs = []string{b.importer(parent)}
			b.deps = append(b.deps, dependency)
		}
	}
}

func quoted(spelled string, class FileClass) string {
	if class == FileCorpus {
		return `"` + spelled + `"`
	}
	return "<" + spelled + ">"
}

// header is the external symbol of a header outside the corpus.
func (b *builder) header(spelled string, authority p.ExternalAuthorityKind) string {
	ref := "c:header:" + spelled
	b.add(p.ObjectInput{SourceRef: ref, Kind: p.ObjectExternalSymbol, Name: spelled, Visibility: p.VisibilityPublic,
		External: &p.ExternalSymbol{AuthorityKind: authority, PackagePath: spelled, Name: spelled}})
	return ref
}

// importer is the dependency catalog's importer for one corpus file.
func (b *builder) importer(file string) string {
	if importer, ok := b.importers[file]; ok {
		return importer.Ref
	}
	importer, err := dependencies.SealImporter(dependencies.Importer{Language: "c", Name: file, ModulePath: b.parsed.Program.Name, PackagePath: file, RepositoryPath: path.Dir(file)})
	if err != nil {
		return ""
	}
	b.importers[file] = importer
	return importer.Ref
}

func (b *builder) importerList() []dependencies.Importer {
	var list []dependencies.Importer
	for _, importer := range b.importers {
		list = append(list, importer)
	}
	slices.SortFunc(list, func(x, y dependencies.Importer) int { return strings.Compare(x.Ref, y.Ref) })
	return list
}
