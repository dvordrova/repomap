// Package makefile reads a makefile as a build's manifest: what `make` builds
// by default and the rules and variables a newcomer runs it with. It reads
// text only; the C adapter's dry run is what the build does.
package makefile

import (
	"regexp"
	"slices"
	"strings"
)

// Row is one row a makefile gives the facts' manifest rows (C.md "A
// makefile as the build's manifest"): its key, its value and the line it
// starts on.
type Row struct {
	Key   string
	Value string
	Line  int
}

// Read reads a makefile as a newcomer runs it, as package.json's
// scripts are read: the default goal `make` builds (GNU make's rule:
// .DEFAULT_GOAL, else the first target of the first rule that names no
// special target, `.x`, or pattern, `%`), with what it builds; each other
// rule a reader may run (`make test`), with what it needs and runs, an
// object's rule (`x.o`) none; and the variables the goal's commands and
// make's own compile and link commands use (CC, CFLAGS, CPPFLAGS, LDFLAGS,
// LDLIBS), followed through the variables their values name, each
// assignment with the conditional it stands under (redis-1.3.6's CFLAGS
// under `ifeq ($(uname_S),SunOS)` and otherwise). Prerequisites read
// through this makefile's own variables (Lua's `all: $(ALL_T)` builds
// liblua.a and lua); a value is quoted as written. Rows are in line order.
func Read(lines []string) []Row {
	file := parseMakefile(lines)
	goal := file.defaultGoal()
	var rows []Row
	if goal != nil {
		rows = append(rows, Row{Key: "default_goal", Value: strings.TrimSpace(goal.target + ": " + file.describe(goal)), Line: goal.line})
	}
	for _, rule := range file.rules {
		if rule == goal || !runnable(rule.target) {
			continue
		}
		key := "rule." + rule.target
		if rule.condition != "" {
			key += " when " + rule.condition
		}
		rows = append(rows, Row{Key: key, Value: file.describe(rule), Line: rule.line})
	}
	for _, assignment := range file.assignments {
		if !file.used(goal)[assignment.name] {
			continue
		}
		key := "variable." + assignment.name
		if assignment.condition != "" {
			key += " when " + assignment.condition
		}
		rows = append(rows, Row{Key: key, Value: assignment.value, Line: assignment.line})
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return a.Line - b.Line })
	return rows
}

type makeRule struct {
	target        string
	prerequisites string
	recipe        []string
	line          int
	condition     string
}

type makeVariable struct {
	name, value string
	line        int
	condition   string
}

type makefile struct {
	rules       []*makeRule
	assignments []makeVariable
	goal        string // .DEFAULT_GOAL, when assigned
	usedBy      map[*makeRule]map[string]bool
}

var (
	// makeAssignment is a variable assignment: its name and value.
	makeAssignment = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_.]*)\s*(?::=|::=|\?=|\+=|=)\s*(.*?)\s*$`)
	// defaultGoalAssignment names the goal make builds when given none.
	defaultGoalAssignment = regexp.MustCompile(`^\.DEFAULT_GOAL\s*(?::=|::=|\?=|=)\s*(.*?)\s*$`)
	makeReference         = regexp.MustCompile(`\$[({]([A-Za-z_][A-Za-z0-9_.]*)[)}]`)
	makeDirective         = regexp.MustCompile(`^(ifeq|ifneq|ifdef|ifndef|else|endif|define|endef|include|-include|sinclude|export|unexport|override|vpath)\b`)
)

func parseMakefile(physical []string) *makefile {
	file := &makefile{}
	var conditions []string
	// current are the rules a recipe line belongs to: every target of the
	// rule line before it.
	var current []*makeRule
	inDefine := false
	for number := 0; number < len(physical); number++ {
		start := number
		text := physical[number]
		// A recipe line keeps its tab; a continued line joins the next.
		for strings.HasSuffix(text, `\`) && number+1 < len(physical) {
			number++
			text = strings.TrimSuffix(text, `\`) + " " + strings.TrimSpace(physical[number])
		}
		if strings.HasPrefix(text, "\t") {
			if command := strings.TrimSpace(text[1:]); command != "" && !inDefine {
				for _, rule := range current {
					rule.recipe = append(rule.recipe, command)
				}
			}
			continue
		}
		line := strings.TrimSpace(stripMakeComment(text))
		if line == "" {
			continue
		}
		if inDefine {
			inDefine = !strings.HasPrefix(line, "endef")
			continue
		}
		if directive := makeDirective.FindString(line); directive != "" {
			current = nil
			switch directive {
			case "ifeq", "ifneq", "ifdef", "ifndef":
				conditions = append(conditions, line)
			case "else":
				if len(conditions) > 0 {
					previous := strings.TrimPrefix(conditions[len(conditions)-1], "not ")
					conditions[len(conditions)-1] = "not " + previous
					if rest := strings.TrimSpace(strings.TrimPrefix(line, "else")); rest != "" {
						conditions[len(conditions)-1] += " and " + rest
					}
				}
			case "endif":
				if len(conditions) > 0 {
					conditions = conditions[:len(conditions)-1]
				}
			case "define":
				inDefine = true
			case "export", "override":
				// export CC = gcc assigns as CC = gcc does.
				if match := makeAssignment.FindStringSubmatch(strings.TrimSpace(strings.TrimPrefix(line, directive))); match != nil {
					file.assign(match[1], match[2], start+1, conditions)
				}
			}
			continue
		}
		if goal := defaultGoalAssignment.FindStringSubmatch(line); goal != nil {
			current = nil
			file.goal = strings.TrimSpace(goal[1])
			continue
		}
		if match := makeAssignment.FindStringSubmatch(line); match != nil {
			current = nil
			file.assign(match[1], match[2], start+1, conditions)
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			current = nil
			continue
		}
		targets, rest := line[:colon], strings.TrimPrefix(line[colon+1:], ":")
		inline := ""
		if semicolon := strings.Index(rest, ";"); semicolon >= 0 {
			rest, inline = rest[:semicolon], strings.TrimSpace(rest[semicolon+1:])
		}
		current = nil
		for _, target := range strings.Fields(file.expand(targets, map[string]bool{})) {
			rule := &makeRule{target: target, prerequisites: strings.TrimSpace(rest), line: start + 1, condition: strings.Join(conditions, " and ")}
			if inline != "" {
				rule.recipe = append(rule.recipe, inline)
			}
			file.rules = append(file.rules, rule)
			current = append(current, rule)
		}
	}
	return file
}

func (file *makefile) assign(name, value string, line int, conditions []string) {
	file.assignments = append(file.assignments, makeVariable{name: name, value: strings.Join(strings.Fields(value), " "), line: line, condition: strings.Join(conditions, " and ")})
}

// stripMakeComment drops a comment: make reads `#` as one unless escaped.
func stripMakeComment(line string) string {
	for at := 0; at < len(line); at++ {
		if line[at] == '#' && (at == 0 || line[at-1] != '\\') {
			return line[:at]
		}
	}
	return line
}

// expand reads this makefile's own variables in text, each through its
// first assignment; a function call ($(shell …)) or a variable it does not
// assign stays as written. seen stops a variable naming itself.
func (file *makefile) expand(text string, seen map[string]bool) string {
	return makeReference.ReplaceAllStringFunc(text, func(reference string) string {
		name := makeReference.FindStringSubmatch(reference)[1]
		if seen[name] {
			return reference
		}
		for _, assignment := range file.assignments {
			if assignment.name == name {
				inner := map[string]bool{name: true}
				for key := range seen {
					inner[key] = true
				}
				return file.expand(assignment.value, inner)
			}
		}
		return reference
	})
}

func (file *makefile) defaultGoal() *makeRule {
	for _, rule := range file.rules {
		if file.goal != "" {
			if rule.target == file.goal {
				return rule
			}
			continue
		}
		if (!strings.HasPrefix(rule.target, ".") || strings.Contains(rule.target, "/")) && !strings.Contains(rule.target, "%") {
			return rule
		}
	}
	return nil
}

// describe is a rule's prerequisites through the makefile's variables and,
// when it has one, its recipe as written.
func (file *makefile) describe(rule *makeRule) string {
	text := strings.Join(strings.Fields(file.expand(rule.prerequisites, map[string]bool{})), " ")
	if len(rule.recipe) > 0 {
		text = strings.TrimSpace(text + " — runs: " + strings.Join(rule.recipe, "; "))
	}
	return text
}

// runnable is a target a reader may name to make: not special (.PHONY,
// .c.o), not a pattern, not an object file.
func runnable(target string) bool {
	return !strings.HasPrefix(target, ".") && !strings.Contains(target, "%") && !strings.HasSuffix(target, ".o")
}

// used are the variables the goal's commands and make's own compile and
// link commands name, followed through the variables their values name.
func (file *makefile) used(goal *makeRule) map[string]bool {
	if file.usedBy == nil {
		file.usedBy = map[*makeRule]map[string]bool{}
	}
	if used, done := file.usedBy[goal]; done {
		return used
	}
	used := map[string]bool{}
	var queue []string
	name := func(text string) {
		for _, match := range makeReference.FindAllStringSubmatch(text, -1) {
			if !used[match[1]] {
				used[match[1]] = true
				queue = append(queue, match[1])
			}
		}
	}
	for _, implicit := range []string{"CC", "CFLAGS", "CPPFLAGS", "LDFLAGS", "LDLIBS"} {
		name("$(" + implicit + ")")
	}
	// The rules the goal builds, through their prerequisites.
	if goal != nil {
		reached := map[*makeRule]bool{}
		rules := []*makeRule{goal}
		for len(rules) > 0 {
			rule := rules[0]
			rules = rules[1:]
			if reached[rule] {
				continue
			}
			reached[rule] = true
			// What a rule needs is already read through its variables in
			// its row; its commands' variables are the build's conditions.
			for _, command := range rule.recipe {
				name(command)
			}
			for _, prerequisite := range strings.Fields(file.expand(rule.prerequisites, map[string]bool{})) {
				for _, other := range file.rules {
					if other.target == prerequisite {
						rules = append(rules, other)
					}
				}
			}
		}
	}
	for len(queue) > 0 {
		variable := queue[0]
		queue = queue[1:]
		for _, assignment := range file.assignments {
			if assignment.name == variable {
				name(assignment.value)
			}
		}
	}
	file.usedBy[goal] = used
	return used
}
