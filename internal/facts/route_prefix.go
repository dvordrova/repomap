package facts

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A router can be mounted under a prefix, and the routes registered inside it
// then answer on that prefix plus their own path. Without composing them the
// page prints chi's article list as `GET /`, which is not a path anyone can
// call. This file composes the prefix from the same registrations that are
// already facts, so every composed path stays checkable at its own anchor.
//
// Nothing here guesses. A prefix is only carried into a router when the
// registration names that router, either as the handler argument itself or as
// the single exact call the same statement makes.
const (
	// maxComposedPrefixes bounds how many mount paths one router may inherit.
	// A router mounted under three API versions really does answer on three
	// paths and all three are printed; the bound only stops a pathological
	// graph from multiplying without end.
	maxComposedPrefixes = 8
	// maxPrefixDepth bounds how deep the mount chain is followed.
	maxPrefixDepth = 8
)

// nestingSelectors are the registrations that mount a router rather than bind
// a handler. chi's Route and Mount are the exact cases this exists for.
var nestingSelectors = map[string]struct{}{"route": {}, "mount": {}}

type routeMount struct {
	ownerID string
	path    string
}

// prefixesByObject returns, for each object that registers routes, the set of
// path prefixes its registrations answer under. An object missing from the
// map is not mounted anywhere and its paths stand alone.
func (target *targetContext) prefixesByObject() map[string][]string {
	mounts := target.collectMounts()
	if len(mounts) == 0 {
		return nil
	}
	result := make(map[string][]string, len(mounts))
	for objectID := range mounts {
		result[objectID] = target.resolvePrefixes(objectID, mounts, 0, make(map[string]struct{}))
	}
	return result
}

// collectMounts maps a mounted router object to every registration that mounts
// it. One router mounted three times has three entries.
func (target *targetContext) collectMounts() map[string][]routeMount {
	result := make(map[string][]routeMount)
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			selector := strings.ToLower(pattern.Selector)
			if _, nesting := nestingSelectors[selector]; !nesting {
				continue
			}
			side := classifyHTTP(target.externalOrigins(relation, pattern), pattern.Form, selector)
			if !side.server {
				continue
			}
			argument, found := positionalArgument(pattern, 1)
			if !found {
				continue
			}
			path, _, literal := literalValue(argument)
			if !literal || !strings.HasPrefix(path, "/") {
				continue
			}
			mountedID := target.mountedRouter(relation, pattern)
			if mountedID == "" || mountedID == relation.FromID {
				continue
			}
			result[mountedID] = append(result[mountedID], routeMount{ownerID: relation.FromID, path: path})
		}
	}
	return result
}

// mountedRouter identifies the router a nesting registration mounts. It is
// either the handler object the registration names, or — when the argument is
// a call result the adapter could not resolve — the single exact function this
// same statement calls.
func (target *targetContext) mountedRouter(
	relation programindex.Relation,
	pattern programindex.RelationPattern,
) string {
	if _, objectID := target.routeHandler(relation, pattern); objectID != "" {
		return objectID
	}
	if pattern.Location == nil {
		return ""
	}
	return target.singleCallAt(relation.FromID, pattern.Location)
}

// singleCallAt returns the one function this owner calls at exactly this
// location, or empty when there is not exactly one. Requiring exactly one
// keeps the link as checkable as the anchor beside it: a reader opening that
// line sees the same single call.
func (target *targetContext) singleCallAt(ownerID string, at *programindex.Location) string {
	var found string
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != ownerID ||
			relation.Location == nil || relation.Location.Path != at.Path ||
			relation.Location.Line != at.Line || len(relation.ToIDs) != 1 {
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

// resolvePrefixes walks the mount chain upwards and returns every path a
// router answers under.
func (target *targetContext) resolvePrefixes(
	objectID string,
	mounts map[string][]routeMount,
	depth int,
	visiting map[string]struct{},
) []string {
	if depth >= maxPrefixDepth {
		return nil
	}
	if _, cycle := visiting[objectID]; cycle {
		return nil
	}
	visiting[objectID] = struct{}{}
	defer delete(visiting, objectID)

	seen := make(map[string]struct{})
	var result []string
	for _, mount := range mounts[objectID] {
		owners := target.resolvePrefixes(mount.ownerID, mounts, depth+1, visiting)
		if len(owners) == 0 {
			owners = []string{""}
		}
		for _, owner := range owners {
			composed := joinRoutePath(owner, mount.path)
			if _, repeated := seen[composed]; repeated {
				continue
			}
			seen[composed] = struct{}{}
			result = append(result, composed)
			if len(result) == maxComposedPrefixes {
				sort.Strings(result)
				return result
			}
		}
	}
	sort.Strings(result)
	return result
}

// joinRoutePath composes a prefix and a path the way a router does: one
// separator between them, and a trailing slash only on the root itself.
func joinRoutePath(prefix, path string) string {
	joined := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
	joined = strings.TrimSuffix(joined, "/")
	if joined == "" {
		return "/"
	}
	return joined
}
