package sourcewindowfacts

import (
	"slices"
	"strings"
	"testing"
)

func TestExtractGoFunctionBuildsBoundedSyntaxObservations(t *testing.T) {
	window, err := NewWindow("evidence-route", "router.go", 10, []string{
		"func route(r *Request) error {",
		"\tcurrent := r.Path",
		"\tif current == \"\" {",
		"\t\treturn missing()",
		"\t}",
		"\thandler := lookup(current)",
		"\thandler.Serve()",
		"\treturn nil",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	function, err := ExtractGoFunction(window, "route")
	if err != nil {
		t.Fatal(err)
	}
	if function.Symbol != "route" || function.StartLine != 10 || function.EndLine != 18 ||
		function.Partial || len(function.ContentSHA256) != 64 {
		t.Fatalf("ExtractGoFunction() = %#v", function)
	}
	if err := function.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []ObservationKind{
		ObservationDeclaration,
		ObservationDirectCall,
		ObservationBranch,
		ObservationAssignment,
		ObservationRead,
		ObservationReturn,
	} {
		if !hasObservationKind(function.Observations, kind) {
			t.Errorf("missing observation kind %q: %#v", kind, function.Observations)
		}
	}
	for _, target := range []string{"missing", "lookup", "handler.Serve"} {
		if !hasCallTarget(function.Observations, target) {
			t.Errorf("missing call target %q: %#v", target, function.Observations)
		}
	}
	if !hasObservationObject(function.Observations, ObservationAssignment, "current") ||
		!hasObservationObject(function.Observations, ObservationRead, "r.Path") {
		t.Fatalf("assignment/read observations are incomplete: %#v", function.Observations)
	}
}

func TestExtractGoFunctionParsesCompleteContextReset(t *testing.T) {
	window, err := NewWindow("evidence-context", "context.go", 78, []string{
		"\tmethodNotAllowed bool",
		"}",
		"",
		"// Reset a routing context to its initial state.",
		"func (x *Context) Reset() {",
		"\tx.Routes = nil",
		"\tx.RoutePath = \"\"",
		"\tx.URLParams.Keys = x.URLParams.Keys[:0]",
		"\tx.parentCtx = nil",
		"}",
		"",
		"func (x *Context) Next() string {",
		"\treturn x.RoutePath",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	function, err := ExtractGoFunction(window, "(*Context).Reset")
	if err != nil {
		t.Fatal(err)
	}
	if function.Symbol != "Context.Reset" || function.Partial ||
		function.StartLine != 82 || function.EndLine != 87 {
		t.Fatalf("ExtractGoFunction() = %#v", function)
	}
	for _, target := range []string{"x.Routes", "x.RoutePath", "x.URLParams.Keys", "x.parentCtx"} {
		if !hasObservationObject(function.Observations, ObservationAssignment, target) {
			t.Errorf("missing assignment target %q: %#v", target, function.Observations)
		}
	}
}

func TestExtractGoFunctionClosesTrailingPartialBodyOnlyForParsing(t *testing.T) {
	lines := []string{
		"func (mx *Mux) ServeHTTP(w ResponseWriter, r *Request) {",
		"\tif mx.handler == nil {",
		"\t\tmx.NotFoundHandler().ServeHTTP(w, r)",
		"\t\treturn",
		"\t}",
		"\trctx := mx.pool.Get()",
		"\trctx.Reset()",
		"\tmx.handler.ServeHTTP(w, r)",
		"\tmx.pool.Put(rctx)",
	}
	window, err := NewWindow("evidence-serve-http", "mux.go", 52, lines)
	if err != nil {
		t.Fatal(err)
	}
	function, err := ExtractGoFunction(window, "(*Mux).ServeHTTP")
	if err != nil {
		t.Fatal(err)
	}
	if function.Symbol != "Mux.ServeHTTP" || !function.Partial ||
		function.StartLine != 52 || function.EndLine != 60 {
		t.Fatalf("ExtractGoFunction() = %#v", function)
	}
	if !slices.Equal(function.Lines, lines) || strings.Contains(strings.Join(function.Lines, "\n"), "\n}") {
		t.Fatalf("partial function exposed synthetic source: %#v", function.Lines)
	}
	for _, target := range []string{
		"mx.NotFoundHandler",
		"mx.NotFoundHandler().ServeHTTP",
		"mx.pool.Get",
		"rctx.Reset",
		"mx.handler.ServeHTTP",
		"mx.pool.Put",
	} {
		if !hasCallTarget(function.Observations, target) {
			t.Errorf("missing partial call target %q: %#v", target, function.Observations)
		}
	}
	for _, observation := range function.Observations {
		if observation.Line < function.StartLine || observation.EndLine > function.EndLine {
			t.Fatalf("observation escaped visible range: %#v", observation)
		}
	}
}

func TestExtractGoFunctionsReturnsDeclarationsInSourceOrder(t *testing.T) {
	window, err := NewWindow("evidence-functions", "router.go", 20, []string{
		"type Router struct{}",
		"",
		"func prepare() error {",
		"\treturn nil",
		"}",
		"",
		"func (router *Router) ServeHTTP() {",
		"\tprepare()",
		"}",
		"",
		"func finish() {}",
	})
	if err != nil {
		t.Fatal(err)
	}
	functions, err := ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := functionSymbols(functions), []string{"prepare", "Router.ServeHTTP", "finish"}; !slices.Equal(got, want) {
		t.Fatalf("ExtractGoFunctions() symbols = %v, want %v", got, want)
	}
	if got, want := functionStartLines(functions), []int{22, 26, 30}; !slices.Equal(got, want) {
		t.Fatalf("ExtractGoFunctions() start lines = %v, want %v", got, want)
	}
	for _, function := range functions {
		if err := function.Validate(); err != nil {
			t.Fatalf("function %q is invalid: %v", function.Symbol, err)
		}
	}
}

func TestExtractGoFunctionsDeduplicatesCanonicalSymbol(t *testing.T) {
	window, err := NewWindow("evidence-duplicate", "router.go", 1, []string{
		"func route() {",
		"\tfirst()",
		"}",
		"",
		"func route() {",
		"\tsecond()",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	functions, err := ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 1 || functions[0].Symbol != "route" ||
		functions[0].StartLine != 1 || !hasCallTarget(functions[0].Observations, "first") {
		t.Fatalf("ExtractGoFunctions() = %#v, want earliest canonical route declaration", functions)
	}
}

func TestExtractGoFunctionsRetainsTrailingPartialFunction(t *testing.T) {
	partialLines := []string{
		"func (router *Router) dispatch() error {",
		"\tif router.ready {",
		"\t\treturn router.serve()",
	}
	windowLines := append([]string{
		"func setup() {}",
		"",
	}, partialLines...)
	window, err := NewWindow("evidence-partial-list", "router.go", 40, windowLines)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := functionSymbols(functions), []string{"setup", "Router.dispatch"}; !slices.Equal(got, want) {
		t.Fatalf("ExtractGoFunctions() symbols = %v, want %v", got, want)
	}
	partial := functions[1]
	if !partial.Partial || partial.StartLine != 42 || partial.EndLine != 44 ||
		!slices.Equal(partial.Lines, partialLines) {
		t.Fatalf("partial function = %#v", partial)
	}
	if !hasCallTarget(partial.Observations, "router.serve") {
		t.Fatalf("partial function lost visible call: %#v", partial.Observations)
	}
}

func TestExtractGoFunctionsHandlesMalformedAndEmptyWindows(t *testing.T) {
	t.Run("syntactically malformed candidate", func(t *testing.T) {
		window, err := NewWindow("evidence-malformed-candidate", "router.go", 1, []string{
			"func broken( {",
			"}",
		})
		if err != nil {
			t.Fatal(err)
		}
		functions, err := ExtractGoFunctions(window)
		if err != nil {
			t.Fatal(err)
		}
		if functions == nil || len(functions) != 0 {
			t.Fatalf("ExtractGoFunctions() = %#v, want malformed candidate omitted", functions)
		}
	})

	t.Run("lexically malformed", func(t *testing.T) {
		window, err := NewWindow("evidence-malformed", "router.go", 1, []string{
			"func broken() {",
			"\tvalue := `unterminated",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ExtractGoFunctions(window); err == nil || !strings.Contains(err.Error(), "scan bounded Go source") {
			t.Fatalf("ExtractGoFunctions() error = %v, want lexical scan error", err)
		}
	})

	t.Run("no function declarations", func(t *testing.T) {
		window, err := NewWindow("evidence-no-functions", "router.go", 1, []string{
			"type Router struct {",
			"\tready bool",
			"}",
		})
		if err != nil {
			t.Fatal(err)
		}
		functions, err := ExtractGoFunctions(window)
		if err != nil {
			t.Fatal(err)
		}
		if functions == nil || len(functions) != 0 {
			t.Fatalf("ExtractGoFunctions() = %#v, want non-nil empty result", functions)
		}
	})
}

func TestExtractGoFunctionRejectsDeclarationThatStartsBeforeWindow(t *testing.T) {
	window, err := NewWindow("evidence-tail", "router.go", 30, []string{
		"\tif ready {",
		"\t\thandle()",
		"\t}",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExtractGoFunction(window, "route")
	if err == nil || !strings.Contains(err.Error(), "not fully anchored") {
		t.Fatalf("ExtractGoFunction() error = %v, want bounded-anchor rejection", err)
	}
}

func TestFunctionValidateRejectsObservationTampering(t *testing.T) {
	window, err := NewWindow("evidence-serve", "router.go", 10, []string{
		"func serve() {",
		"\thelper()",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	function, err := ExtractGoFunction(window, "serve")
	if err != nil {
		t.Fatal(err)
	}
	function.Observations[0].EndColumn = len(function.Lines[0]) + 2
	if err := function.Validate(); err == nil || !strings.Contains(err.Error(), "columns") {
		t.Fatalf("Function.Validate() error = %v, want column-bound rejection", err)
	}

	function, err = ExtractGoFunction(window, "serve")
	if err != nil {
		t.Fatal(err)
	}
	function.ContentSHA256 = strings.Repeat("0", 64)
	if err := function.Validate(); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Function.Validate() error = %v, want content-hash rejection", err)
	}
}

func hasObservationKind(observations []Observation, kind ObservationKind) bool {
	for _, observation := range observations {
		if observation.Kind == kind {
			return true
		}
	}
	return false
}

func hasCallTarget(observations []Observation, target string) bool {
	for _, observation := range observations {
		if observation.Kind == ObservationDirectCall && observation.Target == target {
			return true
		}
	}
	return false
}

func hasObservationObject(observations []Observation, kind ObservationKind, object string) bool {
	for _, observation := range observations {
		if observation.Kind == kind && observation.Object == object {
			return true
		}
	}
	return false
}

func functionSymbols(functions []Function) []string {
	result := make([]string, 0, len(functions))
	for _, function := range functions {
		result = append(result, function.Symbol)
	}
	return result
}

func functionStartLines(functions []Function) []int {
	result := make([]int, 0, len(functions))
	for _, function := range functions {
		result = append(result, function.StartLine)
	}
	return result
}
