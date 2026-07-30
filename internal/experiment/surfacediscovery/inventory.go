package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// The inventory fact model is deliberately framework-neutral. Detectors emit
// local descriptors, bindings between them, and activation sites. A later,
// stricter mechanism stage may prove reachability across these facts; shallow
// inventory collection does not.
type inventoryDescriptor struct {
	ID                           string
	Kind                         string
	Framework                    string
	Package                      string
	Location                     Location
	Identity                     Value
	Handler                      Value
	HandlerLocation              *Location
	Constructor                  Symbol
	InstanceCorrelationAmbiguous bool
	Evidence                     []Evidence
	Provenance                   []Provenance
	Frontiers                    []Frontier
}

type inventoryRef struct {
	DescriptorID string
	CandidateIDs []string
}

type inventoryBinding struct {
	ID         string
	Kind       string
	From       inventoryRef
	To         inventoryRef
	Location   Location
	Scope      Symbol
	Exact      bool
	Evidence   []Evidence
	Provenance []Provenance
	Frontiers  []Frontier
}

type inventoryActivation struct {
	ID         string
	Kind       string
	Surface    inventoryRef
	Location   Location
	Scope      Symbol
	Exact      bool
	Evidence   []Evidence
	Provenance []Provenance
	Frontiers  []Frontier
}

type inventoryFacts struct {
	Descriptors []inventoryDescriptor
	Bindings    []inventoryBinding
	Activations []inventoryActivation
}

type inventoryProjectionContext struct {
	Descriptor        inventoryDescriptor
	Binding           *inventoryBinding
	Activation        *inventoryActivation
	RelatedEvidence   []Evidence
	RelatedProvenance []Provenance
	Frontiers         []Frontier
}

const (
	maxInventoryDescriptors  = 2048
	maxInventoryBindings     = 8192
	maxInventoryActivations  = 1024
	maxInventoryRelatedFacts = 16
)

// projectInventory emits exactly one compatibility record per descriptor.
// One exact relation may supply legacy registration fields; additional and
// ambiguous relations remain attached as bounded evidence/frontiers and never
// create or hide descriptor records.
func projectInventory(
	facts inventoryFacts,
	render func(inventoryProjectionContext) TriggerRecord,
) []TriggerRecord {
	descriptorCount := min(len(facts.Descriptors), maxInventoryDescriptors)
	descriptors := append([]inventoryDescriptor(nil), facts.Descriptors[:descriptorCount]...)
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID < descriptors[j].ID
	})
	bindingCount := min(len(facts.Bindings), maxInventoryBindings)
	bindings := append([]inventoryBinding(nil), facts.Bindings[:bindingCount]...)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	activationCount := min(len(facts.Activations), maxInventoryActivations)
	activations := append([]inventoryActivation(nil), facts.Activations[:activationCount]...)
	sort.Slice(activations, func(i, j int) bool { return activations[i].ID < activations[j].ID })

	bindingsByTarget := make(map[string][]inventoryBinding)
	activationsByTarget := make(map[string][]inventoryActivation)
	frontiersByDescriptor := make(map[string][]Frontier)
	bindingSeen := make(map[string]bool)
	for _, binding := range bindings {
		if bindingSeen[binding.ID] {
			continue
		}
		bindingSeen[binding.ID] = true
		if binding.Exact && binding.To.DescriptorID != "" {
			bindingsByTarget[binding.To.DescriptorID] = append(
				bindingsByTarget[binding.To.DescriptorID],
				binding,
			)
		}
		for _, id := range inventoryRefIDs(binding.From, binding.To) {
			frontiersByDescriptor[id] = append(
				frontiersByDescriptor[id],
				binding.Frontiers...,
			)
		}
	}
	activationSeen := make(map[string]bool)
	for _, activation := range activations {
		if activationSeen[activation.ID] {
			continue
		}
		activationSeen[activation.ID] = true
		if activation.Exact && activation.Surface.DescriptorID != "" {
			activationsByTarget[activation.Surface.DescriptorID] = append(
				activationsByTarget[activation.Surface.DescriptorID],
				activation,
			)
		}
		for _, id := range inventoryRefIDs(activation.Surface) {
			frontiersByDescriptor[id] = append(
				frontiersByDescriptor[id],
				activation.Frontiers...,
			)
		}
	}

	var records []TriggerRecord
	for _, descriptor := range descriptors {
		context := inventoryProjectionContext{
			Descriptor: descriptor,
			Frontiers: append(
				append([]Frontier(nil), descriptor.Frontiers...),
				frontiersByDescriptor[descriptor.ID]...,
			),
		}
		activations := activationsByTarget[descriptor.ID]
		bindings := bindingsByTarget[descriptor.ID]
		if len(activations) == 1 {
			activation := activations[0]
			context.Activation = &activation
			context.Frontiers = append(context.Frontiers, activation.Frontiers...)
		}
		if len(bindings) == 1 {
			binding := bindings[0]
			context.Binding = &binding
			context.Frontiers = append(context.Frontiers, binding.Frontiers...)
		}
		additional := 0
		for index := 0; len(activations) > 1 && index < len(activations); index++ {
			additional = appendInventoryRelation(
				&context,
				"inventory_exact_activation_ambiguous",
				activations[index].ID,
				activations[index].Location,
				activations[index].Evidence,
				activations[index].Provenance,
				additional,
			)
		}
		for index := 0; len(bindings) > 1 && index < len(bindings); index++ {
			additional = appendInventoryRelation(
				&context,
				"inventory_exact_binding_ambiguous",
				bindings[index].ID,
				bindings[index].Location,
				bindings[index].Evidence,
				bindings[index].Provenance,
				additional,
			)
		}
		records = append(records, render(context))
	}
	return records
}

func appendInventoryRelation(
	context *inventoryProjectionContext,
	kind string,
	id string,
	location Location,
	evidence []Evidence,
	provenance []Provenance,
	count int,
) int {
	if count < maxInventoryRelatedFacts {
		context.RelatedEvidence = append(context.RelatedEvidence, evidence...)
		context.RelatedProvenance = append(context.RelatedProvenance, provenance...)
		context.Frontiers = append(context.Frontiers, Frontier{
			Kind: kind, Detail: id, Location: &location,
		})
	} else if count == maxInventoryRelatedFacts {
		context.Frontiers = append(context.Frontiers, Frontier{
			Kind:     "inventory_relation_evidence_limit",
			Detail:   "additional exact relations exceed the per-descriptor evidence limit",
			Location: &context.Descriptor.Location,
		})
	}
	return count + 1
}

func inventoryRefIDs(refs ...inventoryRef) []string {
	seen := make(map[string]struct{})
	for _, ref := range refs {
		if ref.DescriptorID != "" {
			seen[ref.DescriptorID] = struct{}{}
		}
		for _, id := range ref.CandidateIDs {
			if id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func stableInventoryID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "inventory-" + hex.EncodeToString(digest[:12])
}
