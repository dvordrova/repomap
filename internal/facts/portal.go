package facts

import (
	"fmt"
	"strings"
)

// addPortals joins every client call to the one route of another target
// that serves it. Zero or several candidates never become a fact; they are
// recorded as diagnostics so the report can say the boundary is unclear.
func (b *builder) addPortals() {
	routes := b.ofKind(KindHTTPRoute)
	for _, call := range b.ofKind(KindHTTPCall) {
		var candidates []Fact
		for _, route := range routes {
			if route.TargetID == call.TargetID || !methodsMatch(route.Method, call.Method) {
				continue
			}
			if pathsMatch(route.Path, call.Path) {
				candidates = append(candidates, route)
			}
		}
		switch len(candidates) {
		case 1:
			b.addPortal(call, candidates[0])
		case 0:
			b.diagnose("portal_unmatched", fmt.Sprintf("%s %s at %s matches no route of another target", call.Method, call.Path, call.Anchor))
		default:
			b.diagnose("portal_ambiguous", fmt.Sprintf("%s %s at %s matches %d routes", call.Method, call.Path, call.Anchor, len(candidates)))
		}
	}
}

func (b *builder) addPortal(call, route Fact) {
	resolution := ResolutionExact
	if call.Resolution == ResolutionPossible || route.Resolution == ResolutionPossible ||
		route.Method != call.Method || hasPathParameters(route.Path) {
		resolution = ResolutionPossible
	}
	root := ""
	if target, ok := b.targetByID(call.TargetID); ok {
		root = target.Root
	}
	anchor := *call.Anchor
	b.add(root, Fact{
		Kind:         KindPortal,
		TargetID:     call.TargetID,
		PeerTargetID: route.TargetID,
		Anchor:       &anchor,
		Method:       call.Method,
		Path:         route.Path,
		Symbol:       route.Symbol,
		Resolution:   resolution,
		Refs:         []string{call.ID, route.ID},
		Evidence:     []Anchor{*route.Anchor},
	}, call.Method, route.Path, route.Anchor.String())
}

func (b *builder) targetByID(id string) (Target, bool) {
	for _, target := range b.targets {
		if target.target.ID == id {
			return target.target, true
		}
	}
	return Target{}, false
}

func methodsMatch(routeMethod, callMethod string) bool {
	return routeMethod == callMethod || routeMethod == "ANY"
}

// pathsMatch compares paths segment by segment; a route parameter or a call
// template hole matches exactly one segment.
func pathsMatch(routePath, callPath string) bool {
	routeSegments := pathSegments(routePath)
	callSegments := pathSegments(stripOrigin(callPath))
	if len(routeSegments) != len(callSegments) {
		return false
	}
	for position := range routeSegments {
		routeSegment, callSegment := routeSegments[position], callSegments[position]
		if routeSegment == callSegment || isRouteParameter(routeSegment) || callSegment == "{param}" {
			continue
		}
		return false
	}
	return true
}

func hasPathParameters(routePath string) bool {
	for _, segment := range pathSegments(routePath) {
		if isRouteParameter(segment) {
			return true
		}
	}
	return false
}

func pathSegments(value string) []string {
	if question := strings.Index(value, "?"); question >= 0 {
		value = value[:question]
	}
	value = strings.Trim(value, "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

// stripOrigin removes a scheme and host from an absolute URL so the path can
// be compared with a route.
func stripOrigin(value string) string {
	for _, scheme := range []string{"http://", "https://"} {
		if !strings.HasPrefix(value, scheme) {
			continue
		}
		rest := value[len(scheme):]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "/"
		}
		return rest[slash:]
	}
	return value
}

func isRouteParameter(segment string) bool {
	switch {
	case strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"):
		return true
	case strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">"):
		return true
	case strings.HasPrefix(segment, ":") && len(segment) > 1:
		return true
	case segment == "*":
		return true
	default:
		return false
	}
}
