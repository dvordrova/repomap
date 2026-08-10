package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type targetPortfolioClientStub struct {
	response []byte
	err      error
	calls    int
	prompt   targetportfolio.Prompt
}

func (stub *targetPortfolioClientStub) TargetPortfolioPromptJSON(prompt targetportfolio.Prompt) ([]byte, error) {
	stub.prompt = prompt
	return []byte(`{"model":"test","messages":[]}`), nil
}

func (stub *targetPortfolioClientStub) TargetPortfolioBodyMeasured(
	context.Context,
	[]byte,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{
		Content: stub.response, Attempts: 1, RequestBytes: 32, ResponseBytes: len(stub.response),
	}, stub.err
}

func TestTargetPortfolioRuntimeUsesModelDefaultFromCompleteCatalog(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	compilation, err := targetportfolio.Compile("repomap", catalog)
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string)
	for _, target := range compilation.Request.Targets {
		refs[target.DisplayPath] = target.Ref
	}
	response, err := json.Marshal(targetportfolio.Response{
		Version: targetportfolio.ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: refs["cmd/repomap"],
		TargetRefs: []string{refs["pkg/client"], refs["cmd/repomap"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &targetPortfolioClientStub{response: response}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || selected != targetPortfolioRuntimeEntry(t, catalog, "cmd/repomap").Candidate.Target.Ref ||
		outcome.SelectedPath != "cmd/repomap" || outcome.SelectedTargets != 2 || outcome.UsedLocalDefault {
		t.Fatalf("selection/outcome/calls = %q / %#v / %d", selected, outcome, client.calls)
	}
	for _, entry := range catalog.Entries {
		visible := strings.Contains(client.prompt.User, `"display_path":"`+entry.DisplayPath+`"`)
		if visible != targetportfolio.AdvertisedForSelection(entry) {
			t.Fatalf("provider prompt visibility for %q = %t, advertised=%t: %s",
				entry.DisplayPath, visible, targetportfolio.AdvertisedForSelection(entry), client.prompt.User)
		}
	}
}

func TestAllTargetsOnlineUsesOneSelectorDefaultAndKeepsCompleteCanonicalCatalog(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	compilation, err := targetportfolio.Compile("repomap", catalog)
	if err != nil {
		t.Fatal(err)
	}
	requestRef := compilation.Request.RequestRef
	providerRef := ""
	for _, target := range compilation.Request.Targets {
		if target.DisplayPath == "cmd/helper" {
			providerRef = target.Ref
		}
	}
	response, err := json.Marshal(targetportfolio.Response{
		Version: targetportfolio.ResultVersion, RequestRef: requestRef,
		DefaultRef: providerRef, TargetRefs: []string{providerRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &targetPortfolioClientStub{response: response}
	selection, outcome, err := selectAllTargetsForRun(
		context.Background(), "example.com/repomap", catalog, false, "", nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRefs := make([]string, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		wantRefs = append(wantRefs, entry.Candidate.Target.Ref)
	}
	wantDefault := targetPortfolioRuntimeEntry(t, catalog, "cmd/helper").Candidate.Target.Ref
	if client.calls != 1 || selection.DefaultTargetRef != wantDefault ||
		!slices.Equal(selection.TargetRefs, wantRefs) ||
		!slices.Equal(outcome.SelectedTargetRefs, wantRefs) ||
		outcome.SelectedTargets != len(wantRefs) {
		t.Fatalf("all-target selection = %#v / %#v / calls=%d", selection, outcome, client.calls)
	}
}

func TestAllTargetsExplicitDefaultAndOfflineDefaultNeverConfigureSelector(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	factoryCalls := 0
	factory := func() (targetPortfolioClient, error) {
		factoryCalls++
		return nil, errors.New("selector must not be configured")
	}
	for _, test := range []struct {
		name     string
		offline  bool
		override string
		wantRef  string
	}{
		{
			name: "explicit online", override: "pkg/client",
			wantRef: targetPortfolioRuntimeEntry(t, catalog, "pkg/client").Candidate.Target.Ref,
		},
		{
			name: "strong offline default", offline: true,
			wantRef: catalog.DefaultTargetRef,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection, outcome, err := selectAllTargetsForRun(
				context.Background(), "example.com/repomap", catalog,
				test.offline, test.override, nil, factory,
			)
			if err != nil {
				t.Fatal(err)
			}
			if selection.DefaultTargetRef != test.wantRef ||
				len(selection.TargetRefs) != len(catalog.Entries) ||
				outcome.SelectedTargets != len(catalog.Entries) {
				t.Fatalf("all-target local selection = %#v / %#v", selection, outcome)
			}
		})
	}
	if factoryCalls != 0 {
		t.Fatalf("selector factory calls = %d, want 0", factoryCalls)
	}
}

func TestAllTargetsRootExecutableAndModuleLibraryRequireTypedDefaultKey(t *testing.T) {
	const modulePath = "example.com/collision"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root", ModulePath: modulePath, ModuleDir: ".", Main: true,
			PackagesCount: 2, RetainedPackagesCount: 2,
			Coverage: gofacts.ModuleCoverage{PackagesDiscovered: 2, PackagesRetained: 2},
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: modulePath, Name: "main", ModuleID: "root", ModulePath: modulePath,
				PackageDir: ".", ModuleRelativeDir: ".", DisplayPath: ".", Locality: "local",
				DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
			},
			{
				CanonicalPath: modulePath + "/client", Name: "client", ModuleID: "root", ModulePath: modulePath,
				PackageDir: "client", ModuleRelativeDir: "client", DisplayPath: "client", Locality: "local",
				DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
				Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "Open"}},
			},
		},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: modulePath, ImportPath: modulePath, PackageDir: ".", ModuleRelativeDir: ".",
			ModuleDir: ".", Kind: "primary_binary",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "main.go", Line: 7,
			}},
		}},
	}
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	if len(catalog.Entries) != 2 || catalog.Entries[0].DisplayPath != "." || catalog.Entries[1].DisplayPath != "." {
		t.Fatalf("collision catalog = %#v", catalog.Entries)
	}
	if _, _, err := selectAllTargetsForRun(
		context.Background(), modulePath, catalog, true, ".", nil, nil,
	); err == nil || !strings.Contains(err.Error(), "ambiguous") ||
		!strings.Contains(err.Error(), catalog.Entries[0].Candidate.Key) ||
		!strings.Contains(err.Error(), catalog.Entries[1].Candidate.Key) {
		t.Fatalf("untyped root alias = %v", err)
	}
	for _, entry := range catalog.Entries {
		selection, outcome, err := selectAllTargetsForRun(
			context.Background(), modulePath, catalog, true, entry.Candidate.Key, nil, nil,
		)
		if err != nil || selection.DefaultTargetRef != entry.Candidate.Target.Ref ||
			outcome.SelectedKind != entry.Candidate.Target.Kind || len(selection.TargetRefs) != 2 {
			t.Fatalf("typed key %q = %#v / %#v / %v", entry.Candidate.Key, selection, outcome, err)
		}
	}
}

func TestTargetPortfolioRuntimeInvalidResponseUsesOnlyExactLocalDefault(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	client := &targetPortfolioClientStub{response: []byte(`{"version":1,"request_ref":"wrong","default_ref":"t1","target_refs":["t1"]}`)}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected != catalog.DefaultTargetRef || !outcome.UsedLocalDefault ||
		outcome.FailureCode != "response_validation" || client.calls != 1 {
		t.Fatalf("fallback = %q / %#v / calls=%d", selected, outcome, client.calls)
	}
}

func TestTargetPortfolioRuntimeInvalidResponseWithoutDefaultIsActionable(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeAmbiguousFacts())
	if catalog.DefaultTargetRef != "" {
		t.Fatalf("ambiguous catalog default = %q", catalog.DefaultTargetRef)
	}
	client := &targetPortfolioClientStub{response: []byte(`{}`)}
	_, _, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "--target") ||
		!strings.Contains(err.Error(), "cmd/api") || !strings.Contains(err.Error(), "cmd/worker") {
		t.Fatalf("terminal choice error = %v", err)
	}
}

func TestTargetPortfolioRuntimeSoleEligibleExecutableBypassesEmptyLibraryDefault(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioEmptyRootLibraryFacts())
	serverEntry := targetPortfolioRuntimeEntry(t, catalog, "server")
	if len(catalog.Entries) != 1 || catalog.DefaultTargetRef != "" ||
		serverEntry.Candidate.Target.Kind != analysistarget.KindExecutablePackage {
		t.Fatalf("D280 empty-library omission setup = %#v / default %q", catalog.Entries, catalog.DefaultTargetRef)
	}
	client := &targetPortfolioClientStub{response: []byte(`{}`)}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "go.etcd.io/etcd/v3", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	serverRef := serverEntry.Candidate.Target.Ref
	if err != nil || selected != serverRef || outcome.UsedLocalDefault || client.calls != 0 ||
		outcome.SelectedPath != "server" || outcome.SelectedTargets != 1 {
		t.Fatalf("empty-library fallback = %q / %#v / calls=%d / err=%v", selected, outcome, client.calls, err)
	}
}

func TestAllTargetsOfflineWithoutStrongDefaultRequiresExplicitTarget(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeAmbiguousFacts())
	factoryCalls := 0
	_, _, err := selectAllTargetsForRun(
		context.Background(), "example.com/repomap", catalog, true, "", nil,
		func() (targetPortfolioClient, error) {
			factoryCalls++
			return nil, errors.New("must not run")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "--target TARGET") ||
		!strings.Contains(err.Error(), "cmd/api") || !strings.Contains(err.Error(), "cmd/worker") ||
		factoryCalls != 0 {
		t.Fatalf("offline all-target error = %v, factory calls=%d", err, factoryCalls)
	}
}

func TestTargetPortfolioRuntimeCancellationNeverFallsBack(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	client := &targetPortfolioClientStub{err: context.Canceled}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	selected, outcome, err := selectTargetPortfolioForRun(
		ctx, "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if !errors.Is(err, context.Canceled) || selected != "" || outcome.UsedLocalDefault {
		t.Fatalf("cancellation selected fallback: %q / %#v / %v", selected, outcome, err)
	}
}

func TestTargetPortfolioRuntimeProviderTimeoutFallsBackWhenCallerContextIsAlive(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	client := &targetPortfolioClientStub{err: context.DeadlineExceeded}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err != nil || selected != catalog.DefaultTargetRef || !outcome.UsedLocalDefault ||
		outcome.FailureCode != "provider_failed" {
		t.Fatalf("provider timeout fallback = %q / %#v / %v", selected, outcome, err)
	}
}

func TestTargetPortfolioRuntimeSecretResponseIsNotRetained(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	secretResponse := []byte(`{"api_key":"sk-secret-shaped-provider-output"}`)
	client := &targetPortfolioClientStub{response: secretResponse}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/repomap", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	if err != nil || selected != catalog.DefaultTargetRef || !outcome.UsedLocalDefault ||
		outcome.FailureCode != "response_secret_scan" || len(outcome.Response) != 0 ||
		outcome.ResponseUnavailable == nil || outcome.ResponseUnavailable.OriginalBytes != len(secretResponse) {
		t.Fatalf("secret response fallback retained bytes: %q / %#v / %v", selected, outcome, err)
	}
}

func TestTargetPortfolioRuntimeSoleTargetDoesNotConfigureProvider(t *testing.T) {
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{ID: "module-root", ModulePath: "example.com/library", ModuleDir: ".", Main: true}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/library", Name: "library", ModuleID: "module-root",
			ModulePath: "example.com/library", PackageDir: ".", ModuleRelativeDir: ".", Locality: "local",
			DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
			Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "New"}},
		}},
	}
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	factoryCalls := 0
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "example.com/library", catalog, nil,
		func() (targetPortfolioClient, error) {
			factoryCalls++
			return nil, fmt.Errorf("must not run")
		},
	)
	if err != nil || selected != catalog.Entries[0].Candidate.Target.Ref ||
		outcome.SelectedPath != "." || factoryCalls != 0 {
		t.Fatalf("sole target = %q / %#v / factory=%d / err=%v", selected, outcome, factoryCalls, err)
	}
}

func TestD280TelebotFourPackagesPublishOneModuleLibraryAndMakeZeroSelectorCalls(t *testing.T) {
	const modulePath = "gopkg.in/telebot.v3"
	packages := []gofacts.PackageFact{
		{
			CanonicalPath: modulePath, Name: "telebot", ModuleID: "telebot", ModulePath: modulePath,
			PackageDir: ".", ModuleRelativeDir: ".", DisplayPath: ".", Locality: "local",
			DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
			Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot"}},
		},
	}
	for _, dir := range []string{"layout", "middleware", "react"} {
		pkg := targetPortfolioRuntimePackage(modulePath, dir)
		pkg.ModuleID = "telebot"
		pkg.Declarations = []gofacts.PackageDeclaration{{
			Kind: gofacts.PackageDeclarationFunc, Name: "New" + strings.ToUpper(dir[:1]) + dir[1:],
		}}
		packages = append(packages, pkg)
	}
	catalog := targetPortfolioRuntimeCatalog(t, gofacts.Facts{
		Modules:  []gofacts.ModuleFact{{ID: "telebot", ModulePath: modulePath, ModuleDir: ".", Main: true}},
		Packages: packages,
	})
	if len(catalog.Entries) != 1 || catalog.Entries[0].Candidate.Target.Kind != analysistarget.KindModuleLibrary ||
		len(catalog.Entries[0].Candidate.Target.LibraryPackages) != 4 || len(catalog.Entries[0].PackageAPIs) != 4 {
		t.Fatalf("Telebot module surface = %#v", catalog.Entries)
	}
	factoryCalls := 0
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), modulePath, catalog, nil,
		func() (targetPortfolioClient, error) {
			factoryCalls++
			return nil, errors.New("selector must not be configured for one advertised target")
		},
	)
	root := targetPortfolioRuntimeEntry(t, catalog, ".")
	if err != nil || selected != root.Candidate.Target.Ref || factoryCalls != 0 ||
		outcome.SelectedPath != "." || outcome.SelectedTargets != 1 ||
		!slices.Equal(outcome.SelectedTargetRefs, []string{root.Candidate.Target.Ref}) {
		t.Fatalf("Telebot ordinary selection = %q / %#v / factory=%d / err=%v", selected, outcome, factoryCalls, err)
	}
}

func TestD280EtcdSelectorSeesModuleLibrariesAndExactServerMain(t *testing.T) {
	const (
		clientModule = "go.etcd.io/etcd/client/v3"
		serverModule = "go.etcd.io/etcd/server/v3"
	)
	clientRoot := targetPortfolioRuntimeNestedModulePackage("client", clientModule, "client/v3", ".", "clientv3", "New")
	clientSub := targetPortfolioRuntimeNestedModulePackage("client", clientModule, "client/v3", "concurrency", "concurrency", "NewSession")
	serverRoot := targetPortfolioRuntimeNestedModulePackage("server", serverModule, "server", ".", "main", "main")
	serverSub := targetPortfolioRuntimeNestedModulePackage("server", serverModule, "server", "storage/wal", "wal", "Create")
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "client", ModulePath: clientModule, ModuleDir: "client/v3"},
			{ID: "server", ModulePath: serverModule, ModuleDir: "server"},
		},
		Packages: []gofacts.PackageFact{clientRoot, clientSub, serverRoot, serverSub},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: serverModule, ImportPath: serverModule,
			PackageDir: "server", ModuleRelativeDir: ".", ModuleDir: "server", Kind: "primary_binary",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "server/main.go", Line: 30,
			}},
		}},
	}
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	compilation, err := targetportfolio.Compile("etcd", catalog)
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	for _, target := range compilation.Request.Targets {
		refs[target.DisplayPath+"\x00"+string(target.Kind)] = target.Ref
	}
	response, err := json.Marshal(targetportfolio.Response{
		Version: targetportfolio.ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: refs["server\x00executable"],
		TargetRefs: []string{refs["server\x00executable"], refs["client/v3\x00library"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &targetPortfolioClientStub{response: response}
	selected, outcome, err := selectTargetPortfolioForRun(
		context.Background(), "go.etcd.io/etcd/v3", catalog, nil,
		func() (targetPortfolioClient, error) { return client, nil },
	)
	serverRef := ""
	for _, entry := range catalog.Entries {
		if entry.DisplayPath == "server" && entry.Candidate.Target.Kind == analysistarget.KindExecutablePackage {
			serverRef = entry.Candidate.Target.Ref
		}
	}
	if err != nil || client.calls != 1 || selected != serverRef || outcome.SelectedTargets != 2 {
		t.Fatalf("etcd ordinary selection = %q / %#v / calls=%d / err=%v", selected, outcome, client.calls, err)
	}
	if len(compilation.Request.Targets) != 3 {
		t.Fatalf("etcd module surfaces = %#v", compilation.Request.Targets)
	}
	for _, path := range []string{"client/v3", "server"} {
		if !strings.Contains(client.prompt.User, `"display_path":"`+path+`"`) {
			t.Fatalf("etcd selector omitted %q: %s", path, client.prompt.User)
		}
	}
	for _, path := range []string{"client/v3/concurrency", "server/storage/wal"} {
		for _, target := range compilation.Request.Targets {
			if target.DisplayPath == path {
				t.Fatalf("etcd selector exposed package %q as a separate target: %#v", path, target)
			}
		}
	}
}

func TestTargetPortfolioRepoNameUsesModuleBeforeSemanticMajorSuffix(t *testing.T) {
	for _, test := range []struct {
		identity string
		want     string
	}{
		{identity: "go.etcd.io/etcd/v3", want: "etcd"},
		{identity: "github.com/acme/tool/v2/", want: "tool"},
		{identity: "example.com/tool/v10", want: "tool"},
		{identity: "example.com/tool/v999999999999999999999999999999999999", want: "tool"},
		{identity: "example.com/tool/v1", want: "v1"},
		{identity: "example.com/tool/v02", want: "v02"},
		{identity: "example.com/tool/version", want: "version"},
		{identity: "v3", want: "v3"},
		{identity: "example.com/tool", want: "tool"},
	} {
		t.Run(test.identity, func(t *testing.T) {
			if got := targetPortfolioRepoName(test.identity); got != test.want {
				t.Fatalf("targetPortfolioRepoName(%q) = %q, want %q", test.identity, got, test.want)
			}
		})
	}
}

func targetPortfolioRuntimeFacts() gofacts.Facts {
	modulePath := "example.com/repomap"
	packages := []gofacts.PackageFact{
		targetPortfolioRuntimePackage(modulePath, "cmd/helper"),
		targetPortfolioRuntimePackage(modulePath, "cmd/repomap"),
		targetPortfolioRuntimePackage(modulePath, "internal/engine"),
		targetPortfolioRuntimePackage(modulePath, "pkg/client"),
	}
	packages[3].Declarations = []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewClient"},
	}
	packages[3].ModuleID = "module-client"
	packages[3].ModulePath = modulePath + "/pkg/client"
	packages[3].CanonicalPath = packages[3].ModulePath
	packages[3].ModuleRelativeDir = "."
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true},
			{ID: "module-client", ModulePath: modulePath + "/pkg/client", ModuleDir: "pkg/client"},
		},
		Packages: packages,
		EntrypointPackages: []gofacts.Entrypoint{
			targetPortfolioRuntimeEntrypoint(modulePath, "cmd/helper", "tool", 12),
			targetPortfolioRuntimeEntrypoint(modulePath, "cmd/repomap", "unknown", 16),
		},
	}
}

func targetPortfolioEmptyRootLibraryFacts() gofacts.Facts {
	rootModule := "go.etcd.io/etcd/v3"
	serverModule := "go.etcd.io/etcd/server/v3"
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "root", ModulePath: rootModule, ModuleDir: ".", Main: true},
			{ID: "server", ModulePath: serverModule, ModuleDir: "server"},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: rootModule, Name: "main_test", ModuleID: "root",
				ModulePath: rootModule, PackageDir: ".", ModuleRelativeDir: ".",
				DisplayPath: ".", Locality: "local", DeclarationsScanned: true,
			},
			{
				CanonicalPath: serverModule, Name: "main", ModuleID: "server",
				ModulePath: serverModule, PackageDir: "server", ModuleRelativeDir: ".",
				DisplayPath: "server", Locality: "local", DeclarationsScanned: true,
				Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "main"}},
			},
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
}

func targetPortfolioRuntimeAmbiguousFacts() gofacts.Facts {
	facts := targetPortfolioRuntimeFacts()
	facts.Packages = facts.Packages[:2]
	facts.EntrypointPackages = facts.EntrypointPackages[:2]
	// Neither executable matches the module basename, so D257 intentionally
	// leaves the local default ambiguous.
	for index, dir := range []string{"cmd/api", "cmd/worker"} {
		facts.Packages[index].PackageDir = dir
		facts.Packages[index].ModuleRelativeDir = dir
		facts.Packages[index].DisplayPath = dir
		facts.Packages[index].CanonicalPath = "example.com/repomap/" + dir
		facts.EntrypointPackages[index].ImportPath = facts.Packages[index].CanonicalPath
		facts.EntrypointPackages[index].PackageDir = dir
		facts.EntrypointPackages[index].ModuleRelativeDir = dir
		facts.EntrypointPackages[index].Anchors[0].Path = dir + "/main.go"
		facts.EntrypointPackages[index].Kind = "unknown"
	}
	return facts
}

func targetPortfolioRuntimePackage(modulePath, dir string) gofacts.PackageFact {
	return gofacts.PackageFact{
		CanonicalPath: modulePath + "/" + dir, Name: strings.ReplaceAll(dir, "/", "_"),
		ModuleID: "module-root", ModulePath: modulePath, PackageDir: dir,
		ModuleRelativeDir: dir, DisplayPath: dir, Locality: "local", DeclarationsScanned: true,
		LoadCompleteness: completeGoPackageLoad(),
	}
}

func targetPortfolioRuntimeNestedModulePackage(
	moduleID, modulePath, moduleDir, relativeDir, name, declaration string,
) gofacts.PackageFact {
	packageDir := moduleDir
	canonicalPath := modulePath
	if relativeDir != "." {
		packageDir += "/" + relativeDir
		canonicalPath += "/" + relativeDir
	}
	return gofacts.PackageFact{
		CanonicalPath: canonicalPath, Name: name, ModuleID: moduleID, ModulePath: modulePath,
		PackageDir: packageDir, ModuleRelativeDir: relativeDir, DisplayPath: packageDir,
		Locality: "local", DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
		Declarations: []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: declaration}},
	}
}

func completeGoPackageLoad() *gofacts.PackageLoadCompleteness {
	return &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
}

func targetPortfolioRuntimeEntrypoint(modulePath, dir, kind string, line int) gofacts.Entrypoint {
	return gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: modulePath + "/" + dir,
		PackageDir: dir, ModuleRelativeDir: dir, ModuleDir: ".", Kind: kind,
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
			Path: dir + "/main.go", Line: line,
		}},
	}
}

func targetPortfolioRuntimeCatalog(t *testing.T, facts gofacts.Facts) analysistarget.TargetCatalog {
	t.Helper()
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func targetPortfolioRuntimeEntry(
	t *testing.T,
	catalog analysistarget.TargetCatalog,
	displayPath string,
) analysistarget.TargetCatalogEntry {
	t.Helper()
	for _, entry := range catalog.Entries {
		if entry.DisplayPath == displayPath {
			return entry
		}
	}
	t.Fatalf("catalog has no display path %q", displayPath)
	return analysistarget.TargetCatalogEntry{}
}
