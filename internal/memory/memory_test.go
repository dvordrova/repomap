package memory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

func TestSaveAndLoadSeparateFactsClaimsAndSession(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	dir := t.TempDir()

	path, err := Save(dir, Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, SessionFileName) {
		t.Fatalf("Save() path = %q", path)
	}

	sessionJSON, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(sessionJSON, &topLevel); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"symbol", "source", "assessment", "source_report", "tests"} {
		if _, exists := topLevel[forbidden]; exists {
			t.Fatalf("session document embeds %s", forbidden)
		}
	}
	var document sessionDocument
	if err := decodeStrict(sessionJSON, &document); err != nil {
		t.Fatal(err)
	}
	if document.FactsRef == nil || document.ClaimsRef == nil ||
		!contentPathPattern.MatchString(document.FactsRef.Path) || !contentPathPattern.MatchString(document.ClaimsRef.Path) {
		t.Fatalf("references = %#v / %#v", document.FactsRef, document.ClaimsRef)
	}

	loaded, err := Load(path, Current{Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, loaded.Session, session)
	if !reflect.DeepEqual(loaded.Repository, state) || loaded.Facts == nil || loaded.Claims == nil {
		t.Fatalf("loaded record = %#v", loaded)
	}
	if differences := freshness.CompareFactContext(*loaded.Facts, facts); len(differences) != 0 {
		t.Fatalf("fact differences = %#v", differences)
	}
	if loaded.Claims.FactDigest != document.FactsRef.SHA256 {
		t.Fatalf("claim fact digest = %q, want %q", loaded.Claims.FactDigest, document.FactsRef.SHA256)
	}
	for _, ref := range []*reference{document.FactsRef, document.ClaimsRef} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ref.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", ref.Path, info.Mode().Perm())
		}
	}
}

func TestLoadRejectsTamperedFactDocumentBeforeHydration(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	dir := t.TempDir()
	path, err := Save(dir, Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	var document sessionDocument
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrict(data, &document); err != nil {
		t.Fatal(err)
	}
	factPath := filepath.Join(dir, filepath.FromSlash(document.FactsRef.Path))
	if err := os.WriteFile(factPath, []byte(`{"tampered":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Current{Repository: state, Facts: &facts, Claims: &claims}); err == nil || !strings.Contains(err.Error(), "content hash mismatch") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsSymlinkedContentEvenWhenBytesMatch(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	dir := t.TempDir()
	path, err := Save(dir, Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	var document sessionDocument
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrict(data, &document); err != nil {
		t.Fatal(err)
	}
	factPath := filepath.Join(dir, filepath.FromSlash(document.FactsRef.Path))
	factData, err := os.ReadFile(factPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "matching-facts.json")
	if err := os.WriteFile(outside, factData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(factPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, factPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Current{Repository: state, Facts: &facts, Claims: &claims}); err == nil || !strings.Contains(err.Error(), "not regular") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRequiresContentPathToMatchDigest(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	dir := t.TempDir()
	path, err := Save(dir, Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	var document sessionDocument
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrict(data, &document); err != nil {
		t.Fatal(err)
	}
	document.FactsRef.Path = "facts/" + strings.Repeat("f", 64) + ".json"
	data, err = encodeDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Current{Repository: state, Facts: &facts, Claims: &claims}); err == nil || !strings.Contains(err.Error(), "invalid facts reference") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestSaveRejectsRepositoryAndLineageMismatches(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()

	t.Run("session revision", func(t *testing.T) {
		changed := session
		changed.Repository.Revision = strings.Repeat("f", 64)
		if _, err := Save(t.TempDir(), Input{Session: changed, Repository: state, Facts: &facts, Claims: &claims}); err == nil {
			t.Fatal("Save() error = nil")
		}
	})

	t.Run("fact repository", func(t *testing.T) {
		changedFacts := facts
		changedFacts.Repository.Head = strings.Repeat("b", 40)
		if _, err := Save(t.TempDir(), Input{Session: session, Repository: state, Facts: &changedFacts, Claims: &claims}); err == nil {
			t.Fatal("Save() error = nil")
		}
	})

	t.Run("claim fact digest", func(t *testing.T) {
		changedClaims := claims
		changedClaims.FactDigest = strings.Repeat("c", 64)
		if _, err := Save(t.TempDir(), Input{Session: session, Repository: state, Facts: &facts, Claims: &changedClaims}); err == nil {
			t.Fatal("Save() error = nil")
		}
	})
}

func TestLoadInvalidatesRepositoryFactsAndClaimsAtTheirOwnLayer(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	dir := t.TempDir()
	path, err := Save(dir, Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("repository content", func(t *testing.T) {
		currentRepository := state
		currentRepository.Dirty = []freshness.DirtyFile{{
			Status:        "modified",
			Path:          "main.go",
			Kind:          freshness.FileRegular,
			ContentSHA256: strings.Repeat("b", 64),
		}}
		loaded, err := Load(path, Current{Repository: currentRepository})
		if err != nil {
			t.Fatal(err)
		}
		currentRevision, err := currentRepository.Digest()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Session.State != investigation.StateResolvingSymbol || loaded.Session.Repository.Revision != currentRevision ||
			loaded.Session.Symbol != nil || loaded.Facts != nil || loaded.Claims != nil {
			t.Fatalf("loaded = %#v", loaded)
		}
		assertChangeReason(t, loaded.Changes, freshness.ReasonRepositoryDirty)
	})

	t.Run("fact inputs", func(t *testing.T) {
		currentFacts := facts
		currentFacts.InputsSHA256 = strings.Repeat("e", 64)
		loaded, err := Load(path, Current{Repository: state, Facts: &currentFacts, Claims: &claims})
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Session.State != investigation.StateResolvingSymbol || loaded.Session.Symbol != nil || loaded.Facts != nil || loaded.Claims != nil {
			t.Fatalf("loaded = %#v", loaded)
		}
		assertChangeReason(t, loaded.Changes, freshness.ReasonAnalysisInputs)
	})

	t.Run("claim evaluator", func(t *testing.T) {
		currentClaims := claims
		currentClaims.EvaluatorVersion++
		loaded, err := Load(path, Current{Repository: state, Facts: &facts, Claims: &currentClaims})
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Session.State != investigation.StateAssessingSource || loaded.Session.Symbol == nil || loaded.Session.Source == nil ||
			loaded.Session.Assessment == nil || loaded.Session.SourceReport != nil || loaded.Facts == nil || loaded.Claims != nil {
			t.Fatalf("loaded = %#v", loaded)
		}
		assertChangeReason(t, loaded.Changes, freshness.ReasonEvaluatorVersion)
	})
}

func TestLoadRequiresCurrentContextsForStoredLayers(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	session := assessedSession(t, state)
	facts := factContext(state)
	claims := claimContext()
	path, err := Save(t.TempDir(), Input{Session: session, Repository: state, Facts: &facts, Claims: &claims})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Current{Repository: state}); err == nil || !strings.Contains(err.Error(), "current fact context") {
		t.Fatalf("Load() missing facts error = %v", err)
	}
	if _, err := Load(path, Current{Repository: state, Facts: &facts}); err == nil || !strings.Contains(err.Error(), "current claim context") {
		t.Fatalf("Load() missing claims error = %v", err)
	}
}

func TestSaveAndLoadSessionWithoutFacts(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	revision, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "understand fixture"},
			Repository: investigation.Repository{Path: state.Identity, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: "fixture.Run"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := Save(t.TempDir(), Input{Session: session, Repository: state})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path, Current{Repository: state})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, loaded.Session, session)
	if loaded.Facts != nil || loaded.Claims != nil {
		t.Fatalf("unexpected contexts: %#v", loaded)
	}
}

func TestLoadStrictlyRejectsUnknownSessionFields(t *testing.T) {
	t.Parallel()

	state := repositoryState(t)
	revision, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "understand fixture"},
			Repository: investigation.Repository{Path: state.Identity, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: "fixture.Run"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := Save(t.TempDir(), Input{Session: session, Repository: state})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["unexpected"] = true
	data, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Current{Repository: state}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() error = %v", err)
	}
}

func assessedSession(t *testing.T, state freshness.RepositoryState) investigation.Session {
	t.Helper()
	revision, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	var card sourcecard.Card
	if err := json.Unmarshal(deepseektest.SourceCardJSON, &card); err != nil {
		t.Fatal(err)
	}
	structural := structuralForCard(card)
	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "understand " + structural.Query},
			Repository: investigation.Repository{Path: state.Identity, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: structural.Query},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session = reduce(t, session, investigation.Event{Kind: investigation.EventSymbolResolved, ActionID: session.Next[0].ID, Symbol: &structural})
	session = reduce(t, session, investigation.Event{Kind: investigation.EventSourceRead, ActionID: session.Next[0].ID, Source: &card})
	parsed, err := sourceexplain.ParseReport(*session.Assessment, deepseektest.SourceResponseJSON)
	if err != nil {
		t.Fatal(err)
	}
	report := parsed.Report
	return reduce(t, session, investigation.Event{Kind: investigation.EventSourceAssessed, ActionID: session.Next[0].ID, SourceReport: &report})
}

func structuralForCard(card sourcecard.Card) symbol.Bundle {
	target := evidence.Entity{
		ID:       card.Target.EntityID,
		Kind:     card.Target.Kind,
		Name:     card.Target.Name,
		Language: "go",
		Location: &evidence.Location{Path: card.Target.Path, Line: card.Target.Line, Column: card.Target.Column},
	}
	provenance := func(line int) []evidence.Provenance {
		return []evidence.Provenance{{
			Provider:  "gopls",
			Version:   "fixture",
			Operation: "call_hierarchy",
			Location:  &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
		}}
	}
	call := func(id, name string, line int) symbol.CallFact {
		return symbol.CallFact{
			EvidenceID: id,
			Caller:     target,
			Callee: evidence.Entity{
				ID:       "callee-" + id,
				Kind:     evidence.EntityFunction,
				Name:     name,
				Language: "go",
				Location: &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
			},
			Callsite:   &evidence.Location{Path: card.Target.Path, Line: line, Column: 2},
			Certainty:  evidence.CertaintyStatic,
			Provenance: provenance(line),
			Scenarios:  []string{"gopls-active-build"},
		}
	}
	return symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: card.RepoName,
		Query:    card.Target.Name,
		Target: symbol.Fact{
			EvidenceID: card.Target.EvidenceID,
			Entity:     target,
			Certainty:  evidence.CertaintyStatic,
			Provenance: provenance(card.Target.Line),
			Scenarios:  []string{"gopls-active-build"},
		},
		OutgoingCalls: []symbol.CallFact{
			call("call-out-001", "fill", 100),
			call("call-out-002", "checkPutRequest", 91),
			call("call-out-003", "togRPCError", 97),
			call("call-out-004", "Put", 95),
		},
		Scenarios:    []symbol.Scenario{{ID: "gopls-active-build"}},
		AllowedPaths: []string{card.Target.Path},
		Warnings:     []string{},
		Truncated:    map[string]int{},
	}
}

func reduce(t *testing.T, session investigation.Session, event investigation.Event) investigation.Session {
	t.Helper()
	next, _, err := investigation.Reduce(session, event)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func repositoryState(t *testing.T) freshness.RepositoryState {
	t.Helper()
	identity, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: identity,
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
}

func factContext(state freshness.RepositoryState) freshness.FactContext {
	return freshness.FactContext{
		Version:          freshness.FactContextVersion,
		Repository:       state,
		GoVersion:        "go1.24.0",
		Analyzer:         "gopls",
		AnalyzerVersion:  "v0.23.0",
		Collector:        "investigation-facts",
		CollectorVersion: "v1",
		InputsSHA256:     strings.Repeat("d", 64),
		Build: evidence.BuildContext{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	}
}

func claimContext() freshness.ClaimContext {
	return freshness.ClaimContext{
		Version:          freshness.ClaimContextVersion,
		Provider:         "openai-compatible",
		Model:            "deepseek-v4-flash",
		PromptVersion:    "source-assessment-json-v5",
		ParserVersion:    sourceexplain.ParserVersion,
		EvaluatorVersion: sourceexplain.EvaluationVersion,
	}
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("JSON differs\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}
}

func assertChangeReason(t *testing.T, differences []freshness.Difference, reason freshness.Reason) {
	t.Helper()
	for _, difference := range differences {
		if difference.Reason == reason {
			return
		}
	}
	t.Fatalf("differences = %#v, want %q", differences, reason)
}
