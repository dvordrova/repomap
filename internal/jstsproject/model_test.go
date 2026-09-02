package jstsproject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestSealedResultProjectsExactProgramIndexAndDependencies(t *testing.T) {
	result := minimalResult(t, "javascript")
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	index, catalog, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProgramIndex(result, index); err != nil {
		t.Fatal(err)
	}
	input, err := BuildInputFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	fromCommonSeam, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index, fromCommonSeam) {
		t.Fatal("common ProgramIndex sealing changed the JavaScript/TypeScript adapter projection")
	}
	fromDependencySeam, err := BuildDependenciesFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog, fromDependencySeam) {
		t.Fatal("common dependency projection changed the JavaScript/TypeScript adapter authority")
	}
	enriched, err := programindex.Enrich(index, strings.Repeat("d", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProgramIndex(result, enriched); err != nil {
		t.Fatalf("enriched structural projection: %v", err)
	}
	if index.Target.Kind != "library" || len(index.Target.Seeds) != 0 {
		t.Fatalf("tool/library target promoted: %#v", index.Target)
	}
	if catalog.Coverage.State != "complete" {
		t.Fatalf("catalog coverage = %q", catalog.Coverage.State)
	}

	tampered := index
	tampered.Coverage.ObjectsObserved++
	if err := ValidateProgramIndex(result, tampered); err == nil {
		t.Fatal("tampered ProgramIndex was accepted")
	}
}

func TestFormerResultSizeIsWarningOnly(t *testing.T) {
	warnings := scaleWarningsForResultBytes(AdvisoryResultBytes + 1)
	if len(warnings) != 1 || warnings[0].Kind != ScaleWarningResultBytes ||
		warnings[0].Retained != AdvisoryResultBytes+1 ||
		warnings[0].AdvisorySize != AdvisoryResultBytes {
		t.Fatalf("warnings = %#v", warnings)
	}
	if warnings := scaleWarningsForResultBytes(AdvisoryResultBytes); len(warnings) != 0 {
		t.Fatalf("threshold warning = %#v", warnings)
	}
}

func TestValidateRejectsPreviousJSTSResultVersion(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.Version = Version - 1
	if err := result.Validate(); err == nil || !strings.Contains(err.Error(), "invalid producer identity") {
		t.Fatalf("previous JSTS result version error = %v", err)
	}
}

func TestPackageExportIdentityJoinsTypeScriptAndJavaScriptShardsWithoutChangingResolution(t *testing.T) {
	destination := minimalResult(t, "typescript").Snapshot()
	destination.Project.Name = "shared"
	destination.Project.PackagePath = "shared"
	serve := Declaration{
		Ref: "decl:f2:2:1:function:serve", Kind: "function", Name: "serve",
		QualifiedName: "src/index#serve", Exported: true,
		Location: Location{Path: destination.Files[0].Path, FileRef: destination.Files[0].FileRef, Line: 2, Column: 1},
	}
	destination.Declarations = append(destination.Declarations, serve)
	destination.Exports = append(destination.Exports, Export{
		Ref: "export:" + serve.Ref, Kind: "declaration", Name: "serve",
		DeclarationRef: serve.Ref, Resolution: "exact", Location: serve.Location,
	})
	destination = rederiveAndSeal(t, destination)
	destinationIndex, _, err := BuildFromResult(destination)
	if err != nil {
		t.Fatal(err)
	}
	destinationIdentity := identityForSourceRef(t, destinationIndex, serve.Ref, "shared#serve")

	for _, test := range []struct {
		language   string
		resolution string
		want       programindex.Resolution
	}{
		{language: "typescript", resolution: "exact", want: programindex.ResolutionExact},
		{language: "javascript", resolution: "alternatives", want: programindex.ResolutionAlternatives},
	} {
		t.Run(test.language, func(t *testing.T) {
			caller := minimalResult(t, test.language).Snapshot()
			callable := Declaration{
				Ref: "decl:f2:2:1:function:caller", Kind: "function", Name: "caller",
				QualifiedName: "src/index#caller", Exported: true,
				Location: Location{Path: caller.Files[0].Path, FileRef: caller.Files[0].FileRef, Line: 2, Column: 1},
			}
			caller.Declarations = append(caller.Declarations, callable)
			caller.Calls = append(caller.Calls, Call{
				Ref: "call:f2:3:1:serve", CallerRef: callable.Ref, Invocation: "call",
				ExternalPackage: "shared", ExternalExport: "serve", ExternalName: "serve",
				Expression: "serve", Resolution: test.resolution,
				PatternsObserved: 1,
				Location:         Location{Path: caller.Files[0].Path, FileRef: caller.Files[0].FileRef, Line: 3, Column: 1},
			})
			caller = rederiveAndSeal(t, caller)
			index, _, err := BuildFromResult(caller)
			if err != nil {
				t.Fatal(err)
			}
			var relation programindex.Relation
			for _, candidate := range index.Relations {
				if candidate.SourceRef == "program:call:f2:3:1:serve" {
					relation = candidate
					break
				}
			}
			if relation.Resolution != test.want || len(relation.ToIDs) != 1 {
				t.Fatalf("external relation = %#v, want %s", relation, test.want)
			}
			identity := identityForObjectID(t, index, relation.ToIDs[0], "shared#serve")
			if identity.Domain != destinationIdentity.Domain || identity.Key != destinationIdentity.Key {
				t.Fatalf("cross-shard identity = %#v, destination %#v", identity, destinationIdentity)
			}
		})
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

func TestProgramIndexExternalAuthorityPreservesRawLanguageIdentity(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	caller := Declaration{
		Ref: "decl:f2:2:1:function:caller", Kind: "function", Name: "caller", Exported: true,
		Location: Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 2, Column: 1},
	}
	result.Declarations = append(result.Declarations, caller)
	tests := []struct {
		ref         string
		packagePath string
		name        string
		want        programindex.ExternalAuthorityKind
	}{
		{ref: "platform", packagePath: javascriptPlatform, name: "fetch", want: programindex.ExternalAuthorityPlatform},
		{ref: "node-prefixed", packagePath: "node:crypto", name: "createHash", want: programindex.ExternalAuthorityPlatform},
		{ref: "node-bare", packagePath: "fs", name: "readFile", want: programindex.ExternalAuthorityPlatform},
		{ref: "npm", packagePath: "axios", name: "get", want: programindex.ExternalAuthorityPackage},
	}
	for position, test := range tests {
		externalExport := test.name
		if test.packagePath == javascriptPlatform {
			externalExport = ""
		}
		result.Calls = append(result.Calls, Call{
			Ref: "call:" + test.ref, CallerRef: caller.Ref, Invocation: "call",
			ExternalPackage: test.packagePath, ExternalExport: externalExport, ExternalName: test.name,
			Expression: test.name, Resolution: "exact", PatternsObserved: 1,
			Location: Location{
				Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 3 + position, Column: 1,
			},
		})
	}
	result = rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}
	for _, test := range tests {
		var relation programindex.Relation
		for _, candidate := range index.Relations {
			if candidate.SourceRef == "program:call:"+test.ref {
				relation = candidate
				break
			}
		}
		if len(relation.ToIDs) != 1 {
			t.Fatalf("%s relation = %#v", test.ref, relation)
		}
		external := objectsByID[relation.ToIDs[0]].External
		if external == nil || external.AuthorityKind != test.want || external.PackagePath != test.packagePath || external.Name != test.name {
			t.Fatalf("%s external authority = %#v, want kind=%q raw package=%q", test.ref, external, test.want, test.packagePath)
		}
	}
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

func TestScriptSurfaceRetainsEveryExactEntryFile(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	second := File{
		FileRef: "f4", Path: "src/worker.ts", Language: "typescript", Module: "src/worker",
		SHA256: strings.Repeat("d", 64),
	}
	result.Files = append(result.Files, second)
	result.Declarations = append(result.Declarations, Declaration{
		Ref: "module:f4", Kind: "module", Name: "src/worker", QualifiedName: "src/worker",
		Location: second.location(),
	})
	result.SourceSHA256 = sourceDigest(result.Files)
	result.Project.Scripts = []Script{{
		Name: "build:all", Kind: "build", EntryFileRefs: []string{"f4", "f2"},
	}}
	addScriptSurfaces(&result)
	sealed := rederiveAndSeal(t, result)
	if len(sealed.Surfaces) != 1 {
		t.Fatalf("script surfaces = %#v", sealed.Surfaces)
	}
	surface := sealed.Surfaces[0]
	if !reflect.DeepEqual(surface.EntryRefs, []string{"module:f2", "module:f4"}) ||
		surface.Location.Path != "src/index.ts" || surface.Location.FileRef != "f2" {
		t.Fatalf("complete script surface = %#v", surface)
	}
	index, _, err := BuildFromResult(sealed)
	if err != nil {
		t.Fatal(err)
	}
	entrySources := map[string]bool{}
	for _, object := range index.Objects {
		if object.SourceRef == "module:f2" || object.SourceRef == "module:f4" {
			entrySources[object.SourceRef] = true
		}
	}
	if !entrySources["module:f2"] || !entrySources["module:f4"] {
		t.Fatalf("ProgramIndex lost script entry refs: %#v", entrySources)
	}
}

func TestPackageBinaryCommandPastFormerLocalLimitRemainsExactMetadata(t *testing.T) {
	const formerCommandLimit = 240
	command := strings.Repeat("a", formerCommandLimit+1)
	if !validPackageBinaryCommand(command) {
		t.Fatal("safe package binary command was rejected only because it passed the former local length cap")
	}
	if validPackageBinaryCommand(string([]byte{'a', 0xff, 'b'})) {
		t.Fatal("invalid UTF-8 package binary command was accepted")
	}

	result := minimalResult(t, "typescript").Snapshot()
	result.Project.Binaries = []PackageBinary{{Command: command, Path: "bin/sample", FileRef: "f3"}}
	addPackageBinarySurfaces(&result)
	sealed := rederiveAndSeal(t, result)
	if len(sealed.Project.Binaries) != 1 || sealed.Project.Binaries[0].Command != command {
		t.Fatalf("sealed package binary command lost exact metadata: %#v", sealed.Project.Binaries)
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
		PatternsObserved: 1,
		Location:         result.Files[0].location(),
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
	if err := sealed.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildFromResult(sealed); err != nil {
		t.Fatal(err)
	}
}

func TestSealOmitsActualToFormalCandidateWhenItsSourceCallIsRedacted(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	location := result.Files[0].location()
	callee := Declaration{
		Ref: "decl:f2:2:1:function:startServer", Kind: "function", Name: "startServer",
		QualifiedName: "src/index#startServer", Location: location,
	}
	caller := Declaration{
		Ref: "decl:f2:3:1:function:bootstrap", Kind: "function", Name: "bootstrap",
		QualifiedName: "src/index#bootstrap", Location: location,
	}
	result.Declarations = append(result.Declarations, callee, caller)
	const sensitive = "sk-secret-shaped-provider-output"
	sourceCall := Call{
		Ref: "call:sensitive-actual", CallerRef: caller.Ref, CalleeRefs: []string{callee.Ref},
		Invocation: "call", Expression: `startServer("` + sensitive + `")`, Resolution: "exact",
		Location: location, PatternsObserved: 1,
		Pattern: &CallPattern{
			Selector: "startServer", ReceiverOriginRefs: []string{}, ArgumentsObserved: 1,
			Arguments: []CallPatternArgument{{
				Position: 1, Kind: "literal_string", Value: sensitive, Parts: []CallPatternPart{},
				ObjectRefs: []string{}, ValueCandidates: []CallPatternValueCandidate{},
			}},
		},
	}
	destinationCall := Call{
		Ref: "call:dynamic-formal-use", CallerRef: callee.Ref, CalleeRefs: []string{},
		Invocation: "call", Expression: "app.get", Resolution: "unresolved",
		Location: location, PatternsObserved: 1,
		Pattern: &CallPattern{
			Selector: "get", ReceiverOriginRefs: []string{}, ArgumentsObserved: 1,
			Arguments: []CallPatternArgument{{
				Position: 1, Kind: "dynamic", Parts: []CallPatternPart{}, ObjectRefs: []string{},
				ValueCandidatesObserved: 1,
				ValueCandidates: []CallPatternValueCandidate{{
					Kind: "literal_string", Value: sensitive, Parts: []CallPatternPart{}, Resolution: "possible",
					SourceKind: "actual_argument", SourceCallRef: sourceCall.Ref, SourcePosition: 1,
				}},
			}},
		},
	}
	result.Calls = []Call{sourceCall, destinationCall}
	result.ProgramTargetID = ""
	result.SHA256 = ""
	if err := bindProgramTargetIdentity(&result); err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range sealed.Calls {
		switch call.Ref {
		case sourceCall.Ref:
			if call.Expression != redactedExpression || call.Pattern != nil {
				t.Fatalf("sensitive actual source was retained: %#v", call)
			}
		case destinationCall.Ref:
			if call.Pattern == nil || len(call.Pattern.Arguments) != 1 ||
				call.Pattern.Arguments[0].ValueCandidatesObserved != 0 ||
				len(call.Pattern.Arguments[0].ValueCandidates) != 0 {
				t.Fatalf("candidate survived redacted source: %#v", call)
			}
		}
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
	if err := sealed.Validate(); err != nil {
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
			if object.Signature != "" || object.Name != declaration.Name {
				t.Fatalf("sibling-type declaration projection = %#v", object)
			}
		}
	}
	if !projected {
		t.Fatalf("sibling-type declaration was not projected: %#v", index.Objects)
	}
}

func TestProgramIndexUsesPathFreeDisplayNamesWithoutMergingSameNamedDeclarations(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	secondDigest := sha256.Sum256([]byte("export const settings = 2\n"))
	secondFile := File{
		FileRef: "f3", Path: "src/other.ts", Language: "typescript", Module: "src/other",
		SHA256: hex.EncodeToString(secondDigest[:]),
	}
	result.Files = append(result.Files, secondFile)
	result.SourceSHA256 = sourceDigest(result.Files)
	result.Declarations = append(result.Declarations,
		Declaration{
			Ref: "decl:f2:2:1:function:settings", Kind: "function", Name: "settings",
			QualifiedName: "src/index#settings",
			Location:      Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 2, Column: 1},
		},
		Declaration{
			Ref: "module:f3", Kind: "module", Name: "src/other", QualifiedName: "src/other",
			Location: secondFile.location(),
		},
		Declaration{
			Ref: "decl:f3:2:1:function:settings", Kind: "function", Name: "settings",
			QualifiedName: "src/other#settings",
			Location:      Location{Path: secondFile.Path, FileRef: secondFile.FileRef, Line: 2, Column: 1},
		},
		Declaration{
			Ref: "decl:f2:4:1:type:SimulationField", Kind: "type", Name: "SimulationField",
			QualifiedName: "src/index#SimulationField",
			Location:      Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 4, Column: 1},
		},
		Declaration{
			Ref: "decl:f2:5:3:method:animate", Kind: "method", Name: "animate",
			QualifiedName: "src/index#SimulationField.animate", OwnerRef: "decl:f2:4:1:type:SimulationField",
			Location: Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 5, Column: 3},
		},
	)
	result = rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}

	objectsByRef := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByRef[object.SourceRef] = object
	}
	left := objectsByRef["decl:f2:2:1:function:settings"]
	right := objectsByRef["decl:f3:2:1:function:settings"]
	if left.Name != "settings" || right.Name != "settings" {
		t.Fatalf("same-name display projection = %q / %q", left.Name, right.Name)
	}
	if left.ID == right.ID || left.SourceRef == right.SourceRef || left.Location == nil || right.Location == nil || left.Location.Path == right.Location.Path {
		t.Fatalf("same-name declaration identities or locations were merged: left=%#v right=%#v", left, right)
	}
	method := objectsByRef["decl:f2:5:3:method:animate"]
	if method.Name != "SimulationField.animate" || strings.Contains(method.Name, "src/") || strings.Contains(method.Name, "#") {
		t.Fatalf("owned declaration display name = %q", method.Name)
	}
	module := objectsByRef["module:f2"]
	if module.Name != "src/index" {
		t.Fatalf("logical module name = %q", module.Name)
	}
}

func TestQualifiedNameIsOptionalDebugMetadata(t *testing.T) {
	result := minimalResult(t, "typescript").Snapshot()
	result.Declarations[0].QualifiedName = ""
	result.Declarations = append(result.Declarations,
		Declaration{
			Ref: "decl:f2:2:1:type:SimulationField", Kind: "type", Name: "SimulationField",
			Location: Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 2, Column: 1},
		},
		Declaration{
			Ref: "decl:f2:3:3:method:animate", Kind: "method", Name: "animate",
			OwnerRef: "decl:f2:2:1:type:SimulationField",
			Location: Location{Path: result.Files[0].Path, FileRef: result.Files[0].FileRef, Line: 3, Column: 3},
		},
	)
	result = rederiveAndSeal(t, result)
	index, _, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	objectsByRef := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByRef[object.SourceRef] = object
	}
	method := objectsByRef["decl:f2:3:3:method:animate"]
	if method.Name != "SimulationField.animate" || method.Location == nil ||
		method.Location.Path != result.Files[0].Path || method.ID == "" {
		t.Fatalf("ProgramIndex projection without qualified_name = %#v", method)
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
	result := Result{Version: Version, HelperVersion: HelperVersion, CorpusSHA256: strings.Repeat("a", 64), SourceSHA256: sourceDigest([]File{file}), Project: Project{Ref: "project:root-package", Name: "sample", PackagePath: "sample", Language: language, Selector: "jsts:package.json", ManifestPath: "package.json", ManifestFileRef: "f1", PackageManager: "npm", ModuleResolution: "node10", PathAliases: []PathAlias{}, Scripts: []Script{}, SourceRoots: []string{"src"}, EntryFileRefs: []string{}, ToolConfigs: []ProjectFile{}, Dependencies: []PackageDependency{}}, Files: []File{file}, Declarations: []Declaration{{Ref: "module:f2", Kind: "module", Name: "src/index", QualifiedName: "src/index", Location: file.location()}}, Imports: []Import{}, Exports: []Export{}, Calls: []Call{}, Surfaces: []Surface{}, Contracts: []Contract{}}
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

func identityForSourceRef(
	t *testing.T,
	index programindex.Index,
	sourceRef string,
	display string,
) programindex.SymbolLinkIdentity {
	t.Helper()
	for _, object := range index.Objects {
		if object.SourceRef == sourceRef {
			return identityForObject(t, object, display)
		}
	}
	t.Fatalf("object source ref %q not found", sourceRef)
	return programindex.SymbolLinkIdentity{}
}

func identityForObjectID(
	t *testing.T,
	index programindex.Index,
	objectID string,
	display string,
) programindex.SymbolLinkIdentity {
	t.Helper()
	for _, object := range index.Objects {
		if object.ID == objectID {
			return identityForObject(t, object, display)
		}
	}
	t.Fatalf("object id %q not found", objectID)
	return programindex.SymbolLinkIdentity{}
}

func identityForObject(t *testing.T, object programindex.Object, display string) programindex.SymbolLinkIdentity {
	t.Helper()
	for _, identity := range object.SymbolLinkIdentities {
		if identity.Domain == "jsts_package_export_v1" && identity.Display == display {
			return identity
		}
	}
	t.Fatalf("object %q lacks package export identity %q: %#v", object.Name, display, object.SymbolLinkIdentities)
	return programindex.SymbolLinkIdentity{}
}

func (file File) location() Location {
	return Location{Path: file.Path, FileRef: file.FileRef, Line: 1, Column: 1}
}
