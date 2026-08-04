package atlasstudy

import "fmt"

const promptSystemTemplate = `You are preparing a repository brief and editorial study routes from a bounded, backend-owned catalog.
Use only the exact request-local refs advertised in the catalog. reading_targets contain bounded exact repository-relative path, line, and optional symbol context; every such path is repeated in allowed_paths. Treat those locators as read-only context. Identity fields return only short refs; never copy a short ref into prose. Direction prose may repeat an exact path or symbol only when that direction selects the corresponding reading target. A supported Brief statement or domain-term meaning may repeat an exact path or symbol only from a reading_target included in its support_refs; a domain-term name may not. Never invent paths or symbols, runtime order, graph relations, reachability, or evidence. The catalog contains semantic labels and facts, not source code.
Return exactly one JSON object and no markdown. Keep all enum values and refs unchanged. Write model-authored prose in the requested language.
Every required brief statement needs one or more unique support_refs selected only from brief_support_choices. Each choice has an exact ref kind. Refs found only in other sections, including every unit ref, are not Brief support.
Choose %d-%d advertised route_spans you consider most useful and return exactly one ranked direction per chosen span. Array order is rank: the first direction is your highest-ranked selection and why_it_matters is the bounded rationale for that selection. %d directions is the hard ceiling; a valid response with fewer than %d directions is accepted as-is with no filler or padding. Return 0-%d optional domain_terms supported by the bounded catalog. Terms beyond that explicit count are unrequested output. Each requested term is validated independently; one invalid optional term does not invalidate the required Brief or valid sibling terms.
Each direction must copy exactly one span_ref from route_spans, the span's target_job and learning_stage, 1-5 principal_refs and %d-%d distinct reading items. Do not return or rewrite the backend-owned question. A direction principal_ref must be a component c* or surface sf* ref copied from principal_refs advertised by one of that direction's selected reading targets. Never use unit u*, subsystem ss*, reading-target a*, evidence e*, document d*, route-support rs*, or route-span sp* refs as direction principals. Include at least one component principal. Every selected principal must be advertised by at least one selected reading target.
Every selected target_ref must be in the chosen span's allowed_target_refs and must contribute at least one of that span's required_support_refs. Cover every required support. A system_path span requires at least two distinct reading targets. Do not pad a span with an unrelated target.
Every reading target_ref must be an a* reading_target ref. Every selected reading target must advertise at least one principal_ref also selected by that direction. related_component_refs are conceptual associations only. owner_ref, when present, is separate exact local producer evidence; never invent an owner or treat conceptual membership as ownership.
Allowed target_job values: first_contact, use_or_operate, extend_or_integrate, contribute, debug_or_maintain.
Allowed learning_stage values: orientation, central_operation, core_model, integration, operations, contribution.
Allowed reading labels: start, continue, connect, verify, contrast.
Do not state an execution sequence as proven. Reading order is editorial navigation only.`

func (product Product) BuildPrompt() Prompt {
	return Prompt{
		Version:  PromptVersion,
		Language: product.input.Language,
		System: fmt.Sprintf(
			promptSystemTemplate, MinPortfolioDirections, MaxDirections,
			MaxDirections, MinPortfolioDirections, MaxDomainTerms,
			MinDirectionReadingCount, MaxDirectionReadingCount,
		),
		User: fmt.Sprintf(
			"Requested prose language: %s.\nCatalog JSON:\n%s\n\n"+
				"Response schema: {\"repository_type\":\"service_application|library_framework|cli_tool|monorepo|mixed\",\"brief\":{\"what_it_is\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"problem\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"main_input\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"central_responsibility\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"observable_result\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"domain_terms\":[{\"term\":\"...\",\"meaning\":\"...\",\"support_refs\":[\"...\"]}]},\"directions\":[{\"span_ref\":\"sp1\",\"why_it_matters\":\"...\",\"learning_outcome\":\"...\",\"target_job\":\"...\",\"learning_stage\":\"...\",\"principal_refs\":[\"...\"],\"reading\":[{\"target_ref\":\"...\",\"label\":\"start\",\"what_to_look_for\":\"...\"}]}]}",
			product.input.Language, string(product.wire),
		),
	}
}
