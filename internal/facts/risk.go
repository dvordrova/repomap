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

var dynamicRules = map[string]dynamicRule{
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

// cDynamicRules are the libc functions that hand a command line to the shell,
// with the headers that declare them. C has no builtin that runs code, so in
// a C file nothing else counts: a repository function named eval or exec is
// the repository's own code.
var cDynamicRules = map[string][]string{
	"system": {"stdlib.h"},
	"popen":  {"stdio.h"},
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
// JavaScript is a RegExp method, so there it needs a child_process origin;
// "Function" counts only as the constructor form.
func dynamicLabel(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, anchor Anchor, line string) (string, bool) {
	origins := target.externalOrigins(relation, pattern)
	if isCFile(anchor.Path) {
		headers, ok := cDynamicRules[pattern.Selector]
		if !ok {
			return "", false
		}
		if _, found := originPackage(origins, headers...); !found {
			return "", false
		}
		return pattern.Selector, true
	}
	rule, ok := dynamicRules[pattern.Selector]
	if !ok {
		return "", false
	}
	javascript := isJavaScriptFile(anchor.Path)
	switch pattern.Selector {
	case "Function":
		if !strings.Contains(line, "new Function") {
			return "", false
		}
		return "new Function", true
	case "exec":
		if javascript {
			rule = dynamicRule{origins: []string{"child_process"}}
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
