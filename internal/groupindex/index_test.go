package groupindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestBuildRetainsCompleteProgramFactsAndBuildsSparseOverlappingGroups(t *testing.T) {
	program := testProgramIndex(t, "first")
	ids := testSubjectIDs(t, program)
	proposals := testProposals(ids)
	proposals.Groups = append(proposals.Groups, GroupProposal{
		Key: "broken", Title: "Broken", Summary: "Unknown subject must not acquire authority.", Lane: LaneCore,
		MemberSubjectIDs: []string{"program-object-" + strings.Repeat("f", 64)},
	}, GroupProposal{
		Key: "mixed-lane", Title: "Mixed lane", Summary: "One compatible member cannot promote another.", Lane: LaneDependencies,
		MemberSubjectIDs: []string{ids["core"], ids["dependency"]}, EvidenceSubjectIDs: []string{},
	}, GroupProposal{
		Key: "platform-object-evidence", Title: "Platform object evidence", Summary: "Platform API is not dependency evidence.", Lane: LaneDependencies,
		MemberSubjectIDs: []string{ids["dependency"]}, EvidenceSubjectIDs: []string{ids["platform"]},
	}, GroupProposal{
		Key: "platform-pattern-evidence", Title: "Platform pattern evidence", Summary: "Platform-only invocation is not dependency evidence.", Lane: LaneDependencies,
		MemberSubjectIDs: []string{ids["dependency"]}, EvidenceSubjectIDs: []string{ids["platform-pattern"]},
	})
	proposals.Connections = append(proposals.Connections,
		ConnectionProposal{
			FromGroupKey: "triggers", ToGroupKey: "missing", SemanticKind: "unknown_destination",
			Label: "Unknown", Summary: "This row must be discarded.",
		},
		ConnectionProposal{
			FromGroupKey: "triggers", ToGroupKey: "core", SemanticKind: "Not_Snake_Case",
			Label: "Invalid", Summary: "This row must be discarded.",
		},
	)

	index, diagnostics, err := Build(program, proposals)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(diagnostics) != 6 {
		t.Fatalf("diagnostics = %#v, want 6 rejected rows", diagnostics)
	}
	if Version != 4 {
		t.Fatalf("GroupsIndex version = %d, want 4", Version)
	}
	if index.Version != Version || index.ProgramIndexSHA256 != program.SHA256 || index.Target.ID != program.Target.ID {
		t.Fatalf("producer binding = %#v", index)
	}
	if len(index.Subjects) != len(program.Objects)+2 {
		t.Fatalf("subjects = %d, want all %d objects plus two patterns", len(index.Subjects), len(program.Objects))
	}
	if !hasSubject(index.Subjects, ids["ungrouped"]) {
		t.Fatal("sparse ungrouped object disappeared from matching authority")
	}
	if subjectIsGrouped(index.Groups, ids["ungrouped"]) {
		t.Fatal("sparse ungrouped object was invented as a presentation member")
	}

	trigger := groupByTitle(t, index.Groups, "Execution triggers")
	if trigger.Lane != LaneTriggers ||
		!containsString(trigger.MemberSubjectIDs, ids["inbound"]) ||
		!containsString(trigger.MemberSubjectIDs, ids["background"]) ||
		!containsString(trigger.MemberSubjectIDs, ids["pattern"]) {
		t.Fatalf("triggers group = %#v", trigger)
	}
	core := groupByTitle(t, index.Groups, "Core flow")
	if !containsString(core.MemberSubjectIDs, ids["core"]) ||
		!containsString(core.MemberSubjectIDs, ids["pattern"]) {
		t.Fatalf("overlapping core group = %#v", core)
	}
	if trigger.ID == "" || core.ID == "" || trigger.ID == core.ID {
		t.Fatalf("group identities = %q, %q", trigger.ID, core.ID)
	}

	pattern := subjectByID(t, index.Subjects, ids["pattern"])
	if pattern.Pattern == nil || pattern.Pattern.RelationID == "" || pattern.Pattern.FromID != ids["inbound"] ||
		!reflect.DeepEqual(pattern.Pattern.ToIDs, []string{ids["core"]}) || pattern.Pattern.Invocation != "sync" ||
		pattern.Pattern.ReceiverID != ids["ungrouped"] || len(pattern.Pattern.Arguments) != 3 {
		t.Fatalf("pattern facts = %#v", pattern.Pattern)
	}
	wantArgumentKinds := []programindex.PatternValueKind{
		programindex.PatternLiteralString, programindex.PatternStringTemplate, programindex.PatternDynamic,
	}
	for position, want := range wantArgumentKinds {
		if got := pattern.Pattern.Arguments[position].Kind; got != want {
			t.Fatalf("argument %d kind = %q, want %q", position, got, want)
		}
	}
	if !reflect.DeepEqual(pattern.Pattern.Arguments[2].ObjectIDs, []string{ids["background"]}) {
		t.Fatalf("dynamic argument object refs = %#v", pattern.Pattern.Arguments[2].ObjectIDs)
	}
	dependency := subjectByID(t, index.Subjects, ids["dependency"])
	if dependency.Object == nil || dependency.Object.External == nil ||
		dependency.Object.External.AuthorityKind != programindex.ExternalAuthorityPackage ||
		dependency.Object.External.PackagePath != "example.com/queue" ||
		len(dependency.Object.SymbolLinkIdentities) != 1 {
		t.Fatalf("external object facts = %#v", dependency.Object)
	}
	inbound := subjectByID(t, index.Subjects, ids["inbound"])
	if inbound.Object == nil || inbound.Object.Signature != "func Serve()" {
		t.Fatalf("object signature facts = %#v", inbound.Object)
	}
	if len(index.StructuralEdges) < 8 ||
		!hasStructuralEdge(index.StructuralEdges, EdgeObjectOwner, ids["inbound"], ids["core"]) ||
		!hasStructuralEdge(index.StructuralEdges, EdgeObjectContainer, ids["inbound"], ids["background"]) ||
		!hasStructuralEdge(index.StructuralEdges, EdgePatternTarget, ids["pattern"], ids["core"]) {
		t.Fatalf("structural edges = %#v", index.StructuralEdges)
	}

	if len(index.Connections) != 1 {
		t.Fatalf("connections = %#v", index.Connections)
	}
	connection := index.Connections[0]
	if connection.SemanticKind != "awakens_domain_flow" ||
		connection.SupportResolution != programindex.PatternValueExact ||
		!strings.HasPrefix(connection.ID, "program-group-connection-") ||
		len(connection.Evidence) != 1 || connection.Evidence[0] != (SubjectEndpoint{TargetID: program.Target.ID, SubjectID: ids["pattern"]}) {
		t.Fatalf("custom connection = %#v", connection)
	}

	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(string(encoded), `"authority_kind":"package"`) {
		t.Fatalf("encoded GroupsIndex lost external authority kind: %s", encoded)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatalf("round trip changed index:\nwant=%#v\ngot=%#v", index, decoded)
	}

	reordered := proposals
	reordered.Groups = append([]GroupProposal(nil), proposals.Groups...)
	reordered.Connections = append([]ConnectionProposal(nil), proposals.Connections...)
	slices.Reverse(reordered.Groups)
	slices.Reverse(reordered.Connections)
	rebuilt, rebuiltDiagnostics, err := Build(program, reordered)
	if err != nil {
		t.Fatalf("Build reordered: %v", err)
	}
	if !reflect.DeepEqual(rebuilt, index) || !reflect.DeepEqual(rebuiltDiagnostics, diagnostics) {
		t.Fatalf("proposal order changed result:\nfirst=%#v %#v\nsecond=%#v %#v", index, diagnostics, rebuilt, rebuiltDiagnostics)
	}

	snapshot := index.Snapshot()
	snapshot.Subjects[0].Categories = append(snapshot.Subjects[0].Categories, programindex.CategoryCore)
	snapshot.Groups[0].MemberSubjectIDs[0] = ids["ungrouped"]
	snapshot.Connections[0].Evidence[0].SubjectID = ids["ungrouped"]
	if reflect.DeepEqual(snapshot, index) {
		t.Fatal("Snapshot shares mutable collections")
	}
}

func TestBuildRequiresEnrichmentAndDecodeIsStrict(t *testing.T) {
	base := testBaseProgramIndex(t, "strict")
	if _, _, err := Build(base, Proposals{}); err == nil || !strings.Contains(err.Error(), "not categorized") {
		t.Fatalf("Build un-enriched error = %v", err)
	}
	program := enrichTestProgramIndex(t, base)
	index, _, err := Build(program, Proposals{})
	if err != nil {
		t.Fatalf("Build empty: %v", err)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	withUnknown := append([]byte(`{"unexpected":true,`), encoded[1:]...)
	if _, err := Decode(withUnknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Decode unknown field error = %v", err)
	}
	if _, err := Decode(append(encoded, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("Decode trailing value error = %v", err)
	}
	tampered := index.Snapshot()
	tampered.Subjects[0].ID = "program-object-" + strings.Repeat("f", 64)
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate accepted a tampered subject")
	}

	withMissingExternalAuthority := []byte(strings.Replace(
		string(encoded), `"authority_kind":"package",`, "", 1,
	))
	if _, err := Decode(withMissingExternalAuthority); err == nil ||
		!strings.Contains(err.Error(), "invalid object subject") {
		t.Fatalf("Decode missing external authority kind error = %v", err)
	}

	tamperedExternal := index.Snapshot()
	for position := range tamperedExternal.Subjects {
		if tamperedExternal.Subjects[position].Object != nil &&
			tamperedExternal.Subjects[position].Object.External != nil {
			tamperedExternal.Subjects[position].Object.External.AuthorityKind = "registry"
			break
		}
	}
	if err := tamperedExternal.Validate(); err == nil ||
		!strings.Contains(err.Error(), "invalid object subject") {
		t.Fatalf("Validate unknown external authority kind error = %v", err)
	}
}

func TestValidateRejectsPlatformAuthorityAsDependencyGroupEvidence(t *testing.T) {
	program := testProgramIndex(t, "platform-evidence")
	ids := testSubjectIDs(t, program)
	base, diagnostics, err := Build(program, Proposals{Groups: []GroupProposal{{
		Key: "dependency", Title: "Queue dependency", Summary: "External queue boundary.", Lane: LaneDependencies,
		MemberSubjectIDs: []string{ids["dependency"]}, EvidenceSubjectIDs: []string{},
	}}})
	if err != nil || len(diagnostics) != 0 || len(base.Groups) != 1 {
		t.Fatalf("Build dependency baseline: index=%#v diagnostics=%#v err=%v", base, diagnostics, err)
	}
	for _, evidenceID := range []string{ids["platform"], ids["platform-pattern"]} {
		tampered := base.Snapshot()
		tampered.Groups[0].EvidenceSubjectIDs = []string{evidenceID}
		tampered.Groups[0].ID = groupIdentity(tampered.Target.ID, tampered.Groups[0])
		tampered.SHA256 = ""
		digest, digestErr := indexDigest(tampered)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		tampered.SHA256 = digest
		if err := tampered.Validate(); err == nil ||
			!strings.Contains(err.Error(), "platform authority cannot evidence") {
			t.Fatalf("Validate platform evidence %q error = %v", evidenceID, err)
		}
	}
}

func TestBuildRetainsValueCandidatesAndProjectsHonestProvenanceEdges(t *testing.T) {
	program := testValueProvenanceProgramIndex(t)
	index, diagnostics, err := Build(program, Proposals{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	var use, source Subject
	for _, subject := range index.Subjects {
		if subject.Pattern == nil {
			continue
		}
		switch subject.Pattern.Selector {
		case "get":
			use = subject
		case "register":
			source = subject
		}
	}
	if use.Pattern == nil || source.Pattern == nil || len(use.Pattern.Arguments) != 1 || len(source.Pattern.Arguments) != 1 {
		t.Fatalf("pattern subjects = use %#v source %#v", use, source)
	}
	argument := use.Pattern.Arguments[0]
	if argument.ID == "" || argument.ValueCandidatesObserved != 2 || argument.ValueCandidatesOmitted != 0 ||
		len(argument.ValueCandidates) != 2 {
		t.Fatalf("retained argument provenance = %#v", argument)
	}
	var initializer, actual PatternValueCandidate
	for _, candidate := range argument.ValueCandidates {
		switch candidate.SourceKind {
		case programindex.PatternValueSourceInitializer:
			initializer = candidate
		case programindex.PatternValueSourceActualArgument:
			actual = candidate
		}
	}
	if initializer.Value != "/api/dynamic" || len(initializer.SourceObjectIDs) != 1 ||
		initializer.SourceObjectsObserved != 1 || initializer.SourceObjectsOmitted != 0 {
		t.Fatalf("initializer candidate = %#v", initializer)
	}
	if actual.Value != "/products/runtime" || !reflect.DeepEqual(actual.SourceArgumentIDs, []string{source.Pattern.Arguments[0].ID}) ||
		actual.SourceArgumentsObserved != 1 || actual.SourceArgumentsOmitted != 0 ||
		actual.Resolution != programindex.PatternValuePossible {
		t.Fatalf("actual-argument candidate = %#v", actual)
	}

	initializerObjectID := initializer.SourceObjectIDs[0]
	if !hasValueSourceStructuralEdge(index.StructuralEdges, EdgePatternValueSourceObject,
		initializerObjectID, use.ID, argument.ID, initializer.ID, "") {
		t.Fatalf("missing initializer provenance edge: %#v", index.StructuralEdges)
	}
	if !hasValueSourceStructuralEdge(index.StructuralEdges, EdgePatternValueSourceArgument,
		source.ID, use.ID, argument.ID, actual.ID, source.Pattern.Arguments[0].ID) {
		t.Fatalf("missing actual-argument provenance edge: %#v", index.StructuralEdges)
	}
	for _, edge := range index.StructuralEdges {
		if edge.Role != EdgePatternValueSourceObject && edge.Role != EdgePatternValueSourceArgument {
			continue
		}
		if edge.Resolution != programindex.ResolutionUnresolved || edge.ValueResolution != programindex.PatternValuePossible {
			t.Fatalf("provenance was promoted to runtime authority: %#v", edge)
		}
	}

	snapshot := index.Snapshot()
	for position := range snapshot.Subjects {
		if snapshot.Subjects[position].ID == use.ID {
			snapshot.Subjects[position].Pattern.Arguments[0].ValueCandidates[0].SourceArgumentIDs = append(
				snapshot.Subjects[position].Pattern.Arguments[0].ValueCandidates[0].SourceArgumentIDs, "changed",
			)
		}
	}
	if reflect.DeepEqual(snapshot, index) {
		t.Fatal("Snapshot aliases retained value provenance")
	}
}

func TestWithConnectionsOwnsCrossTargetResolutionMergeAndReseal(t *testing.T) {
	leftProgram := testProgramIndex(t, "left")
	rightProgram := testProgramIndex(t, "right")
	leftIDs := testSubjectIDs(t, leftProgram)
	rightIDs := testSubjectIDs(t, rightProgram)
	left, _, err := Build(leftProgram, testProposalsWithoutConnections(leftIDs))
	if err != nil {
		t.Fatalf("Build left: %v", err)
	}
	right, _, err := Build(rightProgram, testProposalsWithoutConnections(rightIDs))
	if err != nil {
		t.Fatalf("Build right: %v", err)
	}
	leftTrigger := groupByTitle(t, left.Groups, "Execution triggers")
	rightCore := groupByTitle(t, right.Groups, "Core flow")
	valid := ConnectionInput{
		From:         Endpoint{TargetID: left.Target.ID, GroupID: leftTrigger.ID},
		To:           Endpoint{TargetID: right.Target.ID, GroupID: rightCore.ID},
		SemanticKind: "dispatches_across_target", Label: "Dispatches", Summary: "A trigger dispatches into the sibling core.",
		SupportResolution: programindex.PatternValuePossible,
		Evidence: []SubjectEndpoint{
			{TargetID: right.Target.ID, SubjectID: rightIDs["pattern"]},
			{TargetID: left.Target.ID, SubjectID: leftIDs["inbound"]},
			{TargetID: right.Target.ID, SubjectID: rightIDs["pattern"]},
		},
	}
	unknownGroup := valid
	unknownGroup.To.GroupID = "program-group-" + strings.Repeat("f", 64)
	unknownEvidence := valid
	unknownEvidence.SemanticKind = "has_unknown_evidence"
	unknownEvidence.Evidence = []SubjectEndpoint{{
		TargetID: right.Target.ID, SubjectID: "program-object-" + strings.Repeat("f", 64),
	}}

	updated, diagnostics, err := WithConnections([]Index{left, right}, []ConnectionInput{unknownEvidence, valid, valid, unknownGroup})
	if err != nil {
		t.Fatalf("WithConnections: %v", err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want unknown group and evidence", diagnostics)
	}
	if len(updated[0].Connections) != 1 || len(updated[1].Connections) != 0 {
		t.Fatalf("connection ownership = left %d, right %d", len(updated[0].Connections), len(updated[1].Connections))
	}
	connection := updated[0].Connections[0]
	if connection.From.TargetID != left.Target.ID || connection.To.TargetID != right.Target.ID ||
		connection.SupportResolution != programindex.PatternValuePossible || len(connection.Evidence) != 2 {
		t.Fatalf("cross-target connection = %#v", connection)
	}
	if updated[0].SHA256 == left.SHA256 || updated[1].SHA256 != right.SHA256 {
		t.Fatalf("reseals = left %q -> %q, right %q -> %q", left.SHA256, updated[0].SHA256, right.SHA256, updated[1].SHA256)
	}
	if err := ValidateSet(updated); err != nil {
		t.Fatalf("ValidateSet: %v", err)
	}
	if err := ValidateSet([]Index{updated[0]}); err == nil || !strings.Contains(err.Error(), "absent from set") {
		t.Fatalf("ValidateSet incomplete portfolio error = %v", err)
	}

	reorderedInput := valid
	slices.Reverse(reorderedInput.Evidence)
	rebuilt, rebuiltDiagnostics, err := WithConnections([]Index{left, right}, []ConnectionInput{unknownGroup, valid, reorderedInput, unknownEvidence})
	if err != nil {
		t.Fatalf("WithConnections reordered: %v", err)
	}
	if !reflect.DeepEqual(rebuilt, updated) || !reflect.DeepEqual(rebuiltDiagnostics, diagnostics) {
		t.Fatalf("connection order changed result")
	}
	merged, mergeDiagnostics, err := WithConnections(updated, []ConnectionInput{valid})
	if err != nil {
		t.Fatalf("WithConnections merge: %v", err)
	}
	if len(mergeDiagnostics) != 0 || !reflect.DeepEqual(merged, updated) {
		t.Fatalf("compatible merge changed authority: %#v %#v", merged, mergeDiagnostics)
	}
}

func TestWithConnectionsRequiresAndSealsClosedSupportResolution(t *testing.T) {
	leftProgram := testProgramIndex(t, "support-left")
	rightProgram := testProgramIndex(t, "support-right")
	leftIDs := testSubjectIDs(t, leftProgram)
	rightIDs := testSubjectIDs(t, rightProgram)
	left, _, err := Build(leftProgram, testProposalsWithoutConnections(leftIDs))
	if err != nil {
		t.Fatalf("Build left: %v", err)
	}
	right, _, err := Build(rightProgram, testProposalsWithoutConnections(rightIDs))
	if err != nil {
		t.Fatalf("Build right: %v", err)
	}
	leftTrigger := groupByTitle(t, left.Groups, "Execution triggers")
	rightCore := groupByTitle(t, right.Groups, "Core flow")
	input := ConnectionInput{
		From:         Endpoint{TargetID: left.Target.ID, GroupID: leftTrigger.ID},
		To:           Endpoint{TargetID: right.Target.ID, GroupID: rightCore.ID},
		SemanticKind: "supports_across_target", Label: "Supports", Summary: "Static evidence supports this relation.",
		Evidence: []SubjectEndpoint{
			{TargetID: left.Target.ID, SubjectID: leftIDs["pattern"]},
			{TargetID: right.Target.ID, SubjectID: rightIDs["pattern"]},
		},
	}

	for _, resolution := range []programindex.PatternValueResolution{
		"", programindex.PatternValueResolution("alternatives"),
	} {
		invalid := input
		invalid.SupportResolution = resolution
		got, diagnostics, runErr := WithConnections([]Index{left, right}, []ConnectionInput{invalid})
		if runErr != nil {
			t.Fatalf("WithConnections invalid %q: %v", resolution, runErr)
		}
		if len(diagnostics) != 1 || !reflect.DeepEqual(got, []Index{left, right}) {
			t.Fatalf("invalid support resolution %q acquired authority: diagnostics=%#v indexes=%#v",
				resolution, diagnostics, got)
		}
	}

	build := func(resolution programindex.PatternValueResolution) ([]Index, Connection) {
		t.Helper()
		accepted := input
		accepted.SupportResolution = resolution
		got, diagnostics, runErr := WithConnections([]Index{left, right}, []ConnectionInput{accepted})
		if runErr != nil {
			t.Fatalf("WithConnections %q: %v", resolution, runErr)
		}
		if len(diagnostics) != 0 || len(got[0].Connections) != 1 {
			t.Fatalf("accepted support resolution %q: diagnostics=%#v indexes=%#v", resolution, diagnostics, got)
		}
		return got, got[0].Connections[0]
	}
	exactIndexes, exact := build(programindex.PatternValueExact)
	possibleIndexes, possible := build(programindex.PatternValuePossible)
	if exact.SupportResolution != programindex.PatternValueExact ||
		possible.SupportResolution != programindex.PatternValuePossible ||
		exact.ID == possible.ID || exactIndexes[0].SHA256 == possibleIndexes[0].SHA256 {
		t.Fatalf("support resolution was not identity-bound: exact=%#v possible=%#v", exact, possible)
	}
}

func testProgramIndex(t *testing.T, selector string) programindex.Index {
	t.Helper()
	return enrichTestProgramIndex(t, testBaseProgramIndex(t, selector))
}

func testValueProvenanceProgramIndex(t *testing.T) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "runtime/index.ts", Line: line, Column: 1}
	}
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("7", 64), SourceSHA256: strings.Repeat("8", 64),
		Target: programindex.TargetInput{
			Language: "jsts", Kind: "application", Name: "runtime", Selector: "jsts:runtime",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "runtime/index.ts"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "caller", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "boot", Visibility: programindex.VisibilityInternal, Location: location(1)},
			{SourceRef: "formal", Kind: programindex.ObjectFunction, Name: "register", Visibility: programindex.VisibilityInternal, Location: location(5)},
			{SourceRef: "get", Kind: programindex.ObjectExternalSymbol, Name: "router.get", Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "router", Receiver: "router", Name: "get"}},
			{SourceRef: "path-constant", Kind: programindex.ObjectVariable, Name: "dynamicPath", Visibility: programindex.VisibilityInternal, Location: location(4)},
		},
		Relations: []programindex.RelationInput{
			{
				SourceRef: "formal-use", Kind: programindex.RelationInvokesExternal, FromRef: "formal", ToRefs: []string{"get"},
				Resolution: programindex.ResolutionExact, TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "syntax", Location: location(6)}}, WitnessesObserved: 1,
				PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
					SourceRef: "formal-use-pattern", Form: programindex.PatternCall, Selector: "get", Location: location(6),
					ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{
						Position: 1, Kind: programindex.PatternDynamic, ValueCandidatesObserved: 2,
						ValueCandidates: []programindex.PatternValueCandidateInput{
							{Kind: programindex.PatternLiteralString, Value: "/api/dynamic", Resolution: programindex.PatternValuePossible, SourceKind: programindex.PatternValueSourceInitializer, SourceObjectRefs: []string{"path-constant"}, SourceObjectsObserved: 1},
							{Kind: programindex.PatternLiteralString, Value: "/products/runtime", Resolution: programindex.PatternValuePossible, SourceKind: programindex.PatternValueSourceActualArgument, SourceArgumentRefs: []programindex.PatternArgumentRefInput{{RelationSourceRef: "actual-call", PatternSourceRef: "actual-pattern", Position: 1}}, SourceArgumentsObserved: 1},
						},
					}},
				}},
			},
			{
				SourceRef: "actual-call", Kind: programindex.RelationCalls, FromRef: "caller", ToRefs: []string{"formal"},
				Resolution: programindex.ResolutionExact, TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "syntax", Location: location(2)}}, WitnessesObserved: 1,
				PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
					SourceRef: "actual-pattern", Form: programindex.PatternCall, Selector: "register", Location: location(2),
					ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{Position: 1, Kind: programindex.PatternLiteralString, Value: "/products/runtime"}},
				}},
			},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 2},
	})
	if err != nil {
		t.Fatalf("programindex.New value provenance: %v", err)
	}
	var formalID string
	for _, object := range base.Objects {
		if object.SourceRef == "formal" {
			formalID = object.ID
		}
	}
	enriched, err := programindex.Enrich(base, strings.Repeat("9", 64), []programindex.CategoryAssignment{{
		SubjectID: formalID, Categories: []programindex.Category{programindex.CategoryCore},
	}})
	if err != nil {
		t.Fatalf("programindex.Enrich value provenance: %v", err)
	}
	return enriched
}

func hasValueSourceStructuralEdge(
	edges []StructuralEdge,
	role StructuralEdgeRole,
	fromID, toID, argumentID, candidateID, sourceArgumentID string,
) bool {
	for _, edge := range edges {
		if edge.Role == role && edge.FromSubjectID == fromID && edge.ToSubjectID == toID &&
			edge.ArgumentID == argumentID && edge.ValueCandidateID == candidateID &&
			edge.SourceArgumentID == sourceArgumentID {
			return true
		}
	}
	return false
}

func testBaseProgramIndex(t *testing.T, selector string) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: selector + "/main.go", Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "inbound", Kind: programindex.ObjectFunction, Name: "Serve", Visibility: programindex.VisibilityPublic, Signature: "func Serve()", Location: location(1)},
		{SourceRef: "background", Kind: programindex.ObjectFunction, Name: "Consume", Visibility: programindex.VisibilityInternal, ContainerRef: "inbound", Location: location(2)},
		{SourceRef: "core", Kind: programindex.ObjectFunction, Name: "Apply", Visibility: programindex.VisibilityInternal, OwnerRef: "inbound", Location: location(3)},
		{
			SourceRef: "dependency", Kind: programindex.ObjectExternalSymbol, Name: "queue.Publish", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.com/queue", Name: "Publish"}, Location: location(4),
			SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{Domain: "go", Parts: []string{"example.com/queue", "Publish"}, Display: "queue.Publish"}},
		},
		{
			SourceRef: "platform", Kind: programindex.ObjectExternalSymbol,
			Name: "platform:javascript.requestAnimationFrame", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPlatform,
				PackagePath:   "platform:javascript", Name: "requestAnimationFrame",
			},
			Location: location(5),
		},
		{SourceRef: "ungrouped", Kind: programindex.ObjectFunction, Name: "DebugOnly", Visibility: programindex.VisibilityInternal, Location: location(6)},
	}
	relations := []programindex.RelationInput{{
		SourceRef: "dispatch", Kind: programindex.RelationCalls, FromRef: "inbound", ToRefs: []string{"core"},
		Resolution: programindex.ResolutionExact, Invocation: "sync", Location: location(10), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(10)}}, WitnessesObserved: 1,
		PatternsObserved: 1,
		Patterns: []programindex.RelationPatternInput{{
			SourceRef: "dispatch-pattern", Form: programindex.PatternCall, Selector: "dispatch", Location: location(10),
			ResultRef: "core", ReceiverRef: "ungrouped", ReceiverOriginRefs: []string{"dependency"},
			ReceiverOriginResolution: programindex.ResolutionExact, ReceiverOriginsObserved: 1,
			ArgumentsObserved: 3,
			Arguments: []programindex.PatternArgumentInput{
				{Position: 1, Kind: programindex.PatternLiteralString, Value: "/api/items"},
				{Position: 2, Kind: programindex.PatternStringTemplate, Parts: []programindex.PatternPartInput{
					{Kind: programindex.PatternPartLiteral, Text: "/orders/"}, {Kind: programindex.PatternPartHole},
				}},
				{Keyword: "handler", Kind: programindex.PatternDynamic, ObjectRefs: []string{"background"}, Resolution: programindex.ResolutionAlternatives, ObjectsObserved: 2},
			},
		}},
	}, {
		SourceRef: "platform-call", Kind: programindex.RelationInvokesExternal,
		FromRef: "background", ToRefs: []string{"platform"}, Resolution: programindex.ResolutionExact,
		Location: location(20), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(20)}}, WitnessesObserved: 1,
		PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
			SourceRef: "platform-pattern", Form: programindex.PatternCall,
			Selector: "requestAnimationFrame", Location: location(20),
			Arguments: []programindex.PatternArgumentInput{}, ArgumentsObserved: 0,
		}},
	}}
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "package", Name: selector, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: selector + "/main.go"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "inbound", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return base
}

func enrichTestProgramIndex(t *testing.T, base programindex.Index) programindex.Index {
	t.Helper()
	ids := testSubjectIDs(t, base)
	enriched, err := programindex.Enrich(base, strings.Repeat("d", 64), []programindex.CategoryAssignment{
		{SubjectID: ids["inbound"], Categories: []programindex.Category{programindex.CategoryInbound}},
		{SubjectID: ids["background"], Categories: []programindex.Category{programindex.CategoryBackgroundActivity}},
		{SubjectID: ids["core"], Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: ids["dependency"], Categories: []programindex.Category{programindex.CategoryDependency}},
		{SubjectID: ids["pattern"], Categories: []programindex.Category{programindex.CategoryInbound, programindex.CategoryCore}},
		{SubjectID: ids["platform"], Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: ids["platform-pattern"], Categories: []programindex.Category{programindex.CategoryBackgroundActivity}},
	})
	if err != nil {
		t.Fatalf("programindex.Enrich: %v", err)
	}
	return enriched
}

func testSubjectIDs(t *testing.T, index programindex.Index) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, object := range index.Objects {
		result[object.SourceRef] = object.ID
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			result[pattern.SourceRef] = pattern.ID
			if pattern.SourceRef == "dispatch-pattern" {
				result["pattern"] = pattern.ID
			}
		}
	}
	return result
}

func testProposals(ids map[string]string) Proposals {
	result := testProposalsWithoutConnections(ids)
	result.Connections = []ConnectionProposal{
		{
			FromGroupKey: "triggers", ToGroupKey: "core", SemanticKind: "awakens_domain_flow",
			Label: "Awakens", Summary: "Execution triggers enter the core flow.", EvidenceSubjectIDs: []string{ids["pattern"]},
		},
		{
			FromGroupKey: "triggers", ToGroupKey: "core", SemanticKind: "awakens_domain_flow",
			Label: "Awakens", Summary: "Execution triggers enter the core flow.", EvidenceSubjectIDs: []string{ids["pattern"]},
		},
	}
	return result
}

func testProposalsWithoutConnections(ids map[string]string) Proposals {
	return Proposals{Groups: []GroupProposal{
		{
			Key: "triggers", Title: "Execution triggers", Summary: "Inbound and background execution starts here.", Lane: LaneTriggers,
			MemberSubjectIDs: []string{ids["background"], ids["inbound"], ids["pattern"]}, EvidenceSubjectIDs: []string{ids["core"]},
		},
		{
			Key: "core", Title: "Core flow", Summary: "The main product work.", Lane: LaneCore,
			MemberSubjectIDs: []string{ids["core"], ids["pattern"]}, EvidenceSubjectIDs: []string{ids["inbound"]},
		},
		{
			Key: "dependencies", Title: "Queue dependency", Summary: "External queue boundary and its local caller.", Lane: LaneDependencies,
			MemberSubjectIDs: []string{ids["dependency"]}, EvidenceSubjectIDs: []string{ids["core"], ids["pattern"]},
		},
	}}
}

func groupByTitle(t *testing.T, groups []Group, title string) Group {
	t.Helper()
	for _, group := range groups {
		if group.Title == title {
			return group
		}
	}
	t.Fatalf("group %q not found in %#v", title, groups)
	return Group{}
}

func subjectByID(t *testing.T, subjects []Subject, id string) Subject {
	t.Helper()
	for _, subject := range subjects {
		if subject.ID == id {
			return subject
		}
	}
	t.Fatalf("subject %q not found", id)
	return Subject{}
}

func hasSubject(subjects []Subject, id string) bool {
	for _, subject := range subjects {
		if subject.ID == id {
			return true
		}
	}
	return false
}

func hasStructuralEdge(edges []StructuralEdge, role StructuralEdgeRole, fromID, toID string) bool {
	for _, edge := range edges {
		if edge.Role == role && edge.FromSubjectID == fromID && edge.ToSubjectID == toID {
			return true
		}
	}
	return false
}

func subjectIsGrouped(groups []Group, id string) bool {
	for _, group := range groups {
		if containsString(group.MemberSubjectIDs, id) {
			return true
		}
	}
	return false
}
