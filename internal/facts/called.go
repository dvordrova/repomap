package facts

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A config read and a dynamic execution are calls of particular outside
// functions. The callee's owner, as the native graph resolves the call,
// decides whether a call is one; the call's word never does (owner, after
// the 2026-10-03 review). A callee the repository declares is the
// repository's own code whatever its name (a local getenv, casdoor's
// slavedb.exec, etcd's txn.eval), and a call the graph leaves unresolved
// names no owner: it stays unknown, neither an exact nor a possible fact.

// outsideFunction is one outside function by the identity the native graph
// gives it: its package, the receiver it is a member of ("" for a function
// of the package itself) and its name. anyReceiver matches the name as a
// member of any of the package's values too (pickle.Unpickler.load).
type outsideFunction struct {
	pkg, receiver, name string
	anyReceiver         bool
}

func (function outsideFunction) matches(symbol programindex.ExternalSymbol) bool {
	if symbol.PackagePath != function.pkg || symbol.Name != function.name {
		return false
	}
	return function.anyReceiver || symbol.Receiver == function.receiver
}

// builtinWitness is the Python adapter's evidence that a bare call's name is
// bound nowhere in its module, so Python looks it up in the builtins module,
// which defines it (PYTHON "Builtins"). The call stays unresolved in the
// graph; its witness names the builtin.
const builtinWitness = "builtin"

// calledSymbols are the outside functions one call reaches, as the native
// graph resolves it: its outside callees, or, when the graph resolved none,
// the member it calls of an outside value's origin, or the builtin the
// adapter witnessed. callees counts every callee the call may reach, the
// repository's own among them, so a caller can tell one exact outside
// callee from one of several alternatives.
func (target *targetContext) calledSymbols(relation programindex.Relation, pattern programindex.RelationPattern) (symbols []programindex.ExternalSymbol, callees int) {
	if len(relation.ToIDs) > 0 {
		for _, id := range relation.ToIDs {
			if object, ok := target.object(id); ok && object.Kind == programindex.ObjectExternalSymbol && object.External != nil && object.External.RepositoryPath == "" {
				symbols = append(symbols, *object.External)
			}
		}
		return symbols, len(relation.ToIDs)
	}
	if target.ownsReceiver(pattern) {
		return nil, 0
	}
	for _, id := range pattern.ReceiverOriginIDs {
		object, ok := target.object(id)
		if !ok || object.Kind != programindex.ObjectExternalSymbol || object.External == nil || object.External.RepositoryPath != "" || pattern.Selector == "" {
			continue
		}
		origin := *object.External
		receiver := origin.Name
		if origin.Receiver != "" {
			receiver = origin.Receiver + "." + origin.Name
		}
		symbols = append(symbols, programindex.ExternalSymbol{AuthorityKind: origin.AuthorityKind, PackagePath: origin.PackagePath, Receiver: receiver, Name: pattern.Selector})
	}
	if len(symbols) > 0 {
		return symbols, len(symbols)
	}
	for _, witness := range relation.Witnesses {
		if witness.Kind == builtinWitness && witness.Detail != "" && !strings.ContainsAny(witness.Detail, ". ") {
			return []programindex.ExternalSymbol{{AuthorityKind: programindex.ExternalAuthorityPlatform, PackagePath: "builtins", Name: witness.Detail}}, 1
		}
	}
	return nil, 0
}

// calledFunction is the first of functions one call reaches and how sure
// that is: exact when it is the call's one callee, possible when the call
// may reach another callee instead (alternatives a condition chooses).
func (target *targetContext) calledFunction(relation programindex.Relation, pattern programindex.RelationPattern, functions []outsideFunction) (outsideFunction, programindex.ExternalSymbol, Resolution, bool) {
	symbols, callees := target.calledSymbols(relation, pattern)
	for _, function := range functions {
		for _, symbol := range symbols {
			if !function.matches(symbol) {
				continue
			}
			resolution := ResolutionExact
			if callees > 1 {
				resolution = ResolutionPossible
			}
			return function, symbol, resolution, true
		}
	}
	return outsideFunction{}, programindex.ExternalSymbol{}, "", false
}

// derivesFrom reports a class whose bases, followed through the
// repository's own classes, reach one of the outside classes.
func (target *targetContext) derivesFrom(classID string, classes []outsideFunction) bool {
	seen := map[string]bool{}
	pending := []string{classID}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		object, ok := target.object(id)
		if !ok {
			continue
		}
		if object.Kind == programindex.ObjectExternalSymbol && object.External != nil && object.External.RepositoryPath == "" {
			for _, class := range classes {
				if class.matches(*object.External) {
					return true
				}
			}
			continue
		}
		if object.Kind == programindex.ObjectType {
			pending = append(pending, target.classBases[id]...)
		}
	}
	return false
}
