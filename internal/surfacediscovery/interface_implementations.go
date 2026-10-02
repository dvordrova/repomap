package surfacediscovery

import (
	"go/types"
	"sort"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// methodSetIndex narrows the named non-interface types that may implement an
// interface: each type by the methods its value and pointer method sets
// hold, so an interface's candidates are the types holding its rarest
// method, and types.Implements then decides. It is shared by the
// `implements` facts (captureInterfaceImplementations) and the calls whose
// value no observed flow gives (interfaceImplementationCandidates).
type methodSetIndex struct {
	count    int
	byMethod map[string]map[int]struct{}
}

func newMethodSetIndex(named []*types.Named) methodSetIndex {
	index := methodSetIndex{count: len(named), byMethod: make(map[string]map[int]struct{})}
	for position, entry := range named {
		seen := make(map[string]struct{})
		for _, receiver := range []types.Type{entry, types.NewPointer(entry)} {
			set := types.NewMethodSet(receiver)
			for at := 0; at < set.Len(); at++ {
				method, ok := set.At(at).Obj().(*types.Func)
				if !ok {
					continue
				}
				key := method.Id()
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
				if index.byMethod[key] == nil {
					index.byMethod[key] = make(map[int]struct{})
				}
				index.byMethod[key][position] = struct{}{}
			}
		}
	}
	return index
}

// candidates are, in order, the positions of the types that may implement
// iface: every type for an empty interface, else the types holding its
// rarest method.
func (index methodSetIndex) candidates(iface *types.Interface) []int {
	result := make([]int, 0)
	if iface.NumMethods() == 0 {
		for position := 0; position < index.count; position++ {
			result = append(result, position)
		}
		return result
	}
	var narrow map[int]struct{}
	narrowSelected := false
	for position := 0; position < iface.NumMethods(); position++ {
		set := index.byMethod[iface.Method(position).Id()]
		if !narrowSelected || len(set) < len(narrow) {
			narrow = set
			narrowSelected = true
		}
	}
	for position := range narrow {
		result = append(result, position)
	}
	sort.Ints(result)
	return result
}

// repositoryNamedTypes are the named non-interface types the target's
// admitted packages declare, generic ones and aliases left out, by package
// and then name, with their method-set index, built once.
func (a *analyzer) repositoryNamedTypes() ([]*types.Named, methodSetIndex) {
	capture := a.dynamicHandoffCapture
	if capture.repositoryTypes != nil {
		return capture.repositoryTypes, capture.repositoryTypeIndex
	}
	paths := make([]string, 0, len(a.input.Packages))
	for _, admitted := range a.input.Packages {
		paths = append(paths, admitted.Path)
	}
	sort.Strings(paths)
	named := make([]*types.Named, 0)
	for _, path := range paths {
		facts := a.packageFacts[path]
		if facts == nil || facts.Types == nil {
			continue
		}
		scope := facts.Types.Scope()
		for _, name := range scope.Names() {
			object, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || object.IsAlias() {
				continue
			}
			entry, ok := object.Type().(*types.Named)
			if !ok || types.IsInterface(entry) || entry.TypeParams().Len() > 0 {
				continue
			}
			named = append(named, entry)
		}
	}
	capture.repositoryTypes, capture.repositoryTypeIndex = named, newMethodSetIndex(named)
	return capture.repositoryTypes, capture.repositoryTypeIndex
}

// interfaceImplementationCandidates are, for an invoke of method on a value
// of declared, a named interface of the target's repository, the methods its
// repository implementations declare, as the candidates of a call whose value
// no observed flow gives (owner, 2026-09-16 and 2026-09-30: an interface call
// follows the repository's implementations, one exact, several
// alternatives). Each is the method the type's method set selects, once: a
// method promoted from an embedded type is that type's own, and one promoted
// from an embedded interface is no implementation. None for an interface
// declared outside the repository.
func (a *analyzer) interfaceImplementationCandidates(declared types.Type, method *types.Func) []godynamichandoff.Candidate {
	named, ok := types.Unalias(declared).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || !a.admittedPackages[named.Obj().Pkg().Path()] || method == nil {
		return nil
	}
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	entries, index := a.repositoryNamedTypes()
	seen := make(map[string]bool)
	var result []godynamichandoff.Candidate
	for _, position := range index.candidates(iface) {
		if candidate, ok := implementedMethod(a, entries[position], iface, method); ok {
			function := externalCallCanonicalFunction(candidate)
			if function == nil || !a.isRepositoryFunction(function) {
				continue
			}
			id, ok := a.directCallIndex.recordFunction(a, function)
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			result = append(result, godynamichandoff.Candidate{FunctionID: id, Evidence: godynamichandoff.EvidenceInterfaceImplementation})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FunctionID < result[j].FunctionID })
	return result
}

// implementedMethod is the method entry's pointer method set selects for
// method when entry implements iface: the declared method, never a method an
// embedded interface promotes.
func implementedMethod(a *analyzer, entry *types.Named, iface *types.Interface, method *types.Func) (*ssa.Function, bool) {
	pointer := types.NewPointer(entry)
	if !types.Implements(pointer, iface) {
		return nil, false
	}
	selection := types.NewMethodSet(pointer).Lookup(method.Pkg(), method.Name())
	if selection == nil {
		return nil, false
	}
	declared, ok := selection.Obj().(*types.Func)
	if !ok {
		return nil, false
	}
	if signature, ok := declared.Type().(*types.Signature); !ok || signature.Recv() == nil || types.IsInterface(signature.Recv().Type()) {
		return nil, false
	}
	function := a.program.FuncValue(declared)
	return function, function != nil
}
