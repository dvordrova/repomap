package facts

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// dynamicExecutions are the outside functions that run code inside this
// program's own process that the source does not show: code evaluated from a
// string (Python's builtin eval and exec, JavaScript's eval, Clojure's eval),
// a function built from one (JavaScript's Function), a value deserialized
// into code (pickle, marshal, and yaml's load with no loader or one of its
// unsafe loaders; yaml.load(f, Loader=yaml.SafeLoader) reads data). Starting
// another program is no such call: the reading asks which calls do
// (atlas.BoundaryRunsProgram) and names the program they start, so no
// library's launching names are kept here. Go and C have no function that
// evaluates code, so their programs have none. label is what the fact names;
// empty is the package and the name.
var dynamicExecutions = []dynamicExecution{
	{function: outsideFunction{pkg: "builtins", name: "eval"}, label: "eval"},
	{function: outsideFunction{pkg: "builtins", name: "exec"}, label: "exec"},
	{function: outsideFunction{pkg: "pickle", name: "loads", anyReceiver: true}},
	{function: outsideFunction{pkg: "pickle", name: "load", anyReceiver: true}},
	{function: outsideFunction{pkg: "marshal", name: "loads", anyReceiver: true}},
	{function: outsideFunction{pkg: "marshal", name: "load", anyReceiver: true}},
	{function: outsideFunction{pkg: "yaml", name: "load"}, loader: yamlLoader},
	{function: outsideFunction{pkg: "yaml", name: "load_all"}, loader: yamlLoader},
	{function: outsideFunction{pkg: "yaml", name: "unsafe_load"}},
	{function: outsideFunction{pkg: "yaml", name: "unsafe_load_all"}},
	{function: outsideFunction{pkg: "platform:javascript", name: "eval"}, label: "eval"},
	{function: outsideFunction{pkg: "platform:javascript", name: "Function"}, label: "Function"},
	{function: outsideFunction{pkg: "clojure.core", name: "eval"}, label: "eval"},
}

type dynamicExecution struct {
	function outsideFunction
	label    string
	// loader, when set, is the argument deciding whether the call builds
	// code: it does when the argument is absent or names one of unsafe.
	loader *loaderArgument
}

type loaderArgument struct {
	keyword  string
	position int
	unsafe   []outsideFunction
}

// yamlLoader is yaml.load's Loader: its Loader and UnsafeLoader (and their C
// twins) construct any Python object a document names.
var yamlLoader = &loaderArgument{keyword: "Loader", position: 2, unsafe: []outsideFunction{
	{pkg: "yaml", name: "Loader"}, {pkg: "yaml", name: "UnsafeLoader"},
	{pkg: "yaml", name: "CLoader"}, {pkg: "yaml", name: "CUnsafeLoader"},
}}

// buildsCode reports whether a call's loader argument lets it build code.
func (loader *loaderArgument) buildsCode(target *targetContext, pattern programindex.RelationPattern) bool {
	argument, found := keywordArgument(pattern, loader.keyword)
	if !found {
		argument, found = positionalArgument(pattern, loader.position)
	}
	if !found {
		return true
	}
	for _, id := range argument.ObjectIDs {
		object, ok := target.object(id)
		if !ok || object.Kind != programindex.ObjectExternalSymbol || object.External == nil {
			continue
		}
		for _, unsafe := range loader.unsafe {
			if unsafe.matches(*object.External) {
				return true
			}
		}
	}
	return false
}

func (b *builder) addDynamicExecution(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		if target.unreachable(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			label, resolution, ok := dynamicLabel(target, relation, pattern)
			if !ok {
				continue
			}
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			symbol, _ := target.enclosingSymbol(relation.FromID)
			b.addDynamicExecutionFact(target, *anchor, label, symbol, resolution)
		}
	}
}

func (b *builder) addDynamicExecutionFact(target *targetContext, anchor Anchor, label, symbol string, resolution Resolution) {
	if !b.once(strings.Join([]string{string(KindDynamicExecution), anchor.Path, itoa(anchor.Line)}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:       KindDynamicExecution,
		TargetID:   target.target.ID,
		Anchor:     &anchor,
		Key:        label,
		Symbol:     symbol,
		Text:       clipText(b.source.line(anchor.Path, anchor.Line)),
		Resolution: resolution,
	}, label)
}

// dynamicLabel names the dynamic execution one call makes, when it calls
// one of dynamicExecutions: a Function the call constructs is "new
// Function".
func dynamicLabel(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern) (string, Resolution, bool) {
	functions := make([]outsideFunction, 0, len(dynamicExecutions))
	for _, execution := range dynamicExecutions {
		functions = append(functions, execution.function)
	}
	function, _, resolution, ok := target.calledFunction(relation, pattern, functions)
	if !ok {
		return "", "", false
	}
	for _, execution := range dynamicExecutions {
		if execution.function != function {
			continue
		}
		if execution.loader != nil && !execution.loader.buildsCode(target, pattern) {
			return "", "", false
		}
		label := execution.label
		if label == "" {
			label = function.pkg + "." + function.name
		}
		if label == "Function" && relation.Invocation == programindex.InvocationConstruct {
			label = "new Function"
		}
		return label, resolution, true
	}
	return "", "", false
}
