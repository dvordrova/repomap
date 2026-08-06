package navigator

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestCompileProductBuildsFixedPermutationStableStartupQuestion(t *testing.T) {
	atlas := twoUnitAtlas()
	first, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileProduct(ProductInput{Atlas: reverseAtlas(atlas), Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	firstCompiled, ok := first.CompiledRequest()
	if !ok || first.Empty() {
		t.Fatal("eligible startup Atlas unexpectedly produced an empty Product")
	}
	secondCompiled, ok := second.CompiledRequest()
	if !ok {
		t.Fatal("permuted Atlas unexpectedly produced an empty Product")
	}
	if first.AtlasSHA256() != second.AtlasSHA256() ||
		firstCompiled.WireSHA256() != secondCompiled.WireSHA256() ||
		firstCompiled.CatalogSHA256() != secondCompiled.CatalogSHA256() ||
		!reflect.DeepEqual(first.Actions(), second.Actions()) {
		t.Fatal("Atlas permutation changed Product identity")
	}

	var wire wireProjection
	if err := json.Unmarshal(firstCompiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Question != ProductQuestion || wire.ScopeRef == "" || len(wire.SeedRefs) != 2 ||
		len(wire.DirectTrails) != 2 || len(wire.Actions) != 2 || len(first.Actions()) != 2 {
		t.Fatalf("Product wire shape = %#v; actions = %#v", wire, first.Actions())
	}
	for _, action := range wire.Actions {
		if action.Operation != StartupActionOperation || !wireHasStartupTrailForTarget(wire, action.TargetRef) {
			t.Fatalf("wire action is not backed by an exact startup trail: %#v", action)
		}
	}
	for _, forbidden := range []string{
		"surface-a-canonical", "operation-a-canonical", "relation-a-canonical",
		"evidence-a-canonical", "cmd/a/main.go", "source_signals", "file_tree", "internal_edges",
	} {
		if strings.Contains(string(firstCompiled.WireJSON()), forbidden) {
			t.Fatalf("Product wire leaked %q: %s", forbidden, firstCompiled.WireJSON())
		}
	}
}

func TestCompileProductUsesOnlySameAppOwnedResolvedStartupRelations(t *testing.T) {
	atlas := twoUnitAtlas()
	// This remains a valid repository-scoped relation, but it no longer encodes
	// the exact app-owned vertical emitted for an available process entry.
	atlas.Relations[0].UnitID = "module-a-canonical"
	// A resolved relation in another phase is also outside the fixed question.
	atlas.Relations[1].Phase = repositoryatlas.PhaseRuntime
	product, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if !product.Empty() || len(product.Actions()) != 0 {
		t.Fatalf("non-eligible Atlas relations produced actions: %#v", product.Actions())
	}
	if _, ok := product.CompiledRequest(); ok {
		t.Fatal("empty Product exposed a provider request")
	}

	atlas = twoUnitAtlas()
	atlas.Relations[0].Authority = repositoryatlas.AuthorityInferred
	product, err = CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	actions := product.Actions()
	if len(actions) != 1 || actions[0].RelationID != "relation-b-canonical" {
		t.Fatalf("eligible relation catalog = %#v", actions)
	}
}

func TestCompileProductZeroEligibleIsExplicitLocalEmpty(t *testing.T) {
	atlas, _ := manyProcessAtlas(0)
	product, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if !product.Empty() {
		t.Fatal("zero eligible relations did not produce empty Product")
	}
	record, err := product.EmptyRecord()
	if err != nil {
		t.Fatal(err)
	}
	if record.State != ProductStateEmpty || len(record.Actions) != 0 || record.Selected != nil {
		t.Fatalf("empty record = %#v", record)
	}
	if _, err := product.RequestRecord(); err == nil {
		t.Fatal("empty Product produced a provider request record")
	}
	if status := product.PreparedStatus(); status.State != ProductStateEmpty || status.ActionCount != 0 {
		t.Fatalf("empty status = %#v", status)
	}
}

func TestResolveRecommendationRestoresOneExactBackendAction(t *testing.T) {
	product, err := CompileProduct(ProductInput{Atlas: twoUnitAtlas(), Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	compiled, _ := product.CompiledRequest()
	valid := validProductResponse(t, compiled, 0)
	record, err := product.ResolveRecommendation(mustJSON(t, valid))
	if err != nil {
		t.Fatal(err)
	}
	if record.State != ProductStateSelected || record.Selected == nil ||
		record.Selected.Operation != StartupActionOperation ||
		record.Selected.Surface.Kind != repositoryatlas.EntitySurface ||
		record.Selected.Application.Kind != repositoryatlas.EntityOperation ||
		record.Selected.RelationID == "" || len(record.Selected.EvidenceIDs) == 0 {
		t.Fatalf("selected recommendation = %#v", record)
	}
	if err := product.ValidateRecommendationRecord(record); err != nil {
		t.Fatal(err)
	}
	// Decision 232 (Archive 9): a permutation of the advertised action
	// catalog must not alter the resolved record — the action key binds the
	// exact backend action regardless of catalog order.
	permutedInput := ProductInput{Atlas: twoUnitAtlas(), Limits: generousLimits()}
	permutedProduct, err := CompileProduct(permutedInput)
	if err != nil {
		t.Fatal(err)
	}
	permutedCompiled, _ := permutedProduct.CompiledRequest()
	permutedValid := validProductResponse(t, permutedCompiled, 0)
	permutedRecord, err := permutedProduct.ResolveRecommendation(mustJSON(t, permutedValid))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.Selected, permutedRecord.Selected) {
		t.Fatalf("catalog permutation changed the resolved record: %#v vs %#v", record.Selected, permutedRecord.Selected)
	}

	other := validProductResponse(t, compiled, 1)
	tests := []struct {
		name   string
		mutate func(*responseEnvelope)
		want   string
	}{
		{name: "zero action", mutate: func(value *responseEnvelope) { value.ActionRefs = nil }, want: "exactly one"},
		{name: "multiple actions", mutate: func(value *responseEnvelope) { value.ActionRefs = append(value.ActionRefs, other.ActionRefs[0]) }, want: "exactly one"},
		{name: "echo forbidden", mutate: func(value *responseEnvelope) {
			value.EntityRefs = []string{"s1"}
		}, want: "must not echo"},
		{name: "raw canonical ref", mutate: func(value *responseEnvelope) { value.ActionRefs = []string{record.Selected.Key} }, want: "raw_canonical_ref"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneResponseEnvelope(valid)
			test.mutate(&candidate)
			_, err := product.ResolveRecommendation(mustJSON(t, candidate))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveRecommendation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileProductPropagatesExplicitResourceFailure(t *testing.T) {
	limits := generousLimits()
	limits.MaxActions = 1
	_, err := CompileProduct(ProductInput{Atlas: twoUnitAtlas(), Limits: limits})
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Section != "actions" {
		t.Fatalf("action resource error = %v", err)
	}

	limits = generousLimits()
	limits.MaxWireBytes = 32
	_, err = CompileProduct(ProductInput{Atlas: twoUnitAtlas(), Limits: limits})
	if !errors.As(err, &limitErr) || limitErr.Section != "wire_bytes" {
		t.Fatalf("wire resource error = %v", err)
	}
}

func validProductResponse(t *testing.T, compiled Compiled, actionIndex int) responseEnvelope {
	t.Helper()
	var wire wireProjection
	if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if actionIndex < 0 || actionIndex >= len(wire.Actions) {
		t.Fatalf("action index %d outside %d actions", actionIndex, len(wire.Actions))
	}
	// Decision 232 (Navigator v2): the response selects the action only;
	// trail/endpoints/evidence are backend-restored.
	return responseEnvelope{
		Version: Version, CatalogRef: compiled.CatalogRef(),
		ActionRefs: []string{wire.Actions[actionIndex].Ref},
	}
}

func cloneResponseEnvelope(value responseEnvelope) responseEnvelope {
	value.EntityRefs = slices.Clone(value.EntityRefs)
	value.TrailRefs = slices.Clone(value.TrailRefs)
	value.IntersectionRefs = slices.Clone(value.IntersectionRefs)
	value.EvidenceRefs = slices.Clone(value.EvidenceRefs)
	value.GapRefs = slices.Clone(value.GapRefs)
	value.ActionRefs = slices.Clone(value.ActionRefs)
	return value
}

func wireHasStartupTrailForTarget(wire wireProjection, targetRef string) bool {
	for _, trail := range wire.DirectTrails {
		if trail.SourceRef == targetRef && trail.Kind == repositoryatlas.RelationExposes &&
			trail.Phase == repositoryatlas.PhaseStartup && trail.Authority == repositoryatlas.AuthorityResolved {
			return true
		}
	}
	return false
}
