package facts

import (
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

func (b *builder) addConfigReads(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			key, value, ok := configKeyFromPattern(target, relation, pattern)
			if !ok {
				continue
			}
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			symbol, _ := target.enclosingSymbol(relation.FromID)
			b.addConfigRead(target, *anchor, key, value, symbol, ResolutionExact)
		}
	}
}

func (b *builder) addConfigRead(target *targetContext, anchor Anchor, key, value, symbol string, resolution Resolution) {
	if !b.once(strings.Join([]string{string(KindConfigRead), anchor.Path, itoa(anchor.Line), key}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:       KindConfigRead,
		TargetID:   target.target.ID,
		Anchor:     &anchor,
		Key:        key,
		Value:      value,
		Symbol:     symbol,
		Resolution: resolution,
	}, key)
}

// configKeyFromPattern reads the env key from a Field(env=...), getenv(...)
// or os.environ.get(...) call together with a literal default when present.
func configKeyFromPattern(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern) (key, value string, ok bool) {
	selector := pattern.Selector
	switch selector {
	case "Field", "getenv", "environ", "Getenv", "LookupEnv":
		if argument, found := keywordArgument(pattern, "env", "envvar"); found {
			if key, _, ok = literalValue(argument); ok && key != "" {
				return key, defaultLiteral(pattern), true
			}
		}
	}
	switch selector {
	case "getenv", "Getenv", "LookupEnv":
	case "get":
		if !hasEnvironOrigin(target.externalOrigins(relation, pattern)) {
			return "", "", false
		}
	default:
		return "", "", false
	}
	argument, found := positionalArgument(pattern, 1)
	if !found {
		return "", "", false
	}
	key, _, ok = literalValue(argument)
	if !ok || key == "" {
		return "", "", false
	}
	return key, defaultLiteral(pattern), true
}

func hasEnvironOrigin(origins []programindex.ExternalSymbol) bool {
	for _, origin := range origins {
		if origin.PackagePath == "os.environ" || (origin.PackagePath == "os" && origin.Name == "environ") {
			return true
		}
	}
	return false
}

func defaultLiteral(pattern programindex.RelationPattern) string {
	if argument, ok := keywordArgument(pattern, "default"); ok {
		if value, _, literal := literalValue(argument); literal {
			return value
		}
	}
	if argument, ok := positionalArgument(pattern, 2); ok {
		if value, _, literal := literalValue(argument); literal {
			return value
		}
	}
	return ""
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
