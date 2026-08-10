package targetportfolio

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestD279TelebotAdvertisesOnlyModuleRootLibrary(t *testing.T) {
	const modulePath = "gopkg.in/telebot.v3"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "telebot", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{
			d279LibraryPackage("telebot", modulePath, ".", ".", modulePath, "telebot", "NewBot"),
			d279LibraryPackage("telebot", modulePath, ".", "layout", modulePath+"/layout", "layout", "Default"),
			d279LibraryPackage("telebot", modulePath, ".", "middleware", modulePath+"/middleware", "middleware", "Recover"),
			d279LibraryPackage("telebot", modulePath, ".", "react", modulePath+"/react", "react", "New"),
		},
	}
	catalog := mustCatalog(t, facts)
	if len(catalog.Entries) != 4 {
		t.Fatalf("complete catalog targets = %d, want 4", len(catalog.Entries))
	}

	compilation, err := Compile("telebot", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := d279VisiblePaths(compilation.Request.Targets); !slices.Equal(got, []string{"."}) {
		t.Fatalf("ordinary wire targets = %#v, want module root only", got)
	}
	rootRef := compilation.Request.Targets[0].Ref
	accepted, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: rootRef, TargetRefs: []string{rootRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, accepted)
	if err != nil || len(selection.Targets) != 1 || selection.Targets[0].DisplayPath != "." {
		t.Fatalf("root selection = %#v, %v", selection, err)
	}

	// t2 would have named layout under the pre-D279 catalog-index mapping. It
	// has no restoration authority in the filtered ordinary compilation.
	unadvertised, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: "t2", TargetRefs: []string{"t2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveResponse(compilation, unadvertised); err == nil ||
		!strings.Contains(err.Error(), "unknown default ref") {
		t.Fatalf("unadvertised library response error = %v", err)
	}
}

func TestD279EtcdAdvertisesClientModuleRootAndServerMainOnly(t *testing.T) {
	const (
		clientModule = "go.etcd.io/etcd/client/v3"
		serverModule = "go.etcd.io/etcd/server/v3"
	)
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "client", ModulePath: clientModule, ModuleDir: "client/v3"},
			{ID: "server", ModulePath: serverModule, ModuleDir: "server"},
		},
		Packages: []gofacts.PackageFact{
			d279LibraryPackage("client", clientModule, "client/v3", ".", clientModule, "clientv3", "New"),
			d279LibraryPackage("client", clientModule, "client/v3", "concurrency", clientModule+"/concurrency", "concurrency", "NewSession"),
			d279ExecutablePackage("server", serverModule, "server", ".", serverModule),
			d279LibraryPackage("server", serverModule, "server", "storage/wal", serverModule+"/storage/wal", "wal", "Create"),
		},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: serverModule, ImportPath: serverModule,
			PackageDir: "server", ModuleRelativeDir: ".", ModuleDir: "server", Kind: "primary_binary",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "server/main.go", Line: 30,
			}},
		}},
	}
	catalog := mustCatalog(t, facts)
	if len(catalog.Entries) != 4 {
		t.Fatalf("complete catalog targets = %d, want 4", len(catalog.Entries))
	}

	compilation, err := Compile("etcd", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := d279VisiblePaths(compilation.Request.Targets); !slices.Equal(got, []string{"client/v3", "server"}) {
		t.Fatalf("ordinary wire targets = %#v, want client module root plus server main", got)
	}
	refs := map[string]string{}
	for _, target := range compilation.Request.Targets {
		refs[target.DisplayPath] = target.Ref
	}
	raw, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: refs["server"], TargetRefs: []string{refs["server"], refs["client/v3"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(selection.Targets))
	for _, entry := range selection.Targets {
		got = append(got, entry.DisplayPath)
	}
	if !slices.Equal(got, []string{"client/v3", "server"}) {
		t.Fatalf("restored ordinary selection = %#v", got)
	}
}

func d279LibraryPackage(
	moduleID, modulePath, moduleDir, relativeDir, canonicalPath, name, exportedFunc string,
) gofacts.PackageFact {
	packageDir := moduleDir
	if relativeDir != "." {
		packageDir += "/" + relativeDir
	}
	if moduleDir == "." {
		packageDir = relativeDir
	}
	return gofacts.PackageFact{
		CanonicalPath: canonicalPath, Name: name, ModuleID: moduleID, ModulePath: modulePath,
		PackageDir: packageDir, ModuleRelativeDir: relativeDir, DisplayPath: packageDir,
		Locality: "local", DeclarationsScanned: true,
		Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: exportedFunc}},
	}
}

func d279ExecutablePackage(
	moduleID, modulePath, moduleDir, relativeDir, canonicalPath string,
) gofacts.PackageFact {
	pkg := d279LibraryPackage(moduleID, modulePath, moduleDir, relativeDir, canonicalPath, "main", "main")
	pkg.Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "main"}}
	return pkg
}

func d279VisiblePaths(targets []Target) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.DisplayPath)
	}
	return paths
}
