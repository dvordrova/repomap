package jstsproject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestSealDecodeAndExactProgramIndexProjection(t *testing.T) {
	result := minimalResult(t, "javascript")
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	index, catalog, err := BuildFromResult(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProgramIndex(decoded, index); err != nil {
		t.Fatal(err)
	}
	if index.Target.Kind != "library" || len(index.Target.Seeds) != 0 {
		t.Fatalf("tool/library target promoted: %#v", index.Target)
	}
	if catalog.Coverage.State != "complete" {
		t.Fatalf("catalog coverage = %q", catalog.Coverage.State)
	}

	tampered := index
	tampered.Coverage.ObjectsObserved++
	if err := ValidateProgramIndex(decoded, tampered); err == nil {
		t.Fatal("tampered ProgramIndex was accepted")
	}
}

func TestDecodeRejectsPreviousJSTSArtifactVersion(t *testing.T) {
	encoded, err := Encode(minimalResult(t, "typescript"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	document["version"] = Version - 1
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded); err == nil || !strings.Contains(err.Error(), "invalid producer identity") {
		t.Fatalf("previous JSTS artifact version error = %v", err)
	}
}

func TestReservedJavaScriptPlatformAuthorityCannotBeDeclaredOrImported(t *testing.T) {
	t.Run("manifest dependency", func(t *testing.T) {
		result := minimalResult(t, "typescript").Snapshot()
		result.Project.Dependencies = []PackageDependency{{PackagePath: javascriptPlatform, Scope: "production"}}
		if _, err := Seal(result); err == nil || !strings.Contains(err.Error(), "invalid manifest dependency") {
			t.Fatalf("reserved manifest dependency error = %v", err)
		}
	})

	t.Run("external import", func(t *testing.T) {
		result := minimalResult(t, "typescript").Snapshot()
		result.Imports = []Import{{
			Ref: "import:reserved-platform", Kind: "import", Specifier: javascriptPlatform,
			ImporterFileRef: result.Files[0].FileRef, ExternalPackage: javascriptPlatform,
			Resolution: "exact", Location: result.Files[0].location(),
		}}
		if _, err := Seal(result); err == nil || !strings.Contains(err.Error(), "cannot originate from an import") {
			t.Fatalf("reserved import authority error = %v", err)
		}
	})
}

func TestProductSurfaceVariableBecomesBoundObjectSeed(t *testing.T) {
	result := minimalResult(t, "typescript")
	entry := Declaration{
		Ref: "decl:f2:2:1:variable:app", Kind: "variable", Name: "app", QualifiedName: "src/index#app",
		Exported: true, Location: Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 2, Column: 1},
	}
	result.Declarations = append(result.Declarations, entry)
	result.Surfaces = []Surface{{
		Ref: "surface:browser", Kind: SurfaceBrowser, Role: SurfaceProduct, Name: "Browser application",
		EntryRefs: []string{entry.Ref}, EvidenceRefs: []string{}, Location: entry.Location,
	}}
	result.SHA256 = ""
	targetID, err := deriveProgramTargetID(result)
	if err != nil {
		t.Fatalf("derive application target: %v", err)
	}
	result.ProgramTargetID = targetID
	sealed, err := Seal(result)
	if err != nil {
		t.Fatalf("seal application: %v", err)
	}
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatalf("project application: %v", err)
	}
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedBoundObject {
		t.Fatalf("product variable seed = %#v", index.Target.Seeds)
	}
	var object programindex.Object
	for _, candidate := range index.Objects {
		if candidate.ID == index.Target.Seeds[0].ObjectID {
			object = candidate
			break
		}
	}
	if object.ID == "" || object.Kind != programindex.ObjectVariable {
		t.Fatalf("product variable object = %#v", object)
	}
}

func TestPackageBinaryCreatesCLIProductAndRuntimeScriptCreatesSeparateSeed(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.Project.Binaries = []PackageBinary{{Command: "sample", Path: "bin/sample", FileRef: "f3"}}
	result.Project.Scripts = []Script{
		{Name: "dev", Kind: "runtime", EntryFileRefs: []string{"f2"}},
		{Name: "dev:temporary", Kind: "other", EntryFileRefs: []string{"f2"}},
	}
	addPackageBinarySurfaces(&result)
	sealed := rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if TargetKind(sealed) != "application" || index.Target.Kind != "application" {
		t.Fatalf("CLI target kind = %q / %q", TargetKind(sealed), index.Target.Kind)
	}
	cliCount := 0
	for _, surface := range sealed.Surfaces {
		if surface.Kind != SurfaceCLI {
			continue
		}
		cliCount++
		if surface.Role != SurfaceProduct || len(surface.EntryRefs) != 0 || len(surface.EvidenceRefs) != 0 ||
			surface.Location.Path != "bin/sample" || surface.Location.FileRef != "f3" {
			t.Fatalf("CLI surface invented implementation authority: %#v", surface)
		}
	}
	if cliCount != 1 {
		t.Fatalf("CLI surfaces = %#v", sealed.Surfaces)
	}
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedScript {
		t.Fatalf("CLI runtime seeds = %#v", index.Target.Seeds)
	}
	seededSourceRef := ""
	for _, object := range index.Objects {
		if object.ID == index.Target.Seeds[0].ObjectID {
			seededSourceRef = object.SourceRef
			break
		}
	}
	if seededSourceRef != "module:f2" {
		t.Fatalf("CLI runtime seed source = %q, want module:f2", seededSourceRef)
	}
	foundBinarySource := false
	for _, source := range index.Target.Sources {
		if source.Path == "bin/sample" && source.FileRef == "f3" {
			foundBinarySource = true
		}
	}
	if !foundBinarySource {
		t.Fatalf("CLI binary is absent from ProgramTarget sources: %#v", index.Target.Sources)
	}
}

func TestRuntimeScriptWithoutPackageBinaryRemainsLibraryAndDoesNotSeed(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.Project.Scripts = []Script{{Name: "dev", Kind: "runtime", EntryFileRefs: []string{"f2"}}}
	sealed := rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if index.Target.Kind != "library" || len(index.Target.Seeds) != 0 {
		t.Fatalf("runtime-only package was promoted: %#v", index.Target)
	}
}

func TestCLIRuntimeScriptWithMultipleSourceRefsDoesNotSeed(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	second := File{
		FileRef: "f4", Path: "src/preload.ts", Language: "typescript", Module: "src/preload",
		SHA256: strings.Repeat("d", 64),
	}
	result.Files = append(result.Files, second)
	result.Declarations = append(result.Declarations, Declaration{
		Ref: "module:f4", Kind: "module", Name: "src/preload", QualifiedName: "src/preload", Location: second.location(),
	})
	result.SourceSHA256 = sourceDigest(result.Files)
	result.Project.Binaries = []PackageBinary{{Command: "sample", Path: "bin/sample", FileRef: "f3"}}
	result.Project.Scripts = []Script{{Name: "start", Kind: "runtime", EntryFileRefs: []string{"f2", "f4"}}}
	addPackageBinarySurfaces(&result)
	sealed := rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if index.Target.Kind != "application" || len(index.Target.Seeds) != 0 {
		t.Fatalf("ambiguous runtime script gained seed authority: %#v", index.Target)
	}
}

func TestCLISurfaceRejectsInventedBinToSourceEntry(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.Project.Binaries = []PackageBinary{{Command: "sample", Path: "bin/sample", FileRef: "f3"}}
	addPackageBinarySurfaces(&result)
	result.Surfaces[0].EntryRefs = []string{"module:f2"}
	if _, err := Seal(result); err == nil || !strings.Contains(err.Error(), "does not match exact package binary authority") {
		t.Fatalf("invented CLI implementation entry error = %v", err)
	}
}

func TestSealOmitsPersistenceSensitiveOptionalSourceMetadata(t *testing.T) {
	result := minimalResult(t, "typescript")
	declaration := Declaration{
		Ref: "decl:f2:1:1:function:headers", Kind: "function", Name: "headers", QualifiedName: "src/index#headers",
		Signature: `(): { Authorization: string; "Content-Type": string; }`, Location: result.Files[0].location(),
	}
	result.Declarations = append(result.Declarations, declaration)
	result.Calls = append(result.Calls, Call{
		Ref: "call:sensitive-test-sentinel", CallerRef: declaration.Ref, CalleeRefs: []string{},
		Invocation: "call", Expression: `client({api_key: "opaque-provider-value-1234"})`, Resolution: "unresolved",
		Location: result.Files[0].location(),
	})
	result.ProductPaths = append(result.ProductPaths, ProductPath{
		Ref: "path:sensitive-test-sentinel", Name: "Test path", Outcome: "Test outcome", Frontier: result.Calls[0].Expression,
		Steps: []PathStep{{
			Ordinal: 1, Kind: "program_call", Label: result.Calls[0].Expression,
			SourceRef: result.Calls[0].Ref, TargetRefs: []string{}, Resolution: "unresolved",
			Authority: "unresolved_frontier", Location: result.Files[0].location(),
		}},
	})
	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range sealed.Declarations {
		if declaration.Name == "headers" && declaration.Signature != "" {
			t.Fatalf("persistence-sensitive optional signature was retained: %q", declaration.Signature)
		}
	}
	if len(sealed.Calls) != 1 || sealed.Calls[0].Expression != redactedExpression {
		t.Fatalf("persistence-sensitive call expression was retained: %#v", sealed.Calls)
	}
	if len(sealed.ProductPaths) != 1 || sealed.ProductPaths[0].Steps[0].Label != redactedExpression {
		t.Fatalf("persistence-sensitive product-path label was retained: %#v", sealed.ProductPaths)
	}
	if sealed.ProductPaths[0].Frontier != redactedExpression {
		t.Fatalf("persistence-sensitive product-path frontier was retained: %#v", sealed.ProductPaths)
	}
	encoded, err := Encode(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatal(err)
	}
	sensitiveEncoded := bytes.Replace(
		encoded, []byte(`"sample"`), []byte(`"sk-secret-shaped-provider-output"`), 1,
	)
	if _, err := Decode(sensitiveEncoded); err == nil || !strings.Contains(err.Error(), "persistence-sensitive") {
		t.Fatalf("sensitive encoded artifact error = %v", err)
	}
	if _, _, err := BuildFromResult(sealed); err != nil {
		t.Fatal(err)
	}
}

func TestSealOmitsAbsoluteSiblingTypeSignature(t *testing.T) {
	result := minimalResult(t, "typescript")
	declaration := Declaration{
		Ref: "decl:f2:2:14:variable:app", Kind: "variable", Name: "app", QualifiedName: "src/index#app",
		Signature: `import("/host/repository/packages/shared/src/index").Shared`, Location: result.Files[0].location(),
	}
	result.Declarations = append(result.Declarations, declaration)

	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range sealed.Declarations {
		if candidate.Ref != declaration.Ref {
			continue
		}
		found = true
		if candidate.Signature != "" || candidate.Name != declaration.Name || candidate.QualifiedName != declaration.QualifiedName || candidate.Location != declaration.Location {
			t.Fatalf("sibling-type declaration identity changed while omitting display signature: %#v", candidate)
		}
	}
	if !found {
		t.Fatalf("sibling-type declaration was dropped: %#v", sealed.Declarations)
	}
	encoded, err := Encode(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatal(err)
	}
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatal(err)
	}
	projected := false
	for _, object := range index.Objects {
		if object.SourceRef == declaration.Ref {
			projected = true
			if object.Signature != "" || object.Name != declaration.QualifiedName {
				t.Fatalf("sibling-type declaration projection = %#v", object)
			}
		}
	}
	if !projected {
		t.Fatalf("sibling-type declaration was not projected: %#v", index.Objects)
	}
}

func TestProgramTargetIdentityOmitsUnsafeSignatureBeforeProjection(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.ProgramTargetID = ""
	result.SHA256 = ""
	declaration := Declaration{
		Ref: "decl:f2:2:14:variable:app", Kind: "variable", Name: "app", QualifiedName: "src/index#app",
		Signature: `{ shared: import("/host/repository/packages/shared/src/index").Shared; ` + strings.Repeat("field: number; ", 200),
		Location:  result.Files[0].location(),
	}
	result.Declarations = append(result.Declarations, declaration)

	if _, err := deriveProgramTargetID(result); err == nil || !strings.Contains(err.Error(), "invalid object input") {
		t.Fatalf("raw unsafe signature ProgramTarget error = %v", err)
	}
	if err := bindProgramTargetIdentity(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.ProgramTargetID, "program-target-") {
		t.Fatalf("program target identity = %q", result.ProgramTargetID)
	}
	for _, candidate := range result.Declarations {
		if candidate.Ref == declaration.Ref && candidate.Signature != "" {
			t.Fatalf("unsafe signature survived identity binding: %q", candidate.Signature)
		}
	}
	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildFromResult(sealed); err != nil {
		t.Fatal(err)
	}
}

func TestResultRejectsDanglingFactReferences(t *testing.T) {
	result := minimalResult(t, "typescript")
	result.Surfaces = []Surface{{Ref: "surface:bad", Kind: SurfaceBrowser, Role: SurfaceProduct, Name: "bad", EntryRefs: []string{"decl:missing"}, EvidenceRefs: []string{}, Location: result.Files[0].location()}}
	result.ProgramTargetID = "program-target-placeholder"
	if _, err := Seal(result); err == nil || !strings.Contains(err.Error(), "unknown evidence ref") {
		t.Fatalf("Seal dangling surface = %v", err)
	}
}

func TestManifestFactsNeverRetainCredentialBearingLocatorsOrCommands(t *testing.T) {
	secret := "super-secret-token-value"
	manifest := packageManifest{
		Dependencies: map[string]string{"safe-package": "git+https://user:" + secret + "@example.invalid/repo.git"},
		Scripts:      map[string]string{"build": "NPM_TOKEN=" + secret + " tsx script/build.ts"},
	}
	dependencies := manifestDependencyFacts(manifest)
	scripts := buildScriptFacts(manifest.Scripts, []File{{FileRef: "f2", Path: "script/build.ts"}})
	encoded := []byte(strings.TrimSpace(strings.Join([]string{dependencies[0].PackagePath, scripts[0].Name, scripts[0].Kind, strings.Join(scripts[0].EntryFileRefs, ",")}, " ")))
	if bytes.Contains(encoded, []byte(secret)) {
		t.Fatal("credential-bearing manifest locator or command was retained")
	}
}

func TestPreparedCaltodoProject(t *testing.T) {
	root := os.Getenv("REPOMAP_JSTS_CALT0DO_ROOT")
	if root == "" {
		t.Skip("set REPOMAP_JSTS_CALT0DO_ROOT to a prepared Caltodo checkout")
	}
	repository, err := corpus.Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, catalog, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ModuleResolution != "bundler" || result.Project.BaseURL != "." {
		t.Fatalf("compiler config = %#v", result.Project)
	}
	if TargetKind(result) != "application" || index.Target.ID != result.ProgramTargetID {
		t.Fatalf("target binding = %#v", index.Target)
	}
	assertSurface(t, result, SurfaceBrowser, SurfaceProduct)
	assertSurface(t, result, SurfaceServer, SurfaceProduct)
	assertSurface(t, result, SurfaceShared, SurfaceSupporting)
	assertSurface(t, result, SurfaceTool, SurfaceScript)
	if !hasRoute(result, RouteBrowser, "", "/settings") || !hasRoute(result, RouteBrowserLink, "", "/settings") || !hasRoute(result, RouteHTTP, "PATCH", "/api/settings") {
		t.Fatal("Caltodo browser/link/server settings routes were not retained")
	}
	declarationNames := map[string]string{}
	for _, declaration := range result.Declarations {
		declarationNames[declaration.Ref] = declaration.Name
	}
	settingsRoute := routeForNamedRefs(result, RouteHTTP, "PATCH", "/api/settings", declarationNames, "requireAuth", "createPatchSettingsHandler")
	if settingsRoute == nil {
		t.Fatalf("Caltodo settings route lost exact middleware/factory refs: %#v", settingsRoute)
	}
	settingsFactoryRef := settingsRoute.HandlerRefs[0]
	returnedHandlerRef := ""
	for _, declaration := range result.Declarations {
		if declaration.OwnerRef == settingsFactoryRef && declaration.Kind == "lambda" && declaration.Name == "returned_handler" {
			returnedHandlerRef = declaration.Ref
			break
		}
	}
	if returnedHandlerRef == "" {
		t.Fatal("Caltodo settings factory lost its exact returned handler")
	}
	if !hasHTTPUse(result, "PATCH", "/api/settings") {
		t.Fatal("Caltodo apiRequest settings mutation was not retained")
	}
	if !hasContract(result, "zod_schema", "updateSettingsSchema") || !hasContract(result, "drizzle_table", "userSettings") {
		t.Fatal("Caltodo Zod/Drizzle settings contracts were not retained")
	}
	if !contractUsedWithin(result, "zod_schema", "updateSettingsSchema", returnedHandlerRef) || !hasContract(result, "query_key", "/api/settings") {
		t.Fatalf("Caltodo exact handler/query contract use was not retained: handler=%s contracts=%#v", returnedHandlerRef, result.Contracts)
	}
	if !hasResource(result, "postgres_database") {
		t.Fatal("Caltodo PostgreSQL resource was not retained")
	}
	if !hasPathKinds(result, []string{"page_route", "mutation_site", "program_call", "client_http_use", "http_method_path_match", "server_route", "handler_factory", "handler", "contract_validation", "storage_call", "resource_boundary"}) {
		t.Fatalf("grounded settings path incomplete: %#v", result.ProductPaths)
	}
	if !catalogHas(catalog, "express") || !catalogHas(catalog, "@tanstack/react-query") || !catalogHas(catalog, "drizzle-orm") {
		t.Fatal("Caltodo dependency catalog is incomplete")
	}
	if !hasUnresolvedCall(result, "fetch") || !pathHasExplicitSettingsBoundary(result) || !pathStorageAndResourceAuthorityIsHonest(result) {
		t.Fatalf("Caltodo unresolved/boundary authority drifted: %#v", result.ProductPaths)
	}
	if countPatchSettingsPaths(result) != 1 || !hasToolSurface(result, "server/integration/testServer.ts") {
		t.Fatalf("test server contaminated the product path/surface authority: %#v / %#v", result.ProductPaths, result.Surfaces)
	}
	assertEveryMethodHasExactTypeReceiver(t, index)
	if _, err := coremap.CompileProgram(result.Project.Name, repository, index, readmetargetscout.Result{}); err != nil {
		t.Fatalf("Caltodo ProgramIndex did not compile into CoreMap evidence: %v", err)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	if offset := bytes.Index(encoded, []byte(root)); offset >= 0 {
		start := offset - 120
		if start < 0 {
			start = 0
		}
		end := offset + len(root) + 120
		if end > len(encoded) {
			end = len(encoded)
		}
		context := strings.ReplaceAll(string(encoded[start:end]), root, "<repository>")
		t.Fatalf("artifact contains absolute analyzed root near %q", context)
	}
}

func assertEveryMethodHasExactTypeReceiver(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	for _, object := range index.Objects {
		if object.Kind != programindex.ObjectMethod {
			continue
		}
		owner, ok := objects[object.OwnerID]
		if !ok || owner.Kind != programindex.ObjectType {
			t.Fatalf("method has no exact type receiver: %#v / owner=%#v", object, owner)
		}
	}
}

func minimalResult(t *testing.T, language string) Result {
	t.Helper()
	sum := sha256.Sum256([]byte("export const value = 1\n"))
	file := File{FileRef: "f2", Path: "src/index.js", Language: language, Module: "src/index", SHA256: hex.EncodeToString(sum[:])}
	if language == "typescript" {
		file.Path = "src/index.ts"
	}
	result := Result{Version: Version, HelperVersion: HelperVersion, CorpusSHA256: strings.Repeat("a", 64), SourceSHA256: sourceDigest([]File{file}), Project: Project{Ref: "project:root-package", Name: "sample", PackagePath: "sample", Language: language, Selector: "jsts:package.json", ManifestPath: "package.json", ManifestFileRef: "f1", PackageManager: "npm", ModuleResolution: "node10", PathAliases: []PathAlias{}, Scripts: []Script{}, SourceRoots: []string{"src"}, EntryFileRefs: []string{}, ToolConfigs: []ProjectFile{}, Dependencies: []PackageDependency{}}, Files: []File{file}, Declarations: []Declaration{{Ref: "module:f2", Kind: "module", Name: "src/index", QualifiedName: "src/index", Location: file.location()}}, Imports: []Import{}, Exports: []Export{}, Calls: []Call{}, Surfaces: []Surface{}, Routes: []Route{}, HTTPUses: []HTTPUse{}, Contracts: []Contract{}, Resources: []Resource{}, ProductPaths: []ProductPath{}}
	targetID, err := deriveProgramTargetID(result)
	if err != nil {
		t.Fatal(err)
	}
	result.ProgramTargetID = targetID
	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func rederiveAndSeal(t *testing.T, result Result) Result {
	t.Helper()
	result.SHA256 = ""
	targetID, err := deriveProgramTargetID(result)
	if err != nil {
		t.Fatal(err)
	}
	result.ProgramTargetID = targetID
	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func (file File) location() Location {
	return Location{Path: file.Path, FileRef: file.FileRef, Line: 1, Column: 1}
}
func assertSurface(t *testing.T, result Result, kind SurfaceKind, role SurfaceRole) {
	t.Helper()
	for _, surface := range result.Surfaces {
		if surface.Kind == kind && surface.Role == role {
			return
		}
	}
	t.Fatalf("surface %s/%s missing", kind, role)
}
func hasRoute(result Result, kind RouteKind, method, path string) bool {
	for _, route := range result.Routes {
		if route.Kind == kind && route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
func routeForNamedRefs(result Result, kind RouteKind, method, routePath string, names map[string]string, middlewareName, handlerName string) *Route {
	for index := range result.Routes {
		if result.Routes[index].Kind == kind && result.Routes[index].Method == method && result.Routes[index].Path == routePath &&
			refsNameExactly(result.Routes[index].MiddlewareRefs, names, middlewareName) && refsNameExactly(result.Routes[index].HandlerRefs, names, handlerName) {
			return &result.Routes[index]
		}
	}
	return nil
}
func refsNameExactly(refs []string, names map[string]string, want string) bool {
	return len(refs) == 1 && names[refs[0]] == want
}
func hasHTTPUse(result Result, method, path string) bool {
	for _, value := range result.HTTPUses {
		if value.Method == method && value.Path == path {
			return true
		}
	}
	return false
}
func hasContract(result Result, kind, name string) bool {
	for _, value := range result.Contracts {
		if value.Kind == kind && value.Name == name {
			return true
		}
	}
	return false
}
func contractUsedWithin(result Result, kind, name, declarationRef string) bool {
	owners := map[string]string{}
	for _, declaration := range result.Declarations {
		owners[declaration.Ref] = declaration.OwnerRef
	}
	for _, value := range result.Contracts {
		if value.Kind == kind && value.Name == name {
			for _, ref := range value.UsedByRefs {
				for ref != "" {
					if ref == declarationRef {
						return true
					}
					ref = owners[ref]
				}
			}
		}
	}
	return false
}
func hasResource(result Result, kind string) bool {
	for _, value := range result.Resources {
		if value.Kind == kind {
			return true
		}
	}
	return false
}
func hasPathKinds(result Result, kinds []string) bool {
	seen := map[string]bool{}
	for _, value := range result.ProductPaths {
		for _, step := range value.Steps {
			seen[step.Kind] = true
		}
	}
	for _, kind := range kinds {
		if !seen[kind] {
			return false
		}
	}
	return true
}
func catalogHas(catalog dependencies.Catalog, name string) bool {
	for _, value := range catalog.Dependencies {
		if value.PackagePath == name {
			return true
		}
	}
	return false
}

func hasUnresolvedCall(result Result, expression string) bool {
	for _, call := range result.Calls {
		if call.Expression == expression && call.Resolution == "unresolved" {
			return true
		}
	}
	return false
}

func pathHasExplicitSettingsBoundary(result Result) bool {
	for _, productPath := range result.ProductPaths {
		if !strings.Contains(productPath.Name, "/settings") {
			continue
		}
		for _, step := range productPath.Steps {
			if step.Kind == "http_method_path_match" && step.Label == "PATCH /api/settings" &&
				step.Resolution == "exact" && step.Authority == "exact_static" && len(step.TargetRefs) == 1 {
				return true
			}
		}
	}
	return false
}

func pathStorageAndResourceAuthorityIsHonest(result Result) bool {
	storage, resource := false, false
	for _, productPath := range result.ProductPaths {
		if !strings.Contains(productPath.Name, "/settings") {
			continue
		}
		for _, step := range productPath.Steps {
			switch step.Kind {
			case "storage_call":
				storage = storage || step.Authority == "possible" || step.Authority == "unresolved_frontier"
			case "resource_boundary":
				resource = resource || (step.Resolution == "alternatives" && step.Authority == "possible")
			}
		}
	}
	return storage && resource
}

func countPatchSettingsPaths(result Result) int {
	count := 0
	for _, productPath := range result.ProductPaths {
		if productPath.Name == "/settings → PATCH /api/settings" {
			count++
		}
	}
	return count
}

func hasToolSurface(result Result, filePath string) bool {
	for _, surface := range result.Surfaces {
		if surface.Kind == SurfaceTool && surface.Role == SurfaceScript && surface.Location.Path == filePath {
			return true
		}
	}
	return false
}
