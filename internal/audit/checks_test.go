package audit

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The deterministic claim checks (design A3). Only a broken flow link
// (AUTO-1), an invented name (AUTO-2) and a missing quote (AUTO-4) are
// falsities; the rest are flags for a verifier and never fail the audit.

type flowLink struct {
	n                                    int
	target, subject, via, status, anchor string
	text                                 string
}

type wordingFlag struct {
	n                         int
	subject, says, code, text string
}

type flowCheck struct {
	links   []flowLink
	tally   map[string]int
	wording []wordingFlag
}

type edge struct {
	kind string
	to   string
}

func relationsOut(program programindex.Index) map[string][]edge {
	out := map[string][]edge{}
	for _, relation := range program.Relations {
		for _, to := range relation.ToIDs {
			out[relation.FromID] = append(out[relation.FromID], edge{string(relation.Kind), to})
		}
	}
	return out
}

// checkFlow is AUTO-1 and AUTO-3. Every step after the first must be a
// relation target of some earlier step, since a flow returns to its
// dispatcher: calls, another relation kind, the same subject again, or
// BROKEN.
func checkFlow(run *auditRun) flowCheck {
	result := flowCheck{tally: map[string]int{}}
	if run.orientation == nil {
		return result
	}
	factByID := map[string]facts.Fact{}
	for _, fact := range run.facts.Facts {
		factByID[fact.ID] = fact
	}
	outs := map[string]map[string][]edge{}
	type seenStep struct{ target, id string }
	var seen []seenStep
	for i, step := range run.orientation.MainFlow.Steps {
		link := flowLink{n: i + 1, target: step.TargetID, text: step.Explanation, via: "subject"}
		target := run.target(step.TargetID)
		if target == nil {
			link.status = "NO_PROGRAM_INDEX"
			result.tally[link.status]++
			result.links = append(result.links, link)
			continue
		}
		if outs[target.id] == nil {
			outs[target.id] = relationsOut(target.program)
		}
		out := outs[target.id]
		id := step.SubjectID
		if id == "" && step.FactID != "" {
			id = factByID[step.FactID].ObjectID
			link.via = "fact " + step.FactID
		}
		object, ok := target.objects[id]
		ok = ok && id != ""
		switch {
		case !ok:
			link.status = "UNRESOLVED"
		case len(seen) == 0:
			link.status = "first"
		default:
			kinds := map[string]bool{}
			same := false
			for _, previous := range seen {
				if previous.target != target.id {
					continue
				}
				same = same || previous.id == id
				for _, e := range out[previous.id] {
					if e.to == id {
						kinds[e.kind] = true
					}
				}
			}
			sorted := slices.Sorted(func(yield func(string) bool) {
				for kind := range kinds {
					if !yield(kind) {
						return
					}
				}
			})
			switch {
			case kinds["calls"]:
				link.status = "calls"
			case len(sorted) > 0:
				link.status = sorted[0]
			case same:
				link.status = "same"
			default:
				link.status = "BROKEN"
			}
		}
		result.tally[link.status]++
		if ok {
			link.subject = object.Name
			link.anchor = locationAt(object.Location).String()
			result.wording = append(result.wording, checkWording(target, id, object.Name, out, step.Explanation, i+1)...)
			seen = append(seen, seenStep{target.id, id})
		}
		result.links = append(result.links, link)
	}
	return result
}

var (
	callWords     = regexp.MustCompile(`\b(call|calls|calling|called|invokes|invoking)\s+((?:the|its|a|an)\s+)?([A-Za-z_][A-Za-z0-9_.]*(?:\(\))?)((?:\s*(?:,|and|,\s*and|or)\s+(?:the\s+)?[A-Za-z_][A-Za-z0-9_.]*(?:\(\))?)*)`)
	registerWords = regexp.MustCompile(`\b(register|registers|registering|registered|installs|installing)\s+((?:the|its|a|an)\s+)?([A-Za-z_][A-Za-z0-9_.]*)((?:\s*(?:,|and|,\s*and)\s+(?:the\s+)?[A-Za-z_][A-Za-z0-9_.]*)*)`)
	nameWord      = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]*(?:\(\))?`)
)

// checkWording is AUTO-3: a step that says it calls X where the code's
// relation to X is not a call, or registers X where the code only calls it.
// A flag for the verifier, never a verdict.
func checkWording(target *auditTarget, id, subject string, out map[string][]edge, text string, n int) []wordingFlag {
	funcs := map[string][]string{}
	for _, object := range target.program.Objects {
		switch object.Kind {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda:
			parts := strings.Split(object.Name, ".")
			funcs[parts[len(parts)-1]] = append(funcs[parts[len(parts)-1]], object.ID)
			funcs[object.Name] = append(funcs[object.Name], object.ID)
		}
	}
	var flags []wordingFlag
	for _, pattern := range []struct {
		re   *regexp.Regexp
		word string
	}{{callWords, "calls"}, {registerWords, "registers"}} {
		for _, m := range pattern.re.FindAllStringSubmatch(text, -1) {
			names := append([]string{m[3]}, nameWord.FindAllString(m[4], -1)...)
			for _, raw := range names {
				callee := strings.TrimRight(raw, "()")
				if callee == "and" || callee == "or" || callee == "the" {
					continue
				}
				parts := strings.Split(callee, ".")
				key := parts[len(parts)-1]
				if funcs[key] == nil && funcs[callee] == nil {
					continue
				}
				ids := map[string]bool{}
				for _, x := range append(slices.Clone(funcs[callee]), funcs[key]...) {
					ids[x] = true
				}
				kindSet := map[string]bool{}
				for _, e := range out[id] {
					if ids[e.to] {
						kindSet[e.kind] = true
					}
				}
				kinds := slices.Sorted(func(yield func(string) bool) {
					for kind := range kindSet {
						if !yield(kind) {
							return
						}
					}
				})
				if pattern.word == "calls" && !kindSet["calls"] {
					code := strings.Join(kinds, "/")
					if code == "" {
						code = "no relation from the step's subject"
					}
					flags = append(flags, wordingFlag{n, subject, m[1] + " " + callee, code, text})
				}
				if pattern.word == "registers" && kindSet["calls"] && len(kinds) == 1 {
					flags = append(flags, wordingFlag{n, subject, m[1] + " " + callee, "calls only (no registration/callback relation)", text})
				}
			}
		}
	}
	return flags
}

type nameMiss struct{ typ, where, token, text string }

type namesCheck struct {
	counts  map[string]int
	types   []string
	perType map[string]map[string]int
	misses  []nameMiss
}

var codeToken = regexp.MustCompile("`([^`]+)`|\\b([A-Za-z_][A-Za-z0-9_]*(?:\\.[A-Za-z_][A-Za-z0-9_]*)+|[a-z]+[A-Z][A-Za-z0-9]*|[A-Za-z0-9]+_[A-Za-z0-9_]+|[A-Za-z_][A-Za-z0-9_]*\\(\\))")

var pathOrDot = regexp.MustCompile(`[/.]`)

type prose struct{ typ, where, text string }

// modelProse is every model-authored sentence the page displays.
func modelProse(run *auditRun) []prose {
	var texts []prose
	if o := run.orientation; o != nil {
		if o.Summary != "" {
			texts = append(texts, prose{"summary", "", o.Summary})
		}
		for _, role := range o.Roles {
			texts = append(texts, prose{"role", role.TargetID, role.Role + " — " + role.Purpose})
		}
		for _, recipe := range o.RunRecipe {
			texts = append(texts, prose{"recipe", recipe.TargetID, recipe.Command + " — " + recipe.Note})
		}
		for i, step := range o.MainFlow.Steps {
			texts = append(texts, prose{"flow", fmt.Sprintf("step %d", i+1), step.Explanation})
		}
	}
	if run.atlas != nil {
		for _, target := range run.atlas.Targets {
			for _, zone := range target.Zones {
				texts = append(texts, prose{"area", target.ID, zone.Title + " — " + zone.Line})
			}
			for _, box := range target.Boxes {
				texts = append(texts, prose{"part", target.ID, box.Title + " — " + box.Line})
			}
		}
	}
	for _, target := range run.targets {
		for _, subject := range target.index.Subjects {
			if subject.Interpretation != nil && subject.Interpretation.Line != "" {
				texts = append(texts, prose{"declaration line", target.id, subject.Interpretation.Line})
			}
		}
	}
	if run.glossary != nil {
		for _, entry := range run.glossary.Entries {
			texts = append(texts, prose{"glossary", strings.Join(entry.Names, ", "), entry.Explanation})
		}
	}
	return texts
}

// checkNames is AUTO-2: every code-like token in model prose is a tracked
// path or a target's name, a ProgramIndex name, or written in the source at
// the revision. Anything else is an invented identifier.
func checkNames(run *auditRun, source sourceTree) (namesCheck, error) {
	result := namesCheck{counts: map[string]int{}, perType: map[string]map[string]int{}}
	names := map[string]bool{}
	for _, target := range run.targets {
		for _, object := range target.program.Objects {
			names[object.Name] = true
			for _, part := range strings.Split(object.Name, ".") {
				names[part] = true
			}
		}
	}
	targets := map[string]bool{}
	for _, target := range run.targets {
		targets[target.display] = true
		for _, part := range pathOrDot.Split(target.display, -1) {
			targets[part] = true
		}
	}
	for _, target := range run.facts.Targets {
		targets[target.Name] = true
		for _, part := range pathOrDot.Split(target.Name, -1) {
			targets[part] = true
		}
	}
	for _, text := range modelProse(run) {
		if result.perType[text.typ] == nil {
			result.perType[text.typ] = map[string]int{}
			result.types = append(result.types, text.typ)
		}
		result.perType[text.typ]["texts"]++
		for _, m := range codeToken.FindAllStringSubmatch(text.text, -1) {
			word := m[1]
			if word == "" {
				word = m[2]
			}
			word = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(word), "()"))
			if word == "" {
				continue
			}
			parts := strings.Split(word, ".")
			how := ""
			switch {
			case targets[word] || source.isPath(word):
				how = "path/target"
			case names[word] || names[parts[len(parts)-1]]:
				how = "index"
			default:
				found, err := source.grep(word, nil)
				if err != nil {
					return result, err
				}
				how = "source"
				if !found {
					how = "NOT FOUND"
					result.misses = append(result.misses, nameMiss{text.typ, text.where, word, shorten(text.text, 160)})
				}
			}
			result.counts[how]++
			result.perType[text.typ][how]++
		}
	}
	return result, nil
}

type quoteCheck struct {
	target, quote string
	files         []string
	how           string // verbatim, format, MISSING
}

var (
	quotedText    = regexp.MustCompile(`"([^"]+)"|“([^”]+)”`)
	stringLiteral = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'`)
	formatVerb    = regexp.MustCompile(`%[-+ #0]*[0-9*]*(?:\.[0-9*]+)?[a-zA-Z]|\{[^{}]*\}`)
)

// checkQuotes is AUTO-4: text quoted in a run recipe's note is written in
// a file the note cites (its facts' files, else its target's files),
// verbatim or with a printf/format placeholder standing for a value.
func checkQuotes(run *auditRun, source sourceTree) ([]quoteCheck, error) {
	if run.orientation == nil {
		return nil, nil
	}
	factByID := map[string]facts.Fact{}
	for _, fact := range run.facts.Facts {
		factByID[fact.ID] = fact
	}
	var result []quoteCheck
	for _, recipe := range run.orientation.RunRecipe {
		quotes := quotedText.FindAllStringSubmatch(recipe.Note, -1)
		if len(quotes) == 0 {
			continue
		}
		var files []string
		for _, id := range recipe.FactIDs {
			if fact, ok := factByID[id]; ok && fact.Anchor != nil && !slices.Contains(files, fact.Anchor.Path) {
				files = append(files, fact.Anchor.Path)
			}
		}
		if len(files) == 0 {
			files = targetFiles(run, recipe.TargetID)
		}
		for _, m := range quotes {
			quote := m[1]
			if quote == "" {
				quote = m[2]
			}
			check := quoteCheck{target: recipe.TargetID, quote: quote, files: files, how: "MISSING"}
			if len(files) > 0 {
				found, err := source.grep(quote, files)
				if err != nil {
					return nil, err
				}
				if found {
					check.how = "verbatim"
				} else if matched, err := formatMatch(source, files, quote); err != nil {
					return nil, err
				} else if matched {
					check.how = "format"
				}
			}
			result = append(result, check)
		}
	}
	return result, nil
}

// formatMatch reports whether quote is a string literal of files with each
// placeholder filled by some value.
func formatMatch(source sourceTree, files []string, quote string) (bool, error) {
	for _, file := range files {
		text, err := source.read(file)
		if err != nil {
			return false, err
		}
		for _, m := range stringLiteral.FindAllStringSubmatch(text, -1) {
			literal := m[1] + m[2]
			if !formatVerb.MatchString(literal) {
				continue
			}
			literal = strings.TrimSpace(strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\'`, "'").Replace(literal))
			var pattern strings.Builder
			pattern.WriteString("^")
			last := 0
			for _, loc := range formatVerb.FindAllStringIndex(literal, -1) {
				pattern.WriteString(regexp.QuoteMeta(literal[last:loc[0]]))
				pattern.WriteString(".+?")
				last = loc[1]
			}
			pattern.WriteString(regexp.QuoteMeta(literal[last:]))
			pattern.WriteString("$")
			if regexp.MustCompile(pattern.String()).MatchString(strings.TrimSpace(quote)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func targetFiles(run *auditRun, targetID string) []string {
	target := run.target(targetID)
	if target == nil {
		return nil
	}
	set := map[string]bool{}
	for _, object := range target.program.Objects {
		if object.Location != nil && object.Location.Path != "" {
			set[object.Location.Path] = true
		}
	}
	return slices.Sorted(func(yield func(string) bool) {
		for path := range set {
			if !yield(path) {
				return
			}
		}
	})
}

type recipeWord struct {
	word     string
	flag     bool
	resolved string
}

type recipeCheck struct {
	target, command, program, programResolved string
	words                                     []recipeWord
}

var (
	environmentAssignment = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=`)
	scriptExtension       = regexp.MustCompile(`\.(py|rb|sh)$`)
	builtinHelp           = map[string]bool{"-h": true, "--help": true, "-help": true}
)

// checkRecipes is AUTO-5: the recipe's program is a target's executable or
// entry, and each flag is a flag literal in facts, a registered input of the
// target, or a quoted literal in its files. A flag for the verifier.
func checkRecipes(run *auditRun, source sourceTree, rows []*reportRow) ([]recipeCheck, error) {
	if run.orientation == nil {
		return nil, nil
	}
	var blob []string
	for _, fact := range run.facts.Facts {
		switch fact.Kind {
		case facts.KindRegistration, facts.KindEntrypoint, facts.KindConfigRead:
			blob = append(blob, strings.Join([]string{fact.Key, fact.Text, fact.Symbol}, " ")+" "+strings.Join(fact.Values, " "))
		}
	}
	factBlob := strings.Join(blob, "\n")
	factWords := map[string]bool{}
	for _, word := range strings.Fields(factBlob) {
		factWords[word] = true
	}
	quotedInFacts := func(literal string) bool {
		return regexp.MustCompile(`["']` + regexp.QuoteMeta(literal) + `["']`).MatchString(factBlob)
	}
	inputNames := map[string]map[string]bool{}
	for _, row := range rows {
		if row.section != "inputs" {
			continue
		}
		if inputNames[row.target] == nil {
			inputNames[row.target] = map[string]bool{}
		}
		name := strings.TrimSpace(row.name)
		inputNames[row.target][name] = true
		if fields := strings.Fields(name); len(fields) > 0 {
			inputNames[row.target][fields[0]] = true
		}
	}
	words := map[string]bool{}
	addName := func(name string) {
		words[name] = true
		slash := strings.Split(name, "/")
		words[slash[len(slash)-1]] = true
		dot := strings.Split(name, ".")
		words[dot[len(dot)-1]] = true
	}
	for _, target := range run.targets {
		addName(target.display)
	}
	for _, target := range run.facts.Targets {
		addName(target.Name)
		words[target.Root] = true
		if target.Anchor.Path != "" {
			words[target.Anchor.Path] = true
		}
	}
	for _, fact := range run.facts.Facts {
		if fact.Kind == facts.KindEntrypoint {
			words[fact.Symbol] = true
			words[strings.Split(fact.Symbol, ".")[0]] = true
		}
	}
	quotedInFiles := func(literal string, files []string) (bool, error) {
		if len(files) == 0 {
			return false, nil
		}
		if found, err := source.grep(`"`+literal+`"`, files); err != nil || found {
			return found, err
		}
		return source.grep(`'`+literal+`'`, files)
	}
	var result []recipeCheck
	for _, recipe := range run.orientation.RunRecipe {
		tokens := strings.Fields(recipe.Command)
		for len(tokens) > 0 && (environmentAssignment.MatchString(tokens[0]) || tokens[0] == "sudo" || tokens[0] == "env") {
			tokens = tokens[1:]
		}
		var program string
		var rest []string
		switch {
		case len(tokens) > 2 && (tokens[0] == "python" || tokens[0] == "python3") && tokens[1] == "-m":
			program, rest = tokens[2], tokens[3:]
		case len(tokens) > 1 && slices.Contains([]string{"python", "python3", "node", "ruby", "bash", "sh", "tclsh"}, tokens[0]):
			program, rest = tokens[1], tokens[2:]
		case len(tokens) > 2 && tokens[0] == "go" && (tokens[1] == "run" || tokens[1] == "build"):
			program, rest = tokens[2], tokens[3:]
		case len(tokens) > 0:
			program, rest = tokens[0], tokens[1:]
		}
		p := program
		if strings.HasPrefix(p, "./") {
			p = strings.TrimLeft(p, "./")
		}
		slash := strings.Split(p, "/")
		last := slash[len(slash)-1]
		resolved := "FLAGGED"
		for _, c := range []string{p, strings.TrimRight(p, "/"), last, scriptExtension.ReplaceAllString(last, "")} {
			if words[c] {
				resolved = "target/entry"
			}
		}
		files := targetFiles(run, recipe.TargetID)
		check := recipeCheck{target: recipe.TargetID, command: recipe.Command, program: program, programResolved: resolved}
		for _, word := range rest {
			if strings.HasPrefix(word, "<") || strings.HasPrefix(word, "[") || strings.HasSuffix(word, "]") || strings.HasSuffix(word, ">") {
				continue
			}
			flag := strings.HasPrefix(word, "-")
			literal := strings.Split(word, "=")[0]
			own := inputNames[recipe.TargetID]
			bare := strings.TrimLeft(literal, "-")
			how := ""
			switch {
			case flag && (quotedInFacts(literal) || factWords[literal]):
				how = "fact"
			case flag && (own[literal] || own[bare] && bare != literal):
				how = "registered input of this target"
			case !flag && (own[literal] || quotedInFacts(literal)):
				how = "registered input / fact"
			}
			if how == "" {
				inFiles, err := quotedInFiles(literal, files)
				if err != nil {
					return nil, err
				}
				switch {
				case inFiles:
					how = "quoted literal in the target's files"
				case flag && builtinHelp[literal]:
					how = "FLAGGED (parser builtin help, not in code)"
				default:
					how = "FLAGGED"
				}
			}
			check.words = append(check.words, recipeWord{word, flag, how})
		}
		result = append(result, check)
	}
	return result, nil
}

type catalogue struct {
	name  string
	n     int
	kinds map[string]int
	first string
	names []string
}

type unboundComponent struct {
	component      string
	inputs         int
	withHandler    int
	handlerUnknown map[string]int
	catalogues     []*catalogue
	unreachable    []*reportRow
}

// checkUnbound is AUTO-6: inputs whose handler is not established, by the
// catalogue that declares them, and handlers ProgramIndex marks unreachable.
// A handler-less request catalogue is flagged for the verifier. Rows the
// product nests under another input are no inputs of their own (fix 0).
func checkUnbound(rows []*reportRow) []*unboundComponent {
	var result []*unboundComponent
	byComponent := map[string]*unboundComponent{}
	for _, row := range rows {
		if row.section != "inputs" || row.nested {
			continue
		}
		entry := byComponent[row.component]
		if entry == nil {
			entry = &unboundComponent{component: row.component, handlerUnknown: map[string]int{}}
			byComponent[row.component] = entry
			result = append(result, entry)
		}
		entry.inputs++
		if row.handlerUnknown {
			entry.handlerUnknown[row.kind]++
			name := row.declaredBy
			if name == "" {
				name = "?"
			}
			if row.declaredOn != "" {
				name += " · " + shorten(row.declaredOn, 70)
			}
			var c *catalogue
			for _, existing := range entry.catalogues {
				if existing.name == name {
					c = existing
				}
			}
			if c == nil {
				c = &catalogue{name: name, kinds: map[string]int{}, first: row.at.String()}
				entry.catalogues = append(entry.catalogues, c)
			}
			c.kinds[row.kind]++
			c.n++
			if len(c.names) < 6 {
				c.names = append(c.names, shorten(row.name, 30))
			}
		} else {
			entry.withHandler++
		}
		if row.handlerUnreachable {
			entry.unreachable = append(entry.unreachable, row)
		}
	}
	return result
}

func shorten(text string, n int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n-1]) + "…"
}
