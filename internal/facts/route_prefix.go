package facts

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A mount is a call the repository does not own that hands a repository
// value (a router, a blueprint, an included module) to something else under a
// path prefix. A constructor given a "/prefix" literal keyword gives its
// result an intrinsic prefix. Both follow observed values, never variable
// names, so two routers in one module stay distinct.
//
// When a mount prefix and an intrinsic prefix meet, frameworks differ on
// whether they compose or the mount replaces the router's own prefix. The
// composed path is retained as possible rather than choosing a framework.

type routePrefix struct {
	path     string
	evidence []Anchor
	possible bool
}

type routeMount struct {
	ownerID, path string
	anchor        Anchor
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

// isRouterMount reports a route-word call whose "handler" is a repository
// value rather than a callable: mount("/api", router) describes a router.
func (target *targetContext) isRouterMount(pattern programindex.RelationPattern) bool {
	return target.mountedValue(pattern) != ""
}

// mountedValue is the one repository non-callable object a call hands over.
func (target *targetContext) mountedValue(pattern programindex.RelationPattern) string {
	for _, argument := range pattern.Arguments {
		if len(argument.ObjectIDs) != 1 || argument.ObjectsOmitted != 0 {
			continue
		}
		id := argument.ObjectIDs[0]
		object, ok := target.object(id)
		if !ok || object.Kind != programindex.ObjectVariable && object.Kind != programindex.ObjectModule {
			continue
		}
		if module := target.includedModule(id); module != "" {
			return module
		}
		return id
	}
	return ""
}

// includedModule maps a call result such as include("app.urls") to the
// indexed module its literal names.
func (target *targetContext) includedModule(resultID string) string {
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != resultID {
				continue
			}
			argument, ok := positionalArgument(pattern, 1)
			if !ok {
				continue
			}
			if len(argument.ObjectIDs) == 1 {
				if object, known := target.object(argument.ObjectIDs[0]); known && object.Kind == programindex.ObjectModule {
					return object.ID
				}
			}
			if value, _, literal := literalValue(argument); literal {
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

// prefixLiteral is the one path literal of a mount, positional or keyword,
// stating where the handed value answers: "/api", or "django/" as Django
// writes it. A host or a name is not a prefix; several paths name no one.
func prefixLiteral(pattern programindex.RelationPattern) (string, bool) {
	found := ""
	for _, argument := range pattern.Arguments {
		value, _, literal := literalValue(argument)
		if !literal || !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") {
			continue
		}
		if found != "" {
			return "", false
		}
		found = value
	}
	return found, found != ""
}

// prefixesByObject composes the prefixes each router value answers under.
// Only a mount or constructor whose external origin is known counts: a
// prefix given to a repository lookalike, or to a value of unknown type,
// says nothing about any route.
func (target *targetContext) prefixesByObject() map[string][]routePrefix {
	mounts := make(map[string][]routeMount)
	intrinsic := make(map[string][]routePrefix)
	originsByValue := target.routeValueOrigins()
	for _, relation := range target.input.Index.Relations {
		if target.ownsCallee(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			at := target.patternAnchor(relation, pattern)
			if at == nil || target.ownsReceiver(pattern) || len(target.callOrigins(relation, pattern, originsByValue)) == 0 {
				continue
			}
			prefix, ok := prefixLiteral(pattern)
			if !ok {
				continue
			}
			if pattern.ResultID != "" {
				if _, keyword := keywordPrefix(pattern); keyword {
					intrinsic[pattern.ResultID] = append(intrinsic[pattern.ResultID], routePrefix{path: prefix, evidence: []Anchor{*at}})
				}
			}
			child := target.mountedValue(pattern)
			if child == "" && (strings.EqualFold(pattern.Selector, "route") || strings.EqualFold(pattern.Selector, "mount")) {
				child = target.mountedRouter(relation, pattern)
			}
			owner := pattern.ReceiverID
			if owner == "" {
				owner = relation.FromID
			}
			if child == "" || child == owner {
				continue
			}
			mounts[child] = append(mounts[child], routeMount{ownerID: owner, path: prefix, anchor: *at})
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
			if len(own) == 0 {
				own = []routePrefix{{}}
			}
			for _, parent := range parents {
				for _, local := range own {
					prefix := joinPrefix(joinPrefix(parent.path, mount.path), local.path)
					evidence := append(append(append([]Anchor{}, parent.evidence...), mount.anchor), local.evidence...)
					values = append(values, routePrefix{path: prefix, evidence: evidence, possible: parent.possible || local.path != ""})
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

func keywordPrefix(pattern programindex.RelationPattern) (string, bool) {
	for _, argument := range pattern.Arguments {
		if argument.Keyword == "" {
			continue
		}
		if value, _, literal := literalValue(argument); literal && strings.HasPrefix(value, "/") {
			return value, true
		}
	}
	return "", false
}

// mountedRouter resolves a route-group closure: Route("/api", func(r) {...})
// registers on the router the closure receives.
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

// joinRoutePath composes a mount prefix and a path; a mounted path is a
// path, so it starts with "/" however the framework wrote its parts.
func joinRoutePath(prefix, path string) string {
	joined := "/" + strings.Trim(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
	joined = strings.TrimSuffix(strings.ReplaceAll(joined, "//", "/"), "/")
	if joined == "" {
		return "/"
	}
	return joined
}
