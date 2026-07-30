package report

import "strings"

const (
	surfaceQualityExact         = "exact"
	surfaceQualityStatic        = "static"
	surfaceQualityPartial       = "partial"
	surfaceQualityUnresolved    = "unresolved"
	surfaceQualityNotApplicable = "not_applicable"
	surfaceQualityRejected      = "rejected"
)

func ensureProjectedSurfaceSemantics(trigger *DiscoveredTrigger) {
	if trigger == nil {
		return
	}
	if hasProducerSurfaceSemantics(*trigger) {
		return
	}
	quality := SurfaceQuality{
		Identity:          projectedIdentityQuality(*trigger),
		RegistrationStart: projectedRegistrationQuality(*trigger),
		HandlerCallback:   projectedHandlerQuality(*trigger),
		Reachability:      projectedReachabilityQuality(*trigger),
		Ownership:         projectedOwnershipQuality(*trigger),
	}
	role, readiness, reason := projectedSurfaceRoleAndReadiness(*trigger, quality)
	quality.Traceability = readiness
	trigger.SurfaceRole = role
	trigger.TraceReadiness = readiness
	trigger.TraceReadinessReason = reason
	trigger.Quality = quality
}

// Current surface artifacts persist one complete semantic assessment alongside
// every record. The report must preserve that producer contract rather than
// deriving a second, potentially contradictory assessment. Report-built and
// legacy records without the complete set retain the canonical fallback below.
func hasProducerSurfaceSemantics(trigger DiscoveredTrigger) bool {
	quality := trigger.Quality
	return trigger.SurfaceRole != "" && trigger.TraceReadiness != "" &&
		trigger.TraceReadinessReason != "" && quality.Identity != "" &&
		quality.RegistrationStart != "" && quality.HandlerCallback != "" &&
		quality.Reachability != "" && quality.Ownership != "" && quality.Traceability != ""
}

func projectedSurfaceRoleAndReadiness(trigger DiscoveredTrigger, quality SurfaceQuality) (string, string, string) {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		reason := strings.TrimSpace(trigger.UnavailableReason)
		if reason == "" {
			reason = "surface evidence is unavailable under the recorded build scenario"
		}
		return SurfaceRoleRejected, SurfaceTraceRejected, reason
	}
	if trigger.ApplicationClass == SurfaceSupportingDependency || trigger.ApplicationClass == SurfaceDependencyOnly {
		return SurfaceRoleNoisy, SurfaceTraceUnsupported,
			"dependency behavior remains supporting evidence and is not an application-owned trace seed"
	}
	switch trigger.Kind {
	case "process_entry":
		return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
			"exact process entry can seed a partial trace; downstream runtime handoff remains unresolved"
	case "cli_command":
		switch trigger.DiscoveryBasis {
		case "build_selected_typed_cobra_activation":
			return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
				"exact direct command activation can seed a partial path; child dispatch and downstream effects remain unresolved"
		case "build_selected_typed_cobra_binding":
			return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
				"exact direct command binding can seed a partial path; process activation and downstream effects remain unresolved"
		case "build_selected_typed_cobra_descriptor":
			return SurfaceRoleDescriptor, SurfaceTracePartialReady,
				"exact typed command descriptor is a useful starting point; registration and activation remain unresolved"
		}
		if quality.Identity == surfaceQualityExact && quality.RegistrationStart == surfaceQualityExact &&
			quality.HandlerCallback == surfaceQualityExact && quality.Reachability == surfaceQualityStatic &&
			trigger.Resolution == "exact" {
			return SurfaceRoleEntrySurface, SurfaceTraceReady,
				"exact command identity, registration, callback, and static process reachability can seed a command trace"
		}
		return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
			"command evidence can seed only a partial path because registration, activation, callback, or reachability evidence is incomplete"
	case "http_route":
		if quality.Identity == surfaceQualityExact && quality.RegistrationStart == surfaceQualityExact &&
			quality.HandlerCallback == surfaceQualityExact && quality.Reachability == surfaceQualityStatic &&
			trigger.Resolution == "exact" {
			return SurfaceRoleEntrySurface, SurfaceTraceReady,
				"exact route identity, registration, handler, and bounded static reachability can seed a request trace"
		}
		if quality.Identity == surfaceQualityExact && quality.RegistrationStart == surfaceQualityExact {
			return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
				"exact route registration can seed only a partial trace because handler or reachability evidence is incomplete"
		}
		return SurfaceRoleDynamicFrontier, SurfaceTraceUnsupported,
			"route identity is unresolved and cannot select an exact request trace seed"
	case "http_server":
		if quality.RegistrationStart == surfaceQualityExact {
			return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
				"exact server start can seed a partial operational trace; accept and dispatch order remain unresolved"
		}
		if quality.RegistrationStart == surfaceQualityPartial {
			return SurfaceRoleEntrySurface, SurfaceTracePartialReady,
				"exact repository-local wrapper call reaches a supported server start; terminal dispatch remains unresolved"
		}
		return SurfaceRoleEntrySurface, SurfaceTraceUnsupported,
			"server start location is unresolved"
	case "http_route_descriptor":
		if quality.Identity == surfaceQualityExact && quality.RegistrationStart == surfaceQualityExact {
			return SurfaceRoleDescriptor, SurfaceTracePartialReady,
				"exact descriptor evidence can seed a partial trace; runtime consumer registration remains unresolved"
		}
		return SurfaceRoleDescriptor, SurfaceTraceUnsupported,
			"descriptor identity or source location is unresolved"
	case "worker", "async_task":
		return SurfaceRoleRuntimeActivity, SurfaceTraceUnsupported,
			"runtime activity is nested asynchronous work and cannot independently establish a top-level trace"
	case "http_route_frontier":
		return SurfaceRoleDynamicFrontier, SurfaceTraceUnsupported,
			"dynamic frontier has no exact executable entry to seed a trace"
	default:
		return SurfaceRoleNoisy, SurfaceTraceUnsupported,
			"surface kind is not supported as an independent trace seed"
	}
}

func projectedIdentityQuality(trigger DiscoveredTrigger) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		if validProjectedSurfaceLocation(trigger.ProcessEntrypoint.Location) {
			return surfaceQualityExact
		}
		return surfaceQualityUnresolved
	}
	if trigger.Identity.Path.Known || strings.TrimSpace(trigger.Identity.Name) != "" && !trigger.ProvisionalID {
		return surfaceQualityExact
	}
	if strings.TrimSpace(trigger.Identity.Path.Text) != "" || len(trigger.Identity.Path.Candidates) > 0 {
		return surfaceQualityPartial
	}
	return surfaceQualityUnresolved
}

func projectedRegistrationQuality(trigger DiscoveredTrigger) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		return surfaceQualityNotApplicable
	}
	location := trigger.RegistrationSite
	if trigger.Kind == "http_route_descriptor" && trigger.DescriptorSite != nil {
		location = trigger.DescriptorSite
	}
	if trigger.Kind == "http_server" && trigger.ServerStartSite != nil {
		location = trigger.ServerStartSite
	}
	if validProjectedSurfaceLocation(location) {
		return surfaceQualityExact
	}
	if trigger.Kind == "http_server" {
		for index := len(trigger.WrapperChain) - 1; index >= 0; index-- {
			if validProjectedSurfaceLocation(trigger.WrapperChain[index].Callsite) {
				return surfaceQualityPartial
			}
		}
	}
	return surfaceQualityUnresolved
}

func projectedHandlerQuality(trigger DiscoveredTrigger) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceQualityRejected
	}
	if trigger.Kind == "process_entry" || trigger.Kind == "http_route_frontier" {
		return surfaceQualityNotApplicable
	}
	if trigger.Handler.Known && strings.TrimSpace(trigger.Handler.Text) != "" {
		return surfaceQualityExact
	}
	if strings.TrimSpace(trigger.Handler.Text) != "" || len(trigger.Handler.Candidates) > 0 {
		return surfaceQualityPartial
	}
	return surfaceQualityUnresolved
}

func projectedReachabilityQuality(trigger DiscoveredTrigger) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		return surfaceQualityPartial
	}
	if trigger.Kind == "http_route_frontier" {
		return surfaceQualityUnresolved
	}
	for _, frontier := range trigger.DynamicFrontier {
		if frontier.Kind == "entrypoint_dispatch_unresolved" || frontier.Kind == "call_target_limit" {
			return surfaceQualityPartial
		}
	}
	if validProjectedSurfaceLocation(trigger.ProcessEntrypoint.Location) {
		return surfaceQualityStatic
	}
	return surfaceQualityUnresolved
}

func projectedOwnershipQuality(trigger DiscoveredTrigger) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceQualityRejected
	}
	hasOwner := strings.TrimSpace(trigger.OwningExecutable) != ""
	hasRole := trigger.ExecutableRole != "" && trigger.ExecutableRole != ExecutableRoleUnknown
	if hasOwner && hasRole {
		return surfaceQualityExact
	}
	if hasOwner || hasRole {
		return surfaceQualityPartial
	}
	return surfaceQualityUnresolved
}

func validProjectedSurfaceLocation(location *SurfaceLocation) bool {
	return location != nil && strings.TrimSpace(location.Path) != "" && location.Line > 0
}
