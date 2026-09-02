package facts

import (
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// riskRule is the closed condition under which a selector is a risk: which
// origin packages qualify (nil means the bare name is enough).
type riskRule struct {
	origins []string
}

var riskRules = map[string]riskRule{
	"exec":         {},
	"eval":         {},
	"system":       {origins: []string{"os"}},
	"popen":        {origins: []string{"os", "subprocess"}},
	"run":          {origins: []string{"subprocess"}},
	"call":         {origins: []string{"subprocess"}},
	"check_output": {origins: []string{"subprocess"}},
	"check_call":   {origins: []string{"subprocess"}},
	"loads":        {origins: []string{"pickle", "yaml", "marshal"}},
	"load":         {origins: []string{"pickle", "yaml", "marshal"}},
	"Function":     {},
	"execSync":     {origins: []string{"child_process"}},
	"execFile":     {origins: []string{"child_process"}},
	"execFileSync": {origins: []string{"child_process"}},
	"spawn":        {origins: []string{"child_process"}},
	"spawnSync":    {origins: []string{"child_process"}},
}

type riskRegex struct {
	expression *regexp.Regexp
	label      func(match []string) string
	python     bool
	javascript bool
}

var riskRegexes = []riskRegex{
	{expression: regexp.MustCompile(`\beval\(`), label: constantLabel("eval"), python: true, javascript: true},
	{expression: regexp.MustCompile(`\bexec\(`), label: constantLabel("exec"), python: true},
	{expression: regexp.MustCompile(`\bsubprocess\.([A-Za-z_]+)`), label: prefixedLabel("subprocess."), python: true},
	{expression: regexp.MustCompile(`\bos\.system\(`), label: constantLabel("os.system"), python: true},
	{expression: regexp.MustCompile(`\bpickle\.loads?\(`), label: constantLabel("pickle.loads"), python: true},
	{expression: regexp.MustCompile(`\bchild_process\b`), label: constantLabel("child_process"), javascript: true},
	{expression: regexp.MustCompile(`\bdangerouslySetInnerHTML\b`), label: constantLabel("dangerouslySetInnerHTML"), javascript: true},
	{expression: regexp.MustCompile(`\bnew Function\(`), label: constantLabel("new Function"), javascript: true},
}

func constantLabel(label string) func([]string) string {
	return func([]string) string { return label }
}

func prefixedLabel(prefix string) func([]string) string {
	return func(match []string) string { return prefix + match[1] }
}

func (b *builder) addRisks(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			label, ok := riskLabel(target, relation, pattern, *anchor, b.source.line(anchor.Path, anchor.Line))
			if !ok {
				continue
			}
			symbol, _ := target.enclosingSymbol(relation.FromID)
			b.addRisk(target, *anchor, label, symbol, ResolutionExact)
		}
	}
	b.addRisksByRegex(target)
}

func (b *builder) addRisk(target *targetContext, anchor Anchor, label, symbol string, resolution Resolution) {
	if !b.once(strings.Join([]string{string(KindRisk), anchor.Path, itoa(anchor.Line)}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:       KindRisk,
		TargetID:   target.target.ID,
		Anchor:     &anchor,
		Key:        label,
		Symbol:     symbol,
		Text:       clipText(b.source.line(anchor.Path, anchor.Line)),
		Resolution: resolution,
	}, label)
}

// riskLabel applies the closed rule for one selector. A bare exec in
// JavaScript is a RegExp method, so there it needs a child_process origin;
// "Function" counts only as the constructor form.
func riskLabel(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, anchor Anchor, line string) (string, bool) {
	rule, ok := riskRules[pattern.Selector]
	if !ok {
		return "", false
	}
	origins := target.externalOrigins(relation, pattern)
	javascript := isJavaScriptFile(anchor.Path)
	switch pattern.Selector {
	case "Function":
		if !strings.Contains(line, "new Function") {
			return "", false
		}
		return "new Function", true
	case "exec":
		if javascript {
			rule = riskRule{origins: []string{"child_process"}}
		}
	}
	if rule.origins == nil {
		if pkg, found := originPackage(origins, "subprocess", "os", "child_process", "pickle", "yaml", "marshal"); found {
			return pkg + "." + pattern.Selector, true
		}
		return pattern.Selector, true
	}
	pkg, found := originPackage(origins, rule.origins...)
	if !found {
		return "", false
	}
	return pkg + "." + pattern.Selector, true
}

func originPackage(origins []programindex.ExternalSymbol, candidates ...string) (string, bool) {
	for _, origin := range origins {
		if pkg, ok := packageMatches(origin.PackagePath, candidates...); ok {
			return pkg, true
		}
	}
	return "", false
}

func (b *builder) addRisksByRegex(target *targetContext) {
	for _, filePath := range b.targetSourcePaths(target) {
		file, ok := b.source.file(filePath)
		if !ok || file.binary {
			continue
		}
		python, javascript := isPythonFile(filePath), isJavaScriptFile(filePath)
		for number, line := range file.lines {
			if isCommentLine(line) {
				continue
			}
			for _, rule := range riskRegexes {
				if !(python && rule.python || javascript && rule.javascript) {
					continue
				}
				match := rule.expression.FindStringSubmatch(line)
				if match == nil {
					continue
				}
				anchor := Anchor{Path: filePath, Line: number + 1}
				b.addRisk(target, anchor, rule.label(match), target.symbolAtLine(filePath, number+1), ResolutionPossible)
			}
		}
	}
}
