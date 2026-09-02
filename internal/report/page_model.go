package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"sort"
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
	SourceIDsJSON template.JS
	CSS           template.CSS
	JS            template.JS

	Summary          *pageSentence
	SummaryMissing   string
	Claims           []pageClaim
	MoreClaims       int
	Cards            []pageTargetCard
	MutedCards       []pageMutedCard
	CardsMissing     string
	Portals          []pagePortal
	PortalsMissing   string
	LooseCalls       []pageHTTPRow
	LooseRoutes      []pageHTTPRow
	Negatives        []pageNegative
	NegativesMissing string
	Recipe           []pageRecipe
	RecipeMissing    string
	Sections         []*pageSection
	Notes            []string
}

type pageSentence struct {
	Text    string
	Anchors []pageAnchor
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
	Risks       int
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

type pageHTTPRow struct {
	Method string
	Path   string
	Symbol string
	Target string
	Anchor *pageAnchor
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

const (
	notAvailableFacts       = "Facts are not available for this run."
	notAvailableOrientation = "Orientation is not available for this run."
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
}

func buildPageView(data *ReportData, reportSHA256 string, localRoots []string) (*pageView, error) {
	if data == nil || data.GroupGraph == nil || data.TargetOutcomePortfolio == nil {
		return nil, fmt.Errorf("report: page requires the final group graph and target inventory")
	}
	builder := &pageBuilder{
		data:        data,
		links:       newPageLinks(data),
		byProgram:   make(map[string]*pageSection),
		byFacts:     make(map[string]*pageSection),
		factsByID:   make(map[string]facts.Fact),
		claimsByID:  make(map[string]claims.Claim),
		subjects:    make(map[string]subjectRef),
		groupTitles: make(map[groupindex.Endpoint]string),
	}
	if data.Facts != nil {
		builder.factsByID = data.Facts.ByID()
	}
	if data.Claims != nil {
		builder.claimsByID = data.Claims.ByID()
	}
	for position := range data.GroupGraph.Indexes {
		index := &data.GroupGraph.Indexes[position]
		for _, subject := range index.Subjects {
			builder.subjects[subject.ID] = subjectRef{subject: subject, programTargetID: index.Target.ID}
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
	if builder.data.Claims == nil {
		return
	}
	primary := ""
	for _, claim := range builder.data.Claims.Claims {
		if claim.Source != claims.SourceReadme {
			continue
		}
		if primary == "" || readmeDepth(claim.Path) < readmeDepth(primary) {
			primary = claim.Path
		}
	}
	for _, claim := range builder.data.Claims.Claims {
		if claim.Source != claims.SourceReadme || claim.Path != primary {
			continue
		}
		if len(view.Claims) >= maxOverviewClaims {
			view.MoreClaims++
			continue
		}
		view.Claims = append(view.Claims, pageClaim{
			Text: claim.Text, Source: claim.Path, Date: claim.Date, AgeDays: claim.AgeDays,
			Anchor: builder.links.anchorPointer(claim.Path, claim.Line, 0),
		})
	}
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
				SectionID: section.ID, Name: section.Name, Language: section.Language,
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
	card.Risks = len(builder.targetFacts(target.ID, facts.KindRisk))
	card.Dead = len(builder.targetFacts(target.ID, facts.KindDeadModule))
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
	view.LooseCalls = builder.httpRows(facts.KindHTTPCall, "")
	view.LooseRoutes = builder.httpRows(facts.KindHTTPRoute, "")
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
			Anchor: builder.links.factAnchor(fact),
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
func (builder *pageBuilder) flow(section *pageSection) *pageFlow {
	orient := builder.data.Orientation
	if orient == nil || len(orient.MainFlow.Steps) == 0 {
		return nil
	}
	flow := &pageFlow{Title: orient.MainFlow.Title}
	for _, step := range orient.MainFlow.Steps {
		flow.Steps = append(flow.Steps, builder.flowStep(step, section))
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
