package sourceexplain

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
)

func TestBuildSeedsBoundedQuestionsAndActions(t *testing.T) {
	t.Parallel()

	bundle, err := Build(structuralFixture(), sourceCardFixture())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantPredicates := []Predicate{
		PredicateValidatesInput,
		PredicateDelegatesOperation,
		PredicateMapsError,
		PredicateFillsResponse,
	}
	if len(bundle.Questions) != len(wantPredicates) {
		t.Fatalf("questions = %d, want %d: %#v", len(bundle.Questions), len(wantPredicates), bundle.Questions)
	}
	for index, want := range wantPredicates {
		if bundle.Questions[index].Predicate != want {
			t.Fatalf("questions[%d].predicate = %q, want %q", index, bundle.Questions[index].Predicate, want)
		}
	}
	if got := bundle.Questions[0].CandidateSourceEvidenceIDs; !equalStrings(got, []string{"source-91"}) {
		t.Fatalf("validation candidates = %#v", got)
	}
	if got := bundle.Questions[2].CandidateSourceEvidenceIDs; !equalStrings(got, []string{"source-96", "source-97"}) {
		t.Fatalf("error candidates = %#v", got)
	}
	if len(bundle.AllowedActions) != 5 || bundle.AllowedActions[0].Operation != OperationFindTests {
		t.Fatalf("allowed actions = %#v", bundle.AllowedActions)
	}
	if bundle.Source.Complete || bundle.Source.FileSHA256 == "" || bundle.Source.Window != sourceCardFixture().Window {
		t.Fatalf("source = %#v", bundle.Source)
	}
}

func TestBuildFindsMinimalMultilineValidationProof(t *testing.T) {
	t.Parallel()

	structural, card := labelsIsValidFixture()
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(bundle.Questions) != 1 {
		t.Fatalf("questions = %#v", bundle.Questions)
	}
	want := []string{"source-119", "source-132"}
	if got := bundle.Questions[0].CandidateSourceEvidenceIDs; !equalStrings(got, want) {
		t.Fatalf("validation candidates = %#v, want %#v", got, want)
	}
}

func TestBuildFindsCompactValidationProofShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*sourcecard.Card)
		want   []string
	}{
		{
			name: "direct condition",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\tif ls.Validate() {")
			},
			want: []string{"source-119"},
		},
		{
			name: "separate condition",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\terr := ls.Validate()")
				setSourceLine(card, 120, "\tif err != nil {")
			},
			want: []string{"source-119", "source-120"},
		},
		{
			name: "same line return",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\terr := ls.Validate(); return err == nil")
			},
			want: []string{"source-119"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			structural, card := labelsIsValidFixture()
			test.mutate(&card)
			card.Window.IncludedBytes = includedBytes(card.Lines)
			bundle, err := Build(structural, card)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := bundle.Questions[0].CandidateSourceEvidenceIDs; !equalStrings(got, test.want) {
				t.Fatalf("validation candidates = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildDoesNotGuessNonImmediateValidationProof(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*sourcecard.Card)
	}{
		{
			name: "result reassigned",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 132, "\terr = nil")
			},
		},
		{
			name: "other result returned",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 132, "\treturn other == nil")
			},
		},
		{
			name: "intervening statement",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 132, "\tfmt.Println(err)")
			},
		},
		{
			name: "discarded call in if initializer",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\tif ls.Validate(); ready {")
			},
		},
		{
			name: "unrelated assignment before call",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\terr := other(); ls.Validate()")
			},
		},
		{
			name: "multiple right hand sides",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\terr, other := otherCall(), ls.Validate()")
			},
		},
		{
			name: "transformed call result",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 119, "\terr := !ls.Validate()")
			},
		},
		{
			name: "split nil comparison",
			mutate: func(card *sourcecard.Card) {
				setSourceLine(card, 132, "\treturn err ==")
				setSourceLine(card, 133, "\t\tnil")
			},
		},
		{
			name: "truncated call",
			mutate: func(card *sourcecard.Card) {
				for index := range card.Lines {
					if card.Lines[index].Line == 131 {
						card.Lines[index].Truncated = true
					}
				}
				card.Window.Truncated = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			structural, card := labelsIsValidFixture()
			test.mutate(&card)
			card.Window.IncludedBytes = includedBytes(card.Lines)
			bundle, err := Build(structural, card)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := bundle.Questions[0].CandidateSourceEvidenceIDs; !equalStrings(got, []string{"source-119"}) {
				t.Fatalf("validation candidates = %#v", got)
			}
		})
	}
}

func TestBuildSeedsSwitchBranchCallQuestionsWithoutNameHints(t *testing.T) {
	t.Parallel()

	structural, card := sourceShapeFixture(
		"client.processInboundMsg",
		[]string{
			"func (c *client) processInboundMsg(msg []byte) {",
			"\tswitch c.kind {",
			"\tcase CLIENT:",
			"\t\tc.processInboundClientMsg(msg)",
			"\tcase ROUTER:",
			"\t\tc.processInboundRoutedMsg(msg)",
			"\tcase GATEWAY:",
			"\t\tc.processInboundGatewayMsg(msg)",
			"\tcase LEAF:",
			"\t\tc.processInboundLeafMsg(msg)",
			"\t}",
			"}",
		},
		[]sourceShapeCall{
			{id: "call-out-client", callee: "processInboundClientMsg", lineOffset: 3},
			{id: "call-out-router", callee: "processInboundRoutedMsg", lineOffset: 5},
			{id: "call-out-gateway", callee: "processInboundGatewayMsg", lineOffset: 7},
			{id: "call-out-leaf", callee: "processInboundLeafMsg", lineOffset: 9},
		},
	)
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(bundle.Questions) != 4 {
		t.Fatalf("questions = %#v", bundle.Questions)
	}
	for index, question := range bundle.Questions {
		if question.Predicate != PredicateCallsFromBranch {
			t.Fatalf("questions[%d].predicate = %q", index, question.Predicate)
		}
		anchorLine := shapeFixtureStartLine + 3 + index*2
		wantEvidence := []string{
			"source-" + strconv.Itoa(anchorLine-1),
			"source-" + strconv.Itoa(anchorLine),
		}
		if !equalStrings(question.CandidateSourceEvidenceIDs, wantEvidence) {
			t.Fatalf(
				"questions[%d].candidate_source_evidence_ids = %#v, want %#v",
				index,
				question.CandidateSourceEvidenceIDs,
				wantEvidence,
			)
		}
	}
}

func TestBuildSeedsCheckedAndReturnedCallQuestionsWithoutNameHints(t *testing.T) {
	t.Parallel()

	structural, card := sourceShapeFixture(
		"runCommand.runAnalysis",
		[]string{
			"func (c *runCommand) runAnalysis(ctx context.Context) ([]*result.Issue, error) {",
			"\tlintersToRun, err := c.dbManager.GetOptimizedLinters()",
			"\tif err != nil {",
			"\t\treturn nil, err",
			"\t}",
			"\tlintCtx, err := c.contextBuilder.Build(ctx)",
			"\tif err != nil {",
			"\t\treturn nil, err",
			"\t}",
			"\trunner, err := lint.NewRunner(lintCtx)",
			"\tif err != nil {",
			"\t\treturn nil, err",
			"\t}",
			"\treturn runner.Run(ctx, lintersToRun)",
			"}",
		},
		[]sourceShapeCall{
			{id: "call-out-linters", callee: "GetOptimizedLinters", lineOffset: 1},
			{id: "call-out-context", callee: "Build", lineOffset: 5},
			{id: "call-out-runner", callee: "NewRunner", lineOffset: 9},
			{id: "call-out-run", callee: "Run", lineOffset: 13},
		},
	)
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	want := []struct {
		predicate Predicate
		evidence  []string
	}{
		{
			predicate: PredicateChecksCallResult,
			evidence:  []string{"source-91", "source-92"},
		},
		{
			predicate: PredicateChecksCallResult,
			evidence:  []string{"source-95", "source-96"},
		},
		{
			predicate: PredicateChecksCallResult,
			evidence:  []string{"source-99", "source-100"},
		},
		{
			predicate: PredicateReturnsCallResult,
			evidence:  []string{"source-103"},
		},
	}
	if len(bundle.Questions) != len(want) {
		t.Fatalf("questions = %#v", bundle.Questions)
	}
	for index, expected := range want {
		question := bundle.Questions[index]
		if question.Predicate != expected.predicate ||
			!equalStrings(question.CandidateSourceEvidenceIDs, expected.evidence) {
			t.Fatalf(
				"questions[%d] = %#v, want predicate %q and evidence %#v",
				index,
				question,
				expected.predicate,
				expected.evidence,
			)
		}
	}
}

func TestBuildDoesNotSeedSyntaxQuestionsWithoutCompleteLocalProof(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       []string
		callOffset int
	}{
		{
			name: "plain call outside branch",
			body: []string{
				"func (s *server) Work() {",
				"\ts.observe()",
				"}",
			},
			callOffset: 1,
		},
		{
			name: "comment looks like case",
			body: []string{
				"func (s *server) Work() {",
				"\t// case ACTIVE:",
				"\ts.observe()",
				"}",
			},
			callOffset: 2,
		},
		{
			name: "checked result is not immediate",
			body: []string{
				"func (s *server) Work() error {",
				"\terr := s.observe()",
				"\tfmt.Println(err)",
				"\tif err != nil {",
				"\t\treturn err",
				"\t}",
				"\treturn nil",
				"}",
			},
			callOffset: 1,
		},
		{
			name: "wrapped return",
			body: []string{
				"func (s *server) Work() error {",
				"\treturn wrap(s.observe())",
				"}",
			},
			callOffset: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			structural, card := sourceShapeFixture(
				"server.Work",
				test.body,
				[]sourceShapeCall{{id: "call-out-observe", callee: "observe", lineOffset: test.callOffset}},
			)
			_, err := Build(structural, card)
			if err == nil || !strings.Contains(err.Error(), "no source questions") {
				t.Fatalf("Build() error = %v, want no source questions", err)
			}
		})
	}
}

func TestBundleValidateRejectsForgedSourceMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*Bundle)
		wantError string
	}{
		{
			name: "invalid sha-256",
			mutate: func(bundle *Bundle) {
				bundle.Source.FileSHA256 = "fixture-sha"
			},
			wantError: "sha-256",
		},
		{
			name: "lexical window marked complete",
			mutate: func(bundle *Bundle) {
				bundle.Source.Complete = true
			},
			wantError: "cannot be marked complete",
		},
		{
			name: "stop reason mismatch",
			mutate: func(bundle *Bundle) {
				bundle.Source.StopReason = sourcecard.StopEndOfFile
			},
			wantError: "does not match window metadata",
		},
		{
			name: "target outside window",
			mutate: func(bundle *Bundle) {
				bundle.Source.Window.StartLine++
			},
			wantError: "does not start at the target line",
		},
		{
			name: "noncontiguous lines",
			mutate: func(bundle *Bundle) {
				bundle.Source.Lines[1].Line++
				bundle.Source.Lines[1].EvidenceID = "source-93"
			},
			wantError: "not contiguous",
		},
		{
			name: "invalid evidence id",
			mutate: func(bundle *Bundle) {
				bundle.Source.Lines[1].EvidenceID = "line-91"
			},
			wantError: "invalid evidence id",
		},
		{
			name: "incorrect included bytes",
			mutate: func(bundle *Bundle) {
				bundle.Source.Window.IncludedBytes++
			},
			wantError: "byte metadata",
		},
		{
			name: "oversized line",
			mutate: func(bundle *Bundle) {
				bundle.Source.Lines[1].Text = strings.Repeat("x", maxProviderSourceLineBytes+1)
				bundle.Source.Window.IncludedBytes = includedBytes(bundle.Source.Lines)
			},
			wantError: "source line 91",
		},
		{
			name: "too many lines",
			mutate: func(bundle *Bundle) {
				extendSource(&bundle.Source, maxProviderSourceLines+1, "")
			},
			wantError: "limit is 160",
		},
		{
			name: "too many bytes",
			mutate: func(bundle *Bundle) {
				extendSource(&bundle.Source, maxProviderSourceLines, strings.Repeat("x", 256))
			},
			wantError: "exceeds provider-safe bounds",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle, err := Build(structuralFixture(), sourceCardFixture())
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&bundle)
			if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestBuildSkipsUnclassifiedAndTruncatedCalls(t *testing.T) {
	t.Parallel()

	structural := structuralFixture()
	structural.OutgoingCalls = append(structural.OutgoingCalls, callFact("call-out-005", "frobnicate", 101))
	card := sourceCardFixture()
	for index := range card.Lines {
		if card.Lines[index].Line == 100 {
			card.Lines[index].Truncated = true
		}
	}
	card.Window.Truncated = true
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(bundle.Questions) != 3 {
		t.Fatalf("questions = %#v", bundle.Questions)
	}
	for _, question := range bundle.Questions {
		if question.AnchorEvidenceID == "call-out-001" || question.AnchorEvidenceID == "call-out-005" {
			t.Fatalf("unexpected question = %#v", question)
		}
	}
}

func TestBuildRejectsTargetMismatch(t *testing.T) {
	t.Parallel()

	card := sourceCardFixture()
	card.Target.Path = "other.go"
	if _, err := Build(structuralFixture(), card); err == nil {
		t.Fatal("Build() error = nil")
	}
}

func TestClassifyCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		callee    string
		predicate Predicate
		ok        bool
	}{
		{name: "validation", target: "server.Put", callee: "checkPutRequest", predicate: PredicateValidatesInput, ok: true},
		{name: "error", target: "server.Put", callee: "togRPCError", predicate: PredicateMapsError, ok: true},
		{name: "response", target: "server.Put", callee: "fill", predicate: PredicateFillsResponse, ok: true},
		{name: "delegation", target: "server.Put", callee: "Put", predicate: PredicateDelegatesOperation, ok: true},
		{name: "persistence", target: "WAL.Save", callee: "saveEntry", predicate: PredicatePersistsState, ok: true},
		{name: "io", target: "WAL.Save", callee: "sync", predicate: PredicatePerformsIO, ok: true},
		{name: "ambiguous", target: "server.Put", callee: "observe", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			predicate, ok := classifyCall(test.target, test.callee)
			if ok != test.ok || predicate != test.predicate {
				t.Fatalf("classifyCall() = %q, %v; want %q, %v", predicate, ok, test.predicate, test.ok)
			}
		})
	}
}

const shapeFixtureStartLine = 90

type sourceShapeCall struct {
	id         string
	callee     string
	lineOffset int
}

func sourceShapeFixture(
	targetName string,
	texts []string,
	calls []sourceShapeCall,
) (symbol.Bundle, sourcecard.Card) {
	target := evidence.Entity{
		ID:   "shape-target",
		Kind: evidence.EntityMethod,
		Name: targetName,
		Location: &evidence.Location{
			Path:   "server/shape.go",
			Line:   shapeFixtureStartLine,
			Column: 1,
		},
	}
	outgoing := make([]symbol.CallFact, 0, len(calls))
	for _, call := range calls {
		line := shapeFixtureStartLine + call.lineOffset
		outgoing = append(outgoing, symbol.CallFact{
			EvidenceID: call.id,
			Caller:     target,
			Callee: evidence.Entity{
				ID:       "callee-" + call.id,
				Kind:     evidence.EntityMethod,
				Name:     call.callee,
				Location: &evidence.Location{Path: "server/dependency.go", Line: line + 100},
			},
			Callsite:  &evidence.Location{Path: target.Location.Path, Line: line, Column: 2},
			Certainty: evidence.CertaintyStatic,
		})
	}
	structural := symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "shape-repo",
		Query:    targetName,
		Target: symbol.Fact{
			EvidenceID: "resolution-001",
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
		},
		OutgoingCalls: outgoing,
		AllowedPaths:  []string{target.Location.Path},
		Truncated:     map[string]int{},
	}
	lines := make([]sourcecard.Line, 0, len(texts))
	for index, text := range texts {
		line := shapeFixtureStartLine + index
		lines = append(lines, sourcecard.Line{
			EvidenceID: "source-" + strconv.Itoa(line),
			Line:       line,
			Text:       text,
		})
	}
	card := sourcecard.Card{
		Version:    sourcecard.Version,
		Language:   "go",
		RepoName:   structural.RepoName,
		FileSHA256: strings.Repeat("c", 64),
		Target: sourcecard.Target{
			EvidenceID: structural.Target.EvidenceID,
			EntityID:   target.ID,
			Name:       target.Name,
			Kind:       target.Kind,
			Path:       target.Location.Path,
			Line:       target.Location.Line,
			Column:     target.Location.Column,
		},
		Window: sourcecard.Window{
			StartLine:     shapeFixtureStartLine,
			EndLine:       shapeFixtureStartLine + len(lines) - 1,
			IncludedBytes: includedBytes(lines),
			StopReason:    sourcecard.StopNextTopLevelFunc,
		},
		Lines: lines,
	}
	return structural, card
}

func structuralFixture() symbol.Bundle {
	target := evidence.Entity{
		ID:   "target-entity",
		Kind: evidence.EntityMethod,
		Name: "kvServer.Put",
		Location: &evidence.Location{
			Path:   "server/key.go",
			Line:   90,
			Column: 20,
		},
	}
	return symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "etcd",
		Query:    "kvServer.Put",
		Target: symbol.Fact{
			EvidenceID: "resolution-001",
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
		},
		OutgoingCalls: []symbol.CallFact{
			callFact("call-out-001", "fill", 100),
			callFact("call-out-002", "checkPutRequest", 91),
			callFact("call-out-003", "togRPCError", 97),
			callFact("call-out-004", "Put", 95),
		},
		AllowedPaths: []string{"server/key.go"},
		Warnings:     []string{},
		Truncated:    map[string]int{},
	}
}

func callFact(id, callee string, line int) symbol.CallFact {
	return symbol.CallFact{
		EvidenceID: id,
		Caller: evidence.Entity{
			ID:       "target-entity",
			Kind:     evidence.EntityMethod,
			Name:     "kvServer.Put",
			Location: &evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
		},
		Callee: evidence.Entity{
			ID:       "callee-" + id,
			Kind:     evidence.EntityFunction,
			Name:     callee,
			Location: &evidence.Location{Path: "server/dependency.go", Line: line + 100},
		},
		Callsite:  &evidence.Location{Path: "server/key.go", Line: line, Column: 2},
		Certainty: evidence.CertaintyStatic,
	}
}

func sourceCardFixture() sourcecard.Card {
	texts := map[int]string{
		90:  "func (s *kvServer) Put(ctx context.Context, r *PutRequest) (*PutResponse, error) {",
		91:  "\tif err := checkPutRequest(r); err != nil {",
		92:  "\t\treturn nil, err",
		93:  "\t}",
		94:  "",
		95:  "\tresp, err := s.kv.Put(ctx, r)",
		96:  "\tif err != nil {",
		97:  "\t\treturn nil, togRPCError(err)",
		98:  "\t}",
		99:  "",
		100: "\ts.hdr.fill(resp.Header)",
		101: "\treturn resp, nil",
		102: "}",
		103: "",
	}
	lines := make([]sourcecard.Line, 0, len(texts))
	for line := 90; line <= 103; line++ {
		lines = append(lines, sourcecard.Line{
			EvidenceID: "source-" + strconv.Itoa(line),
			Line:       line,
			Text:       texts[line],
		})
	}
	return sourcecard.Card{
		Version:    sourcecard.Version,
		Language:   "go",
		RepoName:   "etcd",
		FileSHA256: strings.Repeat("a", 64),
		Target: sourcecard.Target{
			EvidenceID: "resolution-001",
			EntityID:   "target-entity",
			Name:       "kvServer.Put",
			Kind:       evidence.EntityMethod,
			Path:       "server/key.go",
			Line:       90,
			Column:     20,
		},
		Window: sourcecard.Window{
			StartLine:     90,
			EndLine:       103,
			IncludedBytes: includedBytes(lines),
			StopReason:    sourcecard.StopNextTopLevelFunc,
		},
		Lines:    lines,
		Warnings: []sourcecard.Warning{},
	}
}

func labelsIsValidFixture() (symbol.Bundle, sourcecard.Card) {
	target := evidence.Entity{
		ID:   "labels-is-valid",
		Kind: evidence.EntityMethod,
		Name: "Labels.IsValid",
		Location: &evidence.Location{
			Path:   "model/labels/labels_common.go",
			Line:   118,
			Column: 18,
		},
	}
	structural := symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "prometheus",
		Query:    target.Name,
		Target: symbol.Fact{
			EvidenceID: "resolution-001",
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
		},
		OutgoingCalls: []symbol.CallFact{{
			EvidenceID: "call-out-001",
			Caller:     target,
			Callee: evidence.Entity{
				ID:       "labels-validate",
				Kind:     evidence.EntityMethod,
				Name:     "Validate",
				Location: &evidence.Location{Path: "model/labels/labels.go", Line: 400},
			},
			Callsite:  &evidence.Location{Path: target.Location.Path, Line: 119, Column: 12},
			Certainty: evidence.CertaintyStatic,
		}},
		AllowedPaths: []string{target.Location.Path},
		Truncated:    map[string]int{},
	}
	texts := []string{
		"func (ls Labels) IsValid(validationScheme model.ValidationScheme) bool {",
		"\terr := ls.Validate(func(l Label) error {",
		"\t\tif l.Name == model.MetricNameLabel {",
		"\t\t\t// If the default validation scheme has been overridden with legacy mode,",
		"\t\t\t// use the legacy validation checker.",
		"\t\t\tif !validationScheme.IsValidMetricName(l.Value) {",
		"\t\t\t\treturn strconv.ErrSyntax",
		"\t\t\t}",
		"\t\t}",
		"\t\tif !validationScheme.IsValidLabelName(l.Name) {",
		"\t\t\treturn strconv.ErrSyntax",
		"\t\t}",
		"\t\treturn nil",
		"\t})",
		"\treturn err == nil",
		"}",
	}
	lines := make([]sourcecard.Line, 0, len(texts))
	for index, text := range texts {
		line := 118 + index
		lines = append(lines, sourcecard.Line{
			EvidenceID: "source-" + strconv.Itoa(line),
			Line:       line,
			Text:       text,
		})
	}
	card := sourcecard.Card{
		Version:    sourcecard.Version,
		Language:   "go",
		RepoName:   "prometheus",
		FileSHA256: strings.Repeat("b", 64),
		Target: sourcecard.Target{
			EvidenceID: "resolution-001",
			EntityID:   target.ID,
			Name:       target.Name,
			Kind:       target.Kind,
			Path:       target.Location.Path,
			Line:       target.Location.Line,
			Column:     target.Location.Column,
		},
		Window: sourcecard.Window{
			StartLine:     118,
			EndLine:       133,
			IncludedBytes: includedBytes(lines),
			StopReason:    sourcecard.StopNextTopLevelFunc,
		},
		Lines: lines,
	}
	return structural, card
}

func setSourceLine(card *sourcecard.Card, line int, text string) {
	for index := range card.Lines {
		if card.Lines[index].Line == line {
			card.Lines[index].Text = text
			return
		}
	}
}

func includedBytes(lines []sourcecard.Line) int {
	total := 0
	for index, line := range lines {
		if index > 0 {
			total++
		}
		total += len(line.Text)
	}
	return total
}

func extendSource(source *Source, lineCount int, text string) {
	for len(source.Lines) < lineCount {
		lineNumber := source.Lines[len(source.Lines)-1].Line + 1
		source.Lines = append(source.Lines, sourcecard.Line{
			EvidenceID: "source-" + strconv.Itoa(lineNumber),
			Line:       lineNumber,
			Text:       text,
		})
	}
	source.Window.EndLine = source.Lines[len(source.Lines)-1].Line
	source.Window.IncludedBytes = includedBytes(source.Lines)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
