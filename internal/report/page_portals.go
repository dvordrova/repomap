package report

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
)

// A portal is one target's accepted client request answered by another
// target's accepted route: a path the route's parameters admit and no method
// that differs. Both sides are registrations the reading stage classified; the
// join itself is literal comparison, made here so the overlay stays free of
// derived rows. A method is only what the code states: a request that states
// none (a socket connect, a call whose verb is not written) is joined by its
// path alone and stays possible, never taken for a GET.
type portalLink struct {
	call, route  facts.Fact
	method, path string
	possible     bool
}

func (builder *pageBuilder) portalLinks() []portalLink {
	var requests, routes []registrationRole
	for _, section := range builder.sections {
		index := builder.graphIndex(section.programTargetID)
		if index == nil {
			continue
		}
		for _, call := range index.Outbound {
			if fact, ok := builder.registrationFact(call.FactID); ok && call.Kind == "client_request" {
				requests = append(requests, registrationRole{fact: fact, method: firstNonEmpty(call.Method, fact.Method)})
			}
		}
		for _, operation := range index.Operations {
			if fact, ok := builder.registrationFact(operation.FactID); ok && operation.Kind == "request" {
				routes = append(routes, registrationRole{fact: fact, method: fact.Method})
			}
		}
	}
	var result []portalLink
	for _, request := range requests {
		var matches []registrationRole
		for _, route := range routes {
			if route.fact.TargetID == request.fact.TargetID || !methodsMatch(route.method, request.method) || !pathsMatch(route.fact.Path, request.fact.Path) {
				continue
			}
			matches = append(matches, route)
		}
		if len(matches) != 1 {
			continue
		}
		route := matches[0]
		result = append(result, portalLink{
			call: request.fact, route: route.fact, method: firstNonEmpty(request.method, route.method), path: route.fact.Path,
			possible: request.method == "" || request.fact.Resolution == facts.ResolutionPossible || route.fact.Resolution == facts.ResolutionPossible || hasPathParameters(route.fact.Path),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].call.Anchor.String() != result[j].call.Anchor.String() {
			return result[i].call.Anchor.String() < result[j].call.Anchor.String()
		}
		return result[i].route.Anchor.String() < result[j].route.Anchor.String()
	})
	return result
}

type registrationRole struct {
	fact   facts.Fact
	method string
}

func (builder *pageBuilder) registrationFact(id string) (facts.Fact, bool) {
	fact, ok := builder.factsByID[id]
	if !ok || fact.Kind != facts.KindRegistration || fact.Anchor == nil || builder.testPaths[fact.Anchor.Path] {
		return facts.Fact{}, false
	}
	return fact, true
}

// outboundRequests lists a target's accepted client requests, whatever the
// protocol, each with the method its code states or none; an empty target
// lists every target's, naming each.
func (builder *pageBuilder) outboundRequests(programTargetID, _ string) []pageHTTPRow {
	var rows []pageHTTPRow
	for _, section := range builder.sections {
		if programTargetID != "" && section.programTargetID != programTargetID {
			continue
		}
		index := builder.graphIndex(section.programTargetID)
		if index == nil {
			continue
		}
		for _, call := range index.Outbound {
			if call.Kind != "client_request" {
				continue
			}
			fact, ok := builder.registrationFact(call.FactID)
			if !ok {
				continue
			}
			row := builder.registrationRow(fact, call.Method)
			if programTargetID == "" {
				row.Target = section.Name
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// registrationRow shows a registration as method, address and the callable
// handed over, with its source anchors.
func (builder *pageBuilder) registrationRow(fact facts.Fact, method string) pageHTTPRow {
	row := pageHTTPRow{
		Method: firstNonEmpty(method, fact.Method), Path: firstNonEmpty(fact.Path, strings.Join(fact.Values, " ")), Symbol: fact.Symbol,
		Anchor:   builder.links.factAnchor(fact),
		Possible: fact.Resolution == facts.ResolutionPossible,
	}
	if fact.ObjectID != "" {
		if subject, known := builder.subject(fact.TargetID, fact.ObjectID); known {
			if _, anchor := builder.subjectDisplay(subject.subject); anchor != nil {
				row.SymbolAnchor = anchor
			}
		}
	}
	return row
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// methodsMatch compares two stated methods; a side that states none (a route
// registered for every method, a request whose verb is not written) differs
// from nothing.
func methodsMatch(routeMethod, callMethod string) bool {
	return routeMethod == "" || callMethod == "" || routeMethod == "ANY" || routeMethod == callMethod
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

var _ = groupindex.Version
