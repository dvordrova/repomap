package facts

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// dynamicRule is the closed condition under which a selector runs code that
// is not in the source: which origin packages qualify (nil means the bare
// builtin name is enough).
type dynamicRule struct {
	origins []string
}

// dynamicRules are the calls that run code inside this program's own
// process that the source does not show: code evaluated from a string, a
// function built from one, a value deserialized into code. Starting
// another program is no such call: the reading asks which calls do
// (atlas.BoundaryRunsProgram) and names the program they start, so no
// library's launching names are kept here. C has no builtin that
// evaluates code, so a C file has none.
var dynamicRules = map[string]dynamicRule{
	"exec":     {},
	"eval":     {},
	"loads":    {origins: []string{"pickle", "yaml", "marshal"}},
	"load":     {origins: []string{"pickle", "yaml", "marshal"}},
	"Function": {},
}

func (b *builder) addDynamicExecution(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		if target.unreachable(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			label, ok := dynamicLabel(target, relation, pattern, *anchor, b.source.line(anchor.Path, anchor.Line))
			if !ok {
				continue
			}
			symbol, _ := target.enclosingSymbol(relation.FromID)
			b.addDynamicExecutionFact(target, *anchor, label, symbol, ResolutionExact)
		}
	}
}

func (b *builder) addDynamicExecutionFact(target *targetContext, anchor Anchor, label, symbol string, resolution Resolution) {
	if !b.once(strings.Join([]string{string(KindDynamicExecution), anchor.Path, itoa(anchor.Line)}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:       KindDynamicExecution,
		TargetID:   target.target.ID,
		Anchor:     &anchor,
		Key:        label,
		Symbol:     symbol,
		Text:       clipText(b.source.line(anchor.Path, anchor.Line)),
		Resolution: resolution,
	}, label)
}

// dynamicLabel applies the closed rule for one selector. A bare exec in
// JavaScript is a RegExp method, and "Function" counts only as the
// constructor form. A C file evaluates no code.
func dynamicLabel(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, anchor Anchor, line string) (string, bool) {
	if isCFile(anchor.Path) {
		return "", false
	}
	rule, ok := dynamicRules[pattern.Selector]
	if !ok {
		return "", false
	}
	switch pattern.Selector {
	case "Function":
		if !strings.Contains(line, "new Function") {
			return "", false
		}
		return "new Function", true
	case "exec":
		if isJavaScriptFile(anchor.Path) {
			return "", false
		}
	}
	if rule.origins == nil {
		return pattern.Selector, true
	}
	pkg, found := originPackage(target.externalOrigins(relation, pattern), rule.origins...)
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
