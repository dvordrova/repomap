package clojureproject

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	p "github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

type Result struct {
	Target       Target
	Input        p.Input
	Dependencies dependencies.Catalog
}

func project(repository *corpus.Corpus, target Target, a analysis) (*Result, error) {
	bindings := localBindings(a)
	canonicalAnalysis(&a)
	raw, _ := json.Marshal(a)
	kind := "package"
	if target.Platform == "cljs" {
		kind = "executable"
	}
	input := p.Input{ScenarioSHA256: target.CorpusSHA256, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Target: p.TargetInput{
		Language: "clojure", Kind: kind, Name: target.Name, Selector: target.Selector, AnchorFileRef: target.ManifestFileRef,
		Sources: []p.TargetSource{{FileRef: target.ManifestFileRef, Path: target.ManifestPath}},
	}}
	cljs := target.Platform == "cljs"
	sources := map[string]source{}
	codeLines := map[string]map[int]bool{}
	for _, file := range target.Files {
		data, err := repository.ReadFileAll(file.ID)
		if err != nil {
			return nil, err
		}
		sources[file.Path] = newSource(data.Bytes)
		codeLines[file.Path] = sources[file.Path].codeLines()
	}
	// The JVM view reads a .cljc file's :clj branch, a shadow-cljs build its
	// :cljs branch; a .clj or .cljs file has one.
	valid := func(s site) bool {
		_, ok := sources[s.Filename]
		if cljs {
			return ok && s.Lang != "clj" && s.Row > 0
		}
		return ok && s.Lang != "cljs" && s.Row > 0
	}
	location := func(s site) *p.Location { return &p.Location{Path: s.Filename, Line: s.Row, Column: s.Col} }
	objects := map[string]p.ObjectInput{}
	namespaces := map[string][]string{}
	fileModules := map[string]string{}
	vars := map[string][]string{}
	definitions := map[string][]definition{}
	// A definition's metadata and attr-maps run once, when its namespace
	// loads, not when its function is called: a call there belongs to the
	// namespace (or to an enclosing definition), as a Python decorator's
	// arguments and defaults belong to the defining scope.
	loaded := map[string][][2]int{}
	objectRef := func(d definition) string {
		return fmt.Sprintf("var:%s:%d:%d:%s/%s", d.Filename, d.Row, d.Col, d.NS, d.Name)
	}
	addRelation := func(kind p.RelationKind, from string, to []string, at site, dispatch string, pattern *p.RelationPatternInput) {
		if from == "" {
			return
		}
		slices.Sort(to)
		to = slices.Compact(to)
		resolution := p.ResolutionUnresolved
		if len(to) == 1 {
			resolution = p.ResolutionExact
		} else if len(to) > 1 {
			resolution = p.ResolutionAlternatives
		}
		r := p.RelationInput{SourceRef: fmt.Sprintf("relation:%d", len(input.Relations)), Kind: kind, FromRef: from, ToRefs: to, Resolution: resolution, TargetsObserved: max(1, len(to)), Location: location(at), Dispatch: dispatch, Witnesses: []p.Witness{{Kind: "clj_kondo", Detail: "native " + string(kind), Location: location(at)}}, WitnessesObserved: 1}
		if pattern != nil {
			r.Patterns = []p.RelationPatternInput{*pattern}
			r.PatternsObserved = 1
		}
		input.Relations = append(input.Relations, r)
	}
	for _, ns := range a.Namespaces {
		if !valid(ns.site) {
			continue
		}
		ref := fmt.Sprintf("namespace:%s:%d:%s", ns.Filename, ns.Row, ns.Name)
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectModule, Name: ns.Name, Visibility: p.VisibilityPublic, Directory: path.Dir(ns.Filename), Location: location(ns.site), CodeLines: len(codeLines[ns.Filename])}
		namespaces[ns.Name] = append(namespaces[ns.Name], ref)
		fileModules[ns.Filename] = ref
	}
	ensureModule := func(at site, name string) {
		if !valid(at) || fileModules[at.Filename] != "" || name == "" {
			return
		}
		ref := "namespace:" + at.Filename + ":" + name
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectModule, Name: name, Visibility: p.VisibilityPublic, Directory: path.Dir(at.Filename), Location: location(at), CodeLines: len(codeLines[at.Filename])}
		fileModules[at.Filename] = ref
		namespaces[name] = append(namespaces[name], ref)
	}
	for _, d := range a.Definitions {
		ensureModule(d.site, d.NS)
	}
	for _, u := range a.Usages {
		ensureModule(u.site, u.From)
	}

	for _, d := range a.Definitions {
		if !valid(d.site) {
			continue
		}
		owner := fileModules[d.Filename]
		if owner == "" {
			continue
		}
		by := d.LintAs
		if by == "" {
			by = d.DefinedBy
		}
		// A forward declaration names a var its defn defines later: it is
		// no definition of its own (othello's (declare negamax) had stood
		// as a second tile "negamax: negamax" beside the defn), in either
		// view: a ClojureScript view reads it as cljs.core/declare.
		if by == "clojure.core/declare" || by == "cljs.core/declare" {
			continue
		}
		ref := objectRef(d)
		kind := p.ObjectVariable
		switch by {
		case "clojure.core/defn", "clojure.core/defn-", "clojure.core/defmacro", "clojure.core/defmulti":
			kind = p.ObjectFunction
		case "clojure.core/defprotocol", "clojure.core/defrecord", "clojure.core/deftype", "clojure.core/definterface":
			kind = p.ObjectType
		}
		if len(d.Arglists) > 0 && kind == p.ObjectVariable {
			kind = p.ObjectFunction
		}
		visibility := p.VisibilityPublic
		if d.Private {
			visibility = p.VisibilityInternal
		}
		signature := d.Name
		if len(d.Arglists) > 0 {
			signature += " " + strings.Join(d.Arglists, " ")
		}
		// clj-kondo marks a macro's definition; its uses are skipped below, so
		// the index records no use of it and says so (CLOJURE).
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: kind, Name: d.NS + "/" + d.Name, Visibility: visibility, Signature: signature, OwnerRef: owner, ContainerRef: owner, Location: location(d.site), EndLine: d.EndRow, CodeLines: countLines(codeLines[d.Filename], d.Row, d.EndRow),
			Macro: d.Macro && kind == p.ObjectFunction}
		vars[d.NS+"/"+d.Name] = append(vars[d.NS+"/"+d.Name], ref)
		definitions[d.Filename] = append(definitions[d.Filename], d)
		if by == "clojure.core/def" && kind == p.ObjectVariable {
			object := objects[ref]
			object.Rows = sources[d.Filename].mapRows(d.Filename, d.site)
			objects[ref] = object
		}
		switch by {
		case "clojure.core/defn", "clojure.core/defn-", "clojure.core/defmacro":
			loaded[ref] = sources[d.Filename].loadedHeaders(d.site, true)
		case "clojure.core/def", "clojure.core/defonce", "clojure.core/defmulti":
			loaded[ref] = sources[d.Filename].loadedHeaders(d.site, false)
		}
		if d.Name == "-main" && kind == p.ObjectFunction && !cljs {
			fileRef, _ := repository.ID(d.Filename)
			input.Target.Sources = append(input.Target.Sources, p.TargetSource{FileRef: string(fileRef), Path: d.Filename})
			input.Target.Seeds = append(input.Target.Seeds, p.TargetSeedInput{ObjectRef: ref, Kind: p.SeedCallable, Location: location(d.site)})
		}
	}
	// A shadow-cljs build starts from what it names: a module's :init-fn or
	// a node script's :main is a function it calls, a module's :entries a
	// namespace it loads. A name the view declares nowhere starts nothing.
	for _, entry := range target.Entries {
		refs, seed := vars[entry.Symbol], p.SeedCallable
		if !strings.Contains(entry.Symbol, "/") {
			refs, seed = namespaces[entry.Symbol], p.SeedModule
		}
		for _, ref := range refs {
			object := objects[ref]
			if seed == p.SeedCallable && object.Kind != p.ObjectFunction || object.Location == nil {
				continue
			}
			fileRef, _ := repository.ID(object.Location.Path)
			if !slices.ContainsFunc(input.Target.Sources, func(source p.TargetSource) bool { return source.Path == object.Location.Path }) {
				input.Target.Sources = append(input.Target.Sources, p.TargetSource{FileRef: string(fileRef), Path: object.Location.Path})
			}
			input.Target.Seeds = append(input.Target.Seeds, p.TargetSeedInput{ObjectRef: ref, Kind: seed, Location: object.Location})
		}
	}
	definitionOf := map[string]definition{}
	for _, d := range a.Definitions {
		if valid(d.site) {
			definitionOf[objectRef(d)] = d
		}
	}
	ownerAt := func(at site) string {
		best := fileModules[at.Filename]
		start := 0
		offset := sources[at.Filename].offset(at.Row, at.Col)
		for _, d := range definitions[at.Filename] {
			if !(at.Row > d.Row || at.Row == d.Row && at.Col >= d.Col) || !(at.Row < d.EndRow || at.Row == d.EndRow && at.Col < d.EndCol) {
				continue
			}
			ref := objectRef(d)
			if slices.ContainsFunc(loaded[ref], func(span [2]int) bool { return offset >= span[0] && offset < span[1] }) {
				continue
			}
			if n := d.Row*100000 + d.Col; n >= start {
				start, best = n, ref
			}
		}
		return best
	}
	// A native row whose namespace, class or name is no name the index
	// accepts names no outside symbol: its use stays unresolved.
	external := func(ns, name string) (string, bool) {
		if !p.ValidName(ns) || !p.ValidName(name) {
			return "", false
		}
		ref := "external:" + ns + "/" + name
		authority := p.ExternalAuthorityPackage
		if coreNamespace(ns) || platformClass(ns) || cljs && cljsPlatformNamespace(ns) {
			authority = p.ExternalAuthorityPlatform
		}
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectExternalSymbol, Name: ns + "/" + name, Visibility: p.VisibilityUnknown,
			External: &p.ExternalSymbol{PackagePath: ns, Name: name, AuthorityKind: authority}}
		return ref, true
	}
	resolve := func(ns, name string) []string {
		if refs := vars[ns+"/"+name]; len(refs) > 0 {
			return slices.Clone(refs)
		}
		// A JavaScript global (js/setTimeout) is the browser's own: clj-kondo
		// writes it as a name with no namespace.
		if global, ok := strings.CutPrefix(name, "js/"); ok && ns == "" && cljs {
			if ref, ok := external("js", global); ok {
				return []string{ref}
			}
			return nil
		}
		// A missing local var is unresolved; importing a namespace does not prove
		// that an otherwise unknown member exists in that repository namespace.
		if len(namespaces[ns]) > 0 || ns == "" || ns == "clj-kondo/unknown-namespace" {
			return nil
		}
		if ref, ok := external(ns, name); ok {
			return []string{ref}
		}
		return nil
	}
	// A var named in argument position is what that argument carries.
	argumentTargets := map[site][]string{}
	for _, u := range a.Usages {
		if valid(u.site) && !u.Macro && u.Arity == nil {
			argumentTargets[site{Filename: u.Filename, Row: u.NameRow, Col: u.NameCol}] = resolve(u.To, u.Name)
		}
	}
	// A function's own parameters, by where each is written: a local a call
	// is given that is bound there carries that parameter (parameters.go).
	type parameterSite struct {
		position int
		owner    site
	}
	parameterSites := map[string]map[[2]int]parameterSite{}
	for _, d := range a.Definitions {
		if !valid(d.site) || len(d.Arglists) == 0 {
			continue
		}
		for at, position := range sources[d.Filename].parameters(d.Filename, d.site) {
			if parameterSites[d.Filename] == nil {
				parameterSites[d.Filename] = map[[2]int]parameterSite{}
			}
			parameterSites[d.Filename][at] = parameterSite{position: position, owner: d.site}
		}
	}
	parameterAt := func(file string) func(row, col int) (string, int, *sourcevalue.Anchor, bool) {
		return func(row, col int) (string, int, *sourcevalue.Anchor, bool) {
			binding, ok := bindings[site{Filename: file, Row: row, Col: col}]
			if !ok {
				return "", 0, nil, false
			}
			parameter, ok := parameterSites[binding.filename][[2]int{binding.row, binding.col}]
			if !ok {
				return "", 0, nil, false
			}
			return binding.name, parameter.position, &sourcevalue.Anchor{Path: parameter.owner.Filename, Line: parameter.owner.Row, Column: parameter.owner.Col}, true
		}
	}
	argumentsOf := func(u site) []p.PatternArgumentInput {
		args := sources[u.Filename].arguments(u)
		for i := range args {
			args[i].Origin = parameterOrigin(args[i].Origin, parameterAt(u.Filename))
			at := args[i].Origin.Anchor
			if refs := argumentTargets[site{Filename: at.Path, Row: at.Line, Col: at.Column}]; len(refs) > 0 {
				args[i].ObjectRefs, args[i].Resolution, args[i].ObjectsObserved = refs, p.ResolutionExact, len(refs)
				if len(refs) > 1 {
					args[i].Resolution = p.ResolutionAlternatives
				}
			}
		}
		return args
	}
	started := map[site]bool{}
	// joined is, by file, the call each call is another spelling of: an
	// operand of the same `or` form (source.go joinedCalls).
	joined := map[string]map[[2]int][2]int{}
	for _, u := range a.Usages {
		if valid(u.site) && u.Macro && u.To == "clojure.core" && u.Name == "or" {
			for later, first := range sources[u.Filename].joinedCalls(u.site) {
				if joined[u.Filename] == nil {
					joined[u.Filename] = map[[2]int][2]int{}
				}
				joined[u.Filename][later] = first
			}
		}
	}
	for _, u := range a.Usages {
		if !valid(u.site) {
			continue
		}
		owner := ownerAt(u.site)
		// A case form compares its value with the words its tests write
		// (PROGRAM_INDEX Comparison); it stays no call.
		if u.Macro && u.To == "clojure.core" && u.Name == "case" && owner != "" {
			if comparison := sources[u.Filename].caseComparison(u.site); comparison != nil {
				object := objects[owner]
				repeated := slices.ContainsFunc(object.Comparisons, func(recorded p.Comparison) bool { return *recorded.Location == *comparison.Location })
				if (object.Kind.Callable() || object.Kind == p.ObjectModule) && !repeated {
					object.Comparisons = append(object.Comparisons, *comparison)
					objects[owner] = object
				}
			}
		}
		// A future runs its body on a thread of its own: each call written as
		// a form of its body starts there (goroutine, the word every adapter
		// shares with Go's `go f()`), and the future's own use is a call of
		// clojure.core/future handed those calls, as asyncio.create_task is
		// handed a coroutine (facts, started.go). No other macro use leaves a
		// relation.
		if u.Macro && u.To == "clojure.core" && u.Name == "future" && !cljs && owner != "" {
			args := argumentsOf(u.site)
			for i := range args {
				if origin := args[i].Origin; origin != nil && origin.Anchor != nil && strings.HasPrefix(origin.Text, "(") {
					origin.Kind = "call_result"
					started[site{Filename: origin.Anchor.Path, Row: origin.Anchor.Line, Col: origin.Anchor.Column}] = true
				}
			}
			var targets []string
			if ref, ok := external(u.To, u.Name); ok {
				targets = []string{ref}
			}
			pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("call:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: u.To + "/" + u.Name, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
			addRelation(p.RelationInvokesExternal, owner, targets, u.site, "", &pattern)
			continue
		}
		// Definition operators belong to namespace evaluation, not to their newly
		// declared function bodies. They are native compile-time forms, not calls.
		if u.Macro {
			continue
		}
		// A special form (if, do, recur, fn*, try, `.`) is the language's
		// syntax, no var and no call: clj-kondo knows an arity for every
		// function of the core namespaces and none for a special form.
		if u.Arity != nil && (u.To == "clojure.core" || u.To == "cljs.core") && len(u.FixedArities) == 0 && u.VarargsMinArity == nil {
			continue
		}
		targets := resolve(u.To, u.Name)
		if u.Arity == nil {
			addRelation(p.RelationReads, owner, targets, u.site, "", nil)
			continue
		}
		args := argumentsOf(u.site)
		// A name clj-kondo resolves to no namespace (js/setTimeout) is the
		// selector as written.
		selector := u.To + "/" + u.Name
		if u.To == "" {
			selector = u.Name
		}
		pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("call:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: selector, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
		kind := p.RelationCalls
		if len(targets) == 1 && objects[targets[0]].Kind == p.ObjectExternalSymbol {
			kind = p.RelationInvokesExternal
		}
		addRelation(kind, owner, targets, u.site, "", &pattern)
		// A definition's call of itself written in one arity that its
		// argument count sends to another is a call of that arity, not a
		// recursion: its relation says which (an `arity` witness).
		if d, ok := definitionOf[owner]; ok && len(targets) == 1 && targets[0] == owner {
			if params := sources[u.Filename].arityCalled(d.site, u.site, *u.Arity); params != "" {
				relation := &input.Relations[len(input.Relations)-1]
				relation.Witnesses = append(relation.Witnesses, p.Witness{Kind: "arity", Detail: params, Location: location(u.site)})
				relation.WitnessesObserved++
			}
		}
	}
	for _, u := range a.Java {
		// A static method call names its method. clj-kondo also reports, as
		// a call with no method, an imported class that a syntax-quoted
		// constructor names (`(ArrayList.)` in a macro body) and classes in
		// an :import list: like a plain constructor, they call nothing an
		// outside symbol could name.
		if !valid(u.site) || !u.Call || u.Method == "" {
			continue
		}
		var targets []string
		if ref, ok := external(u.Class, u.Method); ok {
			targets = []string{ref}
		}
		args := argumentsOf(u.site)
		pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("java:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: u.Class + "/" + u.Method, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
		addRelation(p.RelationCalls, ownerAt(u.site), targets, u.site, "", &pattern)
	}
	for _, u := range a.Instances {
		if u.Row == 0 {
			u.Row = u.NameRow
			u.Col = u.NameCol
		}
		if !valid(u.site) {
			continue
		}
		owner := ownerAt(u.site)
		if owner == "" {
			continue
		}
		addRelation(p.RelationCalls, owner, nil, u.site, "", nil)
		input.Relations[len(input.Relations)-1].Witnesses[0].Detail = "java instance method " + u.Method
	}
	// clj-kondo binds locals independently from vars: a parameter shadowing a
	// global must never turn into a call of that global.
	for _, u := range a.LocalUsages {
		if !valid(u.site) {
			continue
		}
		s := sources[u.Filename]
		offset := s.offset(u.Row, u.Col)
		if offset >= 0 && offset < len(s.text) && s.text[offset] == '(' {
			// An anonymous function literal's argument (`#(% 1)`) is a local
			// clj-kondo names nothing: the call keeps the name as written.
			name := u.Name
			if name == "" {
				name = s.callee(u.site)
			}
			if !p.ValidName(name) {
				addRelation(p.RelationCalls, ownerAt(u.site), nil, u.site, p.DispatchFunctionValue, nil)
				continue
			}
			args := s.arguments(u.site)
			pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("local:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: name, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
			addRelation(p.RelationCalls, ownerAt(u.site), nil, u.site, p.DispatchFunctionValue, &pattern)
		}
	}
	// A call another spelling of an earlier call names that call's pattern
	// when both make the same relation: one kind, from one declaration, to
	// one target.
	if len(joined) > 0 {
		byPattern := map[string]int{}
		for i, relation := range input.Relations {
			if len(relation.Patterns) == 1 {
				byPattern[relation.Patterns[0].SourceRef] = i
			}
		}
		for i := range input.Relations {
			relation := &input.Relations[i]
			if len(relation.Patterns) != 1 || relation.Patterns[0].Location == nil {
				continue
			}
			at := relation.Patterns[0].Location
			first, ok := joined[at.Path][[2]int{at.Line, at.Column}]
			if !ok {
				continue
			}
			ref := fmt.Sprintf("call:%s:%d:%d", at.Path, first[0], first[1])
			position, ok := byPattern[ref]
			if !ok || !strings.HasPrefix(relation.Patterns[0].SourceRef, "call:") {
				continue
			}
			root := input.Relations[position]
			if root.Kind != relation.Kind || root.FromRef != relation.FromRef || !slices.Equal(root.ToRefs, relation.ToRefs) {
				continue
			}
			relation.Patterns[0].SameValueAs = &p.PatternRefInput{RelationSourceRef: root.SourceRef, PatternSourceRef: ref}
		}
	}
	for i := range input.Relations {
		relation := &input.Relations[i]
		if relation.Kind != p.RelationCalls && relation.Kind != p.RelationInvokesExternal || len(relation.Patterns) == 0 || relation.Patterns[0].Location == nil {
			continue
		}
		at := relation.Patterns[0].Location
		if started[site{Filename: at.Path, Row: at.Line, Col: at.Column}] {
			relation.Invocation = p.InvocationGoroutine
		}
	}
	var importers []dependencies.Importer
	importerRefs := map[string]string{}
	for file, ref := range fileModules {
		ns := objects[ref].Name
		importer, err := dependencies.SealImporter(dependencies.Importer{Language: "clojure", Name: ns, ModulePath: target.Name, PackagePath: ns, RepositoryPath: path.Dir(file)})
		if err != nil {
			return nil, err
		}
		importers = append(importers, importer)
		importerRefs[file] = importer.Ref
	}
	var deps []dependencies.Dependency
	for _, u := range a.Imports {
		if !valid(u.site) {
			continue
		}
		targets := slices.Clone(namespaces[u.To])
		kind := dependencies.KindExternal
		module := u.To
		directory := ""
		if len(targets) > 0 {
			kind = dependencies.KindWorkspace
			module = target.Name
			directory = objects[targets[0]].Directory
		} else {
			ref, ok := external(u.To, "namespace")
			if !ok {
				// A use of no namespace name imports nothing to name.
				continue
			}
			targets = []string{ref}
			if coreNamespace(u.To) || cljs && cljsPlatformNamespace(u.To) {
				kind = dependencies.KindStdlib
				module = ""
			}
		}
		addRelation(p.RelationImports, fileModules[u.Filename], targets, u.site, "", nil)
		if importerRefs[u.Filename] != "" {
			deps = append(deps, dependencies.Dependency{Language: "clojure", Kind: kind, Name: u.To, ModulePath: module, PackagePath: u.To, RepositoryPath: directory, ImporterRefs: []string{importerRefs[u.Filename]}})
		}
		// Native imports, not directory names, identify test-framework source files.
		if u.To == "clojure.test" || u.To == "speclj.core" || u.To == "cljs.test" {
			input.Target.TestSources = append(input.Target.TestSources, u.Filename)
		}
	}
	refsAt := map[string][]string{}
	for _, u := range a.Usages {
		if !valid(u.site) || u.Arity != nil {
			continue
		}
		row, col := u.NameRow, u.NameCol
		if row == 0 {
			row, col = u.Row, u.Col
		}
		refsAt[fmt.Sprintf("%s:%d:%d", u.Filename, row, col)] = resolve(u.To, u.Name)
	}
	callCount := len(input.Relations)
	for i := 0; i < callCount; i++ {
		relation := &input.Relations[i]
		for pi := range relation.Patterns {
			pattern := &relation.Patterns[pi]
			for ai := range pattern.Arguments {
				arg := &pattern.Arguments[ai]
				if arg.Origin == nil || arg.Origin.Anchor == nil {
					continue
				}
				at := arg.Origin.Anchor
				refs := refsAt[fmt.Sprintf("%s:%d:%d", at.Path, at.Line, at.Column)]
				if len(refs) == 0 {
					continue
				}
				arg.ObjectRefs = refs
				arg.ObjectsObserved = len(refs)
				arg.Resolution = p.ResolutionExact
				if len(refs) > 1 {
					arg.Resolution = p.ResolutionAlternatives
				}
			}
		}
	}
	// Append callback transfers after all pointers into the call slice are released.
	for i := 0; i < callCount; i++ {
		call := input.Relations[i]
		for _, pattern := range call.Patterns {
			for _, arg := range pattern.Arguments {
				if len(arg.ObjectRefs) == 0 {
					continue
				}
				callable := true
				for _, ref := range arg.ObjectRefs {
					if objects[ref].Kind != p.ObjectFunction && objects[ref].Kind != p.ObjectMethod {
						callable = false
					}
				}
				if !callable {
					continue
				}
				at := arg.Origin.Anchor
				addRelation(p.RelationPassesCallback, call.FromRef, slices.Clone(arg.ObjectRefs), site{Filename: at.Path, Row: at.Line, Col: at.Column}, "", nil)
				input.Relations[len(input.Relations)-1].SourceArgument = &p.PatternArgumentRefInput{RelationSourceRef: call.SourceRef, PatternSourceRef: pattern.SourceRef, Position: arg.Position, Keyword: arg.Keyword}
			}
		}
	}

	// A directory the build description runs as tests holds test code only:
	// its specs, and the helpers they share (spec_helper.clj) too.
	for _, file := range target.Files {
		for _, dir := range target.TestDirs {
			if strings.HasPrefix(file.Path, dir+"/") {
				input.Target.TestSources = append(input.Target.TestSources, file.Path)
			}
		}
	}
	slices.Sort(input.Target.TestSources)
	input.Target.TestSources = slices.Compact(input.Target.TestSources)
	for _, obj := range objects {
		input.Objects = append(input.Objects, obj)
	}
	slices.SortFunc(input.Objects, func(a, b p.ObjectInput) int { return strings.Compare(a.SourceRef, b.SourceRef) })
	input.Coverage = p.CoverageInput{Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations)}
	catalog, err := dependencies.BuildWithOmissions(importers, deps, nil)
	if err != nil {
		return nil, err
	}
	return &Result{Target: target, Input: input, Dependencies: catalog}, nil
}

// These namespaces ship in org.clojure/clojure itself. Third-party libraries
// under the clojure.* prefix (for example clojure.java.jdbc) remain packages.
func coreNamespace(ns string) bool {
	switch ns {
	case "clojure.core", "clojure.core.protocols", "clojure.core.reducers", "clojure.data", "clojure.edn", "clojure.inspector", "clojure.instant", "clojure.java.browse", "clojure.java.io", "clojure.java.javadoc", "clojure.java.shell", "clojure.main", "clojure.pprint", "clojure.reflect", "clojure.repl", "clojure.set", "clojure.stacktrace", "clojure.string", "clojure.template", "clojure.test", "clojure.walk", "clojure.xml", "clojure.zip":
		return true
	}
	return false
}

// These namespaces ship with ClojureScript itself, and the Closure Library
// and JavaScript globals (js/...) are the browser platform a build runs on.
func cljsPlatformNamespace(ns string) bool {
	switch ns {
	case "js", "goog", "cljs.core", "cljs.reader", "cljs.pprint", "cljs.test", "cljs.repl", "cljs.spec.alpha", "cljs.spec.gen.alpha",
		"clojure.string", "clojure.set", "clojure.walk", "clojure.data", "clojure.zip", "clojure.edn", "clojure.reflect", "clojure.core.reducers":
		return true
	}
	return strings.HasPrefix(ns, "goog.")
}

// Native parallel scheduling and process-local binding IDs are not identities.
func canonicalAnalysis(a *analysis) {
	less := func(x, y site) int {
		if n := strings.Compare(x.Filename, y.Filename); n != 0 {
			return n
		}
		if x.Row != y.Row {
			return x.Row - y.Row
		}
		if x.Col != y.Col {
			return x.Col - y.Col
		}
		return strings.Compare(x.Lang, y.Lang)
	}
	slices.SortFunc(a.Namespaces, func(x, y namespace) int { return less(x.site, y.site) })
	slices.SortFunc(a.Imports, func(x, y namespace) int {
		if n := less(x.site, y.site); n != 0 {
			return n
		}
		return strings.Compare(x.To, y.To)
	})
	slices.SortFunc(a.Definitions, func(x, y definition) int {
		if n := less(x.site, y.site); n != 0 {
			return n
		}
		return strings.Compare(x.Name, y.Name)
	})
	slices.SortFunc(a.Usages, func(x, y usage) int {
		if n := less(x.site, y.site); n != 0 {
			return n
		}
		return strings.Compare(x.To+"/"+x.Name, y.To+"/"+y.Name)
	})
	slices.SortFunc(a.LocalUsages, func(x, y local) int { return less(x.site, y.site) })
	for i := range a.LocalUsages {
		a.LocalUsages[i].ID = 0
	}
	a.Locals = nil
	slices.SortFunc(a.Java, func(x, y javaUsage) int { return less(x.site, y.site) })
	slices.SortFunc(a.Instances, func(x, y javaUsage) int { return less(x.site, y.site) })
}

// JVM classes whose platform ownership is fixed by the Java language/runtime.
func platformClass(name string) bool {
	switch name {
	case "java.lang.System", "java.lang.Math", "java.lang.String", "java.lang.Object", "java.lang.Class", "java.lang.Thread", "java.lang.Runtime", "java.lang.Integer", "java.lang.Long", "java.lang.Double", "java.lang.Boolean", "java.lang.Exception", "java.lang.Throwable":
		return true
	}
	return false
}
