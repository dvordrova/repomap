package gotypes

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func (e *Executor) inspectLifecycle(ctx context.Context, repoPath string, proof flowproof.Proof, task flowproof.Task) (flowproof.Result, error) {
	startTransition, ok := proof.Transition(task.TargetID)
	if !ok {
		return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle transition %q is not in proof", task.TargetID)
	}
	startPlaceholder, ok := proof.Anchor(startTransition.To)
	if !ok || startTransition.Evidence.Path == "" || startTransition.Evidence.Line <= 0 {
		return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle transition %q has no exact start callsite", task.TargetID)
	}

	root, target, err := containedTarget(repoPath, startTransition.Evidence.Path)
	if err != nil {
		return flowproof.Result{}, err
	}
	loaded, err := e.load(ctx, root, target)
	if err != nil {
		return flowproof.Result{}, err
	}
	for _, pkg := range loaded {
		file := syntaxFile(pkg, target)
		if file == nil || pkg.TypesInfo == nil || pkg.Fset == nil {
			continue
		}
		startCall := exactCall(pkg.Fset, file, startTransition.Evidence.Line, startPlaceholder.Label)
		if startCall == nil {
			continue
		}
		function := containingFunction(file, startCall)
		if function == nil {
			return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle start is outside a function")
		}
		return lifecycleResult(root, pkg.Fset, pkg.TypesInfo, function, startCall, startTransition, task)
	}
	return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle start %s not found at %s:%d", startPlaceholder.Label, startTransition.Evidence.Path, startTransition.Evidence.Line)
}

func lifecycleResult(
	root string,
	fileSet *token.FileSet,
	info *types.Info,
	function *ast.FuncDecl,
	startCall *ast.CallExpr,
	startTransition flowproof.Transition,
	task flowproof.Task,
) (flowproof.Result, error) {
	startObject, startSelection, err := calledObject(info, startCall)
	if err != nil {
		return flowproof.Result{}, fmt.Errorf("gotypes: resolve lifecycle start: %w", err)
	}
	startTarget := objectResolution(root, fileSet, startObject, startSelection, startTransition.Evidence.Path)
	receiver := selectorReceiver(startCall.Fun)
	if receiver == "" {
		return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle start has no named receiver")
	}

	callbackCall := callbackBodyCall(startCall)
	if callbackCall == nil {
		return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle start has no inline callback call")
	}
	callbackObject, callbackSelection, err := calledObject(info, callbackCall)
	if err != nil {
		return flowproof.Result{}, fmt.Errorf("gotypes: resolve lifecycle callback: %w", err)
	}
	callbackTarget := objectResolution(root, fileSet, callbackObject, callbackSelection, startTransition.Evidence.Path)

	cancelCall, waitCall := lifecycleTail(fileSet, info, function, startCall, receiver)
	if cancelCall == nil || waitCall == nil {
		return flowproof.Result{}, fmt.Errorf("gotypes: lifecycle tail is incomplete after %s", startObject.Name())
	}
	waitObject, waitSelection, err := calledObject(info, waitCall)
	if err != nil {
		return flowproof.Result{}, fmt.Errorf("gotypes: resolve lifecycle join: %w", err)
	}
	waitTarget := objectResolution(root, fileSet, waitObject, waitSelection, startTransition.Evidence.Path)

	startLocation := sourceLocation(fileSet, startCall.Fun.Pos(), startTransition.Evidence.Path)
	cancelLocation := sourceLocation(fileSet, cancelCall.Fun.Pos(), startTransition.Evidence.Path)
	waitLocation := sourceLocation(fileSet, waitCall.Fun.Pos(), startTransition.Evidence.Path)
	condition := cloneEvidenceCondition(startTransition.Condition)
	taskAnchor := flowproof.Anchor{
		ID:            sourceAnchorID("task", startLocation, callbackTarget.anchor.QualifiedName),
		Kind:          flowproof.AnchorTask,
		Label:         callbackTarget.anchor.Label + " task",
		QualifiedName: receiver + "." + startObject.Name() + ":" + callbackTarget.anchor.QualifiedName,
		Location:      &startLocation,
	}
	cancelAnchor := flowproof.Anchor{
		ID:       sourceAnchorID("cancel", cancelLocation, callName(cancelCall.Fun)),
		Kind:     flowproof.AnchorOperation,
		Label:    callName(cancelCall.Fun),
		Location: &cancelLocation,
	}
	waitAnchor := flowproof.Anchor{
		ID:       sourceAnchorID("wait", waitLocation, callName(waitCall.Fun)),
		Kind:     flowproof.AnchorOperation,
		Label:    callName(waitCall.Fun),
		Location: &waitLocation,
	}

	registrationID := lifecycleTransitionID("registration", startTransition.ID, startTarget.anchor.ID)
	callbackID := lifecycleTransitionID("callback", startTransition.ID, callbackTarget.anchor.ID)
	cancelCallID := lifecycleTransitionID("cancel-call", startTransition.ID, cancelAnchor.ID)
	waitCallID := lifecycleTransitionID("wait-call", startTransition.ID, waitAnchor.ID)
	joinID := lifecycleTransitionID("join", startTransition.ID, taskAnchor.ID)
	newAnchors := []flowproof.Anchor{startTarget.anchor, taskAnchor, callbackTarget.anchor, cancelAnchor, waitAnchor}
	newTransitions := []flowproof.Transition{
		{
			ID: registrationID, From: startTransition.From, To: startTarget.anchor.ID,
			Relation: evidence.RelationCalls, Resolution: startTarget.resolution,
			Invocation: evidence.InvocationSynchronous, Condition: cloneEvidenceCondition(condition), Certainty: evidence.CertaintyStatic,
			Evidence: startLocation, Provider: providerName,
		},
		{
			ID: callbackID, From: taskAnchor.ID, To: callbackTarget.anchor.ID,
			Relation: evidence.RelationCallback, Resolution: callbackTarget.resolution,
			Invocation: evidence.InvocationGoroutine, Condition: cloneEvidenceCondition(condition), Certainty: evidence.CertaintyStatic,
			Evidence: sourceLocation(fileSet, callbackCall.Fun.Pos(), startTransition.Evidence.Path), Provider: providerName,
		},
		{
			ID: cancelCallID, From: startTransition.From, To: cancelAnchor.ID,
			Relation: evidence.RelationCalls, Resolution: evidence.ResolutionStatic,
			Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
			Evidence: cancelLocation, Provider: providerName,
		},
		{
			ID: waitCallID, From: startTransition.From, To: waitAnchor.ID,
			Relation: evidence.RelationCalls, Resolution: waitTarget.resolution,
			Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
			Evidence: waitLocation, Provider: providerName,
		},
		{
			ID: joinID, From: waitAnchor.ID, To: taskAnchor.ID,
			Relation: evidence.RelationJoins, Resolution: evidence.ResolutionStatic,
			Invocation: evidence.InvocationSynchronous, Condition: cloneEvidenceCondition(condition), Certainty: evidence.CertaintyStatic,
			Evidence: waitLocation, Provider: providerName,
		},
	}
	warnings := []string{}
	if cancelTarget, ok := cancellationTarget(fileSet, info, function, cancelCall, startTransition.Evidence.Path); ok {
		cancelID := lifecycleTransitionID("cancel", startTransition.ID, cancelTarget.ID)
		newAnchors = append(newAnchors, cancelTarget)
		newTransitions = append(newTransitions, flowproof.Transition{
			ID: cancelID, From: cancelAnchor.ID, To: cancelTarget.ID,
			Relation: evidence.RelationCancels, Resolution: evidence.ResolutionStatic,
			Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
			Evidence: cancelLocation, Provider: providerName,
		})
		if useLocation, used := directArgumentUse(fileSet, callbackCall, cancelTarget.Label, startTransition.Evidence.Path); used {
			newTransitions = append(newTransitions, flowproof.Transition{
				ID:   lifecycleTransitionID("uses-cancellation", startTransition.ID, cancelTarget.ID),
				From: taskAnchor.ID, To: cancelTarget.ID,
				Relation: evidence.RelationUsesCancellation, Resolution: evidence.ResolutionStatic,
				Invocation: evidence.InvocationUnknown, Condition: cloneEvidenceCondition(condition), Certainty: evidence.CertaintyStatic,
				Evidence: useLocation, Provider: providerName,
			})
		} else {
			warnings = append(warnings, fmt.Sprintf(
				"scanner task does not directly expose cancellation target %s", cancelTarget.Label,
			))
		}
	} else {
		warnings = append(warnings, fmt.Sprintf(
			"cancellation target for %s at %s:%d is unresolved",
			callName(cancelCall.Fun), cancelLocation.Path, cancelLocation.Line,
		))
	}

	files := []string{startTransition.Evidence.Path}
	for _, target := range []resolution{startTarget, callbackTarget, waitTarget} {
		files = append(files, target.files...)
	}
	return flowproof.Result{
		TaskKey:        task.Key,
		NewAnchors:     newAnchors,
		NewTransitions: newTransitions,
		TransitionUpdates: []flowproof.TransitionUpdate{{
			TransitionID: startTransition.ID,
			Resolution:   evidence.ResolutionStatic,
			Certainty:    evidence.CertaintyStatic,
			Target:       &taskAnchor,
		}},
		Files:    uniqueStrings(files),
		Symbols:  uniqueStrings([]string{startTarget.anchor.QualifiedName, callbackTarget.anchor.QualifiedName, waitTarget.anchor.QualifiedName}),
		Warnings: warnings,
	}, nil
}

func cloneEvidenceCondition(condition *evidence.Condition) *evidence.Condition {
	if condition == nil {
		return nil
	}
	copy := *condition
	return &copy
}

func directArgumentUse(fileSet *token.FileSet, call *ast.CallExpr, name, fallbackPath string) (evidence.Location, bool) {
	for _, argument := range call.Args {
		identifier, ok := argument.(*ast.Ident)
		if !ok || identifier.Name != name {
			continue
		}
		return sourceLocation(fileSet, identifier.Pos(), fallbackPath), true
	}
	return evidence.Location{}, false
}

func containingFunction(file *ast.File, target ast.Node) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Body != nil && function.Body.Pos() <= target.Pos() && target.End() <= function.Body.End() {
			return function
		}
	}
	return nil
}

func selectorReceiver(expression ast.Expr) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func callbackBodyCall(start *ast.CallExpr) *ast.CallExpr {
	for _, argument := range start.Args {
		literal, ok := argument.(*ast.FuncLit)
		if !ok {
			continue
		}
		var found *ast.CallExpr
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			if found != nil {
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok {
				found = call
				return false
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}

func lifecycleTail(fileSet *token.FileSet, info *types.Info, function *ast.FuncDecl, start *ast.CallExpr, receiver string) (*ast.CallExpr, *ast.CallExpr) {
	startLine := fileSet.Position(start.Pos()).Line
	var cancelCall, waitCall *ast.CallExpr
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch node := node.(type) {
		case *ast.CallExpr:
			line := fileSet.Position(node.Pos()).Line
			if line <= startLine {
				return true
			}
			if cancelCall == nil && isCancelCall(info, node) {
				cancelCall = node
			}
			if waitCall == nil && isReceiverCall(node, receiver, "Wait") {
				waitCall = node
			}
		}
		return true
	})
	return cancelCall, waitCall
}

// cancellationTarget recognizes only the restic-shaped local binding
//
//	cancelCtx, cancel := context.WithCancel(parent)
//
// and returns cancelCtx as the target of a later cancel() call. This is a
// deliberately bounded same-function fact, not a data-flow analysis.
func cancellationTarget(fileSet *token.FileSet, info *types.Info, function *ast.FuncDecl, cancelCall *ast.CallExpr, fallbackPath string) (flowproof.Anchor, bool) {
	cancelIdentifier, ok := cancelCall.Fun.(*ast.Ident)
	if !ok {
		return flowproof.Anchor{}, false
	}
	var target *ast.Ident
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil || target != nil {
			return target == nil
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= cancelCall.Pos() || len(assignment.Rhs) != 1 {
			return true
		}
		constructor, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || callName(constructor.Fun) != "context.WithCancel" {
			return true
		}
		for index, expression := range assignment.Lhs {
			identifier, ok := expression.(*ast.Ident)
			if !ok || identifier.Name != cancelIdentifier.Name || index == 0 {
				continue
			}
			candidate, ok := assignment.Lhs[index-1].(*ast.Ident)
			if !ok || candidate.Name == "_" {
				continue
			}
			typeName := types.TypeString(info.TypeOf(candidate), func(pkg *types.Package) string { return pkg.Path() })
			if typeName != "context.Context" {
				continue
			}
			target = candidate
			return false
		}
		return true
	})
	if target == nil {
		return flowproof.Anchor{}, false
	}
	location := sourceLocation(fileSet, target.Pos(), fallbackPath)
	return flowproof.Anchor{
		ID:            sourceAnchorID("cancel-target", location, target.Name),
		Kind:          flowproof.AnchorOperation,
		Label:         target.Name,
		QualifiedName: function.Name.Name + "." + target.Name,
		Location:      &location,
	}, true
}

func isCancelCall(info *types.Info, call *ast.CallExpr) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok || !strings.HasPrefix(strings.ToLower(identifier.Name), "cancel") {
		return false
	}
	typeName := types.TypeString(info.TypeOf(identifier), func(pkg *types.Package) string { return pkg.Path() })
	return strings.Contains(typeName, "context.CancelFunc") || typeName == "func()"
}

func isReceiverCall(call *ast.CallExpr, receiver, method string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == receiver
}

func sourceLocation(fileSet *token.FileSet, position token.Pos, fallbackPath string) evidence.Location {
	resolved := fileSet.Position(position)
	return evidence.Location{Path: fallbackPath, Line: resolved.Line, Column: resolved.Column}
}

func sourceAnchorID(kind string, location evidence.Location, label string) string {
	return resolvedAnchorID(fmt.Sprintf("%s:%s:%d:%s", kind, location.Path, location.Line, label))
}

func lifecycleTransitionID(kind, startID, targetID string) string {
	return resolvedAnchorID(kind + ":" + startID + ":" + targetID)
}

func uniqueStrings(values []string) []string {
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
