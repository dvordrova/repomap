package atlasstudy

import "fmt"

const promptSystemTemplate = `You are preparing a repository brief and editorial study routes from a bounded, backend-owned catalog.
Use only the exact request-local refs advertised in the catalog. Do not invent paths, symbols, runtime order, graph relations, reachability, or evidence. The catalog contains semantic labels and facts, not source code.
Return exactly one JSON object and no markdown. Keep all enum values and refs unchanged. Write model-authored prose in the requested language.
Every required brief statement needs 1-8 support_refs selected only from brief_support_choices. Each choice has an exact ref kind. Refs found only in other sections, including every unit ref, are not Brief support.
Return 1-%d directions. Each direction needs 1-5 principal_refs, including at least one component ref, and 3-5 distinct reading items. Every selected reading target must advertise at least one principal_ref also selected by that direction. related_component_refs are conceptual associations only. owner_ref, when present, is separate exact local producer evidence; never invent an owner or treat conceptual membership as ownership.
Allowed target_job values: first_contact, use_or_operate, extend_or_integrate, contribute, debug_or_maintain.
Allowed learning_stage values: orientation, central_operation, core_model, integration, operations, contribution.
Allowed reading labels: start, continue, connect, verify, contrast.
Do not state an execution sequence as proven. Reading order is editorial navigation only.`

func (product Product) BuildPrompt() Prompt {
	return Prompt{
		Version:  PromptVersion,
		Language: product.input.Language,
		System:   fmt.Sprintf(promptSystemTemplate, MaxDirections),
		User: fmt.Sprintf(
			"Requested prose language: %s.\nCatalog JSON:\n%s\n\n"+
				"Response schema: {\"repository_type\":\"service_application|library_framework|cli_tool|monorepo|mixed\",\"brief\":{\"what_it_is\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"problem\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"main_input\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"central_responsibility\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"observable_result\":{\"text\":\"...\",\"support_refs\":[\"...\"]},\"domain_terms\":[{\"term\":\"...\",\"meaning\":\"...\",\"support_refs\":[\"...\"]}]},\"directions\":[{\"question\":\"...?\",\"why_it_matters\":\"...\",\"learning_outcome\":\"...\",\"target_job\":\"...\",\"learning_stage\":\"...\",\"principal_refs\":[\"...\"],\"reading\":[{\"target_ref\":\"...\",\"label\":\"start\",\"what_to_look_for\":\"...\"}]}]}",
			product.input.Language, string(product.wire),
		),
	}
}
