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
)

type Result struct {
	Target       Target
	Input        p.Input
	Dependencies dependencies.Catalog
}

func project(repository *corpus.Corpus, target Target, a analysis) (*Result, error) {
	canonicalAnalysis(&a)
	raw, _ := json.Marshal(a)
	input := p.Input{ScenarioSHA256: target.CorpusSHA256, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Target: p.TargetInput{
		Language: "clojure", Kind: "package", Name: target.Name, Selector: target.Selector, AnchorFileRef: target.ManifestFileRef,
		Sources: []p.TargetSource{{FileRef: target.ManifestFileRef, Path: target.ManifestPath}},
	}}
	sources := map[string]source{}
	for _, file := range target.Files {
		data, err := repository.ReadFileAll(file.ID)
		if err != nil {
			return nil, err
		}
		sources[file.Path] = newSource(data.Bytes)
	}
	valid := func(s site) bool { _, ok := sources[s.Filename]; return ok && s.Lang != "cljs" && s.Row > 0 }
	location := func(s site) *p.Location { return &p.Location{Path: s.Filename, Line: s.Row, Column: s.Col} }
	objects := map[string]p.ObjectInput{}
	namespaces := map[string][]string{}
	fileModules := map[string]string{}
	vars := map[string][]string{}
	definitions := map[string][]definition{}
	objectRef := func(d definition) string {
		return fmt.Sprintf("var:%s:%d:%d:%s/%s", d.Filename, d.Row, d.Col, d.NS, d.Name)
	}
	addRelation := func(kind p.RelationKind, from string, to []string, at site, invocation string, pattern *p.RelationPatternInput) {
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
		r := p.RelationInput{SourceRef: fmt.Sprintf("relation:%d", len(input.Relations)), Kind: kind, FromRef: from, ToRefs: to, Resolution: resolution, TargetsObserved: max(1, len(to)), Location: location(at), Invocation: invocation, Witnesses: []p.Witness{{Kind: "clj_kondo", Detail: "native " + string(kind), Location: location(at)}}, WitnessesObserved: 1}
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
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectModule, Name: ns.Name, Visibility: p.VisibilityPublic, Directory: path.Dir(ns.Filename), Location: location(ns.site)}
		namespaces[ns.Name] = append(namespaces[ns.Name], ref)
		fileModules[ns.Filename] = ref
	}
	ensureModule := func(at site, name string) {
		if !valid(at) || fileModules[at.Filename] != "" || name == "" {
			return
		}
		ref := "namespace:" + at.Filename + ":" + name
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectModule, Name: name, Visibility: p.VisibilityPublic, Directory: path.Dir(at.Filename), Location: location(at)}
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
		ref := objectRef(d)
		kind := p.ObjectVariable
		by := d.LintAs
		if by == "" {
			by = d.DefinedBy
		}
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
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: kind, Name: d.NS + "/" + d.Name, Visibility: visibility, Signature: signature, OwnerRef: owner, ContainerRef: owner, Location: location(d.site),
			SymbolLinkIdentities: []p.SymbolLinkIdentityInput{{Domain: "clojure-var", Parts: []string{d.Filename, d.NS, d.Name}, Display: d.NS + "/" + d.Name}}}
		vars[d.NS+"/"+d.Name] = append(vars[d.NS+"/"+d.Name], ref)
		definitions[d.Filename] = append(definitions[d.Filename], d)
		addRelation(p.RelationContains, owner, []string{ref}, d.site, "", nil)
		if d.Name == "-main" && kind == p.ObjectFunction {
			fileRef, _ := repository.ID(d.Filename)
			input.Target.Sources = append(input.Target.Sources, p.TargetSource{FileRef: string(fileRef), Path: d.Filename})
			input.Target.Seeds = append(input.Target.Seeds, p.TargetSeedInput{ObjectRef: ref, Kind: p.SeedCallable, Location: location(d.site)})
		}
	}
	ownerAt := func(at site) string {
		best := fileModules[at.Filename]
		start := 0
		for _, d := range definitions[at.Filename] {
			if (at.Row > d.Row || at.Row == d.Row && at.Col >= d.Col) && (at.Row < d.EndRow || at.Row == d.EndRow && at.Col < d.EndCol) {
				n := d.Row*100000 + d.Col
				if n >= start {
					start = n
					best = objectRef(d)
				}
			}
		}
		return best
	}
	external := func(ns, name string) string {
		ref := "external:" + ns + "/" + name
		authority := p.ExternalAuthorityPackage
		if coreNamespace(ns) || platformClass(ns) {
			authority = p.ExternalAuthorityPlatform
		}
		objects[ref] = p.ObjectInput{SourceRef: ref, Kind: p.ObjectExternalSymbol, Name: ns + "/" + name, Visibility: p.VisibilityUnknown,
			External: &p.ExternalSymbol{PackagePath: ns, Name: name, AuthorityKind: authority}}
		return ref
	}
	resolve := func(ns, name string) []string {
		if refs := vars[ns+"/"+name]; len(refs) > 0 {
			return slices.Clone(refs)
		}
		// A missing local var is unresolved; importing a namespace does not prove
		// that an otherwise unknown member exists in that repository namespace.
		if len(namespaces[ns]) > 0 || ns == "" || ns == "clj-kondo/unknown-namespace" {
			return nil
		}
		return []string{external(ns, name)}
	}
	for _, u := range a.Usages {
		if !valid(u.site) {
			continue
		}
		owner := ownerAt(u.site)
		// Definition operators belong to namespace evaluation, not to their newly
		// declared function bodies. They are native compile-time forms, not calls.
		if u.Macro {
			continue
		}
		targets := resolve(u.To, u.Name)
		if u.Arity == nil {
			addRelation(p.RelationReads, owner, targets, u.site, "", nil)
			continue
		}
		args := sources[u.Filename].arguments(u.site)
		pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("call:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: u.To + "/" + u.Name, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
		addRelation(p.RelationCalls, owner, targets, u.site, "synchronous", &pattern)
	}
	for _, u := range a.Java {
		if !valid(u.site) || !u.Call {
			continue
		}
		ref := external(u.Class, u.Method)
		args := sources[u.Filename].arguments(u.site)
		pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("java:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: u.Class + "/" + u.Method, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
		addRelation(p.RelationCalls, ownerAt(u.site), []string{ref}, u.site, "synchronous", &pattern)
	}
	for _, u := range a.Instances {
		if u.Row == 0 {
			u.Row = u.NameRow
			u.Col = u.NameCol
		}
		if !valid(u.site) {
			continue
		}
		addRelation(p.RelationCalls, ownerAt(u.site), nil, u.site, u.Method, nil)
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
			args := s.arguments(u.site)
			pattern := p.RelationPatternInput{SourceRef: fmt.Sprintf("local:%s:%d:%d", u.Filename, u.Row, u.Col), Form: p.PatternCall, Selector: u.Name, Location: location(u.site), Arguments: args, ArgumentsObserved: len(args)}
			addRelation(p.RelationCalls, ownerAt(u.site), nil, u.site, "function_value_call:synchronous", &pattern)
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
			targets = []string{external(u.To, "namespace")}
			if coreNamespace(u.To) {
				kind = dependencies.KindStdlib
				module = ""
			}
		}
		addRelation(p.RelationImports, fileModules[u.Filename], targets, u.site, "", nil)
		if importerRefs[u.Filename] != "" {
			deps = append(deps, dependencies.Dependency{Language: "clojure", Kind: kind, Name: u.To, ModulePath: module, PackagePath: u.To, RepositoryPath: directory, ImporterRefs: []string{importerRefs[u.Filename]}})
		}
		// Native imports, not directory names, identify test-framework source files.
		if u.To == "clojure.test" || u.To == "speclj.core" {
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
				addRelation(p.RelationPassesCallback, call.FromRef, slices.Clone(arg.ObjectRefs), site{Filename: at.Path, Row: at.Line, Col: at.Column}, "callback_transfer:synchronous", nil)
				input.Relations[len(input.Relations)-1].SourceArgument = &p.PatternArgumentRefInput{RelationSourceRef: call.SourceRef, PatternSourceRef: pattern.SourceRef, Position: arg.Position}
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
