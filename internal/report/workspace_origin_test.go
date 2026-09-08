package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestExternalMemberChipsKeepWorkspaceOriginsAndInstalledNamesSeparate(t *testing.T) {
	for _, language := range []string{"javascript", "typescript"} {
		t.Run(language, func(t *testing.T) {
			app := &pageSection{ID: "app", Name: "app", Root: "app", Language: language}
			a := &pageSection{ID: "a", Name: "got", Root: "shared/a", Language: "javascript"}
			b := &pageSection{ID: "b", Name: "got", Root: "shared/b", Language: "typescript"}
			root := &pageSection{ID: "root", Name: "got", Root: ".", Language: "typescript"}
			builder := pageBuilder{
				sections:  []*pageSection{app, a, b, root},
				byProgram: map[string]*pageSection{"app": app}, subjects: map[string]subjectRef{},
			}
			ids := []string{"a", "b", "root", "installed", "unselected"}
			for i, directory := range []string{"shared/a", "shared/b", ".", "", "unselected"} {
				builder.subjects[ids[i]] = subjectRef{programTargetID: "app", subject: groupindex.Subject{
					ID: ids[i], Object: &groupindex.ObjectFacts{Kind: programindex.ObjectExternalSymbol,
						External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage,
							PackagePath: "got", Name: "get", RepositoryPath: directory}},
				}}
			}
			chips, externals := builder.memberChips(append(ids, "a"))
			if len(chips) != 0 || len(externals) != len(ids) {
				t.Fatalf("same-name identities merged or repeated member duplicated: %+v / %+v", chips, externals)
			}
			for i, href := range []string{"#a", "#b", "#root", "", ""} {
				if externals[i].Name != "got.get" || externals[i].Href != href {
					t.Fatalf("member %s linked by name instead of origin: %+v", ids[i], externals[i])
				}
			}
		})
	}
}

func TestNativeExternalMemberNavigationStillUsesItsPackageAuthority(t *testing.T) {
	for _, language := range []string{"go", "python"} {
		owner := &pageSection{ID: "owner", Language: language}
		sibling := &pageSection{ID: "shared", Name: "example.com/shared", Language: language}
		builder := pageBuilder{
			byProgram: map[string]*pageSection{"owner": owner},
			owners:    []packageOwner{{section: sibling, packages: []string{"example.com/shared"}}},
			subjects: map[string]subjectRef{"external": {programTargetID: "owner", subject: groupindex.Subject{
				Object: &groupindex.ObjectFacts{Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{
					AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.com/shared", Name: "Read",
				}},
			}}},
		}
		_, chips := builder.memberChips([]string{"external"})
		if len(chips) != 1 || chips[0].Href != "#shared" {
			t.Fatalf("%s native package navigation changed: %+v", language, chips)
		}
	}
}

func TestProgramViewKeepsCanonicalWorkspaceOrigin(t *testing.T) {
	object := programindex.Object{ID: "program-object-external", SourceRef: "external", Name: "got.get",
		Kind: programindex.ObjectExternalSymbol, Visibility: programindex.VisibilityPublic,
		External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "got", Name: "get", RepositoryPath: "shared"},
	}
	view := programViewObject(object)
	object.External.RepositoryPath = "changed"
	if view.External.RepositoryPath != "shared" {
		t.Fatal("program view did not copy the compiler-owned origin")
	}
	for _, directory := range []string{"", ".", "shared", "한글/shared"} {
		view.External.RepositoryPath = directory
		if err := validateProgramViewObject(view); err != nil {
			t.Fatalf("valid workspace directory %q rejected: %v", directory, err)
		}
	}
	for _, directory := range []string{"..", "../shared", "/shared", "a//b", "a/../b", "a\\b", " shared", "a\nb"} {
		view.External.RepositoryPath = directory
		if err := validateProgramViewObject(view); err == nil {
			t.Fatalf("invalid workspace directory %q accepted", directory)
		}
	}
	view.External.RepositoryPath = "."
	view.External.AuthorityKind = programindex.ExternalAuthorityPlatform
	if err := validateProgramViewObject(view); err == nil {
		t.Fatal("platform acquired repository package authority")
	}
}
