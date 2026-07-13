package surfacediscovery

import "strings"

const (
	SurfaceRoleEntrySurface    = "entry_surface"
	SurfaceRoleRuntimeActivity = "runtime_activity"
	SurfaceRoleDescriptor      = "descriptor"
	SurfaceRoleDynamicFrontier = "dynamic_frontier"
	SurfaceRoleRejected        = "rejected"
	SurfaceRoleNoisy           = "noisy"

	TraceReadinessReady       = "trace_ready"
	TraceReadinessPartial     = "partial_trace_ready"
	TraceReadinessUnsupported = "unsupported"
	TraceReadinessRejected    = "rejected"

	SurfaceQualityExact         = "exact"
	SurfaceQualityStatic        = "static"
	SurfaceQualityPartial       = "partial"
	SurfaceQualityUnresolved    = "unresolved"
	SurfaceQualityNotApplicable = "not_applicable"
	SurfaceQualityRejected      = "rejected"
)

func deriveSurfaceSemantics(trigger *TriggerRecord) {
	if trigger == nil {
		return
	}
	quality := SurfaceQuality{
		Identity:          identityQuality(*trigger),
		RegistrationStart: registrationQuality(*trigger),
		HandlerCallback:   handlerQuality(*trigger),
		Reachability:      reachabilityQuality(*trigger),
		Ownership:         ownershipQuality(*trigger),
	}
	role, readiness, reason := surfaceRoleAndReadiness(*trigger, quality)
	quality.Traceability = readiness
	trigger.SurfaceRole = role
	trigger.TraceReadiness = readiness
	trigger.TraceReadinessReason = reason
	trigger.Quality = quality
}

func surfaceRoleAndReadiness(trigger TriggerRecord, quality SurfaceQuality) (string, string, string) {
	if trigger.Availability == AvailabilityUnavailable {
		reason := strings.TrimSpace(trigger.UnavailableReason)
		if reason == "" {
			reason = "surface evidence is unavailable under the recorded build scenario"
		}
		return SurfaceRoleRejected, TraceReadinessRejected, reason
	}
	switch trigger.Kind {
	case "process_entry":
		return SurfaceRoleEntrySurface, TraceReadinessPartial,
			"exact process entry can seed a partial trace; downstream runtime handoff remains unresolved"
	case "http_route":
		if quality.Identity == SurfaceQualityExact && quality.RegistrationStart == SurfaceQualityExact &&
			quality.HandlerCallback == SurfaceQualityExact && quality.Reachability == SurfaceQualityStatic &&
			trigger.Resolution == "exact" {
			return SurfaceRoleEntrySurface, TraceReadinessReady,
				"exact route identity, registration, handler, and bounded static reachability can seed a request trace"
		}
		if quality.Identity == SurfaceQualityExact && quality.RegistrationStart == SurfaceQualityExact {
			return SurfaceRoleEntrySurface, TraceReadinessPartial,
				"exact route registration can seed only a partial trace because handler or reachability evidence is incomplete"
		}
		return SurfaceRoleDynamicFrontier, TraceReadinessUnsupported,
			"route identity is unresolved and cannot select an exact request trace seed"
	case "http_server":
		if quality.RegistrationStart == SurfaceQualityExact {
			return SurfaceRoleEntrySurface, TraceReadinessPartial,
				"exact server start can seed a partial operational trace; accept and dispatch order remain unresolved"
		}
		if quality.RegistrationStart == SurfaceQualityPartial {
			return SurfaceRoleEntrySurface, TraceReadinessPartial,
				"exact repository-local wrapper call reaches a supported server start; terminal dispatch remains unresolved"
		}
		return SurfaceRoleEntrySurface, TraceReadinessUnsupported,
			"server start location is unresolved"
	case "http_route_descriptor":
		if quality.Identity == SurfaceQualityExact && quality.RegistrationStart == SurfaceQualityExact {
			return SurfaceRoleDescriptor, TraceReadinessPartial,
				"exact descriptor evidence can seed a partial trace; runtime consumer registration remains unresolved"
		}
		return SurfaceRoleDescriptor, TraceReadinessUnsupported,
			"descriptor identity or source location is unresolved"
	case "worker", "async_task":
		return SurfaceRoleRuntimeActivity, TraceReadinessUnsupported,
			"runtime activity is nested asynchronous work and cannot independently establish a top-level trace"
	case "http_route_frontier":
		return SurfaceRoleDynamicFrontier, TraceReadinessUnsupported,
			"dynamic frontier has no exact executable entry to seed a trace"
	default:
		return SurfaceRoleNoisy, TraceReadinessUnsupported,
			"surface kind is not supported as an independent trace seed"
	}
}

func identityQuality(trigger TriggerRecord) string {
	if trigger.Availability == AvailabilityUnavailable {
		return SurfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		if validSurfaceLocation(trigger.ProcessEntrypoint.Location) {
			return SurfaceQualityExact
		}
		return SurfaceQualityUnresolved
	}
	if trigger.Identity.Path.Known || strings.TrimSpace(trigger.Identity.Name) != "" && !trigger.ProvisionalID {
		return SurfaceQualityExact
	}
	if strings.TrimSpace(trigger.Identity.Path.Text) != "" || len(trigger.Identity.Path.Candidates) > 0 {
		return SurfaceQualityPartial
	}
	return SurfaceQualityUnresolved
}

func registrationQuality(trigger TriggerRecord) string {
	if trigger.Availability == AvailabilityUnavailable {
		return SurfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		return SurfaceQualityNotApplicable
	}
	location := trigger.RegistrationSite
	if trigger.Kind == "http_route_descriptor" && trigger.DescriptorSite != nil {
		location = *trigger.DescriptorSite
	}
	if trigger.Kind == "http_server" && trigger.ServerStartSite != nil {
		location = *trigger.ServerStartSite
	}
	if validSurfaceLocation(location) {
		return SurfaceQualityExact
	}
	if trigger.Kind == "http_server" {
		for index := len(trigger.WrapperChain) - 1; index >= 0; index-- {
			if validSurfaceLocation(trigger.WrapperChain[index].Callsite) {
				return SurfaceQualityPartial
			}
		}
	}
	return SurfaceQualityUnresolved
}

func handlerQuality(trigger TriggerRecord) string {
	if trigger.Availability == AvailabilityUnavailable {
		return SurfaceQualityRejected
	}
	if trigger.Kind == "process_entry" || trigger.Kind == "http_route_frontier" {
		return SurfaceQualityNotApplicable
	}
	if trigger.Handler.Known && strings.TrimSpace(trigger.Handler.Text) != "" {
		return SurfaceQualityExact
	}
	if strings.TrimSpace(trigger.Handler.Text) != "" || len(trigger.Handler.Candidates) > 0 {
		return SurfaceQualityPartial
	}
	return SurfaceQualityUnresolved
}

func reachabilityQuality(trigger TriggerRecord) string {
	if trigger.Availability == AvailabilityUnavailable {
		return SurfaceQualityRejected
	}
	if trigger.Kind == "process_entry" {
		return SurfaceQualityPartial
	}
	if trigger.Kind == "http_route_frontier" {
		return SurfaceQualityUnresolved
	}
	for _, frontier := range trigger.DynamicFrontier {
		if frontier.Kind == "entrypoint_dispatch_unresolved" || frontier.Kind == "call_target_limit" {
			return SurfaceQualityPartial
		}
	}
	if validSurfaceLocation(trigger.ProcessEntrypoint.Location) {
		return SurfaceQualityStatic
	}
	return SurfaceQualityUnresolved
}

func ownershipQuality(trigger TriggerRecord) string {
	if trigger.Availability == AvailabilityUnavailable {
		return SurfaceQualityRejected
	}
	hasOwner := strings.TrimSpace(trigger.OwningExecutable) != ""
	hasRole := trigger.ExecutableRole != "" && trigger.ExecutableRole != ExecutableRoleUnknown
	if hasOwner && hasRole {
		return SurfaceQualityExact
	}
	if hasOwner || hasRole {
		return SurfaceQualityPartial
	}
	return SurfaceQualityUnresolved
}

func validSurfaceLocation(location Location) bool {
	return cleanRepositoryPath(location.Path) != "" && location.Line > 0
}
