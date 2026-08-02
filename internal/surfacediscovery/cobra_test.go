package surfacediscovery

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestTypedCobraShallowInventoryKeepsDescriptorsAndDirectFacts(t *testing.T) {
	result, err := Analyze(DefaultOptions(filepath.Join("testdata", "cobra")))
	if err != nil {
		t.Fatal(err)
	}

	var cobraRecords []TriggerRecord
	byName := make(map[string][]TriggerRecord)
	descriptorSites := make(map[string]int)
	for _, trigger := range result.Catalog.Triggers {
		if trigger.Kind != "cli_command" {
			continue
		}
		if trigger.Producer != cobraProducer || trigger.Framework != "cobra" {
			t.Fatalf("CLI producer/framework = %#v", trigger)
		}
		cobraRecords = append(cobraRecords, trigger)
		byName[trigger.Identity.Name] = append(byName[trigger.Identity.Name], trigger)
		if trigger.DescriptorSite == nil {
			t.Fatalf("typed Cobra record lacks descriptor site: %#v", trigger)
		}
		descriptorSites[locationKey(*trigger.DescriptorSite)]++
		if trigger.Resolution != "partial" ||
			!hasFrontier(trigger.DynamicFrontier, "shallow_inventory_no_dispatch_proof") {
			t.Fatalf("shallow record overclaims deep proof: %#v", trigger)
		}
	}
	if len(cobraRecords) != 36 || len(descriptorSites) != len(cobraRecords) {
		t.Fatalf(
			"typed Cobra descriptor projection = %d records at %d sites, want one record at each of 36 sites",
			len(cobraRecords),
			len(descriptorSites),
		)
	}
	for site, count := range descriptorSites {
		if count != 1 {
			t.Errorf("descriptor %s projected %d times", site, count)
		}
	}

	for _, name := range []string{
		"direct-main", "nested-func", "fixture", "get", "put", "lease", "lease grant",
		"endpoint health", "snapshot save", "user add",
		"role grant-permission", "initialized",
		"dead", "orphan", "ghost", "ambiguous-a", "ambiguous-b",
		"before-dynamic", "before-conditional",
		"after-conditional-literal",
		"shared-root", "only-a", "only-b",
		"variadic-alpha", "variadic-beta", "variadic-gamma", "variadic-delta",
		"reassigned-before", "reassigned-after",
	} {
		if len(byName[name]) == 0 {
			t.Errorf("missing shallow Cobra descriptor/projection %q", name)
		}
	}
	for _, forbidden := range []string{
		"fake", "also-fake", "dynamic-at-runtime", "after-conditional",
	} {
		if len(byName[forbidden]) > 0 {
			t.Errorf("published lookalike or interpreted assignment %q", forbidden)
		}
	}

	get := oneRecordWithBasis(t, byName["get"], "build_selected_typed_cobra_binding")
	if get.RegistrationSite.Path != "internal/cli/root.go" ||
		get.RegistrationSite.Line != 18 ||
		get.Handler.Text != "example.com/typed-cobra/internal/cli.runGet" {
		t.Fatalf("direct imported-package binding = %#v", get)
	}
	grant := oneRecordWithBasis(t, byName["lease grant"], "build_selected_typed_cobra_binding")
	if grant.RegistrationSite.Path != "internal/cli/commands.go" ||
		grant.RegistrationSite.Line != 14 {
		t.Fatalf("direct constructor-local binding = %#v", grant)
	}
	root := oneRecordWithBasis(t, byName["fixture"], "build_selected_typed_cobra_activation")
	if root.RegistrationSite != (Location{}) ||
		root.ProcessEntrypoint.ID != "" ||
		!hasEvidenceAt(root.Evidence, "cobra_execute_call", "internal/cli/root.go", 38) {
		t.Fatalf("imported Execute must remain activation evidence without registration or process reachability: %#v", root)
	}
	directMain := oneRecordWithBasis(
		t,
		byName["direct-main"],
		"build_selected_typed_cobra_activation",
	)
	if directMain.RegistrationSite != (Location{}) ||
		directMain.ProcessEntrypoint.ID != "example.com/typed-cobra.main" {
		t.Fatalf("direct main activation = %#v", directMain)
	}
	nested := oneRecordWithBasis(
		t,
		byName["nested-func"],
		"build_selected_typed_cobra_activation",
	)
	if nested.ProcessEntrypoint.ID != "" {
		t.Fatalf("nested function-literal activation inherited main reachability: %#v", nested)
	}
	for _, name := range []string{"put", "del"} {
		if len(byName[name]) != 1 ||
			byName[name][0].DiscoveryBasis != "build_selected_typed_cobra_descriptor" ||
			byName[name][0].RegistrationSite != (Location{}) ||
			!hasFrontier(byName[name][0].DynamicFrontier, "inventory_exact_binding_ambiguous") {
			t.Fatalf("multi-parent descriptor %q picked an arbitrary binding: %#v", name, byName[name])
		}
	}
	mutated := byName["before-dynamic"][0]
	if mutated.Identity.Path.Known ||
		!slices.Contains(mutated.Identity.Path.Candidates, "before-dynamic") ||
		!hasFrontier(mutated.DynamicFrontier, "cobra_descriptor_field_mutated") {
		t.Fatalf("separate descriptor mutation was interpreted or hidden: %#v", mutated)
	}
	for _, name := range []string{"put", "del"} {
		record := byName[name][0]
		if !record.Handler.Known ||
			hasFrontier(record.DynamicFrontier, "cobra_descriptor_field_mutated") {
			t.Fatalf("separate constructor instance mutated descriptor %q: %#v", name, record)
		}
	}
	for _, name := range []string{"only-a", "only-b"} {
		record := byName[name][0]
		if record.Identity.Path.Known ||
			record.ProcessEntrypoint.ID != "" ||
			record.ExecutableRole != ExecutableRoleUnknown ||
			record.OwningExecutable != "" ||
			!hasFrontier(record.DynamicFrontier, "inventory_command_path_unresolved") {
			t.Fatalf("multi-instance parent leaked activation into %q: %#v", name, record)
		}
	}
	afterLiteral := oneRecordWithBasis(
		t,
		byName["after-conditional-literal"],
		"build_selected_typed_cobra_binding",
	)
	if hasFrontier(afterLiteral.DynamicFrontier, "cobra_binding_unresolved") {
		t.Fatalf("function literal leaked conditional state into following binding: %#v", afterLiteral)
	}
	if result.Coverage.CobraDescriptorCount != len(cobraRecords) ||
		result.Coverage.CobraRecordCount != len(cobraRecords) ||
		result.Coverage.CobraDroppedRecordCount != 0 ||
		result.Coverage.CobraExactBindingCount == 0 ||
		result.Coverage.CobraExactActivationCount == 0 ||
		result.Coverage.CobraPartialRelationCount == 0 {
		t.Fatalf("Cobra inventory coverage is incomplete: %#v", result.Coverage)
	}

	for _, site := range []string{
		"internal/cli/unsupported.go:28:10",
		"internal/cli/root.go:12:16",
		"internal/cli/root.go:13:16",
		"internal/cli/shallow.go:5:23",
		"internal/cli/shallow.go:8:20",
	} {
		if descriptorSites[site] != 1 {
			t.Errorf("descriptor %q disappeared because it was not deeply reachable", site)
		}
	}
	for _, frontier := range []string{
		"cobra_binding_unresolved",
		"cobra_variadic_binding_unresolved",
		"cobra_activation_unresolved",
		"cobra_descriptor_field_mutated",
	} {
		if !hasFrontier(result.Coverage.UnsupportedDispatch, frontier) {
			t.Errorf("missing explicit shallow frontier %q: %#v",
				frontier, result.Coverage.UnsupportedDispatch)
		}
	}
}

func TestTypedCobraShallowInventoryIsDeterministic(t *testing.T) {
	options := DefaultOptions(filepath.Join("testdata", "cobra"))
	first, err := Analyze(options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(options)
	if err != nil {
		t.Fatal(err)
	}
	first.Coverage.ColdLatencyMillis = 0
	second.Coverage.ColdLatencyMillis = 0
	for index := range first.Coverage.Phases {
		first.Coverage.Phases[index].LatencyMillis = 0
	}
	for index := range second.Coverage.Phases {
		second.Coverage.Phases[index].LatencyMillis = 0
	}
	left, err := MarshalDeterministic(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := MarshalDeterministic(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("repeated shallow inventory differs\nfirst:\n%s\nsecond:\n%s", left, right)
	}
}

func TestInventoryProjectorNeverHidesDescriptors(t *testing.T) {
	location := func(line int) Location {
		return Location{Path: "commands.go", Line: line}
	}
	facts := inventoryFacts{
		Descriptors: []inventoryDescriptor{
			{ID: "a", Location: location(1)},
			{ID: "b", Location: location(2)},
			{ID: "c", Location: location(3)},
		},
		Bindings: []inventoryBinding{
			{
				ID: "exact", Exact: true,
				From: inventoryRef{DescriptorID: "a"},
				To:   inventoryRef{DescriptorID: "b"},
				Evidence: []Evidence{{
					ID: "binding-a-b", Kind: "binding", Location: location(4),
				}},
			},
			{
				ID: "exact-second", Exact: true,
				From: inventoryRef{DescriptorID: "c"},
				To:   inventoryRef{DescriptorID: "b"},
				Evidence: []Evidence{{
					ID: "binding-c-b", Kind: "binding", Location: location(5),
				}},
			},
			{
				ID:        "ambiguous",
				From:      inventoryRef{CandidateIDs: []string{"a", "c"}},
				To:        inventoryRef{DescriptorID: "c"},
				Frontiers: []Frontier{{Kind: "ambiguous"}},
			},
		},
		Activations: []inventoryActivation{{
			ID: "activation", Exact: true,
			Surface:  inventoryRef{DescriptorID: "b"},
			Location: location(6),
		}},
	}
	var contexts []inventoryProjectionContext
	records := projectInventory(facts, func(context inventoryProjectionContext) TriggerRecord {
		contexts = append(contexts, context)
		return TriggerRecord{ID: context.Descriptor.ID}
	})
	var ids []string
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, []string{"a", "b", "c"}) {
		t.Fatalf("projected descriptor ids = %v", ids)
	}
	for _, context := range contexts {
		if context.Descriptor.ID == "b" {
			if context.Activation == nil || context.Binding != nil {
				t.Fatalf("compatibility relation selection = %#v", context)
			}
			if len(context.RelatedEvidence) != 2 ||
				!hasFrontier(context.Frontiers, "inventory_exact_binding_ambiguous") {
				t.Fatalf("additional exact relations were dropped: %#v", context)
			}
		}
		if context.Descriptor.ID == "c" &&
			!hasFrontier(context.Frontiers, "ambiguous") {
			t.Fatalf("ambiguous fact was dropped: %#v", context)
		}
	}
}

func TestCobraPublicationLimitsRemainVisible(t *testing.T) {
	a := &analyzer{ctx: context.Background()}
	d := newCobraDiscovery(a, cobraLimits{
		commands: 1, recordBytes: 1, diagnostics: 1,
	})
	d.descriptorList = []*cobraDescriptor{
		{
			location: Location{Path: "a.go", Line: 1},
			identity: knownValue("command_segment", "a"),
			handler:  dynamicValue("none"), inventoryID: "a",
		},
		{
			location: Location{Path: "b.go", Line: 1},
			identity: knownValue("command_segment", "b"),
			handler:  dynamicValue("none"), inventoryID: "b",
		},
	}
	d.resolveAndPublish()
	for _, budget := range []string{"cobra_commands", "cobra_record_bytes"} {
		if !slices.Contains(a.result.Coverage.BudgetsReached, budget) {
			t.Errorf("missing visible budget %q in %v", budget, a.result.Coverage.BudgetsReached)
		}
	}
}

func oneRecordWithBasis(
	t *testing.T,
	records []TriggerRecord,
	basis string,
) TriggerRecord {
	t.Helper()
	for _, record := range records {
		if record.DiscoveryBasis == basis {
			return record
		}
	}
	t.Fatalf("no %q record in %#v", basis, records)
	return TriggerRecord{}
}

func hasEvidenceAt(
	evidence []Evidence,
	kind string,
	path string,
	line int,
) bool {
	for _, item := range evidence {
		if item.Kind == kind && item.Location.Path == path &&
			item.Location.Line == line {
			return true
		}
	}
	return false
}

func recordNames(records []TriggerRecord) []string {
	var result []string
	for _, record := range records {
		if strings.TrimSpace(record.Identity.Name) != "" {
			result = append(result, record.Identity.Name)
		}
	}
	sort.Strings(result)
	return result
}
