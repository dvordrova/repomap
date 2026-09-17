package surfacediscovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
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
		allFunctions:          make(map[*ssa.Function]bool),
		functionIDs:           make(map[*ssa.Function]string),
		packageFacts:          map[string]*packages.Package{pkgPath: {Module: &packages.Module{Path: pkgPath, Dir: "/fixture"}}},
		directCallIndex:       newDirectCallIndexBuilder(scenario, 0),
		dynamicHandoffCapture: &dynamicHandoffCapture{enabled: true}}
	var functions []*ssa.Function
	for function := range ssautil.AllFunctions(pkg.Prog) {
		if a.isRepositoryFunction(function) {
			a.allFunctions[function] = true
			functions = append(functions, function)
			a.directCallIndex.recordFunction(a, function)
		}
	}
	a.dynamicHandoffCapture.collectInterfaceFieldStores(a, functions)
	return a, pkg
}

func TestInterfaceConstructorParameterUsesActualRepositoryArguments(t *testing.T) {
	a, pkg := resolutionTestAnalyzer(t, `package fixture
type Repository interface{ Get(int) }
type Postgres struct{}
func (*Postgres) Get(int) {}
type Service struct{ repository Repository }
func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (service *Service) Get(id int) { service.repository.Get(id) }
func main() { NewService(&Postgres{}).Get(1) }
`)
	var invocation *ssa.Call
	for function := range ssautil.AllFunctions(pkg.Prog) {
		if function.Name() != "Get" || function.Signature.Recv() == nil ||
			!strings.Contains(function.Signature.Recv().Type().String(), "Service") {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if ok && call.Common().IsInvoke() {
					invocation = call
				}
			}
		}
	}
	if invocation == nil {
		t.Fatal("fixture interface invocation is absent")
	}
	candidates, unresolved, err := dynamicInterfaceCandidates(a, invocation.Common().Value, invocation.Common().Method)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || unresolved != 0 {
		t.Fatalf("constructor-injected implementation = %+v unresolved=%d", candidates, unresolved)
	}
	target := ""
	for function, functionID := range a.directCallIndex.functionNode {
		if functionID == candidates[0].FunctionID && strings.Contains(function.String(), "Postgres") {
			target = function.String()
		}
	}
	if !strings.Contains(target, ".Postgres).Get") {
		t.Fatalf("constructor-injected target = %q, want (*Postgres).Get", target)
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
	if r.functionValue(f, false).functions[f] != godynamichandoff.EvidenceDirectFunctionValue ||
		r.functionValue(f, true).functions[f] != godynamichandoff.EvidenceUniqueValueFlow {
		t.Fatal("throughFlow was not retained in memo identity")
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
	if r.err != nil || result.unresolved != 0 || len(result.functions) != 1 {
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

func BenchmarkDynamicValueDiamonds(b *testing.B) {
	for _, isInterface := range []bool{false, true} {
		for _, known := range []bool{false, true} {
			for _, depth := range []int{8, 12, 16} {
				a, pkg := resolutionTestAnalyzer(b, resolutionDiamondSource(isInterface, known, depth))
				b.Run(fmt.Sprintf("interface=%t/known=%t/depth=%d", isInterface, known, depth), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if isInterface {
							for _, argument := range dynamicInterfaceArguments(lastResolutionCall(pkg).Common()) {
								if _, err := resolveDynamicInterfaceValue(a, argument.value, argument.method); err != nil {
									b.Fatal(err)
								}
							}
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
