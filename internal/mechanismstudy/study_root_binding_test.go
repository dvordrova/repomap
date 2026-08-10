package mechanismstudy

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestStudyReadingRootBindingRestoresTelebotShapedFocusLines(t *testing.T) {
	index := analyzeSource(t, "gopkg.in/telebot.v3", `package telebot

type Bot struct{}

func NewBot() *Bot {
	help()
	return &Bot{}
}

func (b *Bot) Raw() {
	help()
}

func (b *Bot) Download() {
	help()
}

func (b *Bot) File() {
	help()
}

func help() {}
`)
	readings := []themestudy.Reading{
		telebotFocusedReading(t, index, "gopkg.in/telebot.v3.NewBot", "NewBot", "route-span-new-bot"),
		telebotFocusedReading(t, index, "gopkg.in/telebot.v3.(*Bot).Raw", "(*Bot).Raw", "route-span-raw"),
		telebotFocusedReading(t, index, "gopkg.in/telebot.v3.(*Bot).Download", "(*Bot).Download", "route-span-download"),
		telebotFocusedReading(t, index, "gopkg.in/telebot.v3.(*Bot).File", "(*Bot).File", "route-span-file"),
	}
	study := rootBindingStudy(readings)
	before := cloneStudyThemes(t, study)

	roots, err := BindStudyReadingRoots(study, index)
	if err != nil {
		t.Fatalf("BindStudyReadingRoots: %v", err)
	}
	if len(roots.Readings) != len(readings) {
		t.Fatalf("root bindings = %+v, want %d", roots.Readings, len(readings))
	}
	for _, reading := range readings {
		want := requireNodeBySymbol(t, index, canonicalTelebotSymbol(reading.Symbol))
		if got := rootNodeForSpan(roots, reading.CanonicalSpanID); got != want.ID {
			t.Fatalf("span %s root = %q, want %q", reading.CanonicalSpanID, got, want.ID)
		}
	}
	if !reflect.DeepEqual(study, before) {
		t.Fatal("root binding mutated the user-facing Study artifact")
	}

	compilation, err := CompileWithStudyReadingRoots(study, index, studyBinding(), roots)
	if err != nil {
		t.Fatalf("CompileWithStudyReadingRoots: %v", err)
	}
	if len(compilation.Cards) != 1 || len(compilation.Cards[0].Readings) != len(readings) {
		t.Fatalf("compiled cards = %+v", compilation.Cards)
	}
	for _, reading := range compilation.Cards[0].Readings {
		if reading.RootNodeRef == "" {
			t.Fatalf("focused reading remained unbound: %+v", compilation.Cards[0])
		}
	}
	if got := frontierCount(compilation.Cards[0], FrontierNoExactFunction); got != 0 {
		t.Fatalf("no_exact_function = %d after exact root binding", got)
	}
}

func TestStudyReadingRootBindingIgnoresSymbolSpellingAndIsPermutationStable(t *testing.T) {
	index := analyzeSource(t, "example.com/locator", `package locator

func Alpha() {
	help()
}

func Same() {
	help()
}

func help() {}
`)
	alpha := requireNodeBySymbol(t, index, "example.com/locator.Alpha")
	reading := themestudy.Reading{
		Label: "Misleading display symbol", Symbol: "Same",
		Path: alpha.Declaration.Path, Line: alpha.Declaration.Line + 1,
		Fit: themestudy.FitDirect, CanonicalSpanID: "route-span-alpha",
	}
	firstStudy := rootBindingStudy([]themestudy.Reading{reading, {
		Label: "Exact second", Symbol: "Same", Path: "main.go", Line: 7,
		Fit: themestudy.FitDirect, CanonicalSpanID: "route-span-same",
	}})
	secondStudy := cloneStudyThemes(t, firstStudy)
	slices.Reverse(secondStudy.Cards[0].Readings)

	first, err := BindStudyReadingRoots(firstStudy, index)
	if err != nil {
		t.Fatalf("BindStudyReadingRoots first: %v", err)
	}
	second, err := BindStudyReadingRoots(secondStudy, index)
	if err != nil {
		t.Fatalf("BindStudyReadingRoots second: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Study reading order changed canonical roots:\nfirst  %+v\nsecond %+v", first, second)
	}
	if got := rootNodeForSpan(first, reading.CanonicalSpanID); got != alpha.ID {
		t.Fatalf("misleading short symbol selected another declaration: got %q want %q", got, alpha.ID)
	}
}

func TestStudyReadingRootBindingFailsClosedForMissingAndOverlappingLocators(t *testing.T) {
	index := analyzeSource(t, "example.com/ambiguous", `package ambiguous

func entry() {
	nested := func() {
		help()
	}
	nested()
}

func help() {}
`)
	study := rootBindingStudy([]themestudy.Reading{
		{
			Label: "Nested body", Symbol: "entry", Path: "main.go", Line: 5,
			Fit: themestudy.FitDirect, CanonicalSpanID: "route-span-overlap",
		},
		{
			Label: "Missing", Symbol: "entry", Path: "missing.go", Line: 99,
			Fit: themestudy.FitDirect, CanonicalSpanID: "route-span-missing",
		},
	})
	roots, err := BindStudyReadingRoots(study, index)
	if err != nil {
		t.Fatalf("BindStudyReadingRoots: %v", err)
	}
	if len(roots.Readings) != 0 {
		t.Fatalf("ambiguous/missing locators bound roots: %+v", roots.Readings)
	}
	compilation, err := CompileWithStudyReadingRoots(study, index, studyBinding(), roots)
	if err != nil {
		t.Fatalf("CompileWithStudyReadingRoots: %v", err)
	}
	if got := frontierCount(compilation.Cards[0], FrontierNoExactFunction); got != 2 {
		t.Fatalf("no_exact_function = %d, want both locators closed", got)
	}
}

func TestCompileWithStudyReadingRootsRejectsIdentityAndAuthorityTamper(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	study := rootBindingStudy([]themestudy.Reading{{
		Label: "Entry", Symbol: "entry", Path: root.Declaration.Path,
		Line: root.Declaration.Line, Fit: themestudy.FitDirect,
		CanonicalSpanID: "route-span-entry",
	}})
	roots, err := BindStudyReadingRoots(study, index)
	if err != nil || len(roots.Readings) != 1 {
		t.Fatalf("BindStudyReadingRoots: roots=%+v err=%v", roots, err)
	}
	other := requireNodeBySymbol(t, index, "example.com/mechanism.service")
	tests := map[string]func(*StudyReadingRootBindings){
		"version": func(value *StudyReadingRootBindings) { value.Version++ },
		"revision": func(value *StudyReadingRootBindings) {
			value.RepositoryRevision = "ffffffffffffffffffffffffffffffffffffffff"
		},
		"index": func(value *StudyReadingRootBindings) {
			value.DirectCallIndexSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		},
		"scenario": func(value *StudyReadingRootBindings) { value.Scenario.GOOS = "not-the-index-scenario" },
		"span":     func(value *StudyReadingRootBindings) { value.Readings[0].CanonicalSpanID = "route-span-invented" },
		"node":     func(value *StudyReadingRootBindings) { value.Readings[0].NodeID = other.ID },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := cloneStudyRootBindings(roots)
			mutate(&tampered)
			if _, err := CompileWithStudyReadingRoots(study, index, studyBinding(), tampered); err == nil {
				t.Fatalf("CompileWithStudyReadingRoots accepted %s tamper", name)
			}
		})
	}
}

func TestStudyReadingRootBindingRemainsPrivateOnProviderWire(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	spanID := "route-span-private-entry"
	study := rootBindingStudy([]themestudy.Reading{{
		Label: "Entry", Symbol: "entry", Path: root.Declaration.Path,
		Line: root.Declaration.Line, Fit: themestudy.FitDirect, CanonicalSpanID: spanID,
	}})
	compilation, err := Compile(study, index, studyBinding())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil || len(batches) != 1 {
		t.Fatalf("BuildRequestBatches: batches=%d err=%v", len(batches), err)
	}
	wire, err := ProviderVisibleJSON(batches[0])
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	for _, private := range []string{spanID, root.ID, root.Symbol.ID, index.SHA256, fixtureRevision} {
		if strings.Contains(string(wire), private) {
			t.Fatalf("provider wire leaked private root binding %q: %s", private, wire)
		}
	}
}

func telebotFocusedReading(
	t *testing.T,
	index *surfacediscovery.DirectCallIndex,
	canonicalSymbol string,
	displaySymbol string,
	spanID string,
) themestudy.Reading {
	t.Helper()
	node := requireNodeBySymbol(t, index, canonicalSymbol)
	return themestudy.Reading{
		Label: displaySymbol, Symbol: displaySymbol, Path: node.Declaration.Path,
		Line: node.Declaration.Line + 1, Fit: themestudy.FitDirect, CanonicalSpanID: spanID,
	}
}

func canonicalTelebotSymbol(display string) string {
	if display == "NewBot" {
		return "gopkg.in/telebot.v3.NewBot"
	}
	return "gopkg.in/telebot.v3." + display
}

func rootBindingStudy(readings []themestudy.Reading) themestudy.StudyThemes {
	return themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: fixtureRevision,
		Cards: []themestudy.ThemeCard{{
			Ordinal: 1, CanonicalID: "root-binding-theme", FinalTitle: "Exact roots",
			FinalQuestion: "Which exact direct-call structure belongs to these readings?",
			Readings:      readings,
		}},
	}
}

func cloneStudyThemes(t *testing.T, source themestudy.StudyThemes) themestudy.StudyThemes {
	t.Helper()
	result := source
	result.Cards = append([]themestudy.ThemeCard(nil), source.Cards...)
	for index := range result.Cards {
		result.Cards[index].Readings = append([]themestudy.Reading(nil), source.Cards[index].Readings...)
	}
	return result
}

func cloneStudyRootBindings(source StudyReadingRootBindings) StudyReadingRootBindings {
	result := source
	result.Scenario.Tags = append([]string(nil), source.Scenario.Tags...)
	result.Readings = append([]StudyReadingRootBinding(nil), source.Readings...)
	return result
}

func rootNodeForSpan(roots StudyReadingRootBindings, spanID string) string {
	for _, root := range roots.Readings {
		if root.CanonicalSpanID == spanID {
			return root.NodeID
		}
	}
	return ""
}
