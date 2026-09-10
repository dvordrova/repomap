package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dvordrova/repomap/internal/atlas"
	"reflect"
	"slices"
	"strings"
)

type pageQuestion struct {
	ID, Question, Note string
	AnswerNote         string
	Answers            []pageAnswerPart
	Readings           []pageQuestionReading
	Scope              []string
	UserQuestion       bool
	Origins            []pageQuestionOrigin
}

type pageQuestionOrigin struct {
	WhyRef               string
	Title, Question, Why string
	Checks               []pageQuestionStep
}

type pageLearningReview struct {
	ReasonRef            string
	Title, State, Reason string
	PartialContext       bool
	Checks               []pageQuestionStep
}

type pageQuestionReading struct {
	Steps []pageQuestionStep
}

type pageAnswerPart struct {
	TextRef, BasisRef, RemainingRef string
	CheckID                         string
	Text, Basis, Remaining          string
	RequestSHA256                   string
	OriginRow                       string
	Sources                         []pageAnchor
	MapLinks                        []pageQuestionMapLink
	Checks                          []pageQuestionStep
	Terms                           []pageLearnConcept
}

type pageQuestionStep struct {
	WhyRef    string
	Name, Why string
	Column    int
	Source    pageAnchor
	MapLinks  []pageQuestionMapLink
	Excerpts  []pageQuestionExcerpt
}

type pageQuestionExcerpt struct {
	Kind, Text    string
	AnswerCheckID string
	Source        pageAnchor
	Members       []pageQuestionMember
	ShowSource    bool
}

type pageQuestionMember struct {
	Text, Doc string
	Source    pageAnchor
}

type pageQuestionMapLink struct {
	Label, Href, NodeID string
}

func (step pageQuestionStep) ExcerptCheckIDs() []string {
	var ids []string
	for _, excerpt := range step.Excerpts {
		if excerpt.AnswerCheckID != "" && !slices.Contains(ids, excerpt.AnswerCheckID) {
			ids = append(ids, excerpt.AnswerCheckID)
		}
	}
	return ids
}

// Preserve the reading route and separately grounded model answer verbatim.
func (builder *pageBuilder) questionGuides() ([]*pageQuestion, error) {
	var views []*pageQuestion
	for _, route := range builder.data.Questions {
		view, err := builder.questionGuide(route)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (builder *pageBuilder) questionGuide(route atlas.QuestionRoute) (*pageQuestion, error) {
	if route.Version != atlas.QuestionRouteVersion || route.Revision != builder.data.CapturedRevision {
		return nil, fmt.Errorf("report: question route format or revision does not match")
	}
	view := &pageQuestion{ID: fmt.Sprintf("question-%x", sha256.Sum256([]byte(route.Question))), Question: route.Question, Scope: route.Scope}
	view.UserQuestion = route.UserQuestion
	for _, origin := range route.Origins {
		checks, err := builder.questionSources(origin.Sources)
		if err != nil {
			return nil, err
		}
		view.Origins = append(view.Origins, pageQuestionOrigin{Title: origin.Title, Question: origin.Question, Why: origin.Why, Checks: checks})
	}
	if route.Answer != nil {
		switch route.Answer.State {
		case "answered":
		case "partial":
			view.AnswerNote = "Part of the question remains open."
		case "unanswered":
			view.AnswerNote = "The evidence read in this report does not yet answer this question."
		case "not_applicable":
			view.AnswerNote = "This question's premise does not apply here."
		case "unavailable":
			view.AnswerNote = "The answer could not be completed. The available reading sources are below."
		default:
			return nil, fmt.Errorf("report: unknown question answer state")
		}
		for _, part := range route.Answer.Parts {
			if part.State == "unavailable" {
				continue
			}
			answer := pageAnswerPart{Text: part.Text, Basis: part.Basis, Remaining: part.Remaining, RequestSHA256: part.OriginRequest, OriginRow: part.OriginRow}
			for _, step := range part.Steps {
				bound, err := builder.questionStep(route, step)
				if err != nil {
					return nil, err
				}
				answer.Sources = append(answer.Sources, bound.Source)
				answer.Checks = append(answer.Checks, bound)
				for _, link := range bound.MapLinks {
					if !slices.Contains(answer.MapLinks, link) {
						answer.MapLinks = append(answer.MapLinks, link)
					}
				}
			}
			if part.Text != "" && len(answer.Sources) == 0 {
				return nil, fmt.Errorf("report: question answer has no sources")
			}
			view.Answers = append(view.Answers, answer)
		}
	}
	linkQuestionOriginExcerpts(view)
	guide := route.Guide
	if guide == nil || len(guide.Steps) == 0 {
		view.Note = "No reading route was selected for this question."
		if guide != nil && guide.State == "unavailable" {
			view.Note = "The reading guide could not be completed because some model answers were unavailable."
		}
		return view, nil
	}
	if guide.State == "partial" {
		view.Note = "This reading route is incomplete: some model answers were unavailable."
	}
	parts := guide.Parts
	if len(parts) > 0 {
		if len(parts) < 2 || (guide.State != "partitioned" && guide.State != "partial") {
			return nil, fmt.Errorf("report: independent readings have an inconsistent state")
		}
		var combined []atlas.QuestionStep
		for _, part := range parts {
			if len(part.Steps) == 0 || part.Source == atlas.SourceGiven {
				return nil, fmt.Errorf("report: independent reading lacks an accepted selection")
			}
			combined = append(combined, part.Steps...)
		}
		if !reflect.DeepEqual(combined, guide.Steps) {
			return nil, fmt.Errorf("report: independent readings do not match their source selection")
		}
		view.Note = strings.TrimSpace(view.Note + " These sources have separate reading orders; they have not been compared as one sequence.")
	} else {
		if guide.State == "partitioned" {
			return nil, fmt.Errorf("report: independent reading parts are missing")
		}
		parts = []atlas.QuestionGuidePart{{Steps: guide.Steps}}
	}
	for _, part := range parts {
		reading := pageQuestionReading{}
		for _, step := range part.Steps {
			bound, err := builder.questionStep(route, step)
			if err != nil {
				return nil, err
			}
			reading.Steps = append(reading.Steps, bound)
		}
		view.Readings = append(view.Readings, reading)
	}
	return view, nil
}

// Keep every original observation and display binding. Only an exact copy
// within this question may refer to its existing answer disclosure.
func linkQuestionOriginExcerpts(question *pageQuestion) {
	for i := range question.Answers {
		if len(question.Answers[i].Checks) > 0 {
			question.Answers[i].CheckID = fmt.Sprintf("%s-answer-check-%d", question.ID, i+1)
		}
	}
	for i := range question.Origins {
		for j := range question.Origins[i].Checks {
			check := &question.Origins[i].Checks[j]
			for k := range check.Excerpts {
				excerpt := &check.Excerpts[k]
			findAnswer:
				for _, answer := range question.Answers {
					for _, original := range answer.Checks {
						if original.Source != check.Source || original.Column != check.Column {
							continue
						}
						if slices.ContainsFunc(original.Excerpts, func(saved pageQuestionExcerpt) bool {
							return saved.Kind == excerpt.Kind && saved.Text == excerpt.Text && saved.Source == excerpt.Source &&
								saved.ShowSource == excerpt.ShowSource && slices.Equal(saved.Members, excerpt.Members)
						}) {
							excerpt.AnswerCheckID = answer.CheckID
							break findAnswer
						}
					}
				}
			}
		}
	}
}

func (builder *pageBuilder) questionStep(route atlas.QuestionRoute, step atlas.QuestionStep) (pageQuestionStep, error) {
	view := pageQuestionStep{Source: builder.links.anchor(step.Path, step.Line, step.Column), Column: step.Column}
	if len(step.StopIndexes) == 0 {
		return view, fmt.Errorf("report: question step has no candidate anchor")
	}
	for i, index := range step.StopIndexes {
		if index < 0 || index >= len(route.Stops) {
			return view, fmt.Errorf("report: question step has no candidate anchor")
		}
		stop := route.Stops[index]
		if stop.Path != step.Path || stop.Line != step.Line || stop.Column != step.Column {
			return view, fmt.Errorf("report: question step does not match its candidate anchor")
		}
		if i == 0 {
			view.Name, view.Why = stop.Name, stop.Why
		}
		excerpts, err := builder.questionExcerpts(stop)
		if err != nil {
			return view, err
		}
		for _, excerpt := range excerpts {
			if !slices.ContainsFunc(view.Excerpts, func(saved pageQuestionExcerpt) bool {
				return saved.Kind == excerpt.Kind && saved.Text == excerpt.Text && saved.Source == excerpt.Source && slices.Equal(saved.Members, excerpt.Members)
			}) {
				view.Excerpts = append(view.Excerpts, excerpt)
			}
		}
		for _, link := range builder.questionStepMapLinks(stop) {
			if !slices.Contains(view.MapLinks, link) {
				view.MapLinks = append(view.MapLinks, link)
			}
		}
	}
	return view, nil
}

func (builder *pageBuilder) questionSources(stops []atlas.QuestionStop) ([]pageQuestionStep, error) {
	var checks []pageQuestionStep
	for i, stop := range stops {
		check, err := builder.questionStep(atlas.QuestionRoute{Stops: stops}, atlas.QuestionStep{Path: stop.Path, Line: stop.Line, Column: stop.Column, StopIndexes: []int{i}})
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func (builder *pageBuilder) learningReviews(view *pageView) error {
	plan := builder.data.Learning
	if plan == nil {
		return nil
	}
	switch plan.State {
	case "ready":
	case "partial":
		view.LearningNote = "Some topics could not be reviewed. The available questions are below."
	case "unavailable":
		view.LearningNote = "Suggested questions could not be prepared. You can still explore the map and any questions you supplied."
	default:
		return fmt.Errorf("report: unknown learning plan state")
	}
	labels := map[string]string{"questions": "Questions suggested", "unknown": "Still open", "not_applicable": "Does not apply here", "unavailable": "Could not be reviewed"}
	for _, review := range plan.Reviews {
		label, ok := labels[review.State]
		if !ok {
			return fmt.Errorf("report: unknown learning review state")
		}
		checks, err := builder.questionSources(review.Sources)
		if err != nil {
			return err
		}
		reason := review.Reason
		if review.State == "unavailable" {
			reason = "No accepted model review was available for these sources."
		}
		view.LearningReviews = append(view.LearningReviews, pageLearningReview{Title: review.Title, State: label, Reason: reason, PartialContext: review.PartialContext, Checks: checks})
	}
	for _, selection := range plan.Selections {
		labels := map[string]string{"first_day": "In the introduction", "not_selected": "Not selected for this introduction", "unavailable": "Could not be reviewed"}
		label, ok := labels[selection.Audience]
		if !ok {
			return fmt.Errorf("report: unknown learning question audience")
		}
		var sources []atlas.QuestionStop
		for _, origin := range selection.Origins {
			sources = append(sources, origin.Sources...)
		}
		checks, err := builder.questionSources(sources)
		if err != nil {
			return err
		}
		reason := "Menu rationale: " + selection.Reason
		if selection.Audience == "unavailable" {
			reason = "No accepted model selection was available for this proposal."
		}
		if selection.Title != "" {
			label = selection.Title + " · " + label
		}
		if selection.PartialContext {
			label += " · compared within part of the question set"
		}
		view.LearningSelections = append(view.LearningSelections, pageLearningReview{Title: selection.Question, State: label, Reason: reason, Checks: checks})
	}
	return nil
}

func (builder *pageBuilder) questionExcerpts(stop atlas.QuestionStop) ([]pageQuestionExcerpt, error) {
	// Project the original selected observations only. A file candidate can
	// contain several declarations: each keeps its own anchor, never line 1
	// of the file or the location of a neighbouring selected declaration.
	raw, err := json.Marshal(stop.Evidence["evidence"])
	if err != nil {
		return nil, err
	}
	var originals []struct {
		Path           string   `json:"anchor_path"`
		Line           int      `json:"anchor_line"`
		Signature      string   `json:"signature"`
		Doc            string   `json:"author_doc"`
		Text           string   `json:"author_text"`
		Method         string   `json:"method"`
		Values         []string `json:"values"`
		Kind           string   `json:"kind"`
		Name           string   `json:"name"`
		EntrypointKind string   `json:"entrypoint_kind"`
		ManifestKey    string   `json:"manifest_key"`
		Value          string   `json:"value"`
		Language       string   `json:"language"`
		ComponentKind  string   `json:"component_kind"`
		Root           string   `json:"root"`
		Members        []struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Signature string `json:"signature"`
			Doc       string `json:"author_doc"`
		} `json:"owned_declarations"`
	}
	if err := json.Unmarshal(raw, &originals); err != nil {
		return nil, fmt.Errorf("report: question source excerpts: %w", err)
	}
	var excerpts []pageQuestionExcerpt
	for _, original := range originals {
		if original.Path == "" || original.Line < 1 {
			continue
		}
		anchor := builder.links.anchor(original.Path, original.Line, 0)
		// The stop already links its own location. Other original declarations
		// need one shared link for their signature, documentation and members.
		showSource := original.Path != stop.Path || original.Line != stop.Line
		add := func(kind, text string) {
			if strings.TrimSpace(text) != "" {
				excerpts = append(excerpts, pageQuestionExcerpt{Kind: kind, Text: text, Source: anchor, ShowSource: showSource})
				showSource = false
			}
		}
		add("Declaration", original.Signature)
		add("Author's documentation", original.Doc)
		add("Author's documentation", original.Text)
		members := pageQuestionExcerpt{Kind: "Declared members", Source: anchor, ShowSource: showSource}
		for _, member := range original.Members {
			if member.Path == "" || member.Line < 1 {
				continue
			}
			source := builder.links.anchor(member.Path, member.Line, 0)
			text := member.Signature
			if text == "" {
				text = strings.TrimSpace(member.Kind + " " + member.Name)
			}
			if text != "" || member.Doc != "" {
				members.Members = append(members.Members, pageQuestionMember{Text: text, Doc: member.Doc, Source: source})
			}
		}
		if len(members.Members) > 0 {
			excerpts = append(excerpts, members)
			showSource = false
		}
		if original.Kind == "entrypoint" {
			add("Observed entrypoint", original.Name+" · "+strings.ReplaceAll(original.EntrypointKind, "_", " ")+"\n"+original.Language+" "+original.ComponentKind+" in "+original.Root)
		}
		if original.Kind == "manifest" {
			add("Manifest value", original.ManifestKey+" = "+original.Value)
		}
		if original.Method != "" || len(original.Values) > 0 {
			add("Observed boundary", strings.TrimSpace(original.Method+" "+strings.Join(original.Values, ", ")))
		}
	}
	return excerpts, nil
}

func (builder *pageBuilder) questionStepMapLinks(stop atlas.QuestionStop) []pageQuestionMapLink {
	// A question can select an ordinary declaration, not just an operation.
	// Preserve every known owner/membership; same names or nearby lines do not
	// establish group membership. Documentation remains a source-only stop.
	var links []pageQuestionMapLink
	for _, index := range builder.indexes {
		section := builder.byProgram[index.Target.ID]
		if section == nil || section.Map == nil {
			continue
		}
		foundOperation := false
		for _, op := range index.Operations {
			if stop.SubjectID != "" && (op.SubjectID == stop.SubjectID || op.ID == stop.SubjectID) {
				id := operationNodeID(section.ID, op.ID)
				links = append(links, pageQuestionMapLink{Label: section.ShortLabel + " / " + builder.operationDisplayName(op), Href: "#" + id, NodeID: id})
				foundOperation = true
			}
		}
		if foundOperation || stop.SubjectID == "" {
			continue
		}
		exact := false
		for _, group := range index.Groups {
			if slices.Contains(group.MemberSubjectIDs, stop.SubjectID) {
				links = append(links, pageQuestionMapLink{Label: section.ShortLabel + " / " + group.Title, Href: "#" + groupAnchorID(section.ID, group.ID), NodeID: mapNodeID(group.ID)})
				exact = true
			}
		}
		if exact || (stop.Kind != "boundary" && stop.Kind != "file" && stop.Kind != "corpus_file") {
			continue
		}
		// Some boundary facts have no ProgramIndex subject. Their enclosing file
		// can still be explored, without pretending its groups execute this call.
		inFile := make(map[string]bool)
		for _, subject := range index.Subjects {
			if subject.Object != nil && subject.Object.Location != nil && subject.Object.Location.Path == stop.Path {
				inFile[subject.ID] = true
			}
		}
		for _, group := range index.Groups {
			if slices.ContainsFunc(group.MemberSubjectIDs, func(id string) bool { return inFile[id] }) {
				links = append(links, pageQuestionMapLink{Label: section.ShortLabel + " / " + group.Title, Href: "#" + groupAnchorID(section.ID, group.ID), NodeID: mapNodeID(group.ID)})
			}
		}
	}
	return links
}
