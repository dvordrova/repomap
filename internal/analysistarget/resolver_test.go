package analysistarget

import (
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestExactCandidateKeyModuleDirIsOnlyAClosedCanonicalRoutingHint(t *testing.T) {
	tests := map[string]struct {
		value string
		want  string
		ok    bool
	}{
		"root executable": {
			value: "k8s.io/kubernetes@.::k8s.io/kubernetes/cmd/kube-apiserver",
			want:  ".",
			ok:    true,
		},
		"nested module library": {
			value: "k8s.io/client-go@staging/src/k8s.io/client-go::module_library",
			want:  "staging/src/k8s.io/client-go",
			ok:    true,
		},
		"display path is not authority": {value: "cmd/kube-apiserver"},
		"missing module":                {value: "@.::example.com/cmd/app"},
		"missing surface":               {value: "example.com/app@.::"},
		"noncanonical dir":              {value: "example.com/app@tools/../tools::module_library"},
		"escaping dir":                  {value: "example.com/app@../tools::module_library"},
		"duplicate separator":           {value: "example.com/app@.::module_library::extra"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := ExactCandidateKeyModuleDir(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf("ExactCandidateKeyModuleDir(%q) = %q, %t; want %q, %t", test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestCandidatesReturnsEveryExactTargetWithoutSelectingOne(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/workspace", []syntheticPackage{
		{path: "example.com/workspace/cmd/api", dir: "cmd/api", executable: true, line: 10},
		{path: "example.com/workspace/cmd/worker", dir: "cmd/worker", executable: true, line: 20},
	})
	candidates, err := Candidates(facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v", candidates)
	}
	if candidates[0].Target.PackageDir != "cmd/api" || candidates[1].Target.PackageDir != "cmd/worker" {
		t.Fatalf("candidate inventory was ranked or collapsed: %#v", candidates)
	}
}

type syntheticPackage struct {
	path       string
	dir        string
	executable bool
	kind       string
	line       int
}

func syntheticFacts(moduleID, modulePath string, definitions []syntheticPackage) gofacts.Facts {
	facts := gofacts.Facts{Modules: []gofacts.ModuleFact{{
		ID: moduleID, ModulePath: modulePath, ModuleDir: ".", GoMod: "go.mod", Main: true,
		PackagesCount: len(definitions), RetainedPackagesCount: len(definitions),
		Coverage: gofacts.ModuleCoverage{
			State: gofacts.CoverageComplete, PackagesDiscovered: len(definitions), PackagesRetained: len(definitions),
		},
	}}}
	for _, definition := range definitions {
		pkg := gofacts.PackageFact{
			CanonicalPath: definition.path, Name: packageName(definition), ModuleID: moduleID,
			ModulePath: modulePath, PackageDir: definition.dir, ModuleRelativeDir: definition.dir,
			DisplayPath: definition.dir, Locality: "local", LoadCompleteness: completePackageLoad(),
		}
		if !definition.executable {
			pkg.DeclarationsScanned = true
			pkg.Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationType, Name: "Public"}}
		}
		facts.Packages = append(facts.Packages, pkg)
		if !definition.executable {
			continue
		}
		kind := definition.kind
		if kind == "" {
			kind = "unknown"
		}
		facts.EntrypointPackages = append(facts.EntrypointPackages, gofacts.Entrypoint{
			ModulePath: modulePath, ImportPath: definition.path, Dir: definition.dir,
			PackageDir: definition.dir, ModuleRelativeDir: definition.dir, ModuleDir: ".", Kind: kind,
			GoFiles: []string{"main.go"}, Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: definition.dir + "/main.go", Line: definition.line,
			}},
		})
	}
	facts.PackagesCount = len(definitions)
	facts.RetainedPackagesCount = len(definitions)
	return facts
}

func completePackageLoad() *gofacts.PackageLoadCompleteness {
	return &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
}

func packageName(definition syntheticPackage) string {
	if definition.executable {
		return "main"
	}
	if definition.dir == "." {
		return "library"
	}
	return definition.dir
}

func requireCandidateTarget(t *testing.T, facts gofacts.Facts, kind Kind, packageDir string) Target {
	t.Helper()
	candidates, err := Candidates(facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.Target.Kind == kind && candidate.Target.PackageDir == packageDir {
			return candidate.Target.Snapshot()
		}
	}
	t.Fatalf("target %q/%q missing from %#v", kind, packageDir, candidates)
	return Target{}
}
