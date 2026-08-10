package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestCompileProviderBoundaryAndResolve(t *testing.T) {
	catalog := mustCatalog(t, portfolioFacts())
	compilation, err := Compile("repomap", catalog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	if len(wire) > MaxRequestBytes || sha256Hex(wire) != compilation.RequestSHA256 {
		t.Fatalf("wire identity = %d %s, want <=%d and %s", len(wire), sha256Hex(wire), MaxRequestBytes, compilation.RequestSHA256)
	}

	var request Request
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	wantAdvertised := 0
	for _, entry := range catalog.Entries {
		if AdvertisedForSelection(entry) {
			wantAdvertised++
		}
	}
	if request.Version != RequestVersion || request.RepoName != "repomap" ||
		request.RequestRef == "" || len(request.Targets) != wantAdvertised {
		t.Fatalf("request = %#v", request)
	}
	advertisedIndex := 0
	for _, entry := range catalog.Entries {
		if !AdvertisedForSelection(entry) {
			continue
		}
		target := request.Targets[advertisedIndex]
		if target.Ref != fmt.Sprintf("t%d", advertisedIndex+1) || target.DisplayPath != entry.DisplayPath {
			t.Fatalf("target[%d] = %#v, catalog = %#v", advertisedIndex, target, entry)
		}
		advertisedIndex++
	}
	if bytes.Contains(wire, []byte(`"display_path":"internal/engine"`)) {
		t.Fatalf("non-root library leaked into ordinary candidate surface: %s", wire)
	}
	executable := targetByDisplayPath(t, request.Targets, "cmd/repomap")
	if !slices.Equal(symbolNames(executable, "func"), []string{"main", "runDevUICLI", "runProduct"}) {
		t.Fatalf("executable funcs = %#v", executable.Symbols)
	}
	library := targetByDisplayPath(t, request.Targets, "pkg/client")
	if !slices.Equal(symbolNames(library, "func"), []string{"NewClient"}) ||
		!slices.Equal(symbolNames(library, "method"), []string{"Client.Do", "hiddenType.Exported"}) ||
		!slices.Equal(symbolNames(library, "type"), []string{"Client"}) {
		t.Fatalf("library public API symbols = %#v", library.Symbols)
	}
	for _, forbidden := range []string{"internalHelper", "Client.debug"} {
		if bytes.Contains(wire, []byte(forbidden)) {
			t.Fatalf("library-private declaration %q leaked: %s", forbidden, wire)
		}
	}
	assertNoPrivateTargetMaterial(t, wire, catalog)

	prompt, err := BuildPrompt(compilation)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if prompt.Version != PromptVersion || !strings.Contains(strings.ToLower(prompt.User), "json") ||
		!strings.Contains(prompt.User, string(wire)) || !strings.Contains(prompt.System, "no paths") ||
		!strings.Contains(prompt.System, "symbols:[]") ||
		!strings.Contains(prompt.System, "Such a target is ineligible") ||
		!strings.Contains(prompt.System, "separate top-level report scope in the left navigation") ||
		!strings.Contains(prompt.System, "This is not package coverage") ||
		!strings.Contains(prompt.System, "Returning only the default is correct") {
		t.Fatalf("prompt does not carry exact refs-only JSON contract: %#v", prompt)
	}
	promptPrefix := strings.Split(prompt.User, "Exact bounded target catalog JSON:")[0]
	if !strings.Contains(promptPrefix, compilation.Request.RequestRef) ||
		strings.Contains(promptPrefix, `"t1"`) {
		t.Fatalf("prompt schema carries a literal target anchor or omits its dynamic request identity: %s", promptPrefix)
	}
	assertNoPrivateTargetMaterial(t, []byte(prompt.System+prompt.User), catalog)

	refByPath := make(map[string]string, len(request.Targets))
	for _, target := range request.Targets {
		refByPath[target.DisplayPath] = target.Ref
	}
	response := Response{
		Version: ResultVersion, RequestRef: request.RequestRef,
		DefaultRef: refByPath["cmd/repomap"],
		TargetRefs: []string{refByPath["pkg/client"], refByPath["cmd/repomap"]},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, raw)
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	if selection.Version != ResultVersion || selection.CatalogRef != catalog.Ref ||
		selection.RequestRef != request.RequestRef || selection.RequestSHA256 != compilation.RequestSHA256 ||
		selection.Default.DisplayPath != "cmd/repomap" {
		t.Fatalf("selection identity/default = %#v", selection)
	}
	oldRaw, err := json.Marshal(Response{
		Version: ResultVersion - 1, RequestRef: request.RequestRef,
		DefaultRef: refByPath["cmd/repomap"], TargetRefs: []string{refByPath["cmd/repomap"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveResponse(compilation, oldRaw); err == nil {
		t.Fatalf("ResolveResponse accepted prior result contract v%d", ResultVersion-1)
	}
	wantPaths := canonicalSelectedPaths(request.Targets, map[string]bool{
		refByPath["cmd/repomap"]: true, refByPath["pkg/client"]: true,
	})
	gotPaths := make([]string, 0, len(selection.Targets))
	for _, target := range selection.Targets {
		gotPaths = append(gotPaths, target.DisplayPath)
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("restored paths = %#v, want canonical %#v", gotPaths, wantPaths)
	}
	selection.Targets[0].Candidate.Target.Roots = append(selection.Targets[0].Candidate.Target.Roots, analysistarget.Root{Path: "mutated.go", Line: 1})
	if reflectSelectionMutation(compilation, selection.Targets[0].Candidate.Target.Ref) {
		t.Fatal("selection mutation changed private compilation authority")
	}
	selection.Targets[0].Symbols[0].Name = "Mutated"
	if compilationAuthorityHasSymbol(compilation, "Mutated") {
		t.Fatal("selection symbol mutation changed private compilation authority")
	}
}

func TestCompileIsStableAcrossFactPermutation(t *testing.T) {
	facts := portfolioFacts()
	permuted := facts
	permuted.Modules = slices.Clone(facts.Modules)
	permuted.Packages = slices.Clone(facts.Packages)
	permuted.EntrypointPackages = slices.Clone(facts.EntrypointPackages)
	slices.Reverse(permuted.Modules)
	slices.Reverse(permuted.Packages)
	slices.Reverse(permuted.EntrypointPackages)
	for index := range permuted.Packages {
		permuted.Packages[index].Declarations = slices.Clone(permuted.Packages[index].Declarations)
		slices.Reverse(permuted.Packages[index].Declarations)
	}

	leftCatalog := mustCatalog(t, facts)
	rightCatalog := mustCatalog(t, permuted)
	left, err := Compile("repomap", leftCatalog)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Compile("repomap", rightCatalog)
	if err != nil {
		t.Fatal(err)
	}
	leftWire, _ := ProviderVisibleJSON(left)
	rightWire, _ := ProviderVisibleJSON(right)
	if leftCatalog.Ref != rightCatalog.Ref || !bytes.Equal(leftWire, rightWire) ||
		left.Request.RequestRef != right.Request.RequestRef || left.RequestSHA256 != right.RequestSHA256 {
		t.Fatalf("permutation changed catalog/request:\nleft  %s %s %s\nright %s %s %s",
			leftCatalog.Ref, left.Request.RequestRef, leftWire,
			rightCatalog.Ref, right.Request.RequestRef, rightWire,
		)
	}
}

func TestRequestRefBindsPrivateCatalogMapping(t *testing.T) {
	left, err := Compile("same", mustCatalog(t, libraryFacts("example.com/left", 2, "pkg/api-%04d")))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Compile("same", mustCatalog(t, libraryFacts("example.com/right", 2, "pkg/api-%04d")))
	if err != nil {
		t.Fatal(err)
	}
	leftIdentity := requestIdentity{
		Version: left.Request.Version, RepoName: left.Request.RepoName, Targets: left.Request.Targets,
	}
	rightIdentity := requestIdentity{
		Version: right.Request.Version, RepoName: right.Request.RepoName, Targets: right.Request.Targets,
	}
	leftVisible, _ := json.Marshal(leftIdentity)
	rightVisible, _ := json.Marshal(rightIdentity)
	if !bytes.Equal(leftVisible, rightVisible) {
		t.Fatalf("control setup differs provider-visible except request ref:\nleft  %s\nright %s", leftVisible, rightVisible)
	}
	if left.CatalogRef == right.CatalogRef || left.Request.RequestRef == right.Request.RequestRef ||
		left.RequestSHA256 == right.RequestSHA256 {
		t.Fatalf("private mapping was not bound: left=%s %s %s right=%s %s %s",
			left.CatalogRef, left.Request.RequestRef, left.RequestSHA256,
			right.CatalogRef, right.Request.RequestRef, right.RequestSHA256,
		)
	}
	oldResponse, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: left.Request.RequestRef,
		DefaultRef: "t1", TargetRefs: []string{"t1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveResponse(right, oldResponse); err == nil {
		t.Fatal("response for same visible t1 mapping restored against another private catalog")
	}
}

func TestResolveResponseRejectsInvalidAtomicDecision(t *testing.T) {
	compilation, err := Compile("repomap", mustCatalog(t, portfolioFacts()))
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]string, 0, len(compilation.Request.Targets))
	for _, target := range compilation.Request.Targets {
		refs = append(refs, target.Ref)
	}
	valid := func(defaultRef string, targetRefs []string) []byte {
		raw, marshalErr := json.Marshal(Response{
			Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
			DefaultRef: defaultRef, TargetRefs: targetRefs,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}
	tests := map[string][]byte{
		"malformed":            []byte(`{"version":`),
		"null":                 []byte(`null`),
		"empty":                []byte(`{}`),
		"extra field":          []byte(fmt.Sprintf(`{"version":%d,"request_ref":%q,"default_ref":%q,"target_refs":[%q],"reason":"no"}`, ResultVersion, compilation.Request.RequestRef, refs[0], refs[0])),
		"trailing value":       append(valid(refs[0], []string{refs[0]}), []byte(` {}`)...),
		"wrong version":        []byte(fmt.Sprintf(`{"version":%d,"request_ref":%q,"default_ref":%q,"target_refs":[%q]}`, ResultVersion+1, compilation.Request.RequestRef, refs[0], refs[0])),
		"previous version":     []byte(fmt.Sprintf(`{"version":%d,"request_ref":%q,"default_ref":%q,"target_refs":[%q]}`, ResultVersion-1, compilation.Request.RequestRef, refs[0], refs[0])),
		"wrong request":        []byte(fmt.Sprintf(`{"version":%d,"request_ref":"tpq-old","default_ref":%q,"target_refs":[%q]}`, ResultVersion, refs[0], refs[0])),
		"empty targets":        valid(refs[0], nil),
		"unknown default":      valid("t999", []string{"t999"}),
		"unknown target":       valid(refs[0], []string{refs[0], "t999"}),
		"duplicate target":     valid(refs[0], []string{refs[0], refs[0]}),
		"default not selected": valid(refs[0], []string{refs[1]}),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveResponse(compilation, raw); err == nil {
				t.Fatalf("ResolveResponse accepted %s: %s", name, raw)
			}
		})
	}

	secret := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	if _, err := ResolveResponse(compilation, valid(secret, []string{secret})); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unknown secret-shaped ref error = %v, want safe rejection", err)
	}
	oversized := bytes.Repeat([]byte{'x'}, MaxResponseBytes+1)
	if _, err := ResolveResponse(compilation, oversized); err == nil {
		t.Fatal("ResolveResponse accepted oversized response")
	}
}

func TestResolveResponseRejectsEmptyAPILibraryFromPrivateAuthority(t *testing.T) {
	facts := portfolioFacts()
	facts.Packages[3].Declarations = nil
	compilation, err := Compile("repomap", mustCatalog(t, facts))
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string, len(compilation.Request.Targets))
	for _, target := range compilation.Request.Targets {
		refs[target.DisplayPath] = target.Ref
	}
	emptyRef := refs["pkg/client"]
	executableRef := refs["cmd/repomap"]
	if emptyRef == "" || executableRef == "" ||
		len(compilation.authority[emptyRef].Symbols) != 0 ||
		compilation.authority[emptyRef].Candidate.Target.Kind != analysistarget.KindLibraryPackage {
		t.Fatalf("empty-library control setup = %#v", compilation.authority[emptyRef])
	}
	encode := func(defaultRef string, targetRefs []string) []byte {
		raw, marshalErr := json.Marshal(Response{
			Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
			DefaultRef: defaultRef, TargetRefs: targetRefs,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}
	for name, raw := range map[string][]byte{
		"empty library default": encode(emptyRef, []string{emptyRef}),
		"empty library member":  encode(executableRef, []string{executableRef, emptyRef}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveResponse(compilation, raw); err == nil ||
				!strings.Contains(err.Error(), "no advertised public API") {
				t.Fatalf("empty API response error = %v", err)
			}
		})
	}
	selection, err := ResolveResponse(compilation, encode(executableRef, []string{executableRef}))
	if err != nil || selection.Default.DisplayPath != "cmd/repomap" {
		t.Fatalf("eligible current-v%d response = %#v, %v", ResultVersion, selection, err)
	}
}

func TestByteEnvelopeHasNoSemanticTargetCountCap(t *testing.T) {
	smallCatalog := mustCatalog(t, libraryFacts("example.com/many", 400, "pkg/p%04d"))
	compilation, err := Compile("many", smallCatalog)
	if err != nil {
		t.Fatalf("Compile 400 complete targets: %v", err)
	}
	if len(compilation.Request.Targets) != 400 {
		t.Fatalf("compiled targets = %d, want all 400", len(compilation.Request.Targets))
	}
	refs := make([]string, 0, len(compilation.Request.Targets))
	for _, target := range compilation.Request.Targets {
		refs = append(refs, target.Ref)
	}
	raw, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: refs[0], TargetRefs: refs,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, raw)
	if err != nil || len(selection.Targets) != 400 {
		t.Fatalf("ResolveResponse all 400: targets=%d err=%v", len(selection.Targets), err)
	}

	largeCatalog := mustCatalog(t, libraryFacts("example.com/huge", 6000, "packages/long-target-name-%04d"))
	if _, err := Compile("huge", largeCatalog); err == nil || !strings.Contains(err.Error(), "complete request") {
		t.Fatalf("oversized complete catalog error = %v", err)
	}

	symbolHeavy := libraryFacts("example.com/symbols", 1, "pkg/api-%04d")
	for index := 0; index < 20_000; index++ {
		symbolHeavy.Packages[0].Declarations = append(symbolHeavy.Packages[0].Declarations, gofacts.PackageDeclaration{
			Kind: gofacts.PackageDeclarationFunc, Name: fmt.Sprintf("ExportedSymbol%05d", index),
		})
	}
	if _, err := Compile("symbols", mustCatalog(t, symbolHeavy)); err == nil || !strings.Contains(err.Error(), "complete request") {
		t.Fatalf("oversized complete symbol inventory error = %v", err)
	}
}

func TestProviderBoundaryRejectsVisibleSecretsButNotPrivateIdentity(t *testing.T) {
	secret := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	if _, err := Compile(secret, mustCatalog(t, portfolioFacts())); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret repo name error = %v, want safe closed error", err)
	}

	visibleSecretFacts := libraryFacts("example.com/safe", 1, "pkg/"+secret+"-%04d")
	if _, err := Compile("safe", mustCatalog(t, visibleSecretFacts)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret display path error = %v, want safe closed error", err)
	}

	privateSecretModule := libraryFacts("github.com/"+secret+"/safe", 1, "pkg/api-%04d")
	compilation, err := Compile("safe", mustCatalog(t, privateSecretModule))
	if err != nil {
		t.Fatalf("private canonical identity must stay local, Compile: %v", err)
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte(secret)) || bytes.Contains(wire, []byte("github.com")) {
		t.Fatalf("private canonical identity leaked: %s", wire)
	}
}

func TestProviderVisibleJSONRejectsCompilationTamper(t *testing.T) {
	compilation, err := Compile("repomap", mustCatalog(t, portfolioFacts()))
	if err != nil {
		t.Fatal(err)
	}
	compilation.Request.Targets[0].DisplayPath = "invented/path"
	if _, err := ProviderVisibleJSON(compilation); err == nil {
		t.Fatal("ProviderVisibleJSON accepted request/catalog drift")
	}

	compilation, err = Compile("repomap", mustCatalog(t, portfolioFacts()))
	if err != nil {
		t.Fatal(err)
	}
	compilation.Request.Targets[0].Symbols[0].Names[0] = "invented"
	if _, err := ProviderVisibleJSON(compilation); err == nil {
		t.Fatal("ProviderVisibleJSON accepted symbol/catalog drift")
	}

	compilation, err = Compile("repomap", mustCatalog(t, portfolioFacts()))
	if err != nil {
		t.Fatal(err)
	}
	compilation.Request.Version = RequestVersion - 1
	if _, err := ProviderVisibleJSON(compilation); err == nil {
		t.Fatalf("ProviderVisibleJSON accepted prior request contract v%d", RequestVersion-1)
	}
}

func TestCompileRejectsUnavailableDeclarationInventory(t *testing.T) {
	facts := portfolioFacts()
	facts.Packages[0].Declarations = nil
	facts.Packages[0].DeclarationsScanned = false
	if _, err := Compile("repomap", mustCatalog(t, facts)); err == nil || !strings.Contains(err.Error(), "declaration labels are unavailable") {
		t.Fatalf("unavailable declarations error = %v", err)
	}
}

func portfolioFacts() gofacts.Facts {
	modulePath := "github.com/acme/repomap"
	packages := []gofacts.PackageFact{
		packageFact("module-root", modulePath, "cmd/helper"),
		packageFact("module-root", modulePath, "cmd/repomap"),
		packageFact("module-root", modulePath, "internal/engine"),
		packageFact("module-root", modulePath, "pkg/client"),
	}
	packages[0].Declarations = []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "main"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "runDevPreview"},
	}
	packages[1].Declarations = []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "main"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "runDevUICLI"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "runProduct"},
	}
	packages[2].Declarations = []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Exported"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "internalHelper"},
	}
	packages[3].Declarations = []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewClient"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "internalHelper"},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Client", Name: "Do"},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Client", Name: "debug"},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "hiddenType", Name: "Exported"},
		{Kind: gofacts.PackageDeclarationType, Name: "Client"},
		{Kind: gofacts.PackageDeclarationType, Name: "hiddenType"},
	}
	packages[3].ModuleID = "module-client"
	packages[3].ModulePath = modulePath + "/pkg/client"
	packages[3].CanonicalPath = packages[3].ModulePath
	packages[3].ModuleRelativeDir = "."
	entrypoints := []gofacts.Entrypoint{
		entrypointFact(modulePath, "cmd/helper", "tool", 12),
		entrypointFact(modulePath, "cmd/repomap", "", 16),
	}
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true},
			{ID: "module-client", ModulePath: modulePath + "/pkg/client", ModuleDir: "pkg/client"},
		},
		Packages: packages, EntrypointPackages: entrypoints,
	}
}

func libraryFacts(modulePath string, count int, format string) gofacts.Facts {
	packages := make([]gofacts.PackageFact, 0, count)
	modules := make([]gofacts.ModuleFact, 0, count)
	for index := 0; index < count; index++ {
		dir := fmt.Sprintf(format, index)
		moduleID := fmt.Sprintf("module-%04d", index)
		packageModulePath := modulePath + "/" + dir
		pkg := packageFact(moduleID, packageModulePath, dir)
		pkg.CanonicalPath = packageModulePath
		pkg.ModuleRelativeDir = "."
		pkg.Declarations = []gofacts.PackageDeclaration{{
			Kind: gofacts.PackageDeclarationFunc, Name: fmt.Sprintf("Exported%04d", index),
		}}
		packages = append(packages, pkg)
		modules = append(modules, gofacts.ModuleFact{ID: moduleID, ModulePath: packageModulePath, ModuleDir: dir})
	}
	return gofacts.Facts{
		Modules: modules, Packages: packages,
	}
}

func packageFact(moduleID, modulePath, dir string) gofacts.PackageFact {
	return gofacts.PackageFact{
		CanonicalPath: modulePath + "/" + dir, Name: strings.ReplaceAll(dir, "/", "_"),
		ModuleID: moduleID, ModulePath: modulePath, PackageDir: dir,
		ModuleRelativeDir: dir, DisplayPath: dir, Locality: "local", DeclarationsScanned: true,
	}
}

func entrypointFact(modulePath, dir, kind string, line int) gofacts.Entrypoint {
	return gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: modulePath + "/" + dir,
		PackageDir: dir, ModuleRelativeDir: dir, ModuleDir: ".", Kind: kind,
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
			Path: dir + "/main.go", Line: line,
		}},
	}
}

func mustCatalog(t *testing.T, facts gofacts.Facts) analysistarget.TargetCatalog {
	t.Helper()
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	return catalog
}

func assertNoPrivateTargetMaterial(t *testing.T, providerBytes []byte, catalog analysistarget.TargetCatalog) {
	t.Helper()
	for _, forbidden := range []string{
		"github.com/acme/repomap", `"candidate"`, `"module_id"`, `"module_path"`,
		`"package_path"`, `"root_boundary"`, `"roots"`, "cmd/repomap/main.go",
		`"declarations"`, `"receiver"`, `"files"`, `"source"`, `"comments"`,
		catalog.Ref, catalog.DefaultTargetRef,
	} {
		if forbidden != "" && bytes.Contains(providerBytes, []byte(forbidden)) {
			t.Fatalf("private target material %q leaked in %s", forbidden, providerBytes)
		}
	}
}

func canonicalSelectedPaths(targets []Target, selected map[string]bool) []string {
	result := make([]string, 0, len(selected))
	for _, target := range targets {
		if selected[target.Ref] {
			result = append(result, target.DisplayPath)
		}
	}
	return result
}

func reflectSelectionMutation(compilation Compilation, targetRef string) bool {
	for _, entry := range compilation.authority {
		if entry.Candidate.Target.Ref == targetRef {
			for _, root := range entry.Candidate.Target.Roots {
				if root.Path == "mutated.go" {
					return true
				}
			}
		}
	}
	return false
}

func targetByDisplayPath(t *testing.T, targets []Target, displayPath string) Target {
	t.Helper()
	for _, target := range targets {
		if target.DisplayPath == displayPath {
			return target
		}
	}
	t.Fatalf("target %q is absent", displayPath)
	return Target{}
}

func symbolNames(target Target, kind string) []string {
	for _, group := range target.Symbols {
		if group.Kind == kind {
			return group.Names
		}
	}
	return nil
}

func compilationAuthorityHasSymbol(compilation Compilation, name string) bool {
	for _, entry := range compilation.authority {
		for _, symbol := range entry.Symbols {
			if symbol.Name == name {
				return true
			}
		}
	}
	return false
}
