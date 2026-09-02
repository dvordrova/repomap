package facts

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// ownerChainBound stops owner/container walks on a malformed cyclic index.
const ownerChainBound = 64

func (target *targetContext) object(id string) (programindex.Object, bool) {
	object, ok := target.objects[id]
	return object, ok
}

// location returns the object's own location or the nearest owner's, because
// some adapters emit module/package objects without one.
func (target *targetContext) location(id string) *programindex.Location {
	current, ok := target.object(id)
	for step := 0; ok && step < ownerChainBound; step++ {
		if current.Location != nil {
			return current.Location
		}
		next := current.OwnerID
		if next == "" {
			next = current.ContainerID
		}
		if next == "" {
			return nil
		}
		current, ok = target.object(next)
	}
	return nil
}

func (target *targetContext) filePath(id string) string {
	location := target.location(id)
	if location == nil {
		return ""
	}
	return location.Path
}

// enclosingSymbol walks from an object up to the function or method that
// contains it; a module-level object reports its module.
func (target *targetContext) enclosingSymbol(id string) (string, string) {
	current, ok := target.object(id)
	for step := 0; ok && step < ownerChainBound; step++ {
		switch current.Kind {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectModule, programindex.ObjectPackage:
			return current.Name, current.ID
		}
		next := current.OwnerID
		if next == "" {
			next = current.ContainerID
		}
		if next == "" {
			return current.Name, current.ID
		}
		current, ok = target.object(next)
	}
	return "", ""
}

// symbolAtLine names the last function, method or type declared at or before
// a line of a file; regex-derived rows use it as their enclosing symbol.
func (target *targetContext) symbolAtLine(filePath string, line int) string {
	best, bestLine := "", 0
	for _, object := range target.sortedObjects() {
		if object.Location == nil || object.Location.Path != filePath || object.Location.Line > line {
			continue
		}
		switch object.Kind {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectType:
		default:
			continue
		}
		if object.Location.Line >= bestLine {
			best, bestLine = object.Name, object.Location.Line
		}
	}
	return best
}

func (target *targetContext) sortedObjects() []programindex.Object {
	return target.input.Index.Objects
}

// externalOrigins lists the external symbols behind one pattern: receiver
// origins first (Python/Go attach them to the pattern), then the relation's
// external targets (TypeScript resolves the callee directly).
func (target *targetContext) externalOrigins(relation programindex.Relation, pattern programindex.RelationPattern) []programindex.ExternalSymbol {
	var result []programindex.ExternalSymbol
	for _, id := range append(append([]string{}, pattern.ReceiverOriginIDs...), relation.ToIDs...) {
		object, ok := target.object(id)
		if ok && object.Kind == programindex.ObjectExternalSymbol && object.External != nil {
			result = append(result, *object.External)
		}
	}
	return result
}

// files lists the distinct source files that own this target's objects.
func (target *targetContext) files() []string {
	set := make(map[string]struct{})
	for _, object := range target.input.Index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol {
			continue
		}
		if filePath := target.filePath(object.ID); filePath != "" {
			set[filePath] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for filePath := range set {
		result = append(result, filePath)
	}
	sort.Strings(result)
	return result
}

// patternAnchor is the exact source position of one pattern, falling back to
// the relation and then to the source object.
func (target *targetContext) patternAnchor(relation programindex.Relation, pattern programindex.RelationPattern) *Anchor {
	for _, location := range []*programindex.Location{pattern.Location, relation.Location, target.location(relation.FromID)} {
		if location != nil {
			return &Anchor{Path: location.Path, Line: location.Line, Column: location.Column}
		}
	}
	return nil
}

func positionalArgument(pattern programindex.RelationPattern, position int) (programindex.PatternArgument, bool) {
	for _, argument := range pattern.Arguments {
		if argument.Keyword == "" && argument.Position == position {
			return argument, true
		}
	}
	return programindex.PatternArgument{}, false
}

func keywordArgument(pattern programindex.RelationPattern, keywords ...string) (programindex.PatternArgument, bool) {
	for _, keyword := range keywords {
		for _, argument := range pattern.Arguments {
			if argument.Keyword == keyword {
				return argument, true
			}
		}
	}
	return programindex.PatternArgument{}, false
}

// literalValue returns the string of a literal argument or a template with
// its holes rendered as {param}; templated reports whether a hole was seen.
func literalValue(argument programindex.PatternArgument) (value string, templated bool, ok bool) {
	switch argument.Kind {
	case programindex.PatternLiteralString:
		return argument.Value, false, true
	case programindex.PatternStringTemplate:
		var text strings.Builder
		for _, part := range argument.Parts {
			if part.Kind == programindex.PatternPartHole {
				text.WriteString("{param}")
				templated = true
				continue
			}
			text.WriteString(part.Text)
		}
		return text.String(), templated, true
	default:
		return "", false, false
	}
}

// packageMatches reports whether an external package path belongs to one of
// the listed packages, tolerating subpackages and Go major-version suffixes.
func packageMatches(packagePath string, candidates ...string) (string, bool) {
	normalized := trimGoMajorVersion(packagePath)
	for _, candidate := range candidates {
		if normalized == candidate || strings.HasPrefix(normalized, candidate+"/") || strings.HasPrefix(normalized, candidate+".") {
			return candidate, true
		}
	}
	return "", false
}

func trimGoMajorVersion(packagePath string) string {
	slash := strings.LastIndex(packagePath, "/")
	if slash < 0 {
		return packagePath
	}
	suffix := packagePath[slash+1:]
	if len(suffix) >= 2 && suffix[0] == 'v' && strings.Trim(suffix[1:], "0123456789") == "" {
		return packagePath[:slash]
	}
	return packagePath
}
