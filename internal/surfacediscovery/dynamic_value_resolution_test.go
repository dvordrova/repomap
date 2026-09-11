package surfacediscovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func resolutionTestAnalyzer(t testing.TB, source string) (*analyzer, *ssa.Package) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "/fixture/flow.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return resolutionAnalyzerFromFiles(t, fset, []*ast.File{file})
}

func resolutionAnalyzerFromFiles(t testing.TB, fset *token.FileSet, files []*ast.File) (*analyzer, *ssa.Package) {
	t.Helper()
	const pkgPath = "example.com/fixture"
	pkg, _, err := ssautil.BuildPackage(&types.Config{Importer: importer.Default()}, fset,
		types.NewPackage(pkgPath, files[0].Name.Name), files, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	scenario := Scenario{ID: "go:linux/amd64:tags=", GOOS: "linux", GOARCH: "amd64"}
	a := &analyzer{ctx: context.Background(), root: "/fixture", program: pkg.Prog, packages: []*ssa.Package{pkg}, scenario: scenario,
		admittedPackages: map[string]bool{pkgPath: true}, modulePaths: map[string]bool{pkgPath: true},
		functionIDs:           make(map[*ssa.Function]string),
		packageFacts:          map[string]*packages.Package{pkgPath: {Module: &packages.Module{Path: pkgPath, Dir: "/fixture"}}},
		directCallIndex:       newDirectCallIndexBuilder(scenario, 0),
		dynamicHandoffCapture: &dynamicHandoffCapture{enabled: true}}
	var functions []*ssa.Function
	for function := range ssautil.AllFunctions(pkg.Prog) {
		if a.isRepositoryFunction(function) {
			functions = append(functions, function)
			a.directCallIndex.recordFunction(a, function)
		}
	}
	a.dynamicHandoffCapture.collectInterfaceFieldStores(a, functions)
	return a, pkg
}

func TestDynamicValueCanonicalEquivalenceOnCumulativeFixture(t *testing.T) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range []string{"fixtures.go", "handoff_flow.go"} {
		path := filepath.Join("..", "..", "testdata", "repositories", "go", "internal", "storefixture", name)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, "/fixture/"+name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	a, pkg := resolutionAnalyzerFromFiles(t, fset, files)
	actual, original, shapes := compareResolutionIndexes(t, a, pkg)
	if !bytes.Equal(actual, original) {
		t.Fatalf("canonical handoff index differs from the original walker\nactual: %s\noriginal: %s", actual, original)
	}
	for _, required := range []string{"SharedCallbackFlow", "CyclicCallbackFlow", "InvokeInterfaceFlows", "Put", "RegisterServices", "BuildActionPair"} {
		if shapes[required] == 0 {
			t.Errorf("fixture did not exercise %s", required)
		}
	}
}

// Both paths use the same native caller, slot, location and function catalogue;
// only candidate resolution changes. New seals and validates all accounting,
// source sets, evidence and the digest, rather than comparing counts alone.
func compareResolutionIndexes(t testing.TB, a *analyzer, pkg *ssa.Package) ([]byte, []byte, map[string]int) {
	t.Helper()
	var current, original []godynamichandoff.Handoff
	shapes := make(map[string]int)
	for function := range ssautil.AllFunctions(pkg.Prog) {
		if !a.isRepositoryFunction(function) || function.Synthetic != "" {
			continue
		}
		callerID, ok := a.directCallIndex.recordFunction(a, function)
		if !ok {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if store, ok := instruction.(*ssa.Store); ok {
					before := len(a.dynamicHandoffCapture.callableBindings)
					a.dynamicHandoffCapture.observeCallableBinding(a, store)
					if len(a.dynamicHandoffCapture.callableBindings) > before {
						base := a.dynamicHandoffCapture.callableBindings[before].handoff
						appendResolutionPair(t, a, base, dynamicCallbackArgument{value: store.Val, signature: base.Slot.Signature}, &current, &original)
						shapes[function.Name()]++
					}
				}
				call, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				common := call.Common()
				base := godynamichandoff.Handoff{CallerID: callerID, Invocation: dynamicInvocation(call), Callsite: dynamicLocation(a.location(call.Pos()))}
				var values []dynamicCallbackArgument
				if common.IsInvoke() {
					base.Kind = godynamichandoff.InterfaceInvoke
					values = []dynamicCallbackArgument{{value: common.Value, method: common.Method, signature: types.TypeString(common.Signature(), packageQualifier)}}
				} else if common.StaticCallee() == nil {
					if _, builtin := common.Value.(*ssa.Builtin); !builtin {
						base.Kind = godynamichandoff.FunctionValueCall
						values = []dynamicCallbackArgument{{value: common.Value, signature: types.TypeString(common.Signature(), packageQualifier)}}
					}
				}
				for _, value := range values {
					appendResolutionPair(t, a, base, value, &current, &original)
					shapes[function.Name()]++
				}
				if target, ok := dynamicCallbackStaticTarget(a, common); ok {
					base.Kind, base.StaticTarget = godynamichandoff.CallbackTransfer, target
					for _, value := range append(dynamicCallbackArguments(common), dynamicInterfaceArguments(common)...) {
						appendResolutionPair(t, a, base, value, &current, &original)
						shapes[function.Name()]++
					}
				}
			}
		}
	}
	direct := a.directCallIndex.finish()
	seal := func(handoffs []godynamichandoff.Handoff) []byte {
		capture := &dynamicHandoffCapture{handoffs: handoffs}
		index, err := capture.finish(direct, callableBindingSnapshot{})
		if err != nil {
			t.Fatal(err)
		}
		if err := index.Validate(); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(index)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return seal(current), seal(original), shapes
}

func appendResolutionPair(t testing.TB, a *analyzer, base godynamichandoff.Handoff, value dynamicCallbackArgument, current, original *[]godynamichandoff.Handoff) {
	t.Helper()
	base.Slot.Signature, base.Slot.Parameter = value.signature, value.parameter
	var candidates, reference []godynamichandoff.Candidate
	var unknown, referenceUnknown int
	var err error
	if value.method != nil {
		base.Slot.DeclaredType, base.Slot.Method = types.TypeString(value.value.Type(), packageQualifier), value.method.Name()
		candidates, unknown, err = dynamicInterfaceCandidates(a, value.value, value.method)
		reference, referenceUnknown = referenceDynamicInterfaceCandidates(a, value.value, value.method)
	} else {
		candidates, unknown, err = dynamicFunctionCandidates(a, value.value)
		reference, referenceUnknown = referenceDynamicFunctionCandidates(a, value.value)
	}
	if err != nil {
		t.Fatal(err)
	}
	for i, result := range []struct {
		candidates []godynamichandoff.Candidate
		unknown    int
	}{{candidates, unknown}, {reference, referenceUnknown}} {
		handoff := base
		handoff.Candidates, handoff.CandidatesConsidered = result.candidates, len(result.candidates)+result.unknown
		handoff.Resolution = dynamicResolution(result.candidates, result.unknown)
		if i == 0 {
			*current = append(*current, handoff)
		} else {
			*original = append(*original, handoff)
		}
	}
}

func TestDynamicValueCycleContextAndEvidence(t *testing.T) {
	f, g := &ssa.Function{}, &ssa.Function{}
	a, b := &ssa.Phi{}, &ssa.Phi{}
	a.Edges, b.Edges = []ssa.Value{b, f}, []ssa.Value{a, g}
	r := newDynamicValueResolver(nil, nil)
	r.active[b] = true
	blocked := r.functionValue(a, false)
	delete(r.active, b)
	fresh := r.functionValue(a, false)
	if len(blocked.functions) != 1 || len(fresh.functions) != 2 || !blocked.cyclic || !fresh.cyclic {
		t.Fatalf("cycle context was cached: blocked=%+v fresh=%+v", blocked, fresh)
	}
	for _, active := range []map[ssa.Value]bool{{b: true}, {}} {
		want := make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
		unknown := referenceResolveDynamicFunctionValue(a, want, active, false)
		got := fresh
		if active[b] {
			got = blocked
		}
		if unknown != got.unresolved || !reflect.DeepEqual(want, got.functions) {
			t.Fatal("cycle summary changed original evidence or path counts")
		}
	}
	if r.functionValue(f, false).functions[f] != godynamichandoff.EvidenceDirectFunctionValue ||
		r.functionValue(f, true).functions[f] != godynamichandoff.EvidenceUniqueValueFlow {
		t.Fatal("throughFlow was not retained in memo identity")
	}
	// A direct function and a closure can resolve to the same function. Merge
	// must retain the last original observation, not promote the stronger one.
	closure := &ssa.MakeClosure{Fn: f}
	for _, values := range [][]ssa.Value{{f, closure}, {closure, f}} {
		got := dynamicValueSummary{}
		want := make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
		for _, value := range values {
			r.merge(&got, r.functionValue(value, false))
			referenceResolveDynamicFunctionValue(value, want, make(map[ssa.Value]bool), false)
		}
		if !reflect.DeepEqual(got.functions, want) {
			t.Fatal("summary merge changed last-write evidence")
		}
	}
}

func TestDynamicValueOverflowIsOwningFailure(t *testing.T) {
	var value ssa.Value = &ssa.Parameter{}
	for i := 0; i < strconv.IntSize-1; i++ {
		value = &ssa.Phi{Edges: []ssa.Value{value, value}}
	}
	if _, err := resolveDynamicFunctionValue(value); err == nil || !strings.Contains(err.Error(), "overflows int") {
		t.Fatalf("overflow = %v", err)
	}
	if err := checkDynamicCandidateCount(1, int(^uint(0)>>1)); err == nil {
		t.Fatal("final candidate count overflow was accepted")
	}
	// Use a real call instruction so the owning capture must retain the error
	// and refuse finish, rather than returning a smaller/partial handoff.
	a, pkg := resolutionTestAnalyzer(t, "package fixture\nfunc Run(f func()){f()}")
	call := lastResolutionCall(pkg)
	call.Common().Value = value
	a.dynamicHandoffCapture.observeFunctionValueCall(a, call, "caller", Location{Path: "flow.go", Line: 2, Column: 20})
	if a.dynamicHandoffCapture.err == nil || len(a.dynamicHandoffCapture.handoffs) != 0 {
		t.Fatal("overflow did not stop the owning capture")
	}
	if _, err := a.dynamicHandoffCapture.finish(DirectCallIndex{}, callableBindingSnapshot{}); err == nil {
		t.Fatal("overflowing capture published an index")
	}
}

func TestDynamicValueExternalCaptureKeepsOverflow(t *testing.T) {
	for _, invoke := range []bool{false, true} {
		t.Run(fmt.Sprintf("interface=%t", invoke), func(t *testing.T) {
			source := "package fixture\nimport \"time\"\ntype A func();type B func();type C func();func Run(f A,flag bool){"
			if invoke {
				source = "package fixture\nimport \"testing\"\ntype A func();type B func();type C func();func Run(tb testing.TB,f A,flag bool){"
			}
			source += strings.Repeat("if flag{f=A(B(f))}else{f=A(C(f))};", strconv.IntSize-1)
			if invoke {
				source += "tb.Cleanup(f)}"
			} else {
				source += "time.AfterFunc(0,f)}"
			}
			a, pkg := resolutionTestAnalyzer(t, source)
			var err error
			a.externalCallIndex, err = newAnalyzerExternalCallIndexBuilder(a)
			if err != nil {
				t.Fatal(err)
			}
			a.observeExternalCallIndex(lastResolutionCall(pkg))
			if a.externalCallIndexErr == nil || !strings.Contains(a.externalCallIndexErr.Error(), "overflows int") {
				t.Fatalf("owning observer overwrote overflow: %v", a.externalCallIndexErr)
			}
			if len(a.externalCallIndex.familyByID) != 0 {
				t.Fatal("AddWitness accepted a partial overflowing pattern")
			}
		})
	}
}

func TestDynamicValueSharedStoreSummaryIsImmutable(t *testing.T) {
	a, pkg := resolutionTestAnalyzer(t, `package fixture
type I interface{M()};type W struct{};func(W)M(){};type H struct{V I}
func Store(a,b *H){
 x:=I(W{})
 a.V=x
 b.V=x
}
func Run(h *H){h.V.M()}`)
	call := lastResolutionCall(pkg).Common()
	r := newDynamicValueResolver(a, call.Method)
	result := r.interfaceValue(call.Value)
	if r.err != nil || result.unresolved != 1 || len(result.functions) != 1 {
		t.Fatalf("field summary = %+v, %v", result, r.err)
	}
	for function := range result.functions {
		if len(result.assignments[function]) != 2 {
			t.Fatal("shared SSA value lost an assignment source")
		}
	}
	for _, stores := range a.dynamicHandoffCapture.interfaceFields {
		if len(stores) != 2 || stores[0].Val != stores[1].Val {
			t.Fatal("fixture no longer stores the same SSA value twice")
		}
		child := r.interfaceValue(stores[0].Val)
		if len(child.assignments) != 0 || child.unresolved != 0 || len(child.functions) != 1 {
			t.Fatal("parent store locations contaminated the cached value")
		}
	}
}

func lastResolutionCall(pkg *ssa.Package) *ssa.Call {
	var call *ssa.Call
	for _, block := range pkg.Func("Run").Blocks {
		for _, instruction := range block.Instrs {
			if current, ok := instruction.(*ssa.Call); ok {
				call = current
			}
		}
	}
	return call
}

func resolutionDiamondSource(interfaceValue, known bool, depth int) string {
	if !interfaceValue {
		source := "package fixture\ntype A func();type B func();type C func();func Target(){}\n"
		if known {
			source += "func Run(flag bool){f:=A(Target);"
		} else {
			source += "func Run(f A,flag bool){"
		}
		source += strings.Repeat("if flag {f=A(B(f))}else{f=A(C(f))};", depth)
		return source + "f()}"
	}
	source := "package fixture\ntype A interface{M();N();O();P()};type W struct{};func(W)M(){};func(W)N(){};func(W)O(){};func(W)P(){};func Sink(A){}\n"
	if known {
		source += "func I0(x A,flag bool)A{return W{}}\n"
	} else {
		source += "func I0(x A,flag bool)A{return x}\n"
	}
	for i := 1; i <= depth; i++ {
		source += fmt.Sprintf("func I%d(x A,flag bool)A{if flag{return I%d(x,flag)};return I%d(x,flag)}\n", i, i-1, i-1)
	}
	return source + fmt.Sprintf("func Run(x A,flag bool){Sink(I%d(x,flag))}", depth)
}

func TestDynamicValueDiamondsPreserveCanonicalIndexes(t *testing.T) {
	for _, isInterface := range []bool{false, true} {
		for _, known := range []bool{false, true} {
			t.Run(fmt.Sprintf("interface=%t/known=%t", isInterface, known), func(t *testing.T) {
				a, pkg := resolutionTestAnalyzer(t, resolutionDiamondSource(isInterface, known, 8))
				got, want, _ := compareResolutionIndexes(t, a, pkg)
				if !bytes.Equal(got, want) {
					t.Fatal("DAG changed canonical handoff bytes/digest")
				}
			})
		}
	}
}

func BenchmarkDynamicValueDiamonds(b *testing.B) {
	for _, isInterface := range []bool{false, true} {
		for _, known := range []bool{false, true} {
			for _, depth := range []int{8, 12, 16} {
				a, pkg := resolutionTestAnalyzer(b, resolutionDiamondSource(isInterface, known, depth))
				for _, original := range []bool{true, false} {
					b.Run(fmt.Sprintf("interface=%t/known=%t/depth=%d/original=%t", isInterface, known, depth, original), func(b *testing.B) {
						b.ReportAllocs()
						for i := 0; i < b.N; i++ {
							if isInterface {
								for _, argument := range dynamicInterfaceArguments(lastResolutionCall(pkg).Common()) {
									if original {
										referenceResolveDynamicInterfaceValue(a, argument.value, argument.method, make(map[*ssa.Function]struct{}), make(map[*ssa.Function][]godynamichandoff.Location), make(map[ssa.Value]bool))
									} else if _, err := resolveDynamicInterfaceValue(a, argument.value, argument.method); err != nil {
										b.Fatal(err)
									}
								}
							} else if original {
								referenceResolveDynamicFunctionValue(lastResolutionCall(pkg).Common().Value, make(map[*ssa.Function]godynamichandoff.CandidateEvidence), make(map[ssa.Value]bool), false)
							} else if _, err := resolveDynamicFunctionValue(lastResolutionCall(pkg).Common().Value); err != nil {
								b.Fatal(err)
							}
						}
						runtime.KeepAlive(pkg)
					})
				}
			}
		}
	}
}

func TestMethodValueHandlersResolveToTheirMethods(t *testing.T) {
	a, pkg := resolutionTestAnalyzer(t, `package fixture
type Server struct{}
func (s *Server) handleList(w int)   {}
func (s *Server) handleCreate(w int) {}
type Handler func(int)
func register(path string, handler Handler) {}
func main() {
	s := &Server{}
	register("/items", s.handleList)
	register("/items/create", Handler(s.handleCreate))
}`)
	want := map[string]string{"/items": "handleList", "/items/create": "handleCreate"}
	seen := 0
	for _, block := range pkg.Func("main").Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || call.Common().StaticCallee() == nil || call.Common().StaticCallee().Name() != "register" {
				continue
			}
			path := strings.Trim(call.Common().Args[0].(*ssa.Const).Value.String(), `"`)
			candidates, unresolved, err := dynamicFunctionCandidateFacts(a, call.Common().Args[1])
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 1 || unresolved != 0 || candidates[0].function.Name() != want[path] || candidates[0].function.Synthetic != "" {
				t.Fatalf("%s: method value did not resolve to its method: %+v unresolved=%d", path, candidates, unresolved)
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("registrations seen: %d", seen)
	}
}
