package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestJSTSReportViewsPreserveSurfaceDistinctionsAndExplicitHTTPBoundary(t *testing.T) {
	result, index := reportJSTSProjectFixture(t)
	surfaces, err := NewJSTSSurfaceCatalogView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := NewCrossSurfacePathView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.ValidateSurfaceJoins(surfaces); err != nil {
		t.Fatalf("surface joins: %v", err)
	}

	wantDispositions := map[jstsproject.SurfaceKind]JSTSSurfaceDisposition{
		jstsproject.SurfaceBrowser: JSTSSurfaceProduct,
		jstsproject.SurfaceServer:  JSTSSurfaceProduct,
		jstsproject.SurfaceShared:  JSTSSurfaceSupporting,
		jstsproject.SurfaceTool:    JSTSSurfaceTool,
		jstsproject.SurfaceUnknown: JSTSSurfaceUnknown,
	}
	if len(surfaces.Surfaces) != len(wantDispositions) {
		t.Fatalf("surfaces = %#v", surfaces.Surfaces)
	}
	for _, surface := range surfaces.Surfaces {
		if surface.Disposition != wantDispositions[surface.Kind] {
			t.Fatalf("surface %q disposition = %q", surface.SurfaceID, surface.Disposition)
		}
	}

	seenSameName := map[string]bool{}
	for _, fact := range surfaces.Facts {
		if fact.Ref == "decl:client:settings" || fact.Ref == "decl:server:settings" {
			seenSameName[fact.Ref] = true
		}
	}
	if len(seenSameName) != 2 {
		t.Fatalf("same-name declarations merged or omitted: %#v", surfaces.Facts)
	}

	if len(paths.Paths) != 1 || len(paths.Paths[0].Steps) != 11 {
		t.Fatalf("complete Caltodo-shaped path = %#v", paths.Paths)
	}
	match := paths.Paths[0].Steps[3]
	if match.Kind != CrossSurfaceHTTPMethodPathMatch ||
		match.Authority != CrossSurfaceExactStatic ||
		match.Resolution != programindex.ResolutionExact {
		t.Fatalf("HTTP compatibility boundary was promoted or weakened: %#v", match)
	}
	if paths.Coverage.PathsProjected != 1 || paths.Coverage.StepsProjected != 11 ||
		paths.Coverage.ExactStaticSteps == 0 || paths.Coverage.ResolvedIndirectSteps == 0 {
		t.Fatalf("path coverage = %#v", paths.Coverage)
	}
	if err := surfaces.ValidateAgainst(result, index); err != nil {
		t.Fatalf("surface projection authority: %v", err)
	}
	previousSurfaceContract := *surfaces
	previousSurfaceContract.Version--
	if err := previousSurfaceContract.Validate(); err == nil || !strings.Contains(err.Error(), "invalid identity or collection shape") {
		t.Fatalf("previous surface catalog version error = %v", err)
	}
	if err := paths.ValidateAgainst(result, index); err != nil {
		t.Fatalf("path projection authority: %v", err)
	}
}

func TestJSTSSurfaceCatalogClosedKindAcceptsCLIProduct(t *testing.T) {
	if !validJSTSSurfaceKind(jstsproject.SurfaceCLI) {
		t.Fatal("command-line application is absent from the closed JSTS surface vocabulary")
	}
	if disposition := jstsSurfaceDisposition(jstsproject.SurfaceProduct); disposition != JSTSSurfaceProduct {
		t.Fatalf("CLI product disposition = %q", disposition)
	}
}

func TestCrossSurfacePathViewRejectsAuthorityDriftAndMissingSurfaceCitation(t *testing.T) {
	result, index := reportJSTSProjectFixture(t)
	surfaces, err := NewJSTSSurfaceCatalogView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := NewCrossSurfacePathView(result, index)
	if err != nil {
		t.Fatal(err)
	}

	authorityDrift := cloneCrossSurfacePathView(t, *paths)
	authorityDrift.Paths[0].Steps[0].Authority = CrossSurfacePossible
	if err := authorityDrift.Validate(); err == nil || !strings.Contains(err.Error(), "step 0 is invalid") {
		t.Fatalf("authority/resolution drift error = %v", err)
	}

	withoutSurface := cloneCrossSurfacePathView(t, *paths)
	for pathIndex := range withoutSurface.Paths {
		for stepIndex := range withoutSurface.Paths[pathIndex].Steps {
			step := &withoutSurface.Paths[pathIndex].Steps[stepIndex]
			if step.SourceRef == "surface:browser" {
				step.SourceRef = "decl:client:settings"
			}
			for targetIndex := range step.TargetRefs {
				switch step.TargetRefs[targetIndex] {
				case "surface:browser":
					step.TargetRefs[targetIndex] = "decl:client:settings"
				}
			}
			sort.Strings(step.TargetRefs)
		}
	}
	filtered := withoutSurface.Facts[:0]
	for _, fact := range withoutSurface.Facts {
		if fact.Ref != "surface:browser" {
			filtered = append(filtered, fact)
		}
	}
	withoutSurface.Facts = filtered
	if err := withoutSurface.Validate(); err != nil {
		t.Fatalf("standalone path shape after removing surface citations: %v", err)
	}
	if err := withoutSurface.ValidateSurfaceJoins(surfaces); err == nil ||
		!strings.Contains(err.Error(), "one exact browser product surface and one exact server product surface") {
		t.Fatalf("missing surface citation error = %v", err)
	}
}

func cloneCrossSurfacePathView(t *testing.T, value CrossSurfacePathView) CrossSurfacePathView {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result CrossSurfacePathView
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func reportJSTSProjectFixture(t *testing.T) (jstsproject.Result, programindex.Index) {
	t.Helper()
	const corpusSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	location := func(fileRef, path string, line int) jstsproject.Location {
		return jstsproject.Location{Path: path, FileRef: fileRef, Line: line, Column: 1}
	}
	files := []jstsproject.File{
		{FileRef: "f5", Path: "server/auth.ts", Language: "typescript", Module: "server/auth"},
		{FileRef: "f2", Path: "client/src/pages/settings.tsx", Language: "typescript", Module: "client/settings"},
		{FileRef: "f6", Path: "server/handlers/settings.ts", Language: "typescript", Module: "server/settings"},
		{FileRef: "f1", Path: "client/src/lib/queryClient.ts", Language: "typescript", Module: "client/queryClient"},
		{FileRef: "f7", Path: "server/routes.ts", Language: "typescript", Module: "server/routes"},
		{FileRef: "f9", Path: "shared/schema.ts", Language: "typescript", Module: "shared/schema"},
		{FileRef: "f8", Path: "server/storage.ts", Language: "typescript", Module: "server/storage"},
		{FileRef: "f4", Path: "scripts/seed.ts", Language: "typescript", Module: "scripts/seed"},
	}
	for position := range files {
		files[position].SHA256 = strings.Repeat(string("0123456789abcdef"[position%16]), 64)
	}
	sourceSHA256 := reportJSTSSourceDigest(files)
	declarations := []jstsproject.Declaration{}
	for _, file := range files {
		declarations = append(declarations, jstsproject.Declaration{
			Ref: "module:" + file.FileRef, Kind: "module", Name: file.Module,
			QualifiedName: file.Module, Exported: true,
			Location: location(file.FileRef, file.Path, 1),
		})
	}
	declarations = append(declarations,
		jstsproject.Declaration{Ref: "decl:client:settings", Kind: "function", Name: "settings", QualifiedName: "client.settings", Exported: true, Location: location("f2", "client/src/pages/settings.tsx", 10)},
		jstsproject.Declaration{Ref: "decl:mutation", Kind: "function", Name: "saveSettings", QualifiedName: "client.saveSettings", Location: location("f2", "client/src/pages/settings.tsx", 24)},
		jstsproject.Declaration{Ref: "decl:api-request", Kind: "function", Name: "apiRequest", QualifiedName: "client.apiRequest", Exported: true, Location: location("f1", "client/src/lib/queryClient.ts", 8)},
		jstsproject.Declaration{Ref: "decl:require-auth", Kind: "function", Name: "requireAuth", QualifiedName: "server.requireAuth", Exported: true, Location: location("f5", "server/auth.ts", 12)},
		jstsproject.Declaration{Ref: "decl:factory", Kind: "function", Name: "settingsHandler", QualifiedName: "server.settingsHandler", Location: location("f6", "server/handlers/settings.ts", 14)},
		jstsproject.Declaration{Ref: "decl:server:settings", Kind: "function", Name: "settings", QualifiedName: "server.settings", Location: location("f6", "server/handlers/settings.ts", 22)},
		jstsproject.Declaration{Ref: "decl:storage", Kind: "function", Name: "saveSettings", QualifiedName: "server.storage.saveSettings", Location: location("f8", "server/storage.ts", 30)},
	)
	result := jstsproject.Result{
		Version: jstsproject.Version, HelperVersion: jstsproject.HelperVersion,
		CorpusSHA256: corpusSHA256, SourceSHA256: sourceSHA256,
		ProgramTargetID: "program-target-provisional",
		Project: jstsproject.Project{
			Ref: "project:root", Name: "caltodo", PackagePath: "caltodo",
			Language: "typescript", Selector: "jsts:package.json",
			ManifestPath: "package.json", ManifestFileRef: "f3",
			ConfigPath: "tsconfig.json", ConfigFileRef: "f10",
			ModuleResolution: "bundler", SourceRoots: []string{"client", "scripts", "server", "shared"},
		},
		Files: files, Declarations: declarations,
		Surfaces: []jstsproject.Surface{
			{Ref: "surface:browser", Kind: jstsproject.SurfaceBrowser, Role: jstsproject.SurfaceProduct, Name: "Browser application", EntryRefs: []string{}, EvidenceRefs: []string{"decl:client:settings"}, Location: location("f2", "client/src/pages/settings.tsx", 1)},
			{Ref: "surface:server", Kind: jstsproject.SurfaceServer, Role: jstsproject.SurfaceProduct, Name: "Node server", EntryRefs: []string{}, EvidenceRefs: []string{"decl:server:settings"}, Location: location("f7", "server/routes.ts", 1)},
			{Ref: "surface:shared", Kind: jstsproject.SurfaceShared, Role: jstsproject.SurfaceSupporting, Name: "Shared contracts", EntryRefs: []string{}, EvidenceRefs: []string{"contract:settings"}, Location: location("f9", "shared/schema.ts", 1)},
			{Ref: "surface:tool", Kind: jstsproject.SurfaceTool, Role: jstsproject.SurfaceScript, Name: "Seed tool", EntryRefs: []string{}, EvidenceRefs: []string{"f4"}, Location: location("f4", "scripts/seed.ts", 1)},
			{Ref: "surface:unknown", Kind: jstsproject.SurfaceUnknown, Role: jstsproject.SurfaceUnclassified, Name: "Unclassified surface", EntryRefs: []string{}, EvidenceRefs: []string{"contract:settings"}, Location: location("f9", "shared/schema.ts", 4)},
		},
		Routes: []jstsproject.Route{{
			Ref: "route:patch-settings", Kind: jstsproject.RouteHTTP, Method: "PATCH", Path: "/api/settings",
			OwnerRef: "module:f7", MiddlewareRefs: []string{"decl:require-auth"}, HandlerRefs: []string{"decl:factory", "decl:server:settings"},
			Resolution: "exact", Location: location("f7", "server/routes.ts", 40),
		}},
		HTTPUses: []jstsproject.HTTPUse{{
			Ref: "http:patch-settings", Kind: "api_request", Method: "PATCH", Path: "/api/settings",
			CallerRef: "decl:mutation", QueryKeys: []string{}, Resolution: "exact",
			Location: location("f2", "client/src/pages/settings.tsx", 28),
		}},
		Contracts: []jstsproject.Contract{{
			Ref: "contract:settings", Kind: "zod_schema", Name: "settingsSchema",
			UsedByRefs: []string{"decl:server:settings"}, Location: location("f9", "shared/schema.ts", 12),
		}},
		Resources: []jstsproject.Resource{{
			Ref: "resource:user-settings", Kind: "drizzle_table", Name: "userSettings", PackagePath: "drizzle-orm",
			UsedByRefs: []string{"decl:storage"}, EvidenceRefs: []string{"contract:settings"},
			Location: location("f9", "shared/schema.ts", 28),
		}},
		ProductPaths: []jstsproject.ProductPath{{
			Ref: "path:settings-update", Name: "Update settings", Outcome: "Persist validated user settings.",
			Steps: []jstsproject.PathStep{
				{Ordinal: 1, Kind: "page_route", Label: "/settings route renders SettingsPage", SourceRef: "surface:browser", TargetRefs: []string{"decl:client:settings"}, Resolution: "exact", Authority: "exact_static", Location: location("f2", "client/src/pages/settings.tsx", 4)},
				{Ordinal: 2, Kind: "mutation_site", Label: "SettingsPage binds the mutation callback", SourceRef: "decl:client:settings", TargetRefs: []string{"decl:mutation"}, Resolution: "exact", Authority: "resolved_indirect", Location: location("f2", "client/src/pages/settings.tsx", 22)},
				{Ordinal: 3, Kind: "client_http_use", Label: "apiRequest sends PATCH /api/settings", SourceRef: "decl:mutation", TargetRefs: []string{"http:patch-settings"}, Resolution: "exact", Authority: "exact_static", Location: location("f2", "client/src/pages/settings.tsx", 28)},
				{Ordinal: 4, Kind: "http_method_path_match", Label: "PATCH /api/settings matches the Express route", SourceRef: "http:patch-settings", TargetRefs: []string{"route:patch-settings"}, Resolution: "exact", Authority: "exact_static", Location: location("f7", "server/routes.ts", 40)},
				{Ordinal: 5, Kind: "server_route", Label: "Express registration enters the Node server", SourceRef: "route:patch-settings", TargetRefs: []string{"surface:server"}, Resolution: "exact", Authority: "exact_static", Location: location("f7", "server/routes.ts", 40)},
				{Ordinal: 6, Kind: "middleware", Label: "requireAuth guards the route", SourceRef: "surface:server", TargetRefs: []string{"decl:require-auth"}, Resolution: "exact", Authority: "exact_static", Location: location("f7", "server/routes.ts", 40)},
				{Ordinal: 7, Kind: "handler_factory", Label: "The registration calls the handler factory", SourceRef: "decl:require-auth", TargetRefs: []string{"decl:factory"}, Resolution: "exact", Authority: "resolved_indirect", Location: location("f7", "server/routes.ts", 41)},
				{Ordinal: 8, Kind: "handler", Label: "The factory-returned handler receives the request", SourceRef: "decl:factory", TargetRefs: []string{"decl:server:settings"}, Resolution: "exact", Authority: "resolved_indirect", Location: location("f6", "server/handlers/settings.ts", 22)},
				{Ordinal: 9, Kind: "contract_validation", Label: "Zod validates the settings contract", SourceRef: "decl:server:settings", TargetRefs: []string{"contract:settings"}, Resolution: "exact", Authority: "exact_static", Location: location("f6", "server/handlers/settings.ts", 26)},
				{Ordinal: 10, Kind: "storage_call", Label: "The handler delegates to storage", SourceRef: "contract:settings", TargetRefs: []string{"decl:storage"}, Resolution: "exact", Authority: "resolved_indirect", Location: location("f6", "server/handlers/settings.ts", 31)},
				{Ordinal: 11, Kind: "resource_boundary", Label: "Drizzle persists userSettings", SourceRef: "decl:storage", TargetRefs: []string{"resource:user-settings"}, Resolution: "exact", Authority: "exact_static", Location: location("f8", "server/storage.ts", 34)},
			},
		}},
	}

	sources := []programindex.TargetSource{
		{FileRef: "f2", Path: "client/src/pages/settings.tsx"},
		{FileRef: "f1", Path: "client/src/lib/queryClient.ts"},
		{FileRef: "f3", Path: "package.json"},
		{FileRef: "f5", Path: "server/auth.ts"},
		{FileRef: "f6", Path: "server/handlers/settings.ts"},
		{FileRef: "f7", Path: "server/routes.ts"},
		{FileRef: "f8", Path: "server/storage.ts"},
		{FileRef: "f4", Path: "scripts/seed.ts"},
		{FileRef: "f9", Path: "shared/schema.ts"},
		{FileRef: "f10", Path: "tsconfig.json"},
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Path != sources[j].Path {
			return sources[i].Path < sources[j].Path
		}
		return sources[i].FileRef < sources[j].FileRef
	})
	provisional, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: sourceSHA256,
		Target: programindex.TargetInput{
			Language: "typescript", Kind: "application", Name: "caltodo",
			Selector: "jsts:package.json", Sources: sources, AnchorFileRef: "f3",
		},
		Objects: []programindex.ObjectInput{}, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	result.ProgramTargetID = provisional.Target.ID
	result, err = jstsproject.Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	index, _, err := jstsproject.BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	return result, index
}

func reportJSTSSourceDigest(files []jstsproject.File) string {
	ordered := append([]jstsproject.File(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	digest := sha256.New()
	for _, file := range ordered {
		for _, field := range []string{file.Path, file.FileRef, file.SHA256} {
			_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
			_, _ = digest.Write([]byte{0})
			_, _ = digest.Write([]byte(field))
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}
