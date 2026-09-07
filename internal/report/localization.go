package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// DisplayLanguage controls the last presentation pass, never repository analysis.
type DisplayLanguage string

const (
	English            DisplayLanguage = "en"
	Russian            DisplayLanguage = "ru"
	DisplayTextVersion                 = 1
)

func NormalizeDisplayLanguage(value string) (DisplayLanguage, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return English, nil
	case "ru":
		return Russian, nil
	default:
		return "", fmt.Errorf("report: unsupported display language %q (use en or ru)", value)
	}
}

// DisplayProtectedText replaces a verbatim source fragment inside prose. Ref
// is the exact placeholder in Text; its original bytes never need translation.
type DisplayProtectedText struct {
	Ref  string `json:"ref"`
	Text string `json:"text"`
}

type DisplayTextEntry struct {
	Ref       string                 `json:"ref"`
	Role      string                 `json:"role"`
	Text      string                 `json:"text"`
	Protected []DisplayProtectedText `json:"protected,omitempty"`
}

// DisplayTextCatalog contains only explicitly projected display prose. It
// contains no graph, source links, workstation roots or session source IDs.
type DisplayTextCatalog struct {
	Version int                `json:"version"`
	SHA256  string             `json:"sha256"`
	Entries []DisplayTextEntry `json:"entries"`
}

type DisplayTranslationEntry struct {
	Ref  string `json:"ref"`
	Text string `json:"text"`
}

// DisplayTranslations is a presentation artifact bound to one English text
// catalogue. Canonical report.json remains unchanged when this value is used.
type DisplayTranslations struct {
	Version       int                       `json:"version"`
	Language      DisplayLanguage           `json:"language"`
	CatalogSHA256 string                    `json:"catalog_sha256"`
	Entries       []DisplayTranslationEntry `json:"entries"`
}

func (catalog DisplayTextCatalog) Validate() error {
	if catalog.Version != DisplayTextVersion || catalog.Entries == nil {
		return fmt.Errorf("report: invalid display text catalogue")
	}
	for i, entry := range catalog.Entries {
		if entry.Ref != fmt.Sprintf("t%d", i+1) || entry.Role == "" || strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("report: invalid display text entry %d", i)
		}
	}
	if catalog.SHA256 != displayCatalogDigest(catalog.Entries) {
		return fmt.Errorf("report: display text catalogue digest does not match")
	}
	return nil
}

func (translations DisplayTranslations) Validate(catalog DisplayTextCatalog) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	language, err := NormalizeDisplayLanguage(string(translations.Language))
	if err != nil || language != translations.Language || translations.Version != DisplayTextVersion || translations.CatalogSHA256 != catalog.SHA256 {
		return fmt.Errorf("report: translations do not match their display catalogue")
	}
	if len(translations.Entries) != len(catalog.Entries) {
		return fmt.Errorf("report: translations do not cover every display text")
	}
	byRef := make(map[string]string, len(translations.Entries))
	for _, entry := range translations.Entries {
		if _, duplicate := byRef[entry.Ref]; duplicate || strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("report: duplicate or empty translated display text")
		}
		byRef[entry.Ref] = entry.Text
	}
	for _, entry := range catalog.Entries {
		text, ok := byRef[entry.Ref]
		if !ok {
			return fmt.Errorf("report: missing translated display text %s", entry.Ref)
		}
		if err := entry.ValidateTranslation(text); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTranslation is shared by the translation cube before caching and
// the final display pass. It does not require renumbering a window's refs.
func (entry DisplayTextEntry) ValidateTranslation(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("report: empty translated display text %s", entry.Ref)
	}
	allowed := make(map[string]bool, len(entry.Protected))
	for _, protected := range entry.Protected {
		allowed[protected.Ref] = true
		if strings.Count(text, protected.Ref) != strings.Count(entry.Text, protected.Ref) {
			return fmt.Errorf("report: translated text %s changed a source placeholder", entry.Ref)
		}
	}
	for _, token := range displayPlaceholder.FindAllString(text, -1) {
		if !allowed[token] {
			return fmt.Errorf("report: translated text %s introduced a source placeholder", entry.Ref)
		}
	}
	return nil
}

func displayCatalogDigest(entries []DisplayTextEntry) string {
	encoded, _ := json.Marshal(struct {
		Version int                `json:"version"`
		Entries []DisplayTextEntry `json:"entries"`
	}{DisplayTextVersion, entries})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

type displayTextSlot struct {
	entry int
	value *string
}

// PreparedPage owns the English frontend projection, after all identity,
// membership and source-link work. Its catalogue is safe to translate without
// giving the translator authority over that projection's structure.
type PreparedPage struct {
	view     *pageView
	catalog  DisplayTextCatalog
	slots    []displayTextSlot
	uiSlots  []*string
	concepts []displayConcepts
	timing   *RunTiming
}

type displayConcepts struct {
	node   *pageMapNode
	values []pageMapConcept
}

func (page *PreparedPage) TextCatalog() DisplayTextCatalog {
	catalog := page.catalog
	catalog.Entries = append([]DisplayTextEntry{}, page.catalog.Entries...)
	for i := range catalog.Entries {
		catalog.Entries[i].Protected = append([]DisplayProtectedText(nil), catalog.Entries[i].Protected...)
	}
	return catalog
}

func PreparePage(data *ReportData, options RenderOptions) (*PreparedPage, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	// Ordinary publication has not installed its source-path inventory yet;
	// a restored served report has. Build the same inventory without mutating
	// the caller's canonical value so their text catalogues agree.
	copy := *data
	if err := collectOpenablePaths(&copy); err != nil {
		return nil, err
	}
	data = &copy
	view, err := buildPageView(data, options.ReportSHA256, renderPayloadLocalRoots(data, options.LocalRoots))
	if err != nil {
		return nil, err
	}
	page := &PreparedPage{view: view, timing: data.Timing, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(data, options.NoModel); err != nil {
		return nil, err
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	return page, nil
}

// These are actual protected syntax spans, not an inference about which prose
// is a source quotation. Source quotes never enter the display-text walker.
var displayVerbatimSyntax = regexp.MustCompile(strings.Join([]string{
	"(?s)```.*?```", "`[^`]+`", `__REPOMAP_P[0-9]+__`, `https?://[^\s<>]+`,
	`[\p{L}_$][\p{L}\p{N}_$]*(?:\.[\p{L}_$][\p{L}\p{N}_$]*)*\([^()\n]*\)`,
	`[\p{L}_$][\p{L}\p{N}_$]*(?:\.[\p{L}_$][\p{L}\p{N}_$]*)+`,
}, "|"))
var displayPlaceholder = regexp.MustCompile(`__REPOMAP_P[0-9]+__`)

func protectedDisplayText(text string, names []string) (string, []DisplayProtectedText) {
	type span struct{ start, end int }
	var spans []span
	for _, bounds := range displayVerbatimSyntax.FindAllStringIndex(text, -1) {
		spans = append(spans, span{bounds[0], bounds[1]})
	}
	for _, name := range names {
		// A repository may declare Run, service or protocol. Seeing the same
		// word in a sentence does not make that occurrence a code reference.
		// Protect names with identifier/path syntax; bare ambiguous words stay
		// translatable unless explicit code syntax above encloses them.
		if !displayUnambiguousName(name) {
			continue
		}
		for from := 0; from < len(text); {
			at := strings.Index(text[from:], name)
			if at < 0 {
				break
			}
			start, end := from+at, from+at+len(name)
			from = end
			if start > 0 {
				r, _ := utf8.DecodeLastRuneInString(text[:start])
				if displayNameRune(r) {
					continue
				}
			}
			if end < len(text) {
				r, _ := utf8.DecodeRuneInString(text[end:])
				if displayNameRune(r) {
					continue
				}
			}
			spans = append(spans, span{start, end})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})
	var out strings.Builder
	var protected []DisplayProtectedText
	end := 0
	for _, value := range spans {
		if value.start < end {
			continue
		}
		out.WriteString(text[end:value.start])
		ref := fmt.Sprintf("__REPOMAP_P%d__", len(protected)+1)
		protected = append(protected, DisplayProtectedText{Ref: ref, Text: text[value.start:value.end]})
		out.WriteString(ref)
		end = value.end
	}
	out.WriteString(text[end:])
	return out.String(), protected
}

func displayUnambiguousName(name string) bool {
	if strings.ContainsAny(name, "/\\._:$-") {
		return true
	}
	var previous rune
	for i, current := range name {
		if i > 0 && unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsUpper(previous)) {
			return true
		}
		previous = current
	}
	return false
}

func displayNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func displayProtectedNames(data *ReportData) []string {
	names := map[string]bool{}
	add := func(value string) {
		if len(value) > 1 {
			names[value] = true
		}
	}
	add(data.RepoName)
	for _, path := range data.OpenablePaths {
		add(path)
	}
	if data.GroupGraph != nil {
		for _, index := range data.GroupGraph.Indexes {
			add(index.Target.Name)
			for _, subject := range index.Subjects {
				if subject.Object != nil {
					add(subject.Object.Name)
					if subject.Object.Location != nil {
						add(subject.Object.Location.Path)
					}
				}
			}
		}
	}
	var result []string
	for name := range names {
		result = append(result, name)
	}
	sort.Slice(result, func(i, j int) bool {
		if len(result[i]) != len(result[j]) {
			return len(result[i]) > len(result[j])
		}
		return result[i] < result[j]
	})
	return result
}

// collectDisplayTexts is deliberately a typed allow-list of interpretation
// slots. It never traverses arbitrary strings or translates an HTML document.
func (page *PreparedPage) collectDisplayTexts(data *ReportData, noModel bool) error {
	view := page.view
	names := displayProtectedNames(data)
	exactNames := make(map[string]bool, len(names))
	for _, name := range names {
		exactNames[name] = true
	}
	vocabulary, err := uiVocabulary(English)
	if err != nil {
		return err
	}
	known := make(map[string]int)
	userQuestions := make(map[string]bool)
	for _, question := range view.Questions {
		if question.UserQuestion {
			userQuestions[question.Question] = true
		}
	}
	add := func(role string, value *string) {
		if strings.TrimSpace(*value) == "" || userQuestions[*value] || exactNames[*value] {
			return
		}
		page.uiSlots = append(page.uiSlots, value)
		if _, ui := vocabulary[*value]; ui || noModel {
			return
		}
		text, protected := protectedDisplayText(*value, names)
		if strings.IndexFunc(displayPlaceholder.ReplaceAllString(text, ""), unicode.IsLetter) < 0 {
			return
		}
		// Equal display text shares a translation across map,
		// card, search, answer and Learn copies. It does not acquire membership.
		key := *value
		at, exists := known[key]
		if !exists {
			at = len(page.catalog.Entries)
			known[key] = at
			page.catalog.Entries = append(page.catalog.Entries, DisplayTextEntry{Ref: fmt.Sprintf("t%d", at+1), Role: role, Text: text, Protected: protected})
		}
		page.slots = append(page.slots, displayTextSlot{entry: at, value: value})
	}
	// Link labels are composed from their already-bound destination after
	// translation, not sent as competing copies of the destination's name.
	steps := func(values []pageQuestionStep) {
		for i := range values {
			add("reason", &values[i].Why)
		}
	}
	concepts := func(values []pageLearnConcept) {
		for i := range values {
			add("explanation", &values[i].Explanation)
		}
	}
	connections := func(values []pageConnection) {
		for i := range values {
			add("label", &values[i].Title)
			add("label", &values[i].Label)
			add("summary", &values[i].Summary)
		}
	}
	if view.Summary != nil {
		add("summary", &view.Summary.Text)
	}
	for i := range view.Cards {
		add("summary", &view.Cards[i].Purpose)
		add("label", &view.Cards[i].Role)
	}
	for i := range view.Recipe {
		if view.Recipe[i].Model {
			add("explanation", &view.Recipe[i].Note)
		}
	}
	for _, question := range view.Questions {
		if !question.UserQuestion {
			add("question", &question.Question)
		}
		add("question", &question.OpenQuestion)
		for i := range question.Origins {
			origin := &question.Origins[i]
			add("label", &origin.Title)
			add("question", &origin.Question)
			add("reason", &origin.Why)
			steps(origin.Checks)
		}
		for i := range question.Answers {
			answer := &question.Answers[i]
			add("answer", &answer.Text)
			add("basis", &answer.Basis)
			add("remaining", &answer.Remaining)
			steps(answer.Checks)
			concepts(answer.Terms)
		}
		for i := range question.Readings {
			add("question", &question.Readings[i].OpenQuestion)
			steps(question.Readings[i].Steps)
		}
	}
	for _, reviews := range [][]pageLearningReview{view.LearningReviews, view.LearningSelections} {
		for i := range reviews {
			add("label", &reviews[i].Title)
			add("reason", &reviews[i].Reason)
			steps(reviews[i].Checks)
		}
	}
	concepts(view.LearnConcepts)
	for i := range view.LearnQuestionTopics {
		add("label", &view.LearnQuestionTopics[i].Title)
	}
	for i := range view.LearnBands {
		for j := range view.LearnBands[i].Parts {
			part := &view.LearnBands[i].Parts[j]
			add("summary", &part.Summary)
		}
	}
	for _, section := range view.Sections {
		for _, groups := range [][]pageGroup{section.Triggers, section.Core, section.DependencyGroups} {
			for i := range groups {
				group := &groups[i]
				add("label", &group.Title)
				add("summary", &group.Summary)
				add("label", &group.Zone)
				for _, chips := range [][]pageChipRow{group.Highlights, group.Inventory} {
					for j := range chips {
						for k := range chips[j].Members {
							add("explanation", &chips[j].Members[k].Summary)
						}
					}
				}
				for j := range group.Operations {
					if group.Operations[j].Source != "fact" {
						add("label", &group.Operations[j].Name)
					}
					// Source identifies the route name's origin. Its purpose is
					// still display prose, including on an extracted boundary.
					add("summary", &group.Operations[j].Summary)
				}
				connections(group.Connections)
			}
		}
		if section.Flow != nil {
			add("label", &section.Flow.Title)
			for i := range section.Flow.Steps {
				add("explanation", &section.Flow.Steps[i].Explanation)
			}
		}
		for i := range section.Start {
			add("label", &section.Start[i].Group)
			connections(section.Start[i].Reaches)
		}
		if section.Map == nil {
			continue
		}
		for i := range section.Map.Frames {
			add("label", &section.Map.Frames[i].Title)
		}
		for i := range section.Map.Nodes {
			node := &section.Map.Nodes[i]
			node.CanonicalTitle = node.FullTitle
			if node.Activation == "" || node.SourceKind != "fact" {
				add("label", &node.FullTitle)
			}
			add("summary", &node.Summary)
			add("label", &node.OperationGroup)
			if node.Branch == "" {
				add("label", &node.Subtitle)
			}
			if node.Concepts != "" {
				entry := displayConcepts{node: node}
				if err := json.Unmarshal([]byte(node.Concepts), &entry.values); err != nil {
					return fmt.Errorf("report: invalid concept display data: %w", err)
				}
				for j := range entry.values {
					add("explanation", &entry.values[j].Explanation)
				}
				page.concepts = append(page.concepts, entry)
			}
		}
		for i := range section.Map.Edges {
			add("label", &section.Map.Edges[i].Label)
			add("summary", &section.Map.Edges[i].Summary)
		}
	}
	if view.RepoMap != nil {
		for i := range view.RepoMap.Nodes {
			add("summary", &view.RepoMap.Nodes[i].Summary)
		}
	}
	return nil
}

func (page *PreparedPage) applyDisplay(options RenderOptions) error {
	language, err := NormalizeDisplayLanguage(string(options.Language))
	if err != nil {
		return err
	}
	page.view.Language = language
	if options.Translations != nil {
		if options.Translations.Language != language {
			return fmt.Errorf("report: display language differs from translation language")
		}
		if err := options.Translations.Validate(page.catalog); err != nil {
			return err
		}
	}
	if language != English && len(page.catalog.Entries) > 0 {
		if options.Translations == nil {
			return fmt.Errorf("report: translated display text is required for %s", language)
		}
		translated := make(map[string]string, len(options.Translations.Entries))
		for _, value := range options.Translations.Entries {
			translated[value.Ref] = value.Text
		}
		for _, slot := range page.slots {
			entry := page.catalog.Entries[slot.entry]
			pairs := make([]string, 0, 2*len(entry.Protected))
			for _, value := range entry.Protected {
				pairs = append(pairs, value.Ref, value.Text)
			}
			*slot.value = strings.NewReplacer(pairs...).Replace(translated[entry.Ref])
		}
	}
	for _, value := range page.uiSlots {
		*value = englishUI(language, *value)
	}
	for _, entry := range page.concepts {
		raw, err := json.Marshal(entry.values)
		if err != nil {
			return err
		}
		entry.node.Concepts = string(raw)
	}
	for _, section := range page.view.Sections {
		if section.Map == nil {
			continue
		}
		for i := range section.Map.Nodes {
			node := &section.Map.Nodes[i]
			node.Title = mapTitle(node.FullTitle)
		}
		for i := range section.Map.Edges {
			edge := &section.Map.Edges[i]
			if len(edge.Lines) > 0 {
				edge.Lines = wrapToLines(edge.Label, mapEdgeLabelBudget, mapEdgeLabelLines)
			}
		}
	}
	return page.applyUI(language)
}

// Only known presentation fields enter this pass. Source anchors, code,
// quoted claims and enum values are deliberately absent.
func (page *PreparedPage) applyUI(language DisplayLanguage) error {
	view := page.view
	ui := func(value *string) { *value = localizedUIValue(language, *value) }
	for _, value := range []*string{&view.SummaryMissing, &view.LearningNote, &view.CardsMissing, &view.PortalsMissing, &view.NegativesMissing, &view.RecipeMissing} {
		ui(value)
	}
	for _, question := range view.Questions {
		ui(&question.Note)
		ui(&question.AnswerNote)
	}
	for _, reviews := range [][]pageLearningReview{view.LearningReviews, view.LearningSelections} {
		for i := range reviews {
			ui(&reviews[i].State)
			ui(&reviews[i].Reason)
		}
	}
	for i := range view.LearnBands {
		ui(&view.LearnBands[i].Title)
	}
	for i := range view.Figures {
		ui(&view.Figures[i].Value)
		ui(&view.Figures[i].Label)
		ui(&view.Figures[i].Note)
	}
	for i := range view.MutedCards {
		ui(&view.MutedCards[i].Stage)
		ui(&view.MutedCards[i].Reason)
	}
	for i := range view.Negatives {
		ui(&view.Negatives[i].Text)
	}
	for i := range view.Cards {
		ui(&view.Cards[i].Counts)
	}
	for _, section := range view.Sections {
		ui(&section.FlowMissing)
		for i := range section.Entrypoints {
			ui(&section.Entrypoints[i].Kind)
		}
		if section.Map != nil {
			for i := range section.Map.Lanes {
				ui(&section.Map.Lanes[i].Label)
			}
			for i := range section.Map.Nodes {
				node := &section.Map.Nodes[i]
				ui(&node.Subtitle)
				node.Title = mapTitle(node.FullTitle)
			}
		}
	}
	if view.RepoMap != nil {
		ui(&view.RepoMap.Caption)
		for i := range view.RepoMap.Lanes {
			ui(&view.RepoMap.Lanes[i].Title)
		}
		for i := range view.RepoMap.Nodes {
			ui(&view.RepoMap.Nodes[i].Detail)
			ui(&view.RepoMap.Nodes[i].Note)
		}
		for i := range view.RepoMap.Edges {
			ui(&view.RepoMap.Edges[i].Label)
		}
	}
	if language != English && page.timing != nil && page.timing.WallMS > 0 {
		line, err := uiText(language, "This run took {0} of wall clock.", localizedDurationWords(language, page.timing.WallMS))
		if err != nil {
			return err
		}
		view.Timing = []string{line}
		var total int64
		for _, stage := range page.timing.Stages {
			total += stage.ProviderMS
			line, err := uiText(language, "{0}: {1} live model calls, {2} from cache, {3} of provider time, slowest {4}.", englishUI(language, strings.ReplaceAll(stage.Stage, "_", " ")), stage.Live, stage.Cached, localizedDurationWords(language, stage.ProviderMS), localizedDurationWords(language, stage.SlowestMS))
			if err != nil {
				return err
			}
			view.Timing = append(view.Timing, line)
		}
		if len(page.timing.Stages) > 0 {
			line, err := uiText(language, "Provider time in all: {0}.", localizedDurationWords(language, total))
			if err != nil {
				return err
			}
			view.Timing = append(view.Timing, line)
		}
	}
	page.rebuildDisplayLabels(language)
	return nil
}

// Every destination was resolved in the English frontend. Reuse those exact
// links while composing labels; no translated name is used as an identity.
func (page *PreparedPage) rebuildDisplayLabels(language DisplayLanguage) {
	if language == English {
		return
	}
	view := page.view
	bySection := make(map[string]*pageSection, len(view.Sections))
	byTarget := make(map[string]*pageSection, len(view.Sections))
	oldShortByTarget := make(map[string]string, len(view.Sections))
	for _, section := range view.Sections {
		oldShort, oldLabel := section.ShortLabel, section.Label
		oldShortByTarget[section.programTargetID] = oldShort
		kind := englishUI(language, section.Kind)
		base := section.Root
		if base == "." {
			base = section.Name
		}
		if base == "" {
			base = section.Name
		}
		if strings.HasSuffix(oldShort, " ("+section.Kind+")") {
			base += " (" + kind + ")"
		}
		section.ShortLabel = base
		if oldLabel == section.Name+" ("+section.Kind+")" {
			section.Label = section.Name + " (" + kind + ")"
		}
		bySection[section.ID] = section
		byTarget[section.programTargetID] = section
		for i := range view.Cards {
			if view.Cards[i].SectionID == section.ID {
				view.Cards[i].Name = section.Label
				view.Cards[i].ShortName = section.ShortLabel
			}
		}
	}
	type destination struct {
		section *pageSection
		node    *pageMapNode
	}
	byNode := make(map[string]destination)
	byDestination := make(map[string]*pageSection)
	for _, section := range view.Sections {
		byDestination["#"+section.ID] = section
		for _, groups := range [][]pageGroup{section.Triggers, section.Core, section.DependencyGroups} {
			for i := range groups {
				byDestination["#"+groups[i].ID] = section
			}
		}
		if section.Map == nil {
			continue
		}
		for i := range section.Map.Nodes {
			node := &section.Map.Nodes[i]
			if node.Branch == "component" {
				if owner := byTarget[node.Component]; owner != nil {
					node.FullTitle = owner.ShortLabel
				}
			} else if node.Component != "" && node.Component != section.programTargetID {
				if owner := byTarget[node.Component]; owner != nil {
					prefix := oldShortByTarget[node.Component] + " / "
					if strings.HasPrefix(node.FullTitle, prefix) {
						node.FullTitle = owner.ShortLabel + " / " + strings.TrimPrefix(node.FullTitle, prefix)
					}
				}
			}
			node.Title = mapTitle(node.FullTitle)
			byNode[node.ID] = destination{section, node}
			byDestination["#"+node.ID] = section
		}
	}
	updateConnections := func(values []pageConnection) {
		for i := range values {
			link := &values[i]
			if link.OtherTarget != "" {
				if owner := byDestination[link.Href]; owner != nil {
					link.OtherTarget = owner.ShortLabel
				}
			}
		}
	}
	for _, section := range view.Sections {
		for _, groups := range [][]pageGroup{section.Triggers, section.Core, section.DependencyGroups} {
			for i := range groups {
				updateConnections(groups[i].Connections)
			}
		}
		for i := range section.Start {
			updateConnections(section.Start[i].Reaches)
		}
	}
	questionTitles := make(map[string]string, len(view.Questions))
	for _, question := range view.Questions {
		questionTitles[question.ID] = question.Question
	}
	updateLinks := func(values []pageLearnLink, withContext bool) {
		for i := range values {
			link := &values[i]
			id := strings.TrimPrefix(link.Href, "#")
			if title, ok := questionTitles[id]; ok {
				link.Title = title
				continue
			}
			if target, ok := byNode[id]; ok {
				link.Title = target.node.FullTitle
				if withContext {
					link.Title = target.section.ShortLabel + " / " + link.Title
				}
			}
		}
	}
	updateMapLinks := func(values []pageQuestionMapLink) {
		for i := range values {
			link := &values[i]
			if target, ok := byNode[link.NodeID]; ok {
				prefix := target.section.ShortLabel + " / "
				if strings.Contains(link.Label, " / file in ") {
					prefix += englishUI(language, "file in") + " "
				}
				link.Label = prefix + target.node.FullTitle
			}
		}
	}
	updateSteps := func(values []pageQuestionStep) {
		for i := range values {
			updateMapLinks(values[i].MapLinks)
		}
	}
	updateConcepts := func(values []pageLearnConcept) {
		for i := range values {
			updateLinks(values[i].Places, true)
		}
	}
	for i := range view.LearnBands {
		for j := range view.LearnBands[i].Parts {
			part := &view.LearnBands[i].Parts[j]
			if section := bySection[strings.TrimPrefix(part.Href, "#")]; section != nil {
				part.Title = section.ShortLabel
			}
			updateLinks(part.Areas, false)
		}
	}
	for i := range view.LearnQuestionTopics {
		updateLinks(view.LearnQuestionTopics[i].Questions, false)
	}
	updateConcepts(view.LearnConcepts)
	for _, question := range view.Questions {
		for i := range question.Origins {
			updateSteps(question.Origins[i].Checks)
		}
		for i := range question.Answers {
			answer := &question.Answers[i]
			updateMapLinks(answer.MapLinks)
			updateSteps(answer.Checks)
			updateConcepts(answer.Terms)
		}
		for i := range question.Readings {
			updateSteps(question.Readings[i].Steps)
		}
	}
	for _, values := range [][]pageLearningReview{view.LearningReviews, view.LearningSelections} {
		for i := range values {
			updateSteps(values[i].Checks)
		}
	}
	if view.RepoMap != nil {
		for i := range view.RepoMap.Nodes {
			node := &view.RepoMap.Nodes[i]
			if section := bySection[strings.TrimPrefix(node.Href, "#")]; section != nil {
				node.FullName = section.Label
				node.ShortName = section.ShortLabel
				node.Name = wrapToLines(strings.TrimSuffix(section.ShortLabel, " ("+englishUI(language, section.Kind)+")"), repoNameBudget, repoNameLines)
			}
		}
	}
}

var countedUIValue = regexp.MustCompile(`^([0-9,]+) (.+)$`)
var exploreUIValue = regexp.MustCompile(`^([0-9,]+) (parts|groups|components) · explore →$`)
var unreadUIValue = regexp.MustCompile(`^([0-9]+) of ([0-9]+) were read; the pale ones were not, and each says why\.$`)

func localizedDurationWords(language DisplayLanguage, ms int64) string {
	if language == English {
		return durationWords(ms)
	}
	seconds := int64((time.Duration(ms) * time.Millisecond).Round(time.Second) / time.Second)
	parts := make([]string, 0, 3)
	if hours := seconds / 3600; hours != 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hours, englishUI(language, "h")))
	}
	if minutes := seconds % 3600 / 60; minutes != 0 {
		parts = append(parts, fmt.Sprintf("%d %s", minutes, englishUI(language, "min")))
	}
	if remainder := seconds % 60; remainder != 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d %s", remainder, englishUI(language, "s")))
	}
	return strings.Join(parts, " ")
}

func localizedUIValue(language DisplayLanguage, text string) string {
	if language == English || text == "" {
		return text
	}
	if result := englishUI(language, text); result != text {
		return result
	}
	// This caption has one fixed sentence followed by an optional counted
	// sentence. Neither the count nor the failed component's source text is
	// a model translation slot.
	for _, edges := range []int{0, 1} {
		prefix := repoMapCaption(0, 0, edges)
		if suffix, ok := strings.CutPrefix(text, prefix+" "); ok && unreadUIValue.MatchString(suffix) {
			return englishUI(language, prefix) + " " + localizedUIValue(language, suffix)
		}
	}
	if match := exploreUIValue.FindStringSubmatch(text); match != nil {
		if value, err := uiText(language, "{0} "+match[2]+" · explore →", match[1]); err == nil {
			return value
		}
	}
	if match := unreadUIValue.FindStringSubmatch(text); match != nil {
		if value, err := uiText(language, "{0} of {1} were read; the pale ones were not, and each says why.", match[1], match[2]); err == nil {
			return value
		}
	}
	for _, prefix := range []string{"No test files found", "No Dockerfile or docker-compose file found", "No CI configuration found", "no license", "no contributing", "no changelog", "no linter config"} {
		if strings.HasPrefix(text, prefix+" (") && strings.HasSuffix(text, ").") {
			argument := englishUI(language, strings.TrimSuffix(strings.TrimPrefix(text, prefix+" ("), ")."))
			if value, err := uiText(language, prefix+" ({0}).", argument); err == nil {
				return value
			}
		}
	}
	if strings.Contains(text, " · ") {
		parts := strings.Split(text, " · ")
		for i := range parts {
			parts[i] = localizedUIValue(language, parts[i])
		}
		return strings.Join(parts, " · ")
	}
	if pieces := strings.Split(text, " of "); len(pieces) == 2 {
		if _, err := strconv.Atoi(pieces[0]); err == nil {
			if _, err := strconv.Atoi(pieces[1]); err == nil {
				if value, err := uiText(language, "{0} of {1}", pieces[0], pieces[1]); err == nil {
					return value
				}
			}
		}
	}
	if match := countedUIValue.FindStringSubmatch(text); match != nil {
		count, phrase := match[1], match[2]
		keys := map[string]string{"could not be read": "{0} could not be read", "route": "{0} routes", "routes": "{0} routes", "HTTP call out": "{0} HTTP calls out", "HTTP calls out": "{0} HTTP calls out", "place running handed-in code": "{0} places running handed-in code", "places running handed-in code": "{0} places running handed-in code", "file nothing reaches": "{0} files nothing reaches", "files nothing reaches": "{0} files nothing reaches", "HTTP call": "{0} HTTP calls", "HTTP calls": "{0} HTTP calls", "link": "{0} links", "links": "{0} links", "part": "{0} parts", "parts": "{0} parts", "component": "{0} components", "components": "{0} components", "group": "{0} groups", "groups": "{0} groups", "symbol": "{0} symbols", "symbols": "{0} symbols"}
		for _, relation := range []string{"inferred integrations", "callback bindings", "imports", "calls"} {
			keys[relation] = "{0} " + relation
		}
		if key, ok := keys[phrase]; ok {
			if value, err := uiText(language, key, count); err == nil {
				return value
			}
		}
	}
	return text
}
