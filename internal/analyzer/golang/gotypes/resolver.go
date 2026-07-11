// Package gotypes resolves one already selected Go callsite with package type
// information. It deliberately loads only the package enclosing that file; it
// does not build repository-wide SSA or a call graph.
package gotypes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
	"golang.org/x/tools/go/packages"
)

const providerName = "go/packages-types"

type Executor struct {
	loaded map[string]packageLoad
}

var _ flowproof.Executor = (*Executor)(nil)

type packageLoad struct {
	pkgs []*packages.Package
}

func NewExecutor() *Executor {
	return &Executor{loaded: make(map[string]packageLoad)}
}

func (e *Executor) Execute(ctx context.Context, repoPath string, proof flowproof.Proof, task flowproof.Task) (flowproof.Result, error) {
	switch task.Kind {
	case flowproof.TaskResolveCallsite:
		return e.resolveCallsite(ctx, repoPath, proof, task)
	case flowproof.TaskInspectLifecycle:
		return e.inspectLifecycle(ctx, repoPath, proof, task)
	default:
		return flowproof.Result{}, flowproof.ErrUnsupportedTask
	}
}

func (e *Executor) resolveCallsite(ctx context.Context, repoPath string, proof flowproof.Proof, task flowproof.Task) (flowproof.Result, error) {
	transition, ok := proof.Transition(task.TargetID)
	if !ok {
		return flowproof.Result{}, fmt.Errorf("gotypes: transition %q is not in proof", task.TargetID)
	}
	placeholder, ok := proof.Anchor(transition.To)
	if !ok || transition.Evidence.Path == "" || transition.Evidence.Line <= 0 {
		return flowproof.Result{}, fmt.Errorf("gotypes: transition %q has no exact callsite", task.TargetID)
	}

	resolved, err := e.resolve(ctx, repoPath, transition.Evidence, placeholder.Label)
	if err != nil {
		return flowproof.Result{}, err
	}
	result := flowproof.Result{
		TaskKey: task.Key,
		TransitionUpdates: []flowproof.TransitionUpdate{{
			TransitionID: task.TargetID,
			Resolution:   resolved.resolution,
			Certainty:    evidence.CertaintyStatic,
			Target:       &resolved.anchor,
		}},
		Files:   resolved.files,
		Symbols: []string{resolved.anchor.QualifiedName},
	}
	if resolved.resolution == evidence.ResolutionTypeInferred {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"%s resolves to an interface method under the selected build; concrete runtime implementation remains unresolved",
			placeholder.Label,
		))
	}
	return result, nil
}

type resolution struct {
	anchor     flowproof.Anchor
	resolution evidence.ResolutionKind
	files      []string
}

func (e *Executor) resolve(ctx context.Context, repoPath string, callsite evidence.Location, expectedSymbol string) (resolution, error) {
	root, target, err := containedTarget(repoPath, callsite.Path)
	if err != nil {
		return resolution{}, err
	}
	loaded, err := e.load(ctx, root, target)
	if err != nil {
		return resolution{}, err
	}

	for _, pkg := range loaded {
		file := syntaxFile(pkg, target)
		if file == nil || pkg.TypesInfo == nil || pkg.Fset == nil {
			continue
		}
		call := exactCall(pkg.Fset, file, callsite.Line, expectedSymbol)
		if call == nil {
			continue
		}
		object, selection, err := calledObject(pkg.TypesInfo, call)
		if err != nil {
			return resolution{}, fmt.Errorf("gotypes: resolve %s at %s:%d: %w", expectedSymbol, callsite.Path, callsite.Line, err)
		}
		return objectResolution(root, pkg.Fset, object, selection, callsite.Path), nil
	}
	return resolution{}, fmt.Errorf("gotypes: exact call %s not found at %s:%d", expectedSymbol, callsite.Path, callsite.Line)
}

func (e *Executor) load(ctx context.Context, root, target string) ([]*packages.Package, error) {
	if e.loaded == nil {
		e.loaded = make(map[string]packageLoad)
	}
	key := root + "\x00" + target
	if cached, ok := e.loaded[key]; ok {
		return cached.pkgs, nil
	}
	config := &packages.Config{
		Context: ctx,
		Dir:     root,
		Mode:    packages.LoadSyntax,
		Tests:   false,
		Env:     os.Environ(),
	}
	loaded, err := packages.Load(config, "file="+target)
	if err != nil {
		return nil, fmt.Errorf("gotypes: load package for %s: %w", target, err)
	}
	if err := packageErrors(loaded); err != nil {
		return nil, err
	}
	e.loaded[key] = packageLoad{pkgs: loaded}
	return loaded, nil
}

func containedTarget(repoPath, relative string) (string, string, error) {
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("gotypes: absolute repository path: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("gotypes: resolve repository path: %w", err)
	}
	if !filepath.IsLocal(filepath.FromSlash(relative)) {
		return "", "", fmt.Errorf("gotypes: callsite path must be repository-relative: %q", relative)
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", fmt.Errorf("gotypes: resolve callsite path: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(rel) {
		return "", "", fmt.Errorf("gotypes: callsite escapes repository: %q", relative)
	}
	return root, target, nil
}

func packageErrors(loaded []*packages.Package) error {
	var messages []string
	for _, pkg := range loaded {
		for _, packageError := range pkg.Errors {
			messages = append(messages, packageError.Msg)
			if len(messages) == 3 {
				break
			}
		}
		if len(messages) == 3 {
			break
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return fmt.Errorf("gotypes: package load failed: %s", strings.Join(messages, "; "))
}

func syntaxFile(pkg *packages.Package, target string) *ast.File {
	for index, compiled := range pkg.CompiledGoFiles {
		if sameFile(compiled, target) && index < len(pkg.Syntax) {
			return pkg.Syntax[index]
		}
	}
	return nil
}

func sameFile(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func exactCall(fileSet *token.FileSet, file *ast.File, line int, expected string) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || fileSet.Position(call.Fun.Pos()).Line != line {
			return true
		}
		if callName(call.Fun) == expected || strings.HasSuffix(expected, "."+callName(call.Fun)) || strings.HasSuffix(callName(call.Fun), "."+expected) {
			found = call
			return false
		}
		return true
	})
	return found
}

func callName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		prefix := callName(expression.X)
		if prefix == "" {
			return expression.Sel.Name
		}
		return prefix + "." + expression.Sel.Name
	case *ast.IndexExpr:
		return callName(expression.X)
	case *ast.IndexListExpr:
		return callName(expression.X)
	case *ast.ParenExpr:
		return callName(expression.X)
	default:
		return ""
	}
}

func calledObject(info *types.Info, call *ast.CallExpr) (types.Object, *types.Selection, error) {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		object := info.Uses[function]
		if object == nil {
			object = info.Defs[function]
		}
		if object == nil {
			return nil, nil, fmt.Errorf("identifier %s has no type object", function.Name)
		}
		return object, nil, nil
	case *ast.SelectorExpr:
		selection := info.Selections[function]
		if selection != nil {
			return selection.Obj(), selection, nil
		}
		object := info.Uses[function.Sel]
		if object == nil {
			return nil, nil, fmt.Errorf("selector %s has no type object", function.Sel.Name)
		}
		return object, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported callable syntax %T", call.Fun)
	}
}

func objectResolution(root string, fileSet *token.FileSet, object types.Object, selection *types.Selection, callsitePath string) resolution {
	qualified := types.ObjectString(object, func(pkg *types.Package) string { return pkg.Path() })
	anchor := flowproof.Anchor{
		ID:            resolvedAnchorID(qualified),
		Kind:          flowproof.AnchorFunction,
		Label:         object.Name(),
		QualifiedName: qualified,
	}
	resolutionKind := evidence.ResolutionStatic
	if selection != nil {
		anchor.Kind = flowproof.AnchorMethod
		if isInterface(selection.Recv()) {
			resolutionKind = evidence.ResolutionTypeInferred
		}
	} else if signature, ok := object.Type().(*types.Signature); ok && signature.Recv() != nil {
		anchor.Kind = flowproof.AnchorMethod
		if isInterface(signature.Recv().Type()) {
			resolutionKind = evidence.ResolutionTypeInferred
		}
	}
	files := []string{callsitePath}
	position := fileSet.Position(object.Pos())
	if position.IsValid() {
		if relative, err := filepath.Rel(root, position.Filename); err == nil && filepath.IsLocal(relative) {
			relative = filepath.ToSlash(relative)
			anchor.Location = &evidence.Location{Path: relative, Line: position.Line, Column: position.Column}
			files = append(files, relative)
		}
	}
	return resolution{anchor: anchor, resolution: resolutionKind, files: flowproofFiles(files)}
}

func isInterface(value types.Type) bool {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	_, ok := value.Underlying().(*types.Interface)
	return ok
}

func resolvedAnchorID(qualified string) string {
	sum := sha256.Sum256([]byte(qualified))
	return "go-target-" + hex.EncodeToString(sum[:8])
}

func flowproofFiles(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
