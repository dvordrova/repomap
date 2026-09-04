package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// pageView is the complete model of the static report page. It carries only
// reader-facing values: names, paths, lines, sentences. Target and group
// identities, digests, and adapter refs never reach the template.
type pageView struct {
	RepoName      string
	Revision      string
	ShortRevision string
	RepositoryURL string
	FormatVersion int
	ReportSHA256  string
	Served        bool
	// EditorOpens says whether a source link will actually reach an editor on
	// this machine. Promising one that is not installed is a small lie the
	// reader discovers only by clicking.
	EditorOpens   bool
	SourceIDsJSON template.JS
	CSS           template.CSS
	JS            template.JS

	Summary        *pageSentence
	SummaryMissing string
	// Figures are the few counts worth reading before anything else: how much
	// of the repository was read, how big it is, and what it exposes. They
	// are the first thing on the page that is not a sentence, because a
	// paragraph cannot say "one target of twenty holds most of this".
	Figures          []pageFigure
	Claims           []pageClaim
	MoreClaims       int
	ReadmeOverview   *pageClaim
	Cards            []pageTargetCard
	MutedCards       []pageMutedCard
	CardsMissing     string
	Portals          []pagePortal
	PortalsMissing   string
	Boundaries       []pageBoundary
	Negatives        []pageNegative
	NegativesMissing string
	Recipe           []pageRecipe
	RecipeMissing    string
	RepoMap          *pageRepoMap
	Addresses        []pageAddress
	Sections         []*pageSection
	Notes            []string
}

type pageSentence struct {
	Text    string
	Anchors []pageAnchor
}

type pageFigure struct {
	Value string
	Label string
	// Note is the qualifier that keeps a number honest — which targets it
	// counts, or that some were not read.
	Note string
	// Warn marks a figure the reader should not skim past, such as targets
	// the run could not read.
	Warn bool
}

type pageClaim struct {
	Text    string
	Source  string
	Date    string
	AgeDays int
	Anchor  *pageAnchor
}

type pageTargetCard struct {
	SectionID   string
	Name        string
	Language    string
	Kind        string
	Root        string
	Manifest    *pageAnchor
	Entrypoints []pageAnchor
	Routes      int
	Calls       int
	Dynamic     int
	// Counts is the same numbers written out, with the zeros left off.
	Counts      string
	Dead        int
	Role        string
	Purpose     string
	RoleAnchors []pageAnchor
}

type pageMutedCard struct {
	Name     string
	Language string
	Stage    string
	Reason   string
}

type pagePortal struct {
	Method   string
	Path     string
	Possible bool
	Call     pageAnchor
	Route    pageAnchor
}

// pageBoundary is how much HTTP surface one target has when no call of one
// target reaches a route of another. Listing every route of every target here
// would answer "what routes exist", which each target page already answers,
// rather than "how do the parts talk", which is the question above it.
type pageBoundary struct {
	Target    string
	SectionID string
	Routes    int
	Calls     int
}

type pageHTTPRow struct {
	Method string
	Path   string
	Symbol string
	Target string
	Anchor *pageAnchor
	// SymbolAnchor points at where the handler itself is written, which is a
	// different place from where the route is registered. A reader following
	// a route wants one or the other and should not have to guess which of
	// them a single link leads to.
	SymbolAnchor *pageAnchor
	Possible     bool
}

// pageRouteRow is every path one handler answers on, under one method. Three
// versions of an API mounted under three prefixes are one handler and three
// paths, and printing that as three rows repeats the handler three times.
type pageRouteRow struct {
	Paths        []pageRoutePath
	Symbol       string
	SymbolAnchor *pageAnchor
}

type pageRoutePath struct {
	Path   string
	Anchor *pageAnchor
	// Possible marks a path the code builds rather than writes out, so the
	// exact string is a reading of the code and not a quote from it.
	Possible bool
}

type pageNegative struct {
	Text   string
	Anchor *pageAnchor
}

type pageRecipe struct {
	Command string
	Cwd     string
	Note    string
	Model   bool
	Anchors []pageAnchor
}

// maxOverviewClaims bounds how many quotes the overview shows before it says
// how many more the README holds.
const maxOverviewClaims = 4

// A nested README gets a line or two; the page quotes at most this many
// README sentences in all.
const (
	maxNestedReadmeClaims = 2
	maxReadmeClaims       = 8
)

const (
	// A reader has never heard of this tool's stages, so an empty section says
	// what is missing from the page rather than which stage did not produce
	// it. The run's own console and artifacts say the rest.
	notAvailableFacts       = "This run did not read the repository, so nothing here is anchored to it."
	notAvailableOrientation = "This run produced no written summary."
)

// subjectRef locates one GroupsIndex subject and the target that owns it.
type subjectRef struct {
	subject         groupindex.Subject
	programTargetID string
}

type pageBuilder struct {
	data        *ReportData
	links       pageLinks
	sections    []*pageSection
	byProgram   map[string]*pageSection
	byFacts     map[string]*pageSection
	factsByID   map[string]facts.Fact
	claimsByID  map[string]claims.Claim
	subjects    map[string]subjectRef
	groupTitles map[groupindex.Endpoint]string
	// owners is built once: which target indexed which packages.
	owners []packageOwner
	// indexes is the group graph as the page shows it: one group per title
	// in a target, see foldIndexes.
	indexes []groupindex.Index
	// docstrings is what the authors wrote above their symbols, by file, in
	// line order, so a chip can carry the sentence that explains it;
	// declarations is where every symbol of a file is declared, so a
	// docstring goes to the first symbol after it and not to every symbol
	// within reach.
	docstrings   map[string][]claims.Claim
	declarations map[string][]int
}

func buildPageView(data *ReportData, reportSHA256 string, localRoots []string) (*pageView, error) {
	if data == nil || data.GroupGraph == nil || data.TargetOutcomePortfolio == nil {
		return nil, fmt.Errorf("report: page requires the final group graph and target inventory")
	}
	builder := &pageBuilder{
		data:         data,
		links:        newPageLinks(data),
		byProgram:    make(map[string]*pageSection),
		byFacts:      make(map[string]*pageSection),
		factsByID:    make(map[string]facts.Fact),
		claimsByID:   make(map[string]claims.Claim),
		subjects:     make(map[string]subjectRef),
		groupTitles:  make(map[groupindex.Endpoint]string),
		declarations: make(map[string][]int),
	}
	if data.Facts != nil {
		builder.factsByID = data.Facts.ByID()
	}
	if data.Claims != nil {
		builder.claimsByID = data.Claims.ByID()
		builder.docstrings = make(map[string][]claims.Claim)
		for _, claim := range data.Claims.Claims {
			if claim.Source == claims.SourceDocstring && claim.Path != "" {
				builder.docstrings[claim.Path] = append(builder.docstrings[claim.Path], claim)
			}
		}
		for path := range builder.docstrings {
			sort.Slice(builder.docstrings[path], func(left, right int) bool {
				return builder.docstrings[path][left].Line < builder.docstrings[path][right].Line
			})
		}
	}
	builder.indexes = foldIndexes(data.GroupGraph.Indexes)
	for position := range builder.indexes {
		index := &builder.indexes[position]
		for _, subject := range index.Subjects {
			builder.subjects[subject.ID] = subjectRef{subject: subject, programTargetID: index.Target.ID}
			if object := subject.Object; object != nil && object.Location != nil && object.Location.Path != "" {
				builder.declarations[object.Location.Path] = append(
					builder.declarations[object.Location.Path], object.Location.Line,
				)
			}
		}
		for _, group := range index.Groups {
			builder.groupTitles[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}] = group.Title
		}
	}
	view := &pageView{
		RepoName:      data.RepoName,
		Revision:      data.CapturedRevision,
		ShortRevision: shortRevision(data.CapturedRevision),
		RepositoryURL: builder.links.repositoryURL,
		FormatVersion: data.FormatVersion,
		ReportSHA256:  reportSHA256,
		Served:        builder.links.served(),
		EditorOpens:   builder.links.served() && editorOnPath(),
	}
	if view.Served {
		encoded, err := json.Marshal(data.SourceIDs)
		if err != nil {
			return nil, fmt.Errorf("report: encode served source ids: %w", err)
		}
		view.SourceIDsJSON = template.JS(encoded)
	}
	builder.buildSections()
	view.Sections = builder.sections
	builder.overview(view)
	view.RepoMap = builder.buildRepoMap(view)
	builder.addresses(view)
	for _, warning := range data.Warnings {
		view.Notes = append(view.Notes, scrubBrowserLocalPaths(warning, localRoots))
	}
	return view, nil
}

func (builder *pageBuilder) overview(view *pageView) {
	builder.summary(view)
	builder.readmeClaims(view)
	builder.cards(view)
	builder.portals(view)
	builder.negatives(view)
	builder.recipe(view)
	builder.figures(view)
}

// figures is the headline strip: how much of the repository was read and what
// it is made of, in numbers a reader can take in without reading a sentence.
// A count that is zero says nothing and is left out, except the two that are
// always worth stating.
func (builder *pageBuilder) figures(view *pageView) {
	symbols := builder.distinctSymbolCount()
	routes, calls, dead, dynamic := 0, 0, 0, 0
	for _, card := range view.Cards {
		routes += card.Routes
		calls += card.Calls
		dead += card.Dead
		dynamic += card.Dynamic
	}
	analyzed, unread := len(view.Cards), len(view.MutedCards)
	targets := pageFigure{
		Value: strconv.Itoa(analyzed), Label: pluralWord(analyzed, "target", "targets"),
	}
	if unread > 0 {
		targets.Value = fmt.Sprintf("%d of %d", analyzed, analyzed+unread)
		targets.Note = fmt.Sprintf("%d could not be read", unread)
		targets.Warn = true
	}
	view.Figures = append(view.Figures, targets)
	if languages := builder.repoLanguages(view); languages != "" {
		view.Figures = append(view.Figures, pageFigure{Value: languages, Label: "language"})
	}
	optional := []pageFigure{
		{Value: thousands(symbols), Label: "symbols read"},
		{Value: thousands(routes), Label: pluralWord(routes, "route served", "routes served")},
		{Value: thousands(calls), Label: pluralWord(calls, "HTTP call out", "HTTP calls out")},
		{Value: thousands(len(view.Portals)), Label: pluralWord(len(view.Portals), "target crossing", "target crossings")},
		{Value: thousands(dynamic), Label: pluralWord(dynamic, "place running handed-in code", "places running handed-in code")},
		{Value: thousands(dead), Label: pluralWord(dead, "file nothing reaches", "files nothing reach"), Warn: dead > 0},
	}
	for position, figure := range optional {
		if figure.Value == "0" {
			continue
		}
		view.Figures = append(view.Figures, optional[position])
	}
}

// repoLanguages names the languages of the targets that were read, so a
// reader learns in one word whether this is one stack or several.
func (builder *pageBuilder) repoLanguages(view *pageView) string {
	var order []string
	seen := make(map[string]struct{})
	for _, card := range view.Cards {
		if card.Language == "" {
			continue
		}
		if _, repeated := seen[card.Language]; repeated {
			continue
		}
		seen[card.Language] = struct{}{}
		order = append(order, card.Language)
	}
	return strings.Join(order, ", ")
}

// distinctSymbolCount counts a symbol once even when several targets reach
// it. python-dotenv is one package read under three entrypoints, and adding
// its targets up said the repository held three times what it holds.
func (builder *pageBuilder) distinctSymbolCount() int {
	seen := make(map[string]struct{})
	for _, section := range builder.sections {
		index := builder.graphIndex(section.programTargetID)
		if index == nil {
			continue
		}
		for _, subject := range index.Subjects {
			if subject.Kind != groupindex.SubjectObject || subject.Object == nil {
				continue
			}
			seen[symbolIdentity(*subject.Object)] = struct{}{}
		}
	}
	return len(seen)
}

func symbolIdentity(object groupindex.ObjectFacts) string {
	if object.Location == nil {
		return object.Name
	}
	return fmt.Sprintf("%s:%d:%d:%s",
		object.Location.Path, object.Location.Line, object.Location.Column, object.Name)
}

func pluralWord(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// thousandsFromDigits is where a count starts being grouped. Three digits read
// as a number; four do not.
const thousandsFromDigits = 4

// thousands groups a large count so four digits do not read as one number the
// eye has to spell out.
func thousands(value int) string {
	digits := strconv.Itoa(value)
	if len(digits) < thousandsFromDigits {
		return digits
	}
	var out []byte
	for position, digit := range []byte(digits) {
		if position > 0 && (len(digits)-position)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	return string(out)
}

func (builder *pageBuilder) summary(view *pageView) {
	orient := builder.data.Orientation
	if orient == nil || orient.Summary == "" {
		view.SummaryMissing = notAvailableOrientation
		return
	}
	view.Summary = &pageSentence{Text: orient.Summary, Anchors: builder.refAnchors(orient.SummaryRefs)}
}

// readmeClaims quotes what the repository says about itself. Only the
// shallowest README speaks for the whole repository; a nested one describes
// its own directory and would otherwise bury the overview in boilerplate.
func (builder *pageBuilder) readmeClaims(view *pageView) {
	if overview := builder.data.ReadmeOverview; overview != "" {
		view.ReadmeOverview = &pageClaim{Text: overview, Source: "README"}
		if builder.data.reducedDocumentation != nil {
			for _, source := range builder.data.reducedDocumentation.Sources {
				if source.Path != "" {
					view.ReadmeOverview.Source = source.Path
					view.ReadmeOverview.Anchor = builder.links.anchorPointer(source.Path, 1, 0)
					break
				}
			}
		}
	}
	if builder.data.Claims == nil {
		return
	}
	picked, more := selectReadmeClaims(builder.data.Claims.Claims)
	view.MoreClaims = more
	for _, claim := range picked {
		view.Claims = append(view.Claims, pageClaim{
			Text: claim.Text, Source: claim.Path, Date: claim.Date, AgeDays: claim.AgeDays,
			Anchor: builder.links.anchorPointer(claim.Path, claim.Line, 0),
		})
	}
}

// selectReadmeClaims quotes the repository's READMEs, the shallowest first.
// The root README speaks for the whole repository and gets the most room; a
// nested one describes its own directory and gets a line or two, because
// chi's _examples/README.md said what the examples are and the page said
// nothing of it.
func selectReadmeClaims(all []claims.Claim) ([]claims.Claim, int) {
	var readme []claims.Claim
	for _, claim := range all {
		if claim.Source == claims.SourceReadme && claim.Path != "" {
			readme = append(readme, claim)
		}
	}
	sort.SliceStable(readme, func(left, right int) bool {
		if readmeDepth(readme[left].Path) != readmeDepth(readme[right].Path) {
			return readmeDepth(readme[left].Path) < readmeDepth(readme[right].Path)
		}
		if readme[left].Path != readme[right].Path {
			return readme[left].Path < readme[right].Path
		}
		return readme[left].Line < readme[right].Line
	})
	var picked []claims.Claim
	perFile := make(map[string]int)
	more := 0
	for position, claim := range readme {
		room := maxNestedReadmeClaims
		if position < len(readme) && readmeDepth(claim.Path) == readmeDepth(readme[0].Path) && claim.Path == readme[0].Path {
			room = maxOverviewClaims
		}
		if perFile[claim.Path] >= room || len(picked) >= maxReadmeClaims {
			more++
			continue
		}
		perFile[claim.Path]++
		picked = append(picked, claim)
	}
	return picked, more
}

func readmeDepth(path string) int {
	return strings.Count(path, "/")
}

// refAnchors resolves orientation refs (fact, claim or subject ids) to
// anchors. Unknown ids are skipped: the orientation stage already rejected
// rows with unresolved refs, so a miss here is a stale artifact, not a bug
// to repair on screen.
func (builder *pageBuilder) refAnchors(refs []string) []pageAnchor {
	anchors := make([]pageAnchor, 0, len(refs))
	for _, ref := range refs {
		if fact, ok := builder.factsByID[ref]; ok {
			if anchor := builder.links.factAnchor(fact); anchor != nil {
				anchors = append(anchors, *anchor)
			}
			continue
		}
		if claim, ok := builder.claimsByID[ref]; ok && claim.Path != "" {
			anchors = append(anchors, builder.links.anchor(claim.Path, claim.Line, 0))
			continue
		}
		if subject, ok := builder.subjects[ref]; ok {
			if _, anchor := builder.subjectDisplay(subject.subject); anchor != nil {
				anchors = append(anchors, *anchor)
			}
		}
	}
	return anchors
}

func (builder *pageBuilder) cards(view *pageView) {
	if builder.data.Facts != nil {
		for _, target := range builder.data.Facts.Targets {
			view.Cards = append(view.Cards, builder.factsCard(target))
		}
	} else {
		for _, section := range builder.sections {
			view.Cards = append(view.Cards, pageTargetCard{
				SectionID: section.ID, Name: section.Label, Language: section.Language,
				Kind: section.Kind, Root: section.Root,
			})
		}
	}
	for _, outcome := range builder.data.TargetOutcomePortfolio.Outcomes {
		if outcome.State == targetoutcome.StateAnalyzed {
			continue
		}
		view.MutedCards = append(view.MutedCards, pageMutedCard{
			Name:     outcome.DisplayName,
			Language: string(outcome.Language),
			Stage:    strings.ReplaceAll(string(outcome.FailureStage), "_", " "),
			Reason:   strings.ReplaceAll(string(outcome.FailureReason), "_", " "),
		})
	}
	if len(view.Cards) == 0 && len(view.MutedCards) == 0 {
		view.CardsMissing = "No targets were analyzed."
	}
}

func (builder *pageBuilder) factsCard(target facts.Target) pageTargetCard {
	card := pageTargetCard{
		Name: target.Name, Language: target.Language, Kind: target.Kind, Root: target.Root,
	}
	if section := builder.byFacts[target.ID]; section != nil {
		card.SectionID = section.ID
		card.Name = section.Label
	}
	if target.Manifest != "" {
		card.Manifest = builder.links.anchorPointer(target.Manifest, 0, 0)
	} else {
		card.Manifest = builder.links.anchorPointer(target.Anchor.Path, target.Anchor.Line, 0)
	}
	for _, fact := range builder.targetFacts(target.ID, facts.KindEntrypoint) {
		if anchor := builder.links.factAnchor(fact); anchor != nil {
			card.Entrypoints = append(card.Entrypoints, *anchor)
		}
	}
	card.Routes = len(builder.targetFacts(target.ID, facts.KindHTTPRoute))
	card.Calls = len(builder.targetFacts(target.ID, facts.KindHTTPCall))
	card.Dynamic = len(builder.targetFacts(target.ID, facts.KindDynamicExecution))
	card.Dead = len(builder.targetFacts(target.ID, facts.KindDeadModule))
	card.Counts = cardCounts(card)
	if orient := builder.data.Orientation; orient != nil {
		for _, role := range orient.Roles {
			if role.TargetID != target.ID {
				continue
			}
			card.Role, card.Purpose = role.Role, role.Purpose
			refs := append(append(append([]string(nil), role.FactIDs...), role.ClaimIDs...), role.SubjectIDs...)
			card.RoleAnchors = builder.refAnchors(refs)
			break
		}
	}
	return card
}

// cardCounts writes only the counts a target actually has. Four zeros in a
// row read as a broken tool, not as a library with no routes.
func cardCounts(card pageTargetCard) string {
	var parts []string
	for _, count := range []struct {
		value     int
		one, many string
	}{
		{card.Routes, "route", "routes"},
		{card.Calls, "HTTP call out", "HTTP calls out"},
		{card.Dynamic, "place running handed-in code", "places running handed-in code"},
		{card.Dead, "file nothing reaches", "files nothing reach"},
	} {
		if count.value == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", count.value, pluralWord(count.value, count.one, count.many)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

func (builder *pageBuilder) targetFacts(targetID string, kind facts.Kind) []facts.Fact {
	if builder.data.Facts == nil {
		return nil
	}
	var rows []facts.Fact
	for _, fact := range builder.data.Facts.OfKind(kind) {
		if fact.TargetID == targetID {
			rows = append(rows, fact)
		}
	}
	return rows
}

func (builder *pageBuilder) portals(view *pageView) {
	if builder.data.Facts == nil {
		view.PortalsMissing = notAvailableFacts
		return
	}
	for _, portal := range builder.data.Facts.OfKind(facts.KindPortal) {
		row := pagePortal{
			Method: portal.Method, Path: portal.Path,
			Possible: portal.Resolution == facts.ResolutionPossible,
		}
		if call, ok := builder.factsByID[portal.Refs[0]]; ok && call.Anchor != nil {
			row.Call = builder.links.anchor(call.Anchor.Path, call.Anchor.Line, call.Anchor.Column)
		} else if portal.Anchor != nil {
			row.Call = builder.links.anchor(portal.Anchor.Path, portal.Anchor.Line, portal.Anchor.Column)
		}
		if route, ok := builder.factsByID[portal.Refs[1]]; ok && route.Anchor != nil {
			row.Route = builder.links.anchor(route.Anchor.Path, route.Anchor.Line, route.Anchor.Column)
		} else if len(portal.Evidence) > 0 {
			row.Route = builder.links.anchor(portal.Evidence[0].Path, portal.Evidence[0].Line, 0)
		}
		view.Portals = append(view.Portals, row)
	}
	if len(view.Portals) > 0 {
		return
	}
	view.PortalsMissing = "No cross-target HTTP link was found."
	view.Boundaries = builder.boundaryCounts()
}

// httpRows lists http_call or http_route facts; an empty targetID means every
// target, and rows then carry the owning target name.
func (builder *pageBuilder) httpRows(kind facts.Kind, targetID string) []pageHTTPRow {
	if builder.data.Facts == nil {
		return nil
	}
	var rows []pageHTTPRow
	for _, fact := range builder.data.Facts.OfKind(kind) {
		if targetID != "" && fact.TargetID != targetID {
			continue
		}
		row := pageHTTPRow{
			Method: fact.Method, Path: fact.Path, Symbol: fact.Symbol,
			Anchor:   builder.links.factAnchor(fact),
			Possible: fact.Resolution == facts.ResolutionPossible,
		}
		if fact.ObjectID != "" {
			if subject, known := builder.subjects[fact.ObjectID]; known {
				if _, anchor := builder.subjectDisplay(subject.subject); anchor != nil {
					row.SymbolAnchor = anchor
				}
			}
		}
		if targetID == "" {
			if section := builder.byFacts[fact.TargetID]; section != nil {
				row.Target = section.Name
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (builder *pageBuilder) negatives(view *pageView) {
	if builder.data.Facts == nil {
		view.NegativesMissing = notAvailableFacts
		return
	}
	for _, fact := range builder.data.Facts.OfKind(facts.KindNegative) {
		view.Negatives = append(view.Negatives, pageNegative{
			Text: negativeSentence(fact), Anchor: builder.links.factAnchor(fact),
		})
	}
	if len(view.Negatives) == 0 {
		view.NegativesMissing = "Nothing is missing among README, tests, Dockerfile and CI."
	}
}

func (builder *pageBuilder) recipe(view *pageView) {
	if orient := builder.data.Orientation; orient != nil {
		for _, step := range orient.RunRecipe {
			view.Recipe = append(view.Recipe, pageRecipe{
				Command: step.Command, Cwd: step.Cwd, Note: step.Note, Model: true,
				Anchors: builder.refAnchors(step.FactIDs),
			})
		}
		if len(view.Recipe) > 0 {
			return
		}
	}
	if builder.data.Facts == nil {
		view.RecipeMissing = notAvailableFacts
		return
	}
	// Without a model recipe the reader still gets the raw material: the
	// manifest rows and the proved entrypoints, each anchored.
	for _, fact := range builder.data.Facts.OfKind(facts.KindManifest) {
		view.Recipe = append(view.Recipe, pageRecipe{
			Command: factLabel(fact), Anchors: anchorList(builder.links.factAnchor(fact)),
		})
	}
	for _, fact := range builder.data.Facts.OfKind(facts.KindEntrypoint) {
		row := pageRecipe{Command: factLabel(fact), Anchors: anchorList(builder.links.factAnchor(fact))}
		if fact.Anchor != nil {
			row.Cwd = path.Dir(fact.Anchor.Path)
		}
		view.Recipe = append(view.Recipe, row)
	}
	if len(view.Recipe) == 0 {
		view.RecipeMissing = "No manifest rows or entrypoints were found."
	}
}

func anchorList(anchor *pageAnchor) []pageAnchor {
	if anchor == nil {
		return nil
	}
	return []pageAnchor{*anchor}
}

// flowSteps resolves the whole main flow once; every target section shows
// it so a reader never has to leave the section to follow the path.
// flow shows the repository's one main flow on a target page, but only where
// that target takes part in it. A four-target repository was printing a flow
// through one example on all four pages, including the two it never touches,
// which reads as a claim about that target and is not one.
func (builder *pageBuilder) flow(section *pageSection) *pageFlow {
	orient := builder.data.Orientation
	if orient == nil || len(orient.MainFlow.Steps) == 0 {
		return nil
	}
	flow := &pageFlow{Title: orient.MainFlow.Title}
	here := false
	for _, step := range orient.MainFlow.Steps {
		row := builder.flowStep(step, section)
		if row.Target == "" {
			here = true
		}
		flow.Steps = append(flow.Steps, row)
	}
	if !here {
		return nil
	}
	return flow
}

func (builder *pageBuilder) flowStep(step orientation.FlowStep, section *pageSection) pageFlowStep {
	row := pageFlowStep{Explanation: step.Explanation}
	var owner *pageSection
	switch {
	case step.FactID != "":
		if fact, ok := builder.factsByID[step.FactID]; ok {
			row.Label = factLabel(fact)
			row.Anchor = builder.links.factAnchor(fact)
			owner = builder.byFacts[fact.TargetID]
		}
	case step.SubjectID != "":
		if ref, ok := builder.subjects[step.SubjectID]; ok {
			row.Label, row.Anchor = builder.subjectDisplay(ref.subject)
			owner = builder.byProgram[ref.programTargetID]
		}
	}
	if owner == nil {
		owner = builder.byFacts[step.TargetID]
	}
	if owner != nil && owner != section {
		row.Target = owner.Name
	}
	return row
}

// subjectDisplay names one GroupsIndex subject and anchors it when it has a
// repository location. External symbols are named by package and symbol.
func (builder *pageBuilder) subjectDisplay(subject groupindex.Subject) (string, *pageAnchor) {
	switch {
	case subject.Object != nil:
		object := subject.Object
		if object.Location != nil {
			return object.Name, builder.links.anchorPointer(
				object.Location.Path, object.Location.Line, object.Location.Column,
			)
		}
		if object.External != nil {
			return externalSymbolName(object.External.PackagePath, object.External.Receiver, object.External.Name), nil
		}
		return object.Name, nil
	case subject.Pattern != nil:
		pattern := subject.Pattern
		name := pattern.Selector
		if name == "" {
			name = pattern.Invocation
		}
		if pattern.Location != nil {
			return name, builder.links.anchorPointer(
				pattern.Location.Path, pattern.Location.Line, pattern.Location.Column,
			)
		}
		return name, nil
	}
	return "", nil
}

func externalSymbolName(packagePath, receiver, name string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{packagePath, receiver, name} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ".")
}

func sortedMethods(methods map[string][]pageHTTPRow) []string {
	rank := map[string]int{"GET": 0, "POST": 1, "PUT": 2, "PATCH": 3, "DELETE": 4}
	keys := make([]string, 0, len(methods))
	for method := range methods {
		keys = append(keys, method)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, leftKnown := rank[keys[i]]
		right, rightKnown := rank[keys[j]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	return keys
}

// boundaryCounts summarises each analyzed target's HTTP surface.
func (builder *pageBuilder) boundaryCounts() []pageBoundary {
	if builder.data.Facts == nil {
		return nil
	}
	var result []pageBoundary
	for _, section := range builder.sections {
		if section.factsTargetID == "" {
			continue
		}
		row := pageBoundary{Target: section.Label, SectionID: section.ID}
		for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindHTTPRoute) {
			_ = fact
			row.Routes++
		}
		for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindHTTPCall) {
			_ = fact
			row.Calls++
		}
		if row.Routes == 0 && row.Calls == 0 {
			continue
		}
		result = append(result, row)
	}
	return result
}
