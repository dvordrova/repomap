package programindex

import (
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestNewRejectsUnmeasuredAdapterCoverage(t *testing.T) {
	input := shapeInput()
	if _, err := New(input); err == nil || !strings.Contains(err.Error(), "coverage was not measured") {
		t.Fatalf("unmeasured coverage error = %v", err)
	}
}

func TestNewRejectsUnmeasuredObjectVisibility(t *testing.T) {
	input := representativeInput()
	input.Objects[0].Visibility = ""
	_, err := New(input)
	if err == nil || !strings.Contains(err.Error(), "visibility") {
		t.Fatalf("New error = %v, want explicit visibility rejection", err)
	}
}

func TestExternalAuthorityKindIsRequiredClosedRawAndSealed(t *testing.T) {
	input := shapeInput()
	packagePosition := len(input.Objects)
	input.Objects = append(input.Objects,
		ObjectInput{
			SourceRef: "package-external", Kind: ObjectExternalSymbol,
			Name: "misleading.package.call", Visibility: VisibilityPublic,
			External: &ExternalSymbol{
				AuthorityKind: ExternalAuthorityPackage,
				PackagePath:   "platform:raw-package-identity", Name: "call",
			},
		},
		ObjectInput{
			SourceRef: "platform-external", Kind: ObjectExternalSymbol,
			Name: "runtime.schedule", Visibility: VisibilityPublic,
			External: &ExternalSymbol{
				AuthorityKind: ExternalAuthorityPlatform,
				PackagePath:   "raw-runtime-identity", Name: "schedule",
			},
		},
	)
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	packageObject := objectWithSourceRef(t, index, "package-external")
	platformObject := objectWithSourceRef(t, index, "platform-external")
	if packageObject.External == nil ||
		packageObject.External.PackagePath != "platform:raw-package-identity" ||
		!IsExternalPackageAuthority(packageObject.External) ||
		IsExternalPlatformAuthority(packageObject.External) {
		t.Fatalf("package authority = %#v", packageObject.External)
	}
	if platformObject.External == nil ||
		platformObject.External.PackagePath != "raw-runtime-identity" ||
		!IsExternalPlatformAuthority(platformObject.External) ||
		IsExternalPackageAuthority(platformObject.External) {
		t.Fatalf("platform authority = %#v", platformObject.External)
	}
	if IsExternalPackageAuthority(nil) || IsExternalPlatformAuthority(nil) {
		t.Fatal("nil external symbol has authority")
	}

	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(string(encoded), `"authority_kind":"package"`) ||
		!strings.Contains(string(encoded), `"authority_kind":"platform"`) {
		t.Fatalf("encoded authority kinds are missing: %s", encoded)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatal("codec changed external authority")
	}

	missingKind := []byte(strings.Replace(string(encoded), `"authority_kind":"package",`, "", 1))
	if _, err := Decode(missingKind); err == nil || !strings.Contains(err.Error(), "external symbol authority") {
		t.Fatalf("Decode missing authority kind error = %v", err)
	}

	for _, kind := range []ExternalAuthorityKind{"", "registry"} {
		invalid := input
		invalid.Objects = append([]ObjectInput(nil), input.Objects...)
		invalid.Objects[packagePosition].External = cloneExternalSymbol(invalid.Objects[packagePosition].External)
		invalid.Objects[packagePosition].External.AuthorityKind = kind
		if _, err := newMeasuredProgramIndex(invalid); err == nil ||
			!strings.Contains(err.Error(), "external symbol authority") {
			t.Fatalf("New authority kind %q error = %v", kind, err)
		}
	}

	tampered := index.Snapshot()
	position := objectPositionWithSourceRef(t, tampered, "package-external")
	tampered.Objects[position].External.AuthorityKind = ExternalAuthorityPlatform
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("changed authority kind Validate error = %v", err)
	}
}

func TestObservedCountsPastFormerPortableCeilingRemainValidAndWarn(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("the host int cannot represent a count above the former 32-bit advisory threshold")
	}
	former := int64(MaxObservedCount)
	observed := int(former + 1)
	input := representativeInput()
	input.Coverage.ObjectsObserved = observed
	input.Coverage.RelationsObserved = observed
	input.Relations[0].TargetsObserved = observed
	input.Relations[0].WitnessesObserved = observed
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New above former observed-count threshold: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate above former observed-count threshold: %v", err)
	}
	found := false
	for _, warning := range ScaleWarnings(index) {
		if warning.Kind == ScaleWarningObservedCount && warning.MaximumRetained >= observed {
			found = true
		}
	}
	if !found {
		t.Fatalf("former observed-count threshold produced no warning: %#v", ScaleWarnings(index))
	}
}

func TestNewRejectsMethodOwnedByNonType(t *testing.T) {
	input := representativeInput()
	input.Objects[0].OwnerRef = "object-package"
	_, err := newMeasuredProgramIndex(input)
	if err == nil || !strings.Contains(err.Error(), "owner is not a type") {
		t.Fatalf("New error = %v, want exact method receiver rejection", err)
	}
}

func TestNewRejectsUnmeasuredRelationCoverage(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax"}},
	}}
	input.Coverage = CoverageInput{
		Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations),
	}
	if _, err := New(input); err == nil || !strings.Contains(err.Error(), "invalid relation coverage") {
		t.Fatalf("New error = %v, want explicit relation coverage rejection", err)
	}
}

func TestNewCanonicalizesResolvesAndSeals(t *testing.T) {
	input := representativeInput()
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.HasPrefix(index.Target.ID, "program-target-") || len(index.SHA256) != 64 {
		t.Fatalf("unsealed identities: target=%q sha=%q", index.Target.ID, index.SHA256)
	}
	if got, want := index.Target.Sources, []TargetSource{
		{FileRef: "manifest", Path: "project.toml"},
		{FileRef: "root-a", Path: "src/api.lang"},
		{FileRef: "root-b", Path: "src/worker.lang"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target sources = %#v, want %#v", got, want)
	}
	seed := objectWithSourceRef(t, index, "object-method")
	if got, want := index.Target.Seeds, []TargetSeed{{
		ObjectID: seed.ID, Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target seeds = %#v, want %#v", got, want)
	}
	for position := 1; position < len(index.Objects); position++ {
		if index.Objects[position-1].ID >= index.Objects[position].ID {
			t.Fatalf("objects are not canonical: %#v", index.Objects)
		}
	}
	for position := 1; position < len(index.Relations); position++ {
		if index.Relations[position-1].ID >= index.Relations[position].ID {
			t.Fatalf("relations are not canonical: %#v", index.Relations)
		}
	}

	method := objectWithSourceRef(t, index, "object-method")
	owner := objectWithSourceRef(t, index, "object-worker")
	pkg := objectWithSourceRef(t, index, "object-package")
	if method.OwnerID != owner.ID || method.ContainerID != pkg.ID || method.Signature != "run(context) -> error" {
		t.Fatalf("method ownership was not resolved: %#v", method)
	}
	if got, want := method.SymbolLinkIdentities, []SymbolLinkIdentity{{
		Domain:    "neutral.public-callable.v1",
		Key:       stableID("symbol-link", "neutral.public-callable.v1", "example", "WorkerA", "run"),
		Display:   "WorkerA.run",
		PartCount: 3,
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("method symbol link identities = %#v, want %#v", got, want)
	}
	unknownVisibility := objectWithSourceRef(t, index, "object-runner")
	if unknownVisibility.Visibility != VisibilityUnknown {
		t.Fatalf("visibility = %q, want explicit unknown", unknownVisibility.Visibility)
	}
	if got, want := index.Coverage, (Coverage{
		ObjectsObserved: 8, ObjectsIndexed: 6, ObjectsOmitted: 2,
		RelationsObserved: 5, RelationsIndexed: 3, RelationsOmitted: 2,
		ExactRelations: 1, AlternativeRelations: 1, UnresolvedRelations: 1,
		TargetsObserved: 6, TargetsIndexed: 3, TargetsOmitted: 3,
		WitnessesObserved: 4, WitnessesIndexed: 3, WitnessesOmitted: 1,
	}); got != want {
		t.Fatalf("coverage = %#v, want %#v", got, want)
	}

	reordered := representativeInput()
	reordered.Target.Sources[0], reordered.Target.Sources[2] = reordered.Target.Sources[2], reordered.Target.Sources[0]
	reordered.Target.Seeds = append(reordered.Target.Seeds, reordered.Target.Seeds[0])
	reordered.Objects[0], reordered.Objects[5] = reordered.Objects[5], reordered.Objects[0]
	reordered.Relations[0], reordered.Relations[2] = reordered.Relations[2], reordered.Relations[0]
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if reorderedIndex.SHA256 != index.SHA256 || !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatalf("input order changed canonical index:\nfirst=%#v\nsecond=%#v", index, reorderedIndex)
	}

	changedProducer := representativeInput()
	changedProducer.ScenarioSHA256 = strings.Repeat("d", 64)
	changedProducer.SourceSHA256 = strings.Repeat("e", 64)
	changedIndex, err := newMeasuredProgramIndex(changedProducer)
	if err != nil {
		t.Fatalf("New changed producer: %v", err)
	}
	if changedIndex.Target.ID != index.Target.ID {
		t.Fatalf("scenario/source SHA changed target local identity: %q != %q", changedIndex.Target.ID, index.Target.ID)
	}
	for position := range index.Objects {
		if changedIndex.Objects[position].ID != index.Objects[position].ID {
			t.Fatalf("scenario/source SHA changed object local identity")
		}
	}
	for position := range index.Relations {
		if changedIndex.Relations[position].ID != index.Relations[position].ID {
			t.Fatalf("scenario/source SHA changed relation local identity")
		}
	}
	if changedIndex.SHA256 == index.SHA256 {
		t.Fatal("producer SHA change did not change the complete-index seal")
	}

	renamed := representativeInput()
	renamed.Target.Name = "a better presentation label"
	renamedIndex, err := newMeasuredProgramIndex(renamed)
	if err != nil {
		t.Fatalf("New renamed target: %v", err)
	}
	if renamedIndex.Target.ID != index.Target.ID {
		t.Fatal("presentation name changed semantic target identity")
	}
	for position := range index.Objects {
		if renamedIndex.Objects[position].ID != index.Objects[position].ID {
			t.Fatal("presentation name changed object identity")
		}
	}
	if renamedIndex.SHA256 == index.SHA256 {
		t.Fatal("presentation name change did not change complete artifact bytes")
	}

	snapshot := index.Snapshot()
	snapshot.Target.Sources[0].Path = "changed.py"
	snapshot.Target.Seeds[0].ObjectID = "changed"
	snapshot.Target.Seeds[0].Location.Path = "changed.py"
	snapshot.Objects[0].Location = &Location{Path: "changed.py", Line: 1, Column: 1}
	if len(snapshot.Objects[0].SymbolLinkIdentities) > 0 {
		snapshot.Objects[0].SymbolLinkIdentities[0].Display = "changed"
	}
	snapshot.Relations[0].ToIDs = append(snapshot.Relations[0].ToIDs, "changed")
	snapshot.Relations[0].Witnesses[0].SourceExpression = "changed.call"
	snapshot.Relations[0].Witnesses[0].Location.Path = "changed.py"
	if index.Target.Sources[0].Path == "changed.py" || index.Target.Seeds[0].ObjectID == "changed" ||
		index.Target.Seeds[0].Location.Path == "changed.py" ||
		index.Objects[0].Location != nil && index.Objects[0].Location.Path == "changed.py" ||
		index.Relations[0].Witnesses[0].Location.Path == "changed.py" {
		t.Fatal("Snapshot aliases index storage")
	}
}

func TestSymbolLinkIdentityIsLosslessCanonicalAndSealed(t *testing.T) {
	input := shapeInput()
	input.Objects[0].SymbolLinkIdentities = []SymbolLinkIdentityInput{
		{Domain: "synthetic.public-callable.v1", Parts: []string{"acme", "Client", "Send"}, Display: "Client.Send"},
		{Domain: "synthetic.alias.v1", Parts: []string{"urn:acme:send"}, Display: "send"},
		{Domain: "synthetic.public-callable.v1", Parts: []string{"acme", "Client", "Send"}, Display: "Client.Send"},
	}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	caller := objectWithSourceRef(t, index, "caller")
	if len(caller.SymbolLinkIdentities) != 2 {
		t.Fatalf("canonical identities = %#v", caller.SymbolLinkIdentities)
	}
	for position, identity := range caller.SymbolLinkIdentities {
		if !strings.HasPrefix(identity.Key, "symbol-link-") || len(identity.Key) != len("symbol-link-")+64 {
			t.Fatalf("identity %d key = %q", position, identity.Key)
		}
		if position > 0 && symbolLinkIdentityKey(caller.SymbolLinkIdentities[position-1]) >= symbolLinkIdentityKey(identity) {
			t.Fatalf("identities are not canonical: %#v", caller.SymbolLinkIdentities)
		}
	}

	reordered := input
	reordered.Objects = append([]ObjectInput(nil), input.Objects...)
	reordered.Objects[0].SymbolLinkIdentities = append([]SymbolLinkIdentityInput(nil), input.Objects[0].SymbolLinkIdentities...)
	reordered.Objects[0].SymbolLinkIdentities[0], reordered.Objects[0].SymbolLinkIdentities[1] =
		reordered.Objects[0].SymbolLinkIdentities[1], reordered.Objects[0].SymbolLinkIdentities[0]
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatal("identity input order changed the sealed index")
	}

	conflict := input
	conflict.Objects = append([]ObjectInput(nil), input.Objects...)
	conflict.Objects[0].SymbolLinkIdentities = append([]SymbolLinkIdentityInput(nil), input.Objects[0].SymbolLinkIdentities...)
	conflict.Objects[0].SymbolLinkIdentities[2].Display = "different display"
	if _, err := newMeasuredProgramIndex(conflict); err == nil || !strings.Contains(err.Error(), "conflicting display") {
		t.Fatalf("conflicting alias error = %v", err)
	}

	formerBounds := shapeInput()
	identities := make([]SymbolLinkIdentityInput, MaxSymbolLinkIdentitiesPerObject+1)
	parts := make([]string, MaxSymbolLinkIdentityParts+1)
	for position := range parts {
		parts[position] = "part-" + strconv.Itoa(position)
	}
	for position := range identities {
		identities[position] = SymbolLinkIdentityInput{
			Domain:  "synthetic.alias." + strconv.Itoa(position),
			Parts:   append([]string(nil), parts...),
			Display: "alias " + strconv.Itoa(position),
		}
	}
	formerBounds.Objects[0].SymbolLinkIdentities = identities
	retained, err := newMeasuredProgramIndex(formerBounds)
	if err != nil {
		t.Fatalf("New above former identity bounds: %v", err)
	}
	retainedCaller := objectWithSourceRef(t, retained, "caller")
	if len(retainedCaller.SymbolLinkIdentities) != len(identities) {
		t.Fatalf("retained identities = %d, want %d", len(retainedCaller.SymbolLinkIdentities), len(identities))
	}
	for _, identity := range retainedCaller.SymbolLinkIdentities {
		if identity.PartCount != len(parts) {
			t.Fatalf("retained identity part count = %d, want %d", identity.PartCount, len(parts))
		}
	}

	tampered := index.Snapshot()
	position := objectPositionWithSourceRef(t, tampered, "caller")
	tampered.Objects[position].SymbolLinkIdentities[0].Key = "symbol-link-" + strings.Repeat("0", 64)
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a changed symbol link key")
	}

	tamperedCount := index.Snapshot()
	position = objectPositionWithSourceRef(t, tamperedCount, "caller")
	tamperedCount.Objects[position].SymbolLinkIdentities[0].PartCount = 0
	if err := tamperedCount.Validate(); err == nil {
		t.Fatal("Validate accepted a missing symbol identity part count")
	}
}

func TestRelationPatternsResolveCanonicalizeCoverAndSeal(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "pattern-relation", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax_call"}}, PatternsObserved: 3,
		Patterns: []RelationPatternInput{
			{
				SourceRef: "route-call", Form: PatternCall, Selector: "get",
				Location:  &Location{Path: "main.go", Line: 14, Column: 3},
				ResultRef: "target-b", ReceiverRef: "caller",
				ReceiverOriginRefs:       []string{"target-b", "target-a"},
				ReceiverOriginResolution: ResolutionAlternatives, ReceiverOriginsObserved: 3,
				ArgumentsObserved: 4,
				Arguments: []PatternArgumentInput{
					{Keyword: "handler", Kind: PatternDynamic, ObjectRefs: []string{"target-b"},
						Resolution: ResolutionExact, ObjectsObserved: 1},
					{Position: 2, Kind: PatternStringTemplate, Parts: []PatternPartInput{
						{Kind: PatternPartLiteral, Text: "/api/"},
						{Kind: PatternPartLiteral, Text: "level/"},
						{Kind: PatternPartHole},
					}},
					{Position: 1, Kind: PatternLiteralString, Value: "GET"},
				},
			},
			{
				SourceRef: "route-decorator", Form: PatternDecoratorCall, Selector: "get", ArgumentsObserved: 1,
				Arguments: []PatternArgumentInput{{Position: 1, Kind: PatternLiteralString, Value: "/api/levels"}},
			},
		},
	}}

	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	relation := relationWithSourceRef(t, index, "pattern-relation")
	if relation.Patterns == nil || len(relation.Patterns) != 2 || relation.PatternsObserved != 3 || relation.PatternsOmitted != 1 {
		t.Fatalf("sealed patterns = %#v", relation)
	}
	call := patternWithSourceRef(t, relation, "route-call")
	if call.ID != stableID("program-pattern", relation.ID, "route-call") ||
		call.Location == nil || call.Location.Line != 14 ||
		call.ResultID != objectWithSourceRef(t, index, "target-b").ID ||
		call.ReceiverID != objectWithSourceRef(t, index, "caller").ID {
		t.Fatalf("call identity/receiver = %#v", call)
	}
	wantReceiverOrigins := []string{
		objectWithSourceRef(t, index, "target-a").ID,
		objectWithSourceRef(t, index, "target-b").ID,
	}
	sort.Strings(wantReceiverOrigins)
	if got := call.ReceiverOriginIDs; !reflect.DeepEqual(got, wantReceiverOrigins) {
		t.Fatalf("receiver origins = %#v, want %#v", got, wantReceiverOrigins)
	}
	if call.ReceiverOriginsObserved != 3 || call.ReceiverOriginsOmitted != 1 ||
		call.ArgumentsObserved != 4 || call.ArgumentsOmitted != 1 || len(call.Arguments) != 3 {
		t.Fatalf("call coverage = %#v", call)
	}
	if call.Arguments[0].Position != 1 || call.Arguments[1].Position != 2 || call.Arguments[2].Keyword != "handler" {
		t.Fatalf("arguments are not canonical: %#v", call.Arguments)
	}
	for _, argument := range call.Arguments {
		if argument.ID != stableID("program-pattern-argument", call.ID, patternArgumentKey(argument.Position, argument.Keyword)) {
			t.Fatalf("argument identity = %#v", argument)
		}
	}
	if got, want := call.Arguments[1].Parts, []PatternPart{
		{Kind: PatternPartLiteral, Text: "/api/level/"},
		{Kind: PatternPartHole},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("template parts = %#v, want %#v", got, want)
	}
	if got, want := index.Coverage, (Coverage{
		ObjectsObserved: 3, ObjectsIndexed: 3,
		RelationsObserved: 1, RelationsIndexed: 1, ExactRelations: 1,
		TargetsObserved: 1, TargetsIndexed: 1,
		WitnessesObserved: 1, WitnessesIndexed: 1,
		PatternsObserved: 3, PatternsIndexed: 2, PatternsOmitted: 1,
		ArgumentsObserved: 5, ArgumentsIndexed: 4, ArgumentsOmitted: 1,
		ReceiverOriginsObserved: 3, ReceiverOriginsIndexed: 2, ReceiverOriginsOmitted: 1,
		ArgumentObjectsObserved: 1, ArgumentObjectsIndexed: 1,
	}); got != want {
		t.Fatalf("coverage = %#v, want %#v", got, want)
	}

	reordered := input
	reordered.Relations = append([]RelationInput(nil), input.Relations...)
	reorderedPatterns := append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	for position := range reorderedPatterns {
		reorderedPatterns[position].ReceiverOriginRefs = cloneStrings(reorderedPatterns[position].ReceiverOriginRefs)
		reorderedPatterns[position].Arguments = append([]PatternArgumentInput(nil), reorderedPatterns[position].Arguments...)
		for argumentPosition := range reorderedPatterns[position].Arguments {
			reorderedPatterns[position].Arguments[argumentPosition].Parts = append(
				[]PatternPartInput(nil), reorderedPatterns[position].Arguments[argumentPosition].Parts...,
			)
			reorderedPatterns[position].Arguments[argumentPosition].ObjectRefs = cloneStrings(
				reorderedPatterns[position].Arguments[argumentPosition].ObjectRefs,
			)
		}
	}
	reorderedPatterns[0], reorderedPatterns[1] = reorderedPatterns[1], reorderedPatterns[0]
	for position := range reorderedPatterns {
		if reorderedPatterns[position].SourceRef != "route-call" {
			continue
		}
		reorderedPatterns[position].ReceiverOriginRefs[0], reorderedPatterns[position].ReceiverOriginRefs[1] =
			reorderedPatterns[position].ReceiverOriginRefs[1], reorderedPatterns[position].ReceiverOriginRefs[0]
		reorderedPatterns[position].Arguments[0], reorderedPatterns[position].Arguments[2] =
			reorderedPatterns[position].Arguments[2], reorderedPatterns[position].Arguments[0]
	}
	reordered.Relations[0].Patterns = reorderedPatterns
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatal("pattern, argument, or object-ref input order changed the sealed index")
	}

	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatalf("pattern codec changed index:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}

	snapshot := index.Snapshot()
	snapshotRelation := relationPositionWithSourceRef(t, snapshot, "pattern-relation")
	snapshotPattern := patternPositionWithSourceRef(t, snapshot.Relations[snapshotRelation], "route-call")
	snapshot.Relations[snapshotRelation].Patterns[snapshotPattern].ReceiverOriginIDs[0] = "changed"
	snapshot.Relations[snapshotRelation].Patterns[snapshotPattern].Location.Line = 999
	snapshot.Relations[snapshotRelation].Patterns[snapshotPattern].Arguments[1].Parts[0].Text = "changed"
	snapshot.Relations[snapshotRelation].Patterns[snapshotPattern].Arguments[2].ObjectIDs[0] = "changed"
	original := patternWithSourceRef(t, relationWithSourceRef(t, index, "pattern-relation"), "route-call")
	if original.Location.Line == 999 || original.ReceiverOriginIDs[0] == "changed" || original.Arguments[1].Parts[0].Text == "changed" ||
		original.Arguments[2].ObjectIDs[0] == "changed" {
		t.Fatal("Snapshot aliases nested pattern storage")
	}

	tampered := index.Snapshot()
	tamperedRelation := relationPositionWithSourceRef(t, tampered, "pattern-relation")
	tamperedPattern := patternPositionWithSourceRef(t, tampered.Relations[tamperedRelation], "route-call")
	tampered.Relations[tamperedRelation].Patterns[tamperedPattern].Arguments[1].Parts[0].Text = "/changed/"
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered pattern Validate error = %v", err)
	}
}

func TestRelationPatternOmissionAndEmptySealedCollection(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "computed-call", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
		Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax_call"}}, PatternsObserved: 1,
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	relation := relationWithSourceRef(t, index, "computed-call")
	if relation.Patterns == nil || len(relation.Patterns) != 0 || relation.PatternsObserved != 1 || relation.PatternsOmitted != 1 {
		t.Fatalf("omitted patterns = %#v", relation)
	}
	if index.Coverage.PatternsObserved != 1 || index.Coverage.PatternsIndexed != 0 || index.Coverage.PatternsOmitted != 1 {
		t.Fatalf("pattern coverage = %#v", index.Coverage)
	}

	withoutObservedPattern := shapeInput()
	withoutObservedPattern.Relations = []RelationInput{{
		SourceRef: "ordinary-call", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
		Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax_call"}},
	}}
	ordinary, err := newMeasuredProgramIndex(withoutObservedPattern)
	if err != nil {
		t.Fatalf("New ordinary: %v", err)
	}
	if ordinary.Relations[0].Patterns == nil || ordinary.Relations[0].PatternsObserved != 0 {
		t.Fatalf("ordinary relation pattern shape = %#v", ordinary.Relations[0])
	}
}

func TestPatternSourceRefIsScopedToOwningRelation(t *testing.T) {
	input := shapeInput()
	pattern := validRelationPatternInput()
	input.Relations = []RelationInput{
		{
			SourceRef: "first", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
			Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
			Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
		},
		{
			SourceRef: "second", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-b"},
			Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
			Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
		},
	}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first := patternWithSourceRef(t, relationWithSourceRef(t, index, "first"), pattern.SourceRef)
	second := patternWithSourceRef(t, relationWithSourceRef(t, index, "second"), pattern.SourceRef)
	if first.ID == second.ID {
		t.Fatalf("relation-local pattern refs share identity %q", first.ID)
	}
}

func TestCallbackSourceArgumentResolvesAndValidatesAuthority(t *testing.T) {
	input := shapeInput()
	registrationPattern := RelationPatternInput{
		SourceRef: "registration-pattern", Form: PatternCall, Selector: "register",
		ArgumentsObserved: 1,
		Arguments: []PatternArgumentInput{{
			Position: 1, Kind: PatternDynamic, ObjectRefs: []string{"target-a"},
			Resolution: ResolutionExact, ObjectsObserved: 1,
		}},
	}
	input.Relations = []RelationInput{
		{
			SourceRef: "registration", Kind: RelationCalls, FromRef: "caller",
			ToRefs: []string{"target-b"}, Resolution: ResolutionExact,
			Witnesses: []Witness{{Kind: "syntax"}}, WitnessesObserved: 1,
			Patterns: []RelationPatternInput{registrationPattern}, PatternsObserved: 1,
			TargetsObserved: 1,
		},
		{
			SourceRef: "callback-transfer", Kind: RelationPassesCallback, FromRef: "caller",
			ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
			Witnesses: []Witness{{Kind: "value_flow"}}, WitnessesObserved: 1,
			TargetsObserved: 1,
			SourceArgument: &PatternArgumentRefInput{
				RelationSourceRef: "registration", PatternSourceRef: "registration-pattern", Position: 1,
			},
		},
	}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	registration := relationWithSourceRef(t, index, "registration")
	callback := relationWithSourceRef(t, index, "callback-transfer")
	argument := registration.Patterns[0].Arguments[0]
	if callback.SourceArgumentID != argument.ID {
		t.Fatalf("callback source argument = %q, want %q", callback.SourceArgumentID, argument.ID)
	}

	reordered := input
	reordered.Relations = append([]RelationInput(nil), input.Relations...)
	reordered.Relations[0], reordered.Relations[1] = reordered.Relations[1], reordered.Relations[0]
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatal("relation input order changed callback source-argument join")
	}

	unknown := input
	unknown.Relations = append([]RelationInput(nil), input.Relations...)
	unknownRef := *unknown.Relations[1].SourceArgument
	unknownRef.PatternSourceRef = "missing-pattern"
	unknown.Relations[1].SourceArgument = &unknownRef
	if _, err := newMeasuredProgramIndex(unknown); err == nil || !strings.Contains(err.Error(), "unknown reference") {
		t.Fatalf("unknown source argument error = %v", err)
	}

	wrongKind := input
	wrongKind.Relations = append([]RelationInput(nil), input.Relations...)
	wrongKind.Relations[1].Kind = RelationCalls
	if _, err := newMeasuredProgramIndex(wrongKind); err == nil || !strings.Contains(err.Error(), "invalid source argument input") {
		t.Fatalf("non-callback source argument error = %v", err)
	}

	tampered := index.Snapshot()
	callbackPosition := relationPositionWithSourceRef(t, tampered, "callback-transfer")
	tampered.Relations[callbackPosition].ToIDs = []string{objectWithSourceRef(t, tampered, "target-b").ID}
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "source argument authority mismatch") {
		t.Fatalf("tampered callback authority error = %v", err)
	}
}

func TestCallbackSourceArgumentAllowsMeasuredCallableSubset(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{
		{
			SourceRef: "registration", Kind: RelationCalls, FromRef: "caller",
			ToRefs: []string{"target-b"}, Resolution: ResolutionAlternatives,
			Witnesses: []Witness{{Kind: "syntax"}}, WitnessesObserved: 1,
			Patterns: []RelationPatternInput{{
				SourceRef: "registration-pattern", Form: PatternCall, Selector: "register",
				ArgumentsObserved: 1,
				Arguments: []PatternArgumentInput{{
					Position: 1, Kind: PatternDynamic,
					ObjectRefs: []string{"target-a", "target-b"},
					Resolution: ResolutionAlternatives, ObjectsObserved: 2,
				}},
			}},
			PatternsObserved: 1, TargetsObserved: 1,
		},
		{
			SourceRef: "callback-transfer", Kind: RelationPassesCallback, FromRef: "caller",
			ToRefs: []string{"target-a"}, Resolution: ResolutionAlternatives,
			Witnesses: []Witness{{Kind: "value_flow"}}, WitnessesObserved: 1,
			TargetsObserved: 2,
			SourceArgument: &PatternArgumentRefInput{
				RelationSourceRef: "registration", PatternSourceRef: "registration-pattern", Position: 1,
			},
		},
	}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New callable subset: %v", err)
	}
	callback := relationWithSourceRef(t, index, "callback-transfer")
	argument := relationWithSourceRef(t, index, "registration").Patterns[0].Arguments[0]
	if callback.SourceArgumentID != argument.ID || callback.TargetsObserved != 2 || callback.TargetsOmitted != 1 ||
		argument.ObjectsObserved != 2 || argument.ObjectsOmitted != 0 {
		t.Fatalf("callable subset authority = callback %#v argument %#v", callback, argument)
	}

	tampered := index.Snapshot()
	callbackPosition := relationPositionWithSourceRef(t, tampered, "callback-transfer")
	tampered.Relations[callbackPosition].ToIDs = []string{objectWithSourceRef(t, tampered, "caller").ID}
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "source argument authority mismatch") {
		t.Fatalf("foreign callback target error = %v", err)
	}
}

func TestPatternValueCandidatesResolveCanonicalizeSealAndRejectTampering(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "resolved-value", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-b"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax"}}, WitnessesObserved: 1,
		Patterns: []RelationPatternInput{{
			SourceRef: "resolved-value-pattern", Form: PatternCall, Selector: "get",
			ArgumentsObserved: 1,
			Arguments: []PatternArgumentInput{{
				Position: 1, Kind: PatternDynamic,
				ObjectRefs: []string{"target-a"}, Resolution: ResolutionAlternatives, ObjectsObserved: 1,
				ValueCandidatesObserved: 2,
				ValueCandidates: []PatternValueCandidateInput{
					{
						Kind: PatternLiteralString, Value: "/api/dynamic",
						Resolution: PatternValuePossible, SourceKind: PatternValueSourceInitializer,
						SourceObjectRefs: []string{"target-a"}, SourceObjectsObserved: 1,
					},
					{
						Kind: PatternStringTemplate,
						Parts: []PatternPartInput{
							{Kind: PatternPartLiteral, Text: "/api/"},
							{Kind: PatternPartHole},
						},
						Resolution: PatternValueExact, SourceKind: PatternValueSourceInitializer,
						SourceObjectRefs: []string{"target-b", "target-a"}, SourceObjectsObserved: 2,
					},
				},
			}},
		}},
		PatternsObserved: 1, TargetsObserved: 1,
	}}

	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	argument := relationWithSourceRef(t, index, "resolved-value").Patterns[0].Arguments[0]
	if argument.Kind != PatternDynamic || argument.ValueCandidatesObserved != 2 ||
		argument.ValueCandidatesOmitted != 0 || len(argument.ValueCandidates) != 2 {
		t.Fatalf("sealed value candidates = %#v", argument)
	}
	wantTargetA := objectWithSourceRef(t, index, "target-a").ID
	wantSources := []string{wantTargetA, objectWithSourceRef(t, index, "target-b").ID}
	sort.Strings(wantSources)
	foundLiteral, foundTemplate := false, false
	for _, candidate := range argument.ValueCandidates {
		if candidate.ID != patternValueCandidateIdentity(argument.ID, candidate) ||
			candidate.SourceKind != PatternValueSourceInitializer || candidate.SourceObjectsOmitted != 0 {
			t.Fatalf("candidate identity/provenance = %#v", candidate)
		}
		switch candidate.Kind {
		case PatternLiteralString:
			foundLiteral = candidate.Value == "/api/dynamic" && candidate.Resolution == PatternValuePossible &&
				reflect.DeepEqual(candidate.SourceObjectIDs, []string{wantTargetA})
		case PatternStringTemplate:
			foundTemplate = candidate.Resolution == PatternValueExact &&
				reflect.DeepEqual(candidate.SourceObjectIDs, wantSources) &&
				reflect.DeepEqual(candidate.Parts, []PatternPart{
					{Kind: PatternPartLiteral, Text: "/api/"}, {Kind: PatternPartHole},
				})
		}
	}
	if !foundLiteral || !foundTemplate {
		t.Fatalf("resolved candidates literal=%t template=%t: %#v", foundLiteral, foundTemplate, argument.ValueCandidates)
	}
	if index.Coverage.ArgumentValuesObserved != 2 || index.Coverage.ArgumentValuesIndexed != 2 ||
		index.Coverage.ArgumentValuesOmitted != 0 || index.Coverage.ValueSourcesObserved != 3 ||
		index.Coverage.ValueSourcesIndexed != 3 || index.Coverage.ValueSourcesOmitted != 0 {
		t.Fatalf("resolved value coverage = %#v", index.Coverage)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatal("resolved value codec changed ProgramIndex")
	}

	reordered := input
	reordered.Relations = append([]RelationInput(nil), input.Relations...)
	reordered.Relations[0].Patterns = append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	reordered.Relations[0].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[0].Patterns[0].Arguments...,
	)
	reorderedArgument := &reordered.Relations[0].Patterns[0].Arguments[0]
	reorderedArgument.ValueCandidates = append([]PatternValueCandidateInput(nil), reorderedArgument.ValueCandidates...)
	for position := range reorderedArgument.ValueCandidates {
		reorderedArgument.ValueCandidates[position].Parts = append(
			[]PatternPartInput(nil), reorderedArgument.ValueCandidates[position].Parts...,
		)
		reorderedArgument.ValueCandidates[position].SourceObjectRefs = append(
			[]string(nil), reorderedArgument.ValueCandidates[position].SourceObjectRefs...,
		)
		slices.Reverse(reorderedArgument.ValueCandidates[position].SourceObjectRefs)
	}
	slices.Reverse(reorderedArgument.ValueCandidates)
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatal("candidate or source input order changed sealed ProgramIndex")
	}

	snapshot := index.Snapshot()
	snapshotArgument := &snapshot.Relations[0].Patterns[0].Arguments[0]
	snapshotArgument.ValueCandidates[0].SourceObjectIDs[0] = "changed"
	if index.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0].SourceObjectIDs[0] == "changed" {
		t.Fatal("Snapshot aliases resolved value source objects")
	}

	unknown := input
	unknown.Relations = append([]RelationInput(nil), input.Relations...)
	unknown.Relations[0].Patterns = append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	unknown.Relations[0].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[0].Patterns[0].Arguments...,
	)
	unknown.Relations[0].Patterns[0].Arguments[0].ValueCandidates = append(
		[]PatternValueCandidateInput(nil), input.Relations[0].Patterns[0].Arguments[0].ValueCandidates...,
	)
	unknown.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0].SourceObjectRefs = []string{"missing"}
	if _, err := newMeasuredProgramIndex(unknown); err == nil || !strings.Contains(err.Error(), "unknown object ref") {
		t.Fatalf("unknown candidate source error = %v", err)
	}

	nonDynamic := input
	nonDynamic.Relations = append([]RelationInput(nil), input.Relations...)
	nonDynamic.Relations[0].Patterns = append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	nonDynamic.Relations[0].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[0].Patterns[0].Arguments...,
	)
	nonDynamic.Relations[0].Patterns[0].Arguments[0].Kind = PatternLiteralString
	nonDynamic.Relations[0].Patterns[0].Arguments[0].Value = "already exact"
	if _, err := newMeasuredProgramIndex(nonDynamic); err == nil || !strings.Contains(err.Error(), "require a dynamic") {
		t.Fatalf("non-dynamic candidate error = %v", err)
	}

	tampered := index.Snapshot()
	tampered.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0].ID = "program-pattern-value-" + strings.Repeat("0", 64)
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("tampered candidate identity error = %v", err)
	}
}

func TestActualArgumentValueCandidateResolvesAfterAllRelationsAndBindsExactSource(t *testing.T) {
	input := shapeInput()
	actualPattern := RelationPatternInput{
		SourceRef: "actual-pattern", Form: PatternCall, Selector: "startServer", ArgumentsObserved: 1,
		Arguments: []PatternArgumentInput{{
			Position: 1, Kind: PatternLiteralString, Value: "/products/runtime",
		}},
	}
	formalUsePattern := RelationPatternInput{
		SourceRef: "formal-use-pattern", Form: PatternCall, Selector: "get", ArgumentsObserved: 1,
		Arguments: []PatternArgumentInput{{
			Position: 1, Kind: PatternDynamic,
			ValueCandidatesObserved: 1,
			ValueCandidates: []PatternValueCandidateInput{{
				Kind: PatternLiteralString, Value: "/products/runtime",
				Resolution: PatternValuePossible, SourceKind: PatternValueSourceActualArgument,
				SourceArgumentRefs: []PatternArgumentRefInput{{
					RelationSourceRef: "actual-call", PatternSourceRef: "actual-pattern", Position: 1,
				}},
				SourceArgumentsObserved: 1,
			}},
		}},
	}
	// The use precedes its source deliberately: source-argument joins are
	// resolved only after every relation and nested pattern exists.
	input.Relations = []RelationInput{
		{
			SourceRef: "formal-use", Kind: RelationCalls, FromRef: "target-b",
			ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
			Witnesses: []Witness{{Kind: "syntax"}}, WitnessesObserved: 1,
			Patterns: []RelationPatternInput{formalUsePattern}, PatternsObserved: 1, TargetsObserved: 1,
		},
		{
			SourceRef: "actual-call", Kind: RelationCalls, FromRef: "caller",
			ToRefs: []string{"target-b"}, Resolution: ResolutionExact,
			Witnesses: []Witness{{Kind: "syntax"}}, WitnessesObserved: 1,
			Patterns: []RelationPatternInput{actualPattern}, PatternsObserved: 1, TargetsObserved: 1,
		},
	}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	actual := patternWithSourceRef(t, relationWithSourceRef(t, index, "actual-call"), "actual-pattern").Arguments[0]
	use := patternWithSourceRef(t, relationWithSourceRef(t, index, "formal-use"), "formal-use-pattern").Arguments[0]
	if len(use.ValueCandidates) != 1 || use.ValueCandidatesObserved != 1 || use.ValueCandidatesOmitted != 0 {
		t.Fatalf("actual-to-formal candidate = %#v", use)
	}
	candidate := use.ValueCandidates[0]
	if candidate.SourceKind != PatternValueSourceActualArgument || candidate.Resolution != PatternValuePossible ||
		candidate.Kind != PatternLiteralString || candidate.Value != "/products/runtime" ||
		!reflect.DeepEqual(candidate.SourceArgumentIDs, []string{actual.ID}) ||
		candidate.SourceArgumentsObserved != 1 || candidate.SourceArgumentsOmitted != 0 ||
		len(candidate.SourceObjectIDs) != 0 || candidate.SourceObjectsObserved != 0 {
		t.Fatalf("sealed actual-to-formal candidate = %#v, actual=%#v", candidate, actual)
	}
	if index.Coverage.ArgumentValuesObserved != 1 || index.Coverage.ArgumentValuesIndexed != 1 ||
		index.Coverage.ValueArgumentSourcesObserved != 1 || index.Coverage.ValueArgumentSourcesIndexed != 1 ||
		index.Coverage.ValueArgumentSourcesOmitted != 0 {
		t.Fatalf("actual-to-formal coverage = %#v", index.Coverage)
	}
	snapshot := index.Snapshot()
	snapshotUse := &snapshot.Relations[relationPositionWithSourceRef(t, snapshot, "formal-use")].Patterns[0].Arguments[0]
	snapshotUse.ValueCandidates[0].SourceArgumentIDs[0] = "changed"
	originalUse := patternWithSourceRef(t, relationWithSourceRef(t, index, "formal-use"), "formal-use-pattern")
	if originalUse.Arguments[0].ValueCandidates[0].SourceArgumentIDs[0] != actual.ID {
		t.Fatal("ProgramIndex snapshot aliases resolved value source arguments")
	}

	incompatible := index.Snapshot()
	incompatibleSource := &incompatible.Relations[relationPositionWithSourceRef(t, incompatible, "actual-call")]
	incompatibleSource.ToIDs = []string{}
	incompatibleSource.Resolution = ResolutionUnresolved
	incompatibleSource.TargetsOmitted = incompatibleSource.TargetsObserved
	if err := incompatible.Validate(); err == nil || !strings.Contains(err.Error(), "incompatible actual source argument") {
		t.Fatalf("incompatible sealed actual source error = %v", err)
	}

	reordered := input
	reordered.Relations = append([]RelationInput(nil), input.Relations...)
	reordered.Relations[0], reordered.Relations[1] = reordered.Relations[1], reordered.Relations[0]
	reorderedIndex, err := newMeasuredProgramIndex(reordered)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(reorderedIndex, index) {
		t.Fatal("relation order changed deferred actual-argument join")
	}

	unknown := input
	unknown.Relations = append([]RelationInput(nil), input.Relations...)
	unknown.Relations[0].Patterns = append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	unknown.Relations[0].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[0].Patterns[0].Arguments...,
	)
	unknown.Relations[0].Patterns[0].Arguments[0].ValueCandidates = append(
		[]PatternValueCandidateInput(nil), input.Relations[0].Patterns[0].Arguments[0].ValueCandidates...,
	)
	unknownRef := unknown.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0].SourceArgumentRefs[0]
	unknownRef.PatternSourceRef = "missing"
	unknown.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0].SourceArgumentRefs = []PatternArgumentRefInput{unknownRef}
	if _, err := newMeasuredProgramIndex(unknown); err == nil || !strings.Contains(err.Error(), "unknown reference") {
		t.Fatalf("unknown actual source error = %v", err)
	}

	mismatch := input
	mismatch.Relations = append([]RelationInput(nil), input.Relations...)
	mismatch.Relations[1].Patterns = append([]RelationPatternInput(nil), input.Relations[1].Patterns...)
	mismatch.Relations[1].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[1].Patterns[0].Arguments...,
	)
	mismatch.Relations[1].Patterns[0].Arguments[0].Value = "/different"
	if _, err := newMeasuredProgramIndex(mismatch); err == nil || !strings.Contains(err.Error(), "incompatible authority") {
		t.Fatalf("mismatched actual value error = %v", err)
	}

	nonExact := input
	nonExact.Relations = append([]RelationInput(nil), input.Relations...)
	nonExact.Relations[1].Resolution = ResolutionAlternatives
	if _, err := newMeasuredProgramIndex(nonExact); err == nil || !strings.Contains(err.Error(), "incompatible authority") {
		t.Fatalf("non-exact incoming call error = %v", err)
	}

	multiple := input
	multiple.Relations = append([]RelationInput(nil), input.Relations...)
	multiple.Relations[0].Patterns = append([]RelationPatternInput(nil), input.Relations[0].Patterns...)
	multiple.Relations[0].Patterns[0].Arguments = append(
		[]PatternArgumentInput(nil), input.Relations[0].Patterns[0].Arguments...,
	)
	multiple.Relations[0].Patterns[0].Arguments[0].ValueCandidates = append(
		[]PatternValueCandidateInput(nil), input.Relations[0].Patterns[0].Arguments[0].ValueCandidates...,
	)
	multipleCandidate := &multiple.Relations[0].Patterns[0].Arguments[0].ValueCandidates[0]
	multipleCandidate.SourceArgumentRefs = append(multipleCandidate.SourceArgumentRefs, multipleCandidate.SourceArgumentRefs[0])
	multipleCandidate.SourceArgumentsObserved = 2
	if _, err := newMeasuredProgramIndex(multiple); err == nil || !strings.Contains(err.Error(), "duplicate source argument ref") {
		t.Fatalf("multiple actual sources error = %v", err)
	}
}

func TestRelationPatternsRejectUnknownObjectRefs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RelationPatternInput)
		want   string
	}{
		{name: "receiver", mutate: func(pattern *RelationPatternInput) { pattern.ReceiverRef = "missing" }, want: "receiver"},
		{name: "receiver origin", mutate: func(pattern *RelationPatternInput) {
			pattern.ReceiverOriginRefs = []string{"missing"}
			pattern.ReceiverOriginResolution = ResolutionExact
			pattern.ReceiverOriginsObserved = 1
		}, want: "receiver origins"},
		{name: "argument object", mutate: func(pattern *RelationPatternInput) {
			pattern.Arguments[0].ObjectRefs = []string{"missing"}
			pattern.Arguments[0].Resolution = ResolutionExact
			pattern.Arguments[0].ObjectsObserved = 1
		}, want: "objects"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			pattern := validRelationPatternInput()
			test.mutate(&pattern)
			input.Relations = []RelationInput{{
				SourceRef: "relation", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
				Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
				Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
			}}
			_, err := newMeasuredProgramIndex(input)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "unknown object ref") {
				t.Fatalf("New error = %v", err)
			}
		})
	}
}

func TestRelationPatternsRejectInvalidShapesAndDuplicateKeys(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RelationPatternInput)
		want   string
	}{
		{name: "unknown form", mutate: func(pattern *RelationPatternInput) { pattern.Form = "invoke" }, want: "invalid pattern input"},
		{name: "empty selector", mutate: func(pattern *RelationPatternInput) { pattern.Selector = "" }, want: "invalid pattern input"},
		{name: "both argument keys", mutate: func(pattern *RelationPatternInput) { pattern.Arguments[0].Keyword = "path" }, want: "invalid argument input"},
		{name: "neither argument key", mutate: func(pattern *RelationPatternInput) { pattern.Arguments[0].Position = 0 }, want: "invalid argument input"},
		{name: "literal has parts", mutate: func(pattern *RelationPatternInput) {
			pattern.Arguments[0].Parts = []PatternPartInput{{Kind: PatternPartHole}}
		}, want: "invalid literal"},
		{name: "template has no hole", mutate: func(pattern *RelationPatternInput) {
			pattern.Arguments[0].Kind = PatternStringTemplate
			pattern.Arguments[0].Value = ""
			pattern.Arguments[0].Parts = []PatternPartInput{{Kind: PatternPartLiteral, Text: "/api"}}
		}, want: "no hole"},
		{name: "hole carries text", mutate: func(pattern *RelationPatternInput) {
			pattern.Arguments[0].Kind = PatternStringTemplate
			pattern.Arguments[0].Value = ""
			pattern.Arguments[0].Parts = []PatternPartInput{{Kind: PatternPartHole, Text: "name"}}
		}, want: "invalid template part"},
		{name: "dynamic carries value", mutate: func(pattern *RelationPatternInput) {
			pattern.Arguments[0].Kind = PatternDynamic
		}, want: "invalid dynamic"},
		{name: "missing object resolution", mutate: func(pattern *RelationPatternInput) {
			pattern.ReceiverOriginRefs = []string{"target-a"}
			pattern.ReceiverOriginsObserved = 1
		}, want: "missing resolution"},
		{name: "argument count too small", mutate: func(pattern *RelationPatternInput) {
			pattern.ArgumentsObserved = 0
		}, want: "argument coverage"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			pattern := validRelationPatternInput()
			test.mutate(&pattern)
			input.Relations = []RelationInput{{
				SourceRef: "relation", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
				Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
				Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
			}}
			_, err := newMeasuredProgramIndex(input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}

	input := shapeInput()
	pattern := validRelationPatternInput()
	pattern.Arguments = append(pattern.Arguments, pattern.Arguments[0])
	pattern.ArgumentsObserved = 2
	input.Relations = []RelationInput{{
		SourceRef: "duplicate-arguments", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
		Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
		Patterns: []RelationPatternInput{pattern}, PatternsObserved: 1,
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "duplicate argument key") {
		t.Fatalf("duplicate argument key error = %v", err)
	}

	input = shapeInput()
	pattern = validRelationPatternInput()
	input.Relations = []RelationInput{{
		SourceRef: "duplicate-patterns", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"},
		Resolution: ResolutionExact, Witnesses: []Witness{{Kind: "syntax"}},
		Patterns: []RelationPatternInput{pattern, pattern}, PatternsObserved: 2,
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "duplicate pattern source ref") {
		t.Fatalf("duplicate pattern source ref error = %v", err)
	}

}

func TestWitnessSourceExpressionIsLosslessTypedDigestMaterial(t *testing.T) {
	input := representativeInput()
	input.Relations[1].Witnesses[0].SourceExpression = "runtime.schedule"
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := ""
	for _, relation := range index.Relations {
		if relation.SourceRef == "relation-exact" {
			got = relation.Witnesses[0].SourceExpression
		}
	}
	if got != "runtime.schedule" {
		t.Fatalf("source expression = %q", got)
	}

	changed := representativeInput()
	changed.Relations[1].Witnesses[0].SourceExpression = "scheduler.enqueue"
	changedIndex, err := newMeasuredProgramIndex(changed)
	if err != nil {
		t.Fatalf("New changed expression: %v", err)
	}
	if changedIndex.SHA256 == index.SHA256 {
		t.Fatal("source expression did not affect ProgramIndex seal")
	}

	long := representativeInput()
	long.Relations[1].Witnesses[0].SourceExpression = strings.Repeat("x", MaxTextBytes+1)
	longIndex, err := newMeasuredProgramIndex(long)
	if err != nil {
		t.Fatalf("New long source expression: %v", err)
	}
	if relationWithSourceRef(t, longIndex, "relation-exact").Witnesses[0].SourceExpression !=
		long.Relations[1].Witnesses[0].SourceExpression {
		t.Fatal("long source expression was not retained losslessly")
	}
}

func TestCodecIsStrictAndValidatesSeal(t *testing.T) {
	index, err := newMeasuredProgramIndex(representativeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatalf("codec changed index:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}
	if ArtifactFilename != "program-index.json" {
		t.Fatalf("ArtifactFilename = %q", ArtifactFilename)
	}

	if _, err := Decode(append(append([]byte(nil), encoded...), []byte(` {}`)...)); err == nil {
		t.Fatal("Decode accepted trailing JSON")
	}
	unknown := append([]byte(`{"unknown":true,`), encoded[1:]...)
	if _, err := Decode(unknown); err == nil {
		t.Fatal("Decode accepted an unknown field")
	}
	previousVersion := []byte(strings.Replace(
		string(encoded), `"version":`+strconv.Itoa(Version), `"version":`+strconv.Itoa(Version-1), 1,
	))
	if _, err := Decode(previousVersion); err == nil {
		t.Fatal("Decode accepted the previous ProgramIndex contract version")
	}
	tampered := []byte(strings.Replace(string(encoded), "runtime.schedule", "runtime.changed", 1))
	if _, err := Decode(tampered); err == nil {
		t.Fatal("Decode accepted content with a stale seal")
	}
}

func TestNewRejectsInvalidResolutionShapes(t *testing.T) {
	tests := []struct {
		name     string
		relation RelationInput
		want     string
	}{
		{
			name: "exact has two targets",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a", "target-b"}, Resolution: ResolutionExact,
				Witnesses: []Witness{{Kind: "syntax"}}},
			want: "invalid exact relation",
		},
		{
			name: "exact has no witness",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a"}, Resolution: ResolutionExact},
			want: "invalid exact relation",
		},
		{
			name: "alternatives has no target",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{}, Resolution: ResolutionAlternatives,
				Witnesses: []Witness{{Kind: "flow_candidates"}}},
			want: "invalid alternatives relation",
		},
		{
			name: "unresolved retains target",
			relation: RelationInput{SourceRef: "relation", Kind: RelationCalls, FromRef: "caller",
				ToRefs: []string{"target-a"}, Resolution: ResolutionUnresolved,
				Witnesses: []Witness{{Kind: "dynamic_name", Detail: "computed callee"}}},
			want: "invalid unresolved relation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			input.Relations = []RelationInput{test.relation}
			_, err := newMeasuredProgramIndex(input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewAcceptsOneObservedAlternativeWithoutRuntimeExactness(t *testing.T) {
	input := shapeInput()
	input.Relations = []RelationInput{{
		SourceRef: "possible-call", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionAlternatives,
		TargetsObserved: 1, Witnesses: []Witness{{Kind: "syntax_candidate"}}, WitnessesObserved: 1,
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(index.Relations) != 1 || index.Relations[0].Resolution != ResolutionAlternatives ||
		len(index.Relations[0].ToIDs) != 1 || index.Relations[0].TargetsOmitted != 0 {
		t.Fatalf("possible relation = %#v", index.Relations)
	}
}

func TestNewRejectsDuplicateRelationIdentity(t *testing.T) {
	input := shapeInput()
	relation := RelationInput{
		SourceRef: "same-local-relation", Kind: RelationCalls, FromRef: "caller",
		ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
		Witnesses: []Witness{{Kind: "syntax"}},
	}
	input.Relations = []RelationInput{relation, relation}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "duplicate relation identity") {
		t.Fatalf("duplicate relation error = %v", err)
	}
}

func TestFormerIndexEnvelopeBudgetsAreAdvisoryOnly(t *testing.T) {
	if AdvisoryAggregateTextBytes != 64*1024*1024 || AdvisoryIndexBytes != 128*1024*1024 {
		t.Fatalf("advisory index sizes = %d / %d", AdvisoryAggregateTextBytes, AdvisoryIndexBytes)
	}
	if MaxAggregateTextBytes != 0 || MaxIndexBytes != 0 {
		t.Fatalf("local index cutoffs remain enabled = %d / %d", MaxAggregateTextBytes, MaxIndexBytes)
	}
}

func TestTargetSeedsAreExactIdentityBoundLocalObjects(t *testing.T) {
	input := representativeInput()
	first, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "object-worker", Kind: SeedBoundObject,
		Location: &Location{Path: "src/worker.lang", Line: 4, Column: 1},
	}}
	second, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New changed seed: %v", err)
	}
	if first.Target.ID == second.Target.ID || first.SHA256 == second.SHA256 {
		t.Fatal("changing the exact launch seed did not change target identity and index seal")
	}

	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "missing-object", Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "target seed") {
		t.Fatalf("missing target seed error = %v", err)
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "object-external", Kind: SeedCallable,
		Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
	}}
	if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), "not a local program object") {
		t.Fatalf("external target seed error = %v", err)
	}

	tampered := first.Snapshot()
	tampered.Target.Seeds[0].ObjectID = "program-object-missing"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a dangling target seed")
	}
}

func TestTargetSourcesPreservePairsRejectConflictsAndCanonicalize(t *testing.T) {
	firstInput := shapeInput()
	firstInput.Target.Sources = []TargetSource{
		{FileRef: "support", Path: "src/support.lang"},
		{FileRef: "root", Path: "root.lang"},
	}
	first, err := newMeasuredProgramIndex(firstInput)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	secondInput := shapeInput()
	secondInput.Target.Sources = []TargetSource{
		{FileRef: "root", Path: "root.lang"},
		{FileRef: "support", Path: "src/support.lang"},
	}
	second, err := newMeasuredProgramIndex(secondInput)
	if err != nil {
		t.Fatalf("New reordered: %v", err)
	}
	if !reflect.DeepEqual(first.Target, second.Target) || first.SHA256 != second.SHA256 {
		t.Fatalf("source input order changed sealed index:\nfirst=%#v\nsecond=%#v", first.Target, second.Target)
	}
	if got, want := first.Target.Sources, []TargetSource{
		{FileRef: "root", Path: "root.lang"},
		{FileRef: "support", Path: "src/support.lang"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical sources = %#v, want paired sources %#v", got, want)
	}

	for _, test := range []struct {
		name    string
		sources []TargetSource
		want    string
	}{
		{
			name: "one ref names two paths",
			sources: []TargetSource{
				{FileRef: "root", Path: "root.lang"},
				{FileRef: "root", Path: "other.lang"},
			},
			want: "conflicting paths",
		},
		{
			name: "one path names two refs",
			sources: []TargetSource{
				{FileRef: "root", Path: "root.lang"},
				{FileRef: "other", Path: "root.lang"},
			},
			want: "conflicting file refs",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := shapeInput()
			input.Target.Sources = test.sources
			if _, err := newMeasuredProgramIndex(input); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStructuredTargetSeedPreservesLaunchLocationAndRejectsIncompatibleObject(t *testing.T) {
	input := shapeInput()
	input.Objects = []ObjectInput{
		{SourceRef: "module", Kind: ObjectModule, Name: "tool", Visibility: VisibilityPublic,
			Location: &Location{Path: "root.lang", Line: 1, Column: 1}},
		{SourceRef: "function", Kind: ObjectFunction, Name: "declaredFirst", Visibility: VisibilityPublic,
			Location: &Location{Path: "root.lang", Line: 1, Column: 1}},
	}
	input.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "module", Kind: SeedMainGuard,
		Location: &Location{Path: "root.lang", Line: 7, Column: 1},
	}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := index.Target.Seeds[0].Location.Line, 7; got != want {
		t.Fatalf("main-guard launch line = %d, want %d", got, want)
	}
	module := objectWithSourceRef(t, index, "module")
	if module.Location == nil || module.Location.Line != 1 {
		t.Fatalf("module declaration location = %#v, want line 1", module.Location)
	}

	changedLaunch := input
	changedLaunch.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "module", Kind: SeedMainGuard,
		Location: &Location{Path: "root.lang", Line: 8, Column: 1},
	}}
	changed, err := newMeasuredProgramIndex(changedLaunch)
	if err != nil {
		t.Fatalf("New changed launch: %v", err)
	}
	if changed.Target.ID == index.Target.ID {
		t.Fatal("distinct main-guard launch lines share one target identity")
	}

	for _, seed := range []TargetSeedInput{
		{ObjectRef: "function", Kind: SeedMainGuard, Location: &Location{Path: "root.lang", Line: 7, Column: 1}},
		{ObjectRef: "function", Kind: SeedCallable, Location: &Location{Path: "root.lang", Line: 7, Column: 1}},
	} {
		invalid := input
		invalid.Target.Seeds = []TargetSeedInput{seed}
		if _, err := newMeasuredProgramIndex(invalid); err == nil || !strings.Contains(err.Error(), "incompatible with object") {
			t.Fatalf("incompatible seed %#v error = %v", seed, err)
		}
	}
}

func TestPythonTargetSelectorDistinguishesOtherwiseIdenticalViews(t *testing.T) {
	common := shapeInput()
	common.Target.Language = "python"
	common.Target.Name = "same display"
	common.Target.Seeds = []TargetSeedInput{{
		ObjectRef: "root-callable", Kind: SeedCallable,
		Location: &Location{Path: "root.lang", Line: 1, Column: 1},
	}}
	common.Objects = []ObjectInput{{
		SourceRef: "root-callable", Kind: ObjectFunction, Name: "main", Visibility: VisibilityPublic,
		Location: &Location{Path: "root.lang", Line: 1, Column: 1},
	}}
	firstInput := common
	firstInput.Target.Selector = "python:.:script:first"
	first, err := newMeasuredProgramIndex(firstInput)
	if err != nil {
		t.Fatalf("New first view: %v", err)
	}

	secondInput := common
	secondInput.Target.Selector = "python:.:script:second"
	second, err := newMeasuredProgramIndex(secondInput)
	if err != nil {
		t.Fatalf("New second view: %v", err)
	}
	if first.Target.ID == second.Target.ID {
		t.Fatalf("Python views with selectors %q and %q share target ID %q",
			first.Target.Selector, second.Target.Selector, first.Target.ID)
	}
}

func TestValidateRejectsTampering(t *testing.T) {
	index, err := newMeasuredProgramIndex(representativeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tampered := index.Snapshot()
	method := objectPositionWithSourceRef(t, tampered, "object-method")
	tampered.Objects[method].Signature = "run()"
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered signature Validate error = %v", err)
	}

	tampered = index.Snapshot()
	tampered.Relations[0].FromID = "program-object-unknown"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a dangling relation source")
	}

	tampered = index.Snapshot()
	tampered.Coverage.RelationsObserved++
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted tampered coverage")
	}
}

func representativeInput() Input {
	return Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: TargetInput{
			Language: "neutral-test", Kind: "executable", Name: "example", Selector: "fixture",
			Sources: []TargetSource{
				{FileRef: "root-b", Path: "src/worker.lang"},
				{FileRef: "manifest", Path: "project.toml"},
				{FileRef: "root-a", Path: "src/api.lang"},
				{FileRef: "root-a", Path: "src/api.lang"},
			},
			AnchorFileRef: "manifest",
			Seeds: []TargetSeedInput{{
				ObjectRef: "object-method", Kind: SeedCallable,
				Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
			}},
		},
		Objects: []ObjectInput{
			{SourceRef: "object-method", Kind: ObjectMethod, Name: "run", Visibility: VisibilityPublic,
				Signature: "run(context) -> error", OwnerRef: "object-worker", ContainerRef: "object-package",
				Location: &Location{Path: "src/worker.lang", Line: 12, Column: 3},
				SymbolLinkIdentities: []SymbolLinkIdentityInput{
					{Domain: "neutral.public-callable.v1", Parts: []string{"example", "WorkerA", "run"}, Display: "WorkerA.run"},
					{Domain: "neutral.public-callable.v1", Parts: []string{"example", "WorkerA", "run"}, Display: "WorkerA.run"},
				}},
			{SourceRef: "object-external", Kind: ObjectExternalSymbol, Name: "runtime.schedule", Visibility: VisibilityPublic},
			{SourceRef: "object-impl-b", Kind: ObjectType, Name: "WorkerB", Visibility: VisibilityInternal,
				ContainerRef: "object-package", Location: &Location{Path: "src/b.lang", Line: 4, Column: 1}},
			{SourceRef: "object-package", Kind: ObjectPackage, Name: "example", Visibility: VisibilityPublic,
				Location: &Location{Path: "src/package.lang", Line: 1, Column: 1}},
			{SourceRef: "object-runner", Kind: ObjectType, Name: "Runner", Visibility: VisibilityUnknown,
				ContainerRef: "object-package", Location: &Location{Path: "src/api.lang", Line: 2, Column: 1}},
			{SourceRef: "object-worker", Kind: ObjectType, Name: "WorkerA", Visibility: VisibilityInternal,
				ContainerRef: "object-package", Location: &Location{Path: "src/worker.lang", Line: 4, Column: 1}},
		},
		Relations: []RelationInput{
			{
				SourceRef: "relation-unresolved", Kind: RelationInvokesExternal, FromRef: "object-method",
				Resolution: ResolutionUnresolved, Invocation: "runtime selected", TargetsObserved: 3,
				WitnessesObserved: 2, Witnesses: []Witness{{Kind: "dynamic_name", Detail: "callee name is computed",
					Location: &Location{Path: "src/worker.lang", Line: 20, Column: 7}}},
			},
			{
				SourceRef: "relation-exact", Kind: RelationCalls, FromRef: "object-method", ToRefs: []string{"object-external"},
				Resolution: ResolutionExact, Invocation: "deferred", Location: &Location{Path: "src/worker.lang", Line: 14, Column: 5},
				Witnesses: []Witness{{Kind: "syntax_call", Location: &Location{Path: "src/worker.lang", Line: 14, Column: 5}}},
			},
			{
				SourceRef: "relation-alternatives", Kind: RelationImplements, FromRef: "object-runner",
				ToRefs: []string{"object-impl-b", "object-worker"}, Resolution: ResolutionAlternatives,
				Witnesses: []Witness{{Kind: "compatible_declaration", Detail: "both declarations satisfy the local contract",
					Location: &Location{Path: "src/api.lang", Line: 2, Column: 1}}},
			},
		},
		Coverage: CoverageInput{Measured: true, ObjectsObserved: 8, RelationsObserved: 5},
	}
}

func shapeInput() Input {
	return Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: TargetInput{Language: "neutral-test", Kind: "executable", Name: "shape",
			Selector: "shape", Sources: []TargetSource{{FileRef: "root", Path: "root.lang"}}, AnchorFileRef: "root"},
		Objects: []ObjectInput{
			{SourceRef: "caller", Kind: ObjectFunction, Name: "caller", Visibility: VisibilityInternal},
			{SourceRef: "target-a", Kind: ObjectFunction, Name: "a", Visibility: VisibilityInternal},
			{SourceRef: "target-b", Kind: ObjectFunction, Name: "b", Visibility: VisibilityInternal},
		},
	}
}

func newMeasuredProgramIndex(input Input) (Index, error) {
	for position := range input.Relations {
		if input.Relations[position].TargetsObserved == 0 {
			input.Relations[position].TargetsObserved = len(input.Relations[position].ToRefs)
			if input.Relations[position].TargetsObserved == 0 {
				input.Relations[position].TargetsObserved = 1
			}
		}
		if input.Relations[position].WitnessesObserved == 0 {
			input.Relations[position].WitnessesObserved = len(input.Relations[position].Witnesses)
			if input.Relations[position].WitnessesObserved == 0 {
				input.Relations[position].WitnessesObserved = 1
			}
		}
	}
	if !input.Coverage.Measured {
		input.Coverage = CoverageInput{
			Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations),
		}
	}
	return New(input)
}

func objectWithSourceRef(t *testing.T, index Index, ref string) Object {
	t.Helper()
	return index.Objects[objectPositionWithSourceRef(t, index, ref)]
}

func objectPositionWithSourceRef(t *testing.T, index Index, ref string) int {
	t.Helper()
	for position, object := range index.Objects {
		if object.SourceRef == ref {
			return position
		}
	}
	t.Fatalf("object source ref %q not found", ref)
	return -1
}

func relationWithSourceRef(t *testing.T, index Index, ref string) Relation {
	t.Helper()
	return index.Relations[relationPositionWithSourceRef(t, index, ref)]
}

func relationPositionWithSourceRef(t *testing.T, index Index, ref string) int {
	t.Helper()
	for position, relation := range index.Relations {
		if relation.SourceRef == ref {
			return position
		}
	}
	t.Fatalf("relation source ref %q not found", ref)
	return -1
}

func patternWithSourceRef(t *testing.T, relation Relation, ref string) RelationPattern {
	t.Helper()
	return relation.Patterns[patternPositionWithSourceRef(t, relation, ref)]
}

func patternPositionWithSourceRef(t *testing.T, relation Relation, ref string) int {
	t.Helper()
	for position, pattern := range relation.Patterns {
		if pattern.SourceRef == ref {
			return position
		}
	}
	t.Fatalf("pattern source ref %q not found", ref)
	return -1
}

func validRelationPatternInput() RelationPatternInput {
	return RelationPatternInput{
		SourceRef: "pattern", Form: PatternCall, Selector: "get", ArgumentsObserved: 1,
		Arguments: []PatternArgumentInput{{Position: 1, Kind: PatternLiteralString, Value: "/api/levels"}},
	}
}
