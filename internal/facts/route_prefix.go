package facts

import (
	"github.com/dvordrova/repomap/internal/programindex"
	"sort"
	"strings"
)

// Prefixes follow observed router values and arguments, never variable names.
// Different routers in one module remain distinct, with original mount anchors.
type routePrefix struct {
	path     string
	evidence []Anchor
}
type routeMount struct {
	ownerID, path     string
	anchor            Anchor
	overrideIntrinsic bool
}

func (target *targetContext) routeValueOrigins() map[string][]programindex.ExternalSymbol {
	result := make(map[string][]programindex.ExternalSymbol)
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != "" {
				result[pattern.ResultID] = append(result[pattern.ResultID], target.externalOrigins(relation, pattern)...)
			}
		}
	}
	return result
}

func (target *targetContext) prefixesByObject(originsByValue map[string][]programindex.ExternalSymbol) map[string][]routePrefix {
	mounts := make(map[string][]routeMount)
	intrinsic := make(map[string][]routePrefix)
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			origins := append(target.externalOrigins(relation, pattern), originsByValue[pattern.ReceiverID]...)
			if pattern.ResultID != "" {
				for _, origin := range origins {
					var key string
					if _, ok := packageMatches(origin.PackagePath, "fastapi"); ok && strings.EqualFold(pattern.Selector, "APIRouter") {
						key = "prefix"
					}
					if _, ok := packageMatches(origin.PackagePath, "flask"); ok && strings.EqualFold(pattern.Selector, "Blueprint") {
						key = "url_prefix"
					}
					if argument, ok := keywordArgument(pattern, key); key != "" && ok {
						if value, _, literal := literalValue(argument); literal && strings.HasPrefix(value, "/") {
							if at := target.patternAnchor(relation, pattern); at != nil {
								intrinsic[pattern.ResultID] = append(intrinsic[pattern.ResultID], routePrefix{value, []Anchor{*at}})
							}
						}
					}
				}
			}
			child, prefix, override, ok := target.routerMount(relation, pattern, origins)
			if !ok || child == "" {
				continue
			}
			owner := pattern.ReceiverID
			if owner == "" {
				owner = relation.FromID
			}
			if child == owner {
				continue
			}
			if at := target.patternAnchor(relation, pattern); at != nil {
				mounts[child] = append(mounts[child], routeMount{owner, prefix, *at, override})
			}
		}
	}
	result := make(map[string][]routePrefix)
	var resolve func(string, map[string]bool) []routePrefix
	resolve = func(id string, visiting map[string]bool) []routePrefix {
		if visiting[id] {
			return nil
		}
		visiting[id] = true
		defer delete(visiting, id)
		if len(mounts[id]) == 0 {
			return intrinsic[id]
		}
		var values []routePrefix
		for _, mount := range mounts[id] {
			if visiting[mount.ownerID] {
				continue
			}
			parents := resolve(mount.ownerID, visiting)
			if len(parents) == 0 {
				parents = []routePrefix{{}}
			}
			own := intrinsic[id]
			if mount.overrideIntrinsic || len(own) == 0 {
				own = []routePrefix{{}}
			}
			for _, parent := range parents {
				for _, local := range own {
					prefix := joinPrefix(joinPrefix(parent.path, mount.path), local.path)
					evidence := append(append(append([]Anchor{}, parent.evidence...), mount.anchor), local.evidence...)
					values = append(values, routePrefix{prefix, evidence})
				}
			}
		}
		sort.SliceStable(values, func(i, j int) bool { return values[i].path < values[j].path })
		return values
	}
	for id := range mounts {
		result[id] = resolve(id, make(map[string]bool))
	}
	for id := range intrinsic {
		if _, ok := result[id]; !ok {
			result[id] = intrinsic[id]
		}
	}
	return result
}

func (target *targetContext) isRouterMount(relation programindex.Relation, pattern programindex.RelationPattern, origins []programindex.ExternalSymbol) bool {
	_, _, _, ok := target.routerMount(relation, pattern, origins)
	return ok
}

func (target *targetContext) routerMount(relation programindex.Relation, pattern programindex.RelationPattern, origins []programindex.ExternalSymbol) (string, string, bool, bool) {
	selector := strings.ToLower(pattern.Selector)
	side := classifyHTTP(origins, pattern.Form, selector)
	if !side.server {
		return "", "", false, false
	}
	position, key, override := 0, "", false
	switch selector {
	case "include_router":
		if _, ok := packageMatches(side.pkgPath, "fastapi", "starlette"); !ok {
			return "", "", false, false
		}
		position, key = 1, "prefix"
	case "register_blueprint":
		if _, ok := packageMatches(side.pkgPath, "flask"); !ok {
			return "", "", false, false
		}
		position, key = 1, "url_prefix"
	case "use":
		if _, ok := packageMatches(side.pkgPath, "express"); !ok {
			return "", "", false, false
		}
		position = 2
	case "path":
		if _, ok := packageMatches(side.pkgPath, "django"); !ok {
			return "", "", false, false
		}
		position = 2
	case "route", "mount":
		if _, ok := packageMatches(side.pkgPath, "github.com/go-chi/chi"); !ok {
			return "", "", false, false
		}
		position = 2
	default:
		return "", "", false, false
	}
	argument, ok := positionalArgument(pattern, position)
	if !ok {
		return "", "", false, false
	}
	child := ""
	if len(argument.ObjectIDs) == 1 && argument.ObjectsOmitted == 0 {
		child = argument.ObjectIDs[0]
	}
	if selector == "path" {
		child = target.includedModule(child)
	}
	if child == "" && (selector == "route" || selector == "mount") {
		child = target.mountedRouter(relation, pattern)
	}
	if child == "" {
		return "", "", false, false
	}
	var value string
	if key != "" {
		if arg, present := keywordArgument(pattern, key); present {
			var literal bool
			value, _, literal = literalValue(arg)
			if !literal {
				return "", "", false, false
			}
			override = selector == "register_blueprint"
		}
	} else {
		arg, present := positionalArgument(pattern, 1)
		var literal bool
		value, _, literal = literalValue(arg)
		if !present || !literal {
			return "", "", false, false
		}
	}
	if selector == "path" {
		value = "/" + strings.TrimPrefix(value, "/")
	}
	if value != "" && !strings.HasPrefix(value, "/") {
		return "", "", false, false
	}
	return child, value, override, true
}

// Only an observed Django include call can map its literal module name to a
// module identity already present in the index.
func (target *targetContext) includedModule(resultID string) string {
	if resultID == "" {
		return ""
	}
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != resultID || pattern.Selector != "include" {
				continue
			}
			if _, ok := packageMatches(classifyHTTP(target.externalOrigins(relation, pattern), pattern.Form, "include").pkgPath, "django"); !ok {
				continue
			}
			arg, ok := positionalArgument(pattern, 1)
			if !ok {
				continue
			}
			if len(arg.ObjectIDs) == 1 {
				if object, known := target.object(arg.ObjectIDs[0]); known && object.Kind == programindex.ObjectModule {
					return object.ID
				}
			}
			if value, _, literal := literalValue(arg); literal {
				for _, object := range target.input.Index.Objects {
					if object.Kind == programindex.ObjectModule && object.Name == value {
						return object.ID
					}
				}
			}
		}
	}
	return ""
}

func (target *targetContext) mountedRouter(relation programindex.Relation, pattern programindex.RelationPattern) string {
	if _, objectID := target.routeHandler(relation, pattern); objectID != "" {
		return objectID
	}
	if pattern.Location == nil {
		return ""
	}
	return target.singleCallAt(relation.FromID, pattern.Location)
}

func (target *targetContext) singleCallAt(ownerID string, at *programindex.Location) string {
	var found string
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != ownerID || relation.Location == nil || relation.Location.Path != at.Path || relation.Location.Line != at.Line || len(relation.ToIDs) != 1 {
			continue
		}
		object, known := target.object(relation.ToIDs[0])
		if !known || object.Kind != programindex.ObjectFunction && object.Kind != programindex.ObjectMethod {
			continue
		}
		if found != "" && found != object.ID {
			return ""
		}
		found = object.ID
	}
	return found
}

func joinPrefix(prefix, path string) string {
	if path == "" {
		return prefix
	}
	return joinRoutePath(prefix, path)
}
func joinRoutePath(prefix, path string) string {
	joined := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
	joined = strings.TrimSuffix(joined, "/")
	if joined == "" {
		return "/"
	}
	return joined
}
