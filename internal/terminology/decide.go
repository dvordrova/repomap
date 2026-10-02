package terminology

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The term decision's options: only a decided domain concept goes into the
// glossary.
const (
	TermDomainConcept     = "domain_concept"
	TermGeneralVocabulary = "general_vocabulary"
	TermCodeElement       = "code_element"
)

// termPrompt is what we want, the state every term question shares.
//
//go:embed prompts/term.md
var termPrompt string

//go:embed prompts/term_options.md
var termOptionsText string

var termOptions = mustTermOptions()

func mustTermOptions() map[string]llm.Criteria {
	options, err := parseOptionCriteria(termOptionsText)
	if err != nil {
		panic(fmt.Sprintf("terminology: prompts/term_options.md: %v", err))
	}
	for _, name := range []string{TermDomainConcept, TermGeneralVocabulary, TermCodeElement} {
		if _, found := options[name]; !found {
			panic(fmt.Sprintf("terminology: prompts/term_options.md lacks option %q", name))
		}
	}
	if len(options) != 3 {
		panic("terminology: prompts/term_options.md has an option the decision does not know")
	}
	return options
}

// parseOptionCriteria reads "## option" sections, each with What, Includes,
// Not for and a list of Examples.
func parseOptionCriteria(text string) (map[string]llm.Criteria, error) {
	options := map[string]llm.Criteria{}
	sections := strings.Split(text, "\n## ")
	for _, section := range sections[1:] {
		lines := strings.Split(section, "\n")
		name := strings.TrimSpace(lines[0])
		var criteria llm.Criteria
		examples := false
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			switch {
			case line == "":
			case strings.HasPrefix(line, "What: "):
				criteria.What, examples = strings.TrimPrefix(line, "What: "), false
			case strings.HasPrefix(line, "Includes: "):
				criteria.Includes, examples = strings.TrimPrefix(line, "Includes: "), false
			case strings.HasPrefix(line, "Not for: "):
				criteria.NotFor, examples = strings.TrimPrefix(line, "Not for: "), false
			case line == "Examples:":
				examples = true
			case examples && strings.HasPrefix(line, "- "):
				criteria.Examples = append(criteria.Examples, strings.TrimPrefix(line, "- "))
			default:
				return nil, fmt.Errorf("option %q: unreadable line %q", name, line)
			}
		}
		if name == "" || criteria.What == "" || criteria.Includes == "" || criteria.NotFor == "" || len(criteria.Examples) == 0 {
			return nil, fmt.Errorf("option %q lacks what, includes, not for or examples", name)
		}
		if _, repeated := options[name]; repeated {
			return nil, fmt.Errorf("option %q is given twice", name)
		}
		options[name] = criteria
	}
	return options, nil
}

// TermDefinition is the closed question asked of every found name: what it
// is for a newcomer's glossary. Each option carries its criteria; the
// categorizer's margin rule decides, and a near-tie stays undecided.
func TermDefinition() table.Definition {
	return table.Definition{
		Stage: StageName, Contract: "repomap.glossary.term.v1", System: termPrompt,
		Classifier: true,
		Columns: []table.Column{{
			Name: "term", Kind: table.Choice, Options: []string{TermDomainConcept, TermGeneralVocabulary, TermCodeElement},
			Criteria: termOptions, Item: "candidate",
			Ask: "For a newcomer's glossary of this program, is `candidate` a domain concept, general vocabulary or a code element?",
		}},
	}
}

// termFields is the item of one name's question: the name and every prose
// text that writes it, as written, each once and in row order. Nothing is
// left out: FitClassifierWindows packs whole items, and an item over the
// categorizer's envelope is refused before any request, never cut.
func termFields(items []proseSource, name glossaryName) []table.Field {
	written := []string{}
	seen := make(map[string]bool)
	for _, row := range name.Rows {
		for _, text := range items[row].Texts {
			if !seen[text] && mentionsTerm(text, name.Name) {
				seen[text] = true
				written = append(written, text)
			}
		}
	}
	return []table.Field{{Name: "name", Value: name.Name}, {Name: "written", Value: written}}
}

// rememberedTerm is a name's memo: the categorizer response and the row in
// it that answered the name's exact question.
type rememberedTerm struct {
	RequestKey string `json:"request_key"`
	RowKey     string `json:"row_key"`
}

// termBasis is the identity of one name's decision: the provider, the
// question's task, options and rule, the shared context and the name's own
// item. The same name asked again with the same item takes the same answer,
// whatever other names shared its request.
func termBasis(c llm.Categorizer, def table.Definition, context, fields []table.Field) (string, error) {
	values := func(fields []table.Field) map[string]any {
		result := make(map[string]any, len(fields))
		for _, field := range fields {
			result[field.Name] = field.Value
		}
		return result
	}
	system := sha256.Sum256([]byte(def.System))
	evidence, err := json.Marshal(map[string]any{
		"contract": def.Contract, "system_sha256": hex.EncodeToString(system[:]), "ask": def.Columns[0].Ask,
		"options": def.Columns[0].Options, "criteria": def.Columns[0].Criteria, "margin": table.ClassifierMargin,
		"context": values(context), "item": values(fields),
	})
	if err != nil {
		return "", err
	}
	return llm.MemoIdentityBytes(c, []byte(`{"memo":"repomap.glossary.term"}`), evidence)
}

// decideNames asks the categorizer one closed question per name and returns,
// by name, its decided option, NameUndecided for a near-tie or NameUnanswered
// when no accepted answer reached it. Every decided or undecided answer is
// remembered per name: a warm run asks nothing, and a name whose decision was
// a near-tie is not asked again for a clearer draw.
func decideNames(ctx context.Context, executor llm.Executor, c llm.Categorizer, items []proseSource, names []glossaryName, program string, progress func(state, detail string)) ([]string, error) {
	decisions := make([]string, len(names))
	if len(names) == 0 {
		return decisions, nil
	}
	def := table.ForClassifier(TermDefinition())
	var shared []table.Field
	if program = strings.TrimSpace(program); program != "" {
		shared = []table.Field{{Name: "program", Value: program}}
	}
	decide := func(window table.Window, verdicts map[string]llm.Verdict) (string, bool) {
		result, err := table.DecodeClassifierAnswers(def, window, verdicts)
		switch {
		case err != nil:
			return "", false
		case result.Answers[0] != nil:
			return result.Answers[0]["term"], true
		case result.Uncertain(0):
			return NameUndecided, true
		}
		return "", false
	}
	rows := make([]table.Row, len(names))
	bases := make([]string, len(names))
	var missing []table.Row
	byRow := make(map[string]int, len(names))
	for i, name := range names {
		fields := termFields(items, name)
		rows[i] = table.Row{ID: name.Name, Fields: fields}
		byRow[name.Name] = i
		var err error
		if bases[i], err = termBasis(c, def, shared, fields); err != nil {
			return nil, err
		}
		decisions[i] = NameUnanswered
		memo, found, err := llm.LoadMemo(executor, bases[i], llm.DecodeJSON(func(value rememberedTerm) error {
			if len(value.RequestKey) != 64 || value.RowKey == "" {
				return fmt.Errorf("glossary: invalid remembered term")
			}
			return nil
		}))
		if err == nil && found {
			exchange, found, err := llm.CachedExchange(executor.RootDir, memo.RequestKey)
			if err == nil && found {
				if verdicts, err := c.Verdicts(exchange.Response); err == nil {
					original := table.Window{Stage: def.Stage, Context: shared, Rows: []table.Row{{ID: memo.RowKey, Fields: fields}}}
					if decision, ok := decide(original, verdicts); ok {
						decisions[i] = decision
						continue
					}
				}
			}
		}
		missing = append(missing, rows[i])
	}
	if len(missing) == 0 {
		return decisions, nil
	}
	windows, err := table.WindowsWithContext(def, 0, shared, missing)
	if err != nil {
		return nil, err
	}
	if windows, err = table.FitClassifierWindows(c, def, windows); err != nil {
		return nil, err
	}
	calls := make([]llm.Call[termAnswers], 0, len(windows))
	for _, window := range windows {
		if window.Refused != "" {
			// A name over the categorizer's envelope is never sent: it
			// stays unanswered, and the run says why.
			if progress != nil {
				progress("refused", window.Refused)
			}
			continue
		}
		call, err := table.ClassifierCall(c, def, window)
		if err != nil {
			return nil, err
		}
		decode := call.DecodeValidate
		calls = append(calls, llm.Call[termAnswers]{State: call.State, Prompt: call.Prompt, Limits: call.Limits,
			DecodeValidate: func(raw []byte) (termAnswers, error) {
				result, err := decode(raw)
				return termAnswers{Result: result, window: window}, err
			}})
	}
	// The categorizer has its own rate limits and its own gate.
	executor.BatchConcurrency, executor.BatchController = table.ClassifierConcurrency, &llm.BatchController{}
	outcomes := llm.ExecuteJSONEach(ctx, executor, c, calls)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, outcome := range outcomes {
		if outcome.Err != nil {
			// The window stays unanswered; the executor journaled the refusal.
			continue
		}
		for j, row := range outcome.Outcome.Value.window.Rows {
			i := byRow[row.ID]
			switch {
			case outcome.Outcome.Value.Answers[j] != nil:
				decisions[i] = outcome.Outcome.Value.Answers[j]["term"]
			case outcome.Outcome.Value.Uncertain(j):
				decisions[i] = NameUndecided
			default:
				continue
			}
			if outcome.Outcome.CacheKey == "" {
				continue
			}
			raw, err := json.Marshal(rememberedTerm{RequestKey: outcome.Outcome.CacheKey, RowKey: row.ID})
			if err != nil {
				return nil, err
			}
			if err := llm.SaveMemo(executor, bases[i], raw); err != nil {
				return nil, fmt.Errorf("glossary: remember term decision: %w", err)
			}
		}
	}
	return decisions, nil
}

// termAnswers is one categorizer window's decisions. Its journal names the
// undecided names and every name decided to be no domain concept, which the
// glossary leaves out.
type termAnswers struct {
	table.Result
	window table.Window
}

func (answers termAnswers) ResponseRejections() []llm.ResponseRejection {
	rejections := answers.Result.ResponseRejections()
	for i := range rejections {
		if rejections[i].Kind == "row_rejected" {
			rejections[i].Kind = "glossary_name_undecided"
		}
	}
	declined := make(map[string]int)
	for j, answer := range answers.Answers {
		if answer == nil || answer["term"] == TermDomainConcept {
			continue
		}
		index, found := declined[answer["term"]]
		if !found {
			index = len(rejections)
			declined[answer["term"]] = index
			rejections = append(rejections, llm.ResponseRejection{Kind: "glossary_name_declined", Reason: "decided " + answer["term"]})
		}
		rejections[index].Count++
		if len(rejections[index].Samples) < 5 {
			rejections[index].Samples = append(rejections[index].Samples, answers.window.Rows[j].ID)
		}
	}
	return rejections
}
