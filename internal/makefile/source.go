package makefile

import (
	"fmt"
	"strings"
	"unicode"
)

// ReadSource reads one owning makefile and only files its include directives
// name. resolve binds filenames to the invocation's working directory and
// repository namespace; read supplies complete source from that namespace.
// No shell, Make function or conditional is executed. Every include occurrence
// and every rule/assignment retain their original source location and condition.
func ReadSource(manifest string, read func(string) ([]string, bool), resolve func(string) (string, bool)) []Row {
	file := &makefile{sourceAware: true}
	active := map[string]bool{}
	var enter func(string, []string) bool
	enter = func(sourcePath string, conditions []string) bool {
		lines, available := read(sourcePath)
		if !available {
			return false
		}
		active[sourcePath] = true
		defer delete(active, sourcePath)
		file.read(lines, sourcePath, conditions, func(from, directive string, line int, inherited []string) {
			expression := ""
			if at := strings.IndexFunc(directive, unicode.IsSpace); at >= 0 {
				expression = directive[at:]
			}
			expression = strings.TrimSpace(expression)
			row := Row{Path: from, Key: "include", Value: directive, Line: line}
			if condition := strings.Join(inherited, " and "); condition != "" {
				row.Key += " when " + condition
			}
			// Keep this occurrence before its children in source reading order.
			position := len(file.includes)
			file.includes = append(file.includes, row)
			var status []string
			for _, word := range makeWords(file.expand(expression, map[string]bool{})) {
				if strings.ContainsAny(word, "$*?[]\\") {
					status = append(status, "unresolved expression "+word)
					continue
				}
				name, canonical := resolve(word)
				if !canonical {
					status = append(status, "unresolved repository path "+word)
					continue
				}
				if active[name] {
					status = append(status, "cycle to "+name)
					continue
				}
				if !enter(name, inherited) {
					status = append(status, "unavailable source "+name)
					continue
				}
				status = append(status, "reads "+name)
			}
			if len(status) == 0 {
				status = append(status, "unresolved filenames")
			}
			file.includes[position].Value += " — " + strings.Join(status, "; ")
		})
		return true
	}
	if !enter(manifest, nil) {
		return nil
	}
	goal := file.defaultGoal()
	var rows []Row
	var advertised []*makeRule
	for _, rule := range file.rules {
		if rule != goal && !runnable(rule.target) {
			continue
		}
		advertised = append(advertised, rule)
		key, value := "rule."+rule.target, file.describe(rule)
		if rule == goal {
			key, value = "default_goal", file.describeGoal(goal)
			// The complete default catalogue must not replace the first
			// source fragment's own prerequisites or recipe.
			if strings.Contains(value, " — fragments: ") {
				fragmentKey := "rule." + rule.target
				if rule.condition != "" {
					fragmentKey += " when " + rule.condition
				}
				rows = append(rows, Row{Path: rule.path, Target: rule.target, HasRecipe: len(rule.recipe) > 0, Condition: rule.condition, Key: fragmentKey, Value: file.describe(rule), Line: rule.line})
			}
		}
		if rule.condition != "" {
			key += " when " + rule.condition
		}
		rows = append(rows, Row{Path: rule.path, Target: rule.target, HasRecipe: len(rule.recipe) > 0, Condition: rule.condition, Key: key, Value: value, Line: rule.line})
	}
	used := file.used(advertised...)
	for _, assignment := range file.assignments {
		if !used[assignment.name] {
			continue
		}
		key := "variable." + assignment.name
		if assignment.condition != "" {
			key += " when " + assignment.condition
		}
		rows = append(rows, Row{Path: assignment.path, Key: key, Value: assignment.value, Line: assignment.line})
	}
	rows = append(rows, file.goalRows...)
	if file.goalUnknown && len(file.goalRows) > 0 {
		last := file.goalRows[len(file.goalRows)-1]
		rows = append(rows, Row{Path: last.Path, Key: "default_goal_unresolved", Value: "the conditional or unresolved .DEFAULT_GOAL assignments are not a selected default target", Line: last.Line})
	}
	return append(rows, file.includes...)
}

// describeGoal identifies every fragment of the default target. Only
// unconditional prerequisites are combined; conditional fragments remain
// explicitly conditional and every fragment still has its own native row.
func (file *makefile) describeGoal(goal *makeRule) string {
	var prerequisites, fragments []string
	for _, rule := range file.rules {
		if rule.target != goal.target {
			continue
		}
		fragment := fmt.Sprintf("%s:%d", rule.path, rule.line)
		if rule.condition != "" {
			fragment += " when " + rule.condition
		} else if text := strings.TrimSpace(file.rulePrerequisites(rule)); text != "" {
			prerequisites = append(prerequisites, text)
		}
		fragments = append(fragments, fragment)
	}
	value := strings.TrimSpace(goal.target + ": " + strings.Join(prerequisites, " "))
	if len(fragments) > 1 {
		value += " — fragments: " + strings.Join(fragments, "; ")
	} else if len(goal.recipe) > 0 {
		value += " — runs: " + strings.Join(goal.recipe, "; ")
	}
	return value
}

// literalVariable follows authored assignment operators but never selects an
// unknown conditional branch. An unresolved assignment remains source text.
func (file *makefile) literalVariable(name string) (string, bool) {
	value, present, conditional, known := "", false, false, true
	for _, assignment := range file.assignments {
		if assignment.name != name {
			continue
		}
		if assignment.condition != "" {
			conditional = true
			continue
		}
		switch assignment.operator {
		case "?=":
			if !present {
				value, present, known = assignment.value, true, true
			}
		case "+=":
			addition := assignment.value
			if assignment.appendUnknown || assignment.appendSimple && !assignment.resolved {
				known = false
			} else if assignment.appendSimple {
				addition = assignment.immediate
			}
			value, present = strings.TrimSpace(value+" "+addition), true
		case ":=", "::=":
			value, present, conditional, known = assignment.immediate, true, false, assignment.resolved
		default:
			value, present, conditional, known = assignment.value, true, false, true
		}
	}
	return value, present && !conditional && known
}

// A += assignment expands immediately only when its variable was previously
// simply expanded. Unknown conditional flavors are not guessed.
func (file *makefile) simpleVariable(name string) (simple, known bool) {
	known, present := true, false
	for _, assignment := range file.assignments {
		if assignment.name != name {
			continue
		}
		if assignment.condition != "" {
			known = false
			continue
		}
		switch assignment.operator {
		case "+=":
			present = true
		case "?=":
			if !present && known {
				simple, present = false, true
			}
		default:
			simple, known, present = assignment.operator == ":=" || assignment.operator == "::=", true, true
		}
	}
	return simple, known
}

// makeWords splits source words without splitting a Make expression containing
// spaces. Unknown functions remain one explicit unresolved expression.
func makeWords(text string) []string {
	var words []string
	start, depth := -1, 0
	escaped := false
	for at, r := range text {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			if start < 0 {
				start = at
			}
			escaped = true
			continue
		}
		if depth == 0 && unicode.IsSpace(r) {
			if start >= 0 {
				words = append(words, text[start:at])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = at
		}
		switch r {
		case '(', '{':
			depth++
		case ')', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	if start >= 0 {
		words = append(words, text[start:])
	}
	return words
}
