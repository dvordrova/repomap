package surfacediscovery

import (
	"encoding/json"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	cobraPackagePath = "github.com/spf13/cobra"
	cobraCommandName = "Command"
	cobraProducer    = "deterministic_cobra"
)

type cobraLimits struct {
	packages         int
	files            int
	astNodes         int
	descriptors      int
	definitions      int
	functions        int
	constructorCalls int
	bindings         int
	bindingArgs      int
	activations      int
	mutations        int
	commands         int
	useBytes         int
	recordBytes      int
	resolveDepth     int
	diagnostics      int
}

func defaultCobraLimits() cobraLimits {
	return cobraLimits{
		packages: 2048, files: 2048, astNodes: 500_000, descriptors: 2048,
		definitions: 16_384, functions: 16_384, constructorCalls: 16_384,
		bindings: 4096, bindingArgs: 8192, activations: 256, mutations: 4096,
		commands: 512, useBytes: 4 * 1024, recordBytes: 2 * 1024 * 1024,
		resolveDepth: 16, diagnostics: 512,
	}
}

type cobraExpression struct {
	expr ast.Expr
	pkg  *packages.Package
}

type cobraDescriptor struct {
	location    Location
	packagePath string
	identity    Value
	handler     Value
	handlerSite *Location
	constructor *types.Func
	inventoryID string
	frontiers   []Frontier
}

type cobraFunction struct {
	pkg  *packages.Package
	body *ast.BlockStmt
}

type cobraRawBinding struct {
	receiver    cobraExpression
	children    []cobraExpression
	location    Location
	function    *types.Func
	conditional bool
	variadic    bool
}

type cobraRawActivation struct {
	receiver cobraExpression
	method   string
	location Location
	function *types.Func
}

type cobraRawMutation struct {
	receiver cobraExpression
	field    string
	location Location
}

type cobraSourceFile struct {
	pkg      *packages.Package
	file     *ast.File
	filename string
}

type cobraDiscovery struct {
	analyzer *analyzer
	limits   cobraLimits

	descriptors          map[*ast.CompositeLit]*cobraDescriptor
	descriptorList       []*cobraDescriptor
	definitions          map[*types.Var][]cobraExpression
	mutated              map[*types.Var]bool
	functions            map[*types.Func]cobraFunction
	constructorMemo      map[*types.Func]*cobraDescriptor
	constructorSeen      map[*types.Func]bool
	constructorActive    map[*types.Func]bool
	constructorCalls     map[*types.Func]map[string]struct{}
	ambiguousInstances   map[string]bool
	bindings             []cobraRawBinding
	activations          []cobraRawActivation
	mutations            []cobraRawMutation
	frontierSeen         map[string]struct{}
	budgetSeen           map[string]struct{}
	astNodeCount         int
	definitionCount      int
	bindingArgCount      int
	constructorCallCount int
}

func (a *analyzer) discoverCobraCommandInventory(limits cobraLimits) {
	if a == nil || a.program == nil || len(a.packageFacts) == 0 || a.ctx.Err() != nil {
		return
	}
	discovery := newCobraDiscovery(a, limits)
	discovery.scan()
	if a.ctx.Err() != nil {
		return
	}
	discovery.resolveAndPublish()
}

func newCobraDiscovery(a *analyzer, limits cobraLimits) *cobraDiscovery {
	return &cobraDiscovery{
		analyzer: a, limits: limits,
		descriptors:        make(map[*ast.CompositeLit]*cobraDescriptor),
		definitions:        make(map[*types.Var][]cobraExpression),
		mutated:            make(map[*types.Var]bool),
		functions:          make(map[*types.Func]cobraFunction),
		constructorMemo:    make(map[*types.Func]*cobraDescriptor),
		constructorSeen:    make(map[*types.Func]bool),
		constructorActive:  make(map[*types.Func]bool),
		constructorCalls:   make(map[*types.Func]map[string]struct{}),
		ambiguousInstances: make(map[string]bool),
		frontierSeen:       make(map[string]struct{}),
		budgetSeen:         make(map[string]struct{}),
	}
}

func (d *cobraDiscovery) scan() {
	stopped := false
	for _, source := range d.sourceFiles() {
		if stopped || d.analyzer.ctx.Err() != nil {
			break
		}
		for _, declaration := range source.file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				stopped = d.inspectNode(source.pkg, nil, declaration)
			case *ast.FuncDecl:
				function, _ := source.pkg.TypesInfo.Defs[declaration.Name].(*types.Func)
				if function != nil {
					if _, known := d.functions[function]; known {
						// The build-selected package load should expose a
						// declaration once, but duplicate syntax must not
						// consume the function budget.
					} else if len(d.functions) >= d.limits.functions {
						d.hitBudget("cobra_functions")
					} else {
						d.functions[function] = cobraFunction{
							pkg: source.pkg, body: declaration.Body,
						}
					}
				}
				stopped = d.inspectNode(source.pkg, function, declaration.Body)
			}
			if stopped {
				break
			}
		}
	}
	sort.Slice(d.descriptorList, func(i, j int) bool {
		return locationKey(d.descriptorList[i].location) <
			locationKey(d.descriptorList[j].location)
	})
	sort.Slice(d.bindings, func(i, j int) bool {
		return locationKey(d.bindings[i].location) < locationKey(d.bindings[j].location)
	})
	sort.Slice(d.activations, func(i, j int) bool {
		return locationKey(d.activations[i].location) <
			locationKey(d.activations[j].location)
	})
}

func (d *cobraDiscovery) sourceFiles() []cobraSourceFile {
	packageLimit := max(d.limits.packages, 0)
	packagePaths := make([]string, 0, packageLimit)
	eligiblePackages := 0
	for packagePath, pkg := range d.analyzer.packageFacts {
		if pkg != nil && pkg.TypesInfo != nil &&
			d.analyzer.isRepositoryPackagePath(packagePath) {
			eligiblePackages++
			packagePaths = insertSortedLimitedString(
				packagePaths,
				packagePath,
				packageLimit,
			)
		}
	}
	if eligiblePackages > len(packagePaths) {
		d.hitBudget("cobra_packages")
	}
	var result []cobraSourceFile
	dropped := false
	for packageIndex, packagePath := range packagePaths {
		pkg := d.analyzer.packageFacts[packagePath]
		remaining := max(d.limits.files-len(result), 0)
		files := make([]cobraSourceFile, 0, remaining)
		eligibleFiles := 0
		for _, file := range pkg.Syntax {
			if file == nil {
				continue
			}
			eligibleFiles++
			position := d.analyzer.program.Fset.PositionFor(file.Pos(), true)
			files = insertSortedLimitedSourceFile(
				files,
				cobraSourceFile{
					pkg: pkg, file: file, filename: position.Filename,
				},
				remaining,
			)
		}
		if eligibleFiles > len(files) {
			dropped = true
		}
		result = append(result, files...)
		if len(result) >= d.limits.files {
			dropped = dropped || packageIndex+1 < len(packagePaths)
			break
		}
	}
	if dropped {
		d.hitBudget("cobra_files")
	}
	return result
}

func insertSortedLimitedString(values []string, value string, limit int) []string {
	if limit <= 0 {
		return values
	}
	index := sort.SearchStrings(values, value)
	if index < len(values) && values[index] == value {
		return values
	}
	if len(values) < limit {
		values = append(values, "")
		copy(values[index+1:], values[index:])
		values[index] = value
		return values
	}
	if index >= limit {
		return values
	}
	copy(values[index+1:], values[index:limit-1])
	values[index] = value
	return values
}

func insertSortedLimitedSourceFile(
	values []cobraSourceFile,
	value cobraSourceFile,
	limit int,
) []cobraSourceFile {
	if limit <= 0 {
		return values
	}
	index := sort.Search(len(values), func(index int) bool {
		return values[index].filename >= value.filename
	})
	if index < len(values) && values[index].filename == value.filename {
		return values
	}
	if len(values) < limit {
		values = append(values, cobraSourceFile{})
		copy(values[index+1:], values[index:])
		values[index] = value
		return values
	}
	if index >= limit {
		return values
	}
	copy(values[index+1:], values[index:limit-1])
	values[index] = value
	return values
}

type cobraASTBudgetStop struct{}

// isNilASTNode detects a typed-nil ast.Node (e.g. a *ast.BlockStmt(nil) from
// a body-less FuncDecl). ast.Inspect would dereference it and panic; a plain
// interface == nil check does not catch the typed nil.
func isNilASTNode(node ast.Node) bool {
	value := reflect.ValueOf(node)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (d *cobraDiscovery) inspectNode(
	pkg *packages.Package,
	function *types.Func,
	node ast.Node,
) (stopped bool) {
	if node == nil || isNilASTNode(node) {
		return false
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(cobraASTBudgetStop); ok {
				stopped = true
				return
			}
			panic(recovered)
		}
	}()
	var conditionalStack []bool
	ast.Inspect(node, func(current ast.Node) bool {
		if current == nil {
			if len(conditionalStack) > 0 {
				conditionalStack = conditionalStack[:len(conditionalStack)-1]
			}
			return true
		}
		if d.astNodeCount >= d.limits.astNodes {
			d.hitBudget("cobra_ast_nodes")
			panic(cobraASTBudgetStop{})
		}
		d.astNodeCount++
		conditional := len(conditionalStack) > 0 &&
			conditionalStack[len(conditionalStack)-1]
		switch current.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
			*ast.TypeSwitchStmt, *ast.SelectStmt:
			conditional = true
		}
		conditionalStack = append(conditionalStack, conditional)
		switch current := current.(type) {
		case *ast.FuncLit:
			// ast.Inspect does not deliver a matching nil callback when the
			// visitor returns false. Pop the frame explicitly before scanning
			// the nested function with its own conditional scope.
			conditionalStack = conditionalStack[:len(conditionalStack)-1]
			if d.inspectNode(pkg, nil, current.Body) {
				panic(cobraASTBudgetStop{})
			}
			return false
		case *ast.CompositeLit:
			d.recordDescriptor(pkg, current)
		case *ast.ValueSpec:
			d.recordValueSpec(pkg, current)
		case *ast.AssignStmt:
			d.recordAssignment(pkg, current)
		case *ast.CallExpr:
			d.recordCall(pkg, function, current, conditional)
		}
		return true
	})
	return false
}

func (d *cobraDiscovery) recordDescriptor(
	pkg *packages.Package,
	literal *ast.CompositeLit,
) {
	if literal == nil || !isExactCobraCommandType(pkg.TypesInfo.TypeOf(literal)) ||
		d.descriptors[literal] != nil {
		return
	}
	if len(d.descriptorList) >= d.limits.descriptors {
		d.hitBudget("cobra_descriptors")
		return
	}
	location := d.analyzer.location(literal.Pos())
	descriptor := &cobraDescriptor{
		location: location, packagePath: pkg.PkgPath,
		identity: Value{
			Kind: "command_segment", Known: false, Candidates: []string{},
		},
		handler: dynamicValue("Run/RunE descriptor initializer unresolved"),
	}
	descriptor.inventoryID = stableInventoryID(
		"descriptor", cobraProducer, pkg.PkgPath, locationKey(location),
	)
	var handlers []struct {
		value    Value
		location Location
	}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		field, ok := pair.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch field.Name {
		case "Use":
			if value, ok := cobraConstantString(pkg.TypesInfo, pair.Value); ok &&
				(d.limits.useBytes <= 0 || len(value) <= d.limits.useBytes) {
				if fields := strings.Fields(value); len(fields) > 0 {
					descriptor.identity = Value{
						Kind: "command_segment", Text: fields[0], Known: true,
						Candidates: []string{fields[0]},
					}
				}
			} else if value != "" {
				d.hitBudget("cobra_use_bytes")
			}
		case "Run", "RunE":
			if handler, site, ok := d.handlerInitializer(
				cobraExpression{expr: pair.Value, pkg: pkg},
			); ok {
				handlers = append(handlers, struct {
					value    Value
					location Location
				}{value: handler, location: site})
			}
		}
	}
	if len(handlers) == 1 {
		descriptor.handler = handlers[0].value
		site := handlers[0].location
		descriptor.handlerSite = &site
	}
	d.descriptors[literal] = descriptor
	d.descriptorList = append(d.descriptorList, descriptor)
}

func (d *cobraDiscovery) recordValueSpec(pkg *packages.Package, spec *ast.ValueSpec) {
	if spec == nil || len(spec.Names) != len(spec.Values) {
		return
	}
	for index, name := range spec.Names {
		d.addDefinition(pkg, name, spec.Values[index])
	}
}

func (d *cobraDiscovery) recordAssignment(
	pkg *packages.Package,
	assignment *ast.AssignStmt,
) {
	if assignment == nil {
		return
	}
	for index, left := range assignment.Lhs {
		if selector, ok := left.(*ast.SelectorExpr); ok {
			d.recordFieldMutation(pkg, selector)
			continue
		}
		identifier, ok := left.(*ast.Ident)
		if !ok {
			continue
		}
		if assignment.Tok == token.DEFINE {
			if _, defined := pkg.TypesInfo.Defs[identifier].(*types.Var); defined &&
				index < len(assignment.Rhs) {
				d.addDefinition(pkg, identifier, assignment.Rhs[index])
				continue
			}
		}
		if variable, ok := expressionObject(pkg.TypesInfo, identifier).(*types.Var); ok &&
			isExactCobraCommandType(variable.Type()) {
			d.mutated[variable] = true
		}
	}
}

func (d *cobraDiscovery) recordFieldMutation(
	pkg *packages.Package,
	selector *ast.SelectorExpr,
) {
	if selector == nil ||
		!isExactCobraCommandType(pkg.TypesInfo.TypeOf(selector.X)) {
		return
	}
	switch selector.Sel.Name {
	case "Use", "Run", "RunE", "PreRun", "PreRunE":
	default:
		return
	}
	if d.limits.mutations > 0 && len(d.mutations) >= d.limits.mutations {
		d.hitBudget("cobra_mutations")
		return
	}
	d.mutations = append(d.mutations, cobraRawMutation{
		receiver: cobraExpression{expr: selector.X, pkg: pkg},
		field:    selector.Sel.Name,
		location: d.analyzer.location(selector.Sel.Pos()),
	})
}

func (d *cobraDiscovery) addDefinition(
	pkg *packages.Package,
	identifier *ast.Ident,
	value ast.Expr,
) {
	if identifier == nil || value == nil {
		return
	}
	variable, _ := pkg.TypesInfo.Defs[identifier].(*types.Var)
	if variable == nil || !isExactCobraCommandType(variable.Type()) {
		return
	}
	if d.definitionCount >= d.limits.definitions {
		d.hitBudget("cobra_definitions")
		return
	}
	d.definitions[variable] = append(
		d.definitions[variable],
		cobraExpression{expr: value, pkg: pkg},
	)
	d.definitionCount++
}

func (d *cobraDiscovery) recordCall(
	pkg *packages.Package,
	function *types.Func,
	call *ast.CallExpr,
	conditional bool,
) {
	callee := calledFunction(pkg.TypesInfo, call)
	if callee == nil {
		return
	}
	d.recordConstructorCall(callee, d.analyzer.location(call.Lparen))
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	location := d.analyzer.location(call.Lparen)
	switch {
	case isExactCobraMethod(callee, "AddCommand"):
		if len(d.bindings) >= d.limits.bindings {
			d.hitBudget("cobra_bindings")
			return
		}
		remaining := max(d.limits.bindingArgs-d.bindingArgCount, 0)
		argCount := min(len(call.Args), remaining)
		children := make([]cobraExpression, 0, argCount)
		for _, argument := range call.Args[:argCount] {
			children = append(children, cobraExpression{expr: argument, pkg: pkg})
		}
		d.bindingArgCount += argCount
		if argCount < len(call.Args) {
			d.hitBudget("cobra_binding_args")
		}
		if len(children) == 0 {
			return
		}
		d.bindings = append(d.bindings, cobraRawBinding{
			receiver: cobraExpression{expr: selector.X, pkg: pkg},
			children: children, location: location, function: function,
			conditional: conditional, variadic: call.Ellipsis.IsValid(),
		})
	case isExactCobraMethod(callee, "Execute", "ExecuteContext"):
		if len(d.activations) >= d.limits.activations {
			d.hitBudget("cobra_activations")
			return
		}
		d.activations = append(d.activations, cobraRawActivation{
			receiver: cobraExpression{expr: selector.X, pkg: pkg},
			method:   callee.Name(), location: location, function: function,
		})
	}
}

func (d *cobraDiscovery) recordConstructorCall(
	function *types.Func,
	location Location,
) {
	if !isCobraConstructorFunction(function) {
		return
	}
	key := locationKey(location)
	if _, exists := d.constructorCalls[function][key]; exists {
		return
	}
	if d.constructorCallCount >= d.limits.constructorCalls {
		d.hitBudget("cobra_constructor_calls")
		return
	}
	if d.constructorCalls[function] == nil {
		d.constructorCalls[function] = make(map[string]struct{})
	}
	d.constructorCalls[function][key] = struct{}{}
	d.constructorCallCount++
}

func (d *cobraDiscovery) resolveAndPublish() {
	d.assignConstructors()
	d.applyFieldMutations()
	facts := d.inventoryFacts()
	records := projectInventory(facts, func(context inventoryProjectionContext) TriggerRecord {
		return d.projectRecord(facts, context)
	})
	sort.Slice(records, func(i, j int) bool {
		left := records[i].ProcessEntrypoint.ID + "\x00" +
			records[i].Identity.Name + "\x00" +
			locationKey(records[i].RegistrationSite) + "\x00" + records[i].ID
		right := records[j].ProcessEntrypoint.ID + "\x00" +
			records[j].Identity.Name + "\x00" +
			locationKey(records[j].RegistrationSite) + "\x00" + records[j].ID
		return left < right
	})
	rawRecordCount := len(records)
	if len(records) > d.limits.commands {
		d.hitBudget("cobra_commands")
		records = records[:d.limits.commands]
	}
	bounded := make([]TriggerRecord, 0, len(records))
	bytes := 2
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		next := bytes + len(encoded)
		if len(bounded) > 0 {
			next++
		}
		if d.limits.recordBytes > 0 && next > d.limits.recordBytes {
			d.hitBudget("cobra_record_bytes")
			break
		}
		bounded = append(bounded, record)
		bytes = next
	}
	d.recordInventoryCoverage(facts, rawRecordCount, len(bounded))
	d.analyzer.result.Catalog.Triggers = append(
		d.analyzer.result.Catalog.Triggers,
		bounded...,
	)
}

func (d *cobraDiscovery) recordInventoryCoverage(
	facts inventoryFacts,
	rawRecordCount int,
	retainedRecordCount int,
) {
	coverage := &d.analyzer.result.Coverage
	coverage.CobraDescriptorCount = len(facts.Descriptors)
	seen := make(map[string]struct{})
	for _, binding := range facts.Bindings {
		key := "binding\x00" + binding.ID
		if _, duplicate := seen[key]; duplicate {
			coverage.CobraDuplicateRelationCount++
		} else {
			seen[key] = struct{}{}
		}
		if binding.Exact {
			coverage.CobraExactBindingCount++
		} else {
			coverage.CobraPartialRelationCount++
		}
	}
	for _, activation := range facts.Activations {
		key := "activation\x00" + activation.ID
		if _, duplicate := seen[key]; duplicate {
			coverage.CobraDuplicateRelationCount++
		} else {
			seen[key] = struct{}{}
		}
		if activation.Exact {
			coverage.CobraExactActivationCount++
		} else {
			coverage.CobraPartialRelationCount++
		}
	}
	coverage.CobraRecordCount = retainedRecordCount
	coverage.CobraDroppedRecordCount = max(rawRecordCount-retainedRecordCount, 0)
}

func (d *cobraDiscovery) inventoryFacts() inventoryFacts {
	facts := inventoryFacts{}
	for _, descriptor := range d.descriptorList {
		facts.Descriptors = append(facts.Descriptors, d.inventoryDescriptor(descriptor))
	}
	for _, raw := range d.bindings {
		parent := d.resolveExpression(raw.receiver, 0, make(map[*types.Var]bool))
		if len(raw.children) == 0 {
			continue
		}
		for _, childExpression := range raw.children {
			child := d.resolveExpression(childExpression, 0, make(map[*types.Var]bool))
			exact := parent != nil && child != nil && !raw.conditional && !raw.variadic
			frontiers := []Frontier{}
			if !exact {
				kind := "cobra_binding_unresolved"
				detail := "typed AddCommand relation is not a direct, unconditional single-definition binding"
				if raw.variadic {
					kind = "cobra_variadic_binding_unresolved"
					detail = "typed AddCommand variadic expansion is outside shallow inventory proof"
				}
				frontier := Frontier{Kind: kind, Detail: detail, Location: &raw.location}
				frontiers = append(frontiers, frontier)
				d.recordFrontier(frontier)
			}
			from := inventoryRef{}
			to := inventoryRef{}
			if parent != nil {
				from.DescriptorID = parent.inventoryID
			}
			if child != nil {
				to.DescriptorID = child.inventoryID
			}
			facts.Bindings = append(facts.Bindings, inventoryBinding{
				ID: stableInventoryID(
					"binding", locationKey(raw.location),
					locationKey(d.analyzer.location(childExpression.expr.Pos())),
					from.DescriptorID,
					to.DescriptorID,
				),
				Kind: "registers_child", From: from, To: to,
				Location: raw.location, Scope: d.symbolForTypesFunction(raw.function),
				Exact: exact,
				Evidence: []Evidence{{
					ID:   "cobra-registration:" + locationKey(raw.location),
					Kind: "cobra_add_command_call", Location: raw.location,
					Detail: "exact canonical Cobra AddCommand call",
				}},
				Provenance: []Provenance{d.inventoryProvenance("detect_binding")},
				Frontiers:  frontiers,
			})
		}
	}
	for _, raw := range d.activations {
		descriptor := d.resolveExpression(raw.receiver, 0, make(map[*types.Var]bool))
		exact := descriptor != nil
		ref := inventoryRef{}
		if descriptor != nil {
			ref.DescriptorID = descriptor.inventoryID
		}
		frontiers := []Frontier{}
		if !exact {
			frontier := Frontier{
				Kind:     "cobra_activation_unresolved",
				Detail:   "typed Execute receiver is not a direct single-definition command",
				Location: &raw.location,
			}
			frontiers = append(frontiers, frontier)
			d.recordFrontier(frontier)
		}
		facts.Activations = append(facts.Activations, inventoryActivation{
			ID: stableInventoryID(
				"activation", raw.method, locationKey(raw.location), ref.DescriptorID,
			),
			Kind: raw.method, Surface: ref, Location: raw.location,
			Scope: d.symbolForTypesFunction(raw.function), Exact: exact,
			Evidence: []Evidence{{
				ID:   "cobra-execute:" + locationKey(raw.location),
				Kind: "cobra_execute_call", Location: raw.location,
				Detail: "exact canonical Cobra " + raw.method + " call",
			}},
			Provenance: []Provenance{d.inventoryProvenance("detect_activation")},
			Frontiers:  frontiers,
		})
	}
	return facts
}

func (d *cobraDiscovery) inventoryDescriptor(
	descriptor *cobraDescriptor,
) inventoryDescriptor {
	constructor := d.symbolForTypesFunction(descriptor.constructor)
	evidence := []Evidence{{
		ID:   "cobra-descriptor:" + locationKey(descriptor.location),
		Kind: "cobra_command_descriptor", Location: descriptor.location,
		Detail: "exact canonical Cobra command composite literal",
	}}
	if constructor.ID != "" {
		evidence = append(evidence, Evidence{
			ID:   "cobra-constructor:" + locationKey(constructor.Location),
			Kind: "cobra_constructor_declaration", Location: constructor.Location,
			Detail: constructor.ID,
		})
	}
	if descriptor.handlerSite != nil {
		evidence = append(evidence, Evidence{
			ID:   "cobra-handler:" + locationKey(*descriptor.handlerSite),
			Kind: "cobra_handler_declaration", Location: *descriptor.handlerSite,
			Detail: descriptor.handler.Text,
		})
	}
	return inventoryDescriptor{
		ID: descriptor.inventoryID, Kind: "cli_command", Framework: "cobra",
		Package: descriptor.packagePath, Location: descriptor.location,
		Identity: descriptor.identity, Handler: descriptor.handler,
		HandlerLocation: descriptor.handlerSite, Constructor: constructor,
		InstanceCorrelationAmbiguous: d.ambiguousInstances[descriptor.inventoryID],
		Evidence:                     evidence,
		Provenance:                   []Provenance{d.inventoryProvenance("detect_descriptor")},
		Frontiers:                    append([]Frontier(nil), descriptor.frontiers...),
	}
}

func (d *cobraDiscovery) projectRecord(
	facts inventoryFacts,
	context inventoryProjectionContext,
) TriggerRecord {
	descriptor := context.Descriptor
	identity := descriptor.Identity
	if identity.Text == "" {
		identity.Text = d.derivedDescriptorIdentity(descriptor)
		identity.Candidates = []string{identity.Text}
	}
	basis := "build_selected_typed_cobra_descriptor"
	status := "partial_cobra_descriptor"
	finalSeed := "cobra-command-descriptor"
	registration := Location{}
	dispatcher := dynamicValue("no exact shallow registration or activation")
	evidence := append([]Evidence(nil), descriptor.Evidence...)
	evidence = append(evidence, context.RelatedEvidence...)
	provenance := append([]Provenance(nil), descriptor.Provenance...)
	provenance = append(provenance, context.RelatedProvenance...)
	promotion := PromotionNone
	process := Symbol{}
	if context.Activation != nil {
		basis = "build_selected_typed_cobra_activation"
		status = "partial_cobra_activation"
		finalSeed = "cobra-command-activation"
		evidence = append(evidence, context.Activation.Evidence...)
		provenance = append(provenance, context.Activation.Provenance...)
		process = d.directProcessEntrypoint(context.Activation.Scope)
	}
	if context.Binding != nil {
		basis = "build_selected_typed_cobra_binding"
		status = "partial_cobra_registration"
		finalSeed = "cobra-command-binding"
		registration = context.Binding.Location
		dispatcher = symbolValue(context.Binding.Scope)
		evidence = append(evidence, context.Binding.Evidence...)
		provenance = append(provenance, context.Binding.Provenance...)
		promotion = PromotionRepositoryRegistration
		if path, frontier := inventoryCommandPath(
			facts,
			descriptor.ID,
			d.limits.resolveDepth,
		); path.Known {
			identity = path
		} else if frontier != nil {
			identity.Known = false
			context.Frontiers = append(context.Frontiers, *frontier)
		}
	}
	frontiers := append([]Frontier(nil), context.Frontiers...)
	frontiers = append(frontiers, Frontier{
		Kind:     "shallow_inventory_no_dispatch_proof",
		Detail:   "descriptor and direct relation facts do not prove process-to-effect dispatch; that proof is deferred to Mechanism",
		Location: &descriptor.Location,
	})
	path := identity
	if context.Binding == nil && context.Activation == nil {
		path.Known = false
	}
	record := TriggerRecord{
		Kind: "cli_command", Producer: cobraProducer,
		Identity:  Identity{Name: identity.Text, Path: path},
		Transport: "cli", Framework: "cobra",
		ProcessEntrypoint: process, Dispatcher: dispatcher,
		Constructor: descriptor.Constructor, RegistrationSite: registration,
		DescriptorSite: &descriptor.Location,
		Handler:        descriptor.Handler, HandlerLocation: descriptor.HandlerLocation,
		Middleware: []Value{}, WrapperChain: []Wrapper{},
		FinalSeed: finalSeed, DiscoveryBasis: basis,
		Certainty: "static", Resolution: "partial",
		ScenarioID: d.analyzer.scenario.ID,
		Evidence:   evidence, Provenance: provenance,
		DynamicFrontier: frontiers, Status: status,
		Availability:        AvailabilityAvailable,
		TerminalSourceScope: "repository",
		ApplicationClass:    ApplicationSurface, PromotionBasis: promotion,
	}
	record.ProvisionalID = !path.Known
	record.ID = stableInventoryTriggerID(descriptor.ID, record.ScenarioID)
	return record
}

func inventoryDescriptorByID(
	descriptors []inventoryDescriptor,
	id string,
) *inventoryDescriptor {
	for index := range descriptors {
		if descriptors[index].ID == id {
			return &descriptors[index]
		}
	}
	return nil
}

func inventoryCommandPath(
	facts inventoryFacts,
	descriptorID string,
	maxDepth int,
) (Value, *Frontier) {
	descriptor := inventoryDescriptorByID(facts.Descriptors, descriptorID)
	if descriptor == nil {
		return Value{}, nil
	}
	path := descriptor.Identity
	segments, exact, frontier := inventoryCommandPathSegments(
		facts,
		descriptorID,
		true,
		maxDepth,
		make(map[string]bool),
	)
	if !exact {
		path.Known = false
		return path, frontier
	}
	path.Text = strings.Join(segments, " ")
	path.Known = path.Text != ""
	path.Candidates = []string{path.Text}
	return path, nil
}

func inventoryCommandPathSegments(
	facts inventoryFacts,
	descriptorID string,
	requireBinding bool,
	depth int,
	active map[string]bool,
) ([]string, bool, *Frontier) {
	descriptor := inventoryDescriptorByID(facts.Descriptors, descriptorID)
	if descriptor == nil || !descriptor.Identity.Known ||
		strings.TrimSpace(descriptor.Identity.Text) == "" {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"command segment is not an exact declared constant",
		)
	}
	if descriptor.InstanceCorrelationAmbiguous {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"descriptor has multiple constructor invocation sites; a specific runtime instance cannot be selected",
		)
	}
	if depth < 0 {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"unique binding chain exceeds the shallow resolution limit",
		)
	}
	if active[descriptorID] {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"unique binding chain contains a cycle",
		)
	}
	active[descriptorID] = true
	defer delete(active, descriptorID)

	incoming := inventoryExactBindingsTo(facts.Bindings, descriptorID)
	if len(incoming) == 0 {
		if requireBinding {
			return nil, false, inventoryPathFrontier(
				descriptor,
				"descriptor has no unique exact incoming binding",
			)
		}
		return []string{descriptor.Identity.Text}, true, nil
	}
	if len(incoming) != 1 {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"descriptor has multiple exact incoming bindings",
		)
	}
	parentID := incoming[0].From.DescriptorID
	parent := inventoryDescriptorByID(facts.Descriptors, parentID)
	if parent == nil {
		return nil, false, inventoryPathFrontier(
			descriptor,
			"exact incoming binding has no known parent descriptor",
		)
	}
	if parent.InstanceCorrelationAmbiguous {
		return nil, false, inventoryPathFrontier(
			parent,
			"parent descriptor has multiple constructor invocation sites; its binding and activation facts cannot be correlated",
		)
	}
	if inventoryHasActivation(facts.Activations, parentID) {
		return []string{descriptor.Identity.Text}, true, nil
	}
	parentSegments, exact, frontier := inventoryCommandPathSegments(
		facts,
		parentID,
		false,
		depth-1,
		active,
	)
	if !exact {
		return nil, false, frontier
	}
	return append(parentSegments, descriptor.Identity.Text), true, nil
}

func inventoryExactBindingsTo(
	bindings []inventoryBinding,
	descriptorID string,
) []inventoryBinding {
	var result []inventoryBinding
	seen := make(map[string]bool)
	for _, binding := range bindings {
		if binding.Exact && binding.To.DescriptorID == descriptorID &&
			!seen[binding.ID] {
			seen[binding.ID] = true
			result = append(result, binding)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func inventoryPathFrontier(
	descriptor *inventoryDescriptor,
	detail string,
) *Frontier {
	frontier := &Frontier{Kind: "inventory_command_path_unresolved", Detail: detail}
	if descriptor != nil {
		location := descriptor.Location
		frontier.Location = &location
	}
	return frontier
}

func inventoryHasActivation(activations []inventoryActivation, id string) bool {
	for _, activation := range activations {
		if activation.Exact && activation.Surface.DescriptorID == id {
			return true
		}
	}
	return false
}

func symbolValue(symbol Symbol) Value {
	if symbol.ID == "" {
		return dynamicValue("containing function unresolved")
	}
	return knownValue("function", symbol.ID)
}

func stableInventoryTriggerID(descriptorID, scenarioID string) string {
	id := stableInventoryID("trigger", descriptorID, scenarioID)
	return "trigger-" + strings.TrimPrefix(id, "inventory-")
}

func (d *cobraDiscovery) directProcessEntrypoint(scope Symbol) Symbol {
	if scope.ID == "" {
		return Symbol{}
	}
	for _, entrypoint := range d.analyzer.entrypoints() {
		candidate := d.analyzer.symbol(entrypoint)
		if candidate.ID == scope.ID {
			return candidate
		}
	}
	return Symbol{}
}

func (d *cobraDiscovery) assignConstructors() {
	functions := make([]*types.Func, 0, len(d.functions))
	for function := range d.functions {
		functions = append(functions, function)
	}
	sort.Slice(functions, func(i, j int) bool {
		return d.typesFunctionID(functions[i]) < d.typesFunctionID(functions[j])
	})
	owners := make(map[*cobraDescriptor][]*types.Func)
	for _, function := range functions {
		if descriptor := d.constructorDescriptor(function, 0); descriptor != nil {
			owners[descriptor] = append(owners[descriptor], function)
		}
	}
	for descriptor, constructors := range owners {
		if len(constructors) == 1 {
			descriptor.constructor = constructors[0]
		}
	}
	invocationSites := make(map[*cobraDescriptor]map[string]struct{})
	for function, sites := range d.constructorCalls {
		descriptor := d.constructorDescriptor(function, 0)
		if descriptor == nil {
			continue
		}
		if invocationSites[descriptor] == nil {
			invocationSites[descriptor] = make(map[string]struct{})
		}
		for site := range sites {
			invocationSites[descriptor][site] = struct{}{}
		}
	}
	for descriptor, sites := range invocationSites {
		if len(sites) <= 1 {
			continue
		}
		d.ambiguousInstances[descriptor.inventoryID] = true
		frontier := Frontier{
			Kind: "cobra_constructor_instance_correlation_ambiguous",
			Detail: "the same command descriptor is instantiated at multiple constructor callsites; " +
				"relations remain local facts and are not composed across instances",
			Location: &descriptor.location,
		}
		descriptor.frontiers = append(descriptor.frontiers, frontier)
		d.recordFrontier(frontier)
	}
}

func (d *cobraDiscovery) applyFieldMutations() {
	for _, mutation := range d.mutations {
		descriptor := d.resolveMutationExpression(
			mutation.receiver,
			0,
			make(map[*types.Var]bool),
		)
		if descriptor == nil {
			d.recordFrontier(Frontier{
				Kind:     "cobra_field_mutation_unresolved",
				Detail:   mutation.field + " assignment receiver is outside shallow resolution",
				Location: &mutation.location,
			})
			continue
		}
		frontier := Frontier{
			Kind: "cobra_descriptor_field_mutated",
			Detail: mutation.field +
				" has a separate assignment; the descriptor initializer is only a declared candidate",
			Location: &mutation.location,
		}
		descriptor.frontiers = append(descriptor.frontiers, frontier)
		d.recordFrontier(frontier)
		switch mutation.field {
		case "Use":
			descriptor.identity.Known = false
			if descriptor.identity.Text != "" &&
				len(descriptor.identity.Candidates) == 0 {
				descriptor.identity.Candidates = []string{descriptor.identity.Text}
			}
		case "Run", "RunE":
			wasKnown := descriptor.handler.Known
			descriptor.handler.Known = false
			if wasKnown && descriptor.handler.Text != "" &&
				len(descriptor.handler.Candidates) == 0 {
				descriptor.handler.Candidates = []string{descriptor.handler.Text}
			}
		}
	}
}

func (d *cobraDiscovery) resolveMutationExpression(
	expression cobraExpression,
	depth int,
	activeVariables map[*types.Var]bool,
) *cobraDescriptor {
	if expression.expr == nil || expression.pkg == nil ||
		depth > d.limits.resolveDepth ||
		!isExactCobraCommandType(expression.pkg.TypesInfo.TypeOf(expression.expr)) {
		return nil
	}
	switch value := expression.expr.(type) {
	case *ast.CompositeLit:
		return d.descriptors[value]
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return d.resolveMutationExpression(
				cobraExpression{expr: value.X, pkg: expression.pkg},
				depth+1,
				activeVariables,
			)
		}
	case *ast.ParenExpr:
		return d.resolveMutationExpression(
			cobraExpression{expr: value.X, pkg: expression.pkg},
			depth+1,
			activeVariables,
		)
	case *ast.Ident:
		variable, _ := expressionObject(expression.pkg.TypesInfo, value).(*types.Var)
		return d.resolveMutationVariable(variable, depth+1, activeVariables)
	case *ast.SelectorExpr:
		variable, _ := selectorObject(expression.pkg.TypesInfo, value).(*types.Var)
		return d.resolveMutationVariable(variable, depth+1, activeVariables)
	}
	// A constructor call creates a fresh runtime instance. Crossing it would
	// incorrectly apply an instance mutation to the shared declaration-site
	// descriptor returned by that constructor.
	return nil
}

func (d *cobraDiscovery) resolveMutationVariable(
	variable *types.Var,
	depth int,
	active map[*types.Var]bool,
) *cobraDescriptor {
	if variable == nil || active[variable] || d.mutated[variable] ||
		len(d.definitions[variable]) != 1 {
		return nil
	}
	active[variable] = true
	defer delete(active, variable)
	return d.resolveMutationExpression(d.definitions[variable][0], depth+1, active)
}

func (d *cobraDiscovery) resolveExpression(
	expression cobraExpression,
	depth int,
	activeVariables map[*types.Var]bool,
) *cobraDescriptor {
	if expression.expr == nil || expression.pkg == nil ||
		depth > d.limits.resolveDepth ||
		!isExactCobraCommandType(expression.pkg.TypesInfo.TypeOf(expression.expr)) {
		return nil
	}
	switch value := expression.expr.(type) {
	case *ast.CompositeLit:
		return d.descriptors[value]
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return d.resolveExpression(
				cobraExpression{expr: value.X, pkg: expression.pkg},
				depth+1, activeVariables,
			)
		}
	case *ast.ParenExpr:
		return d.resolveExpression(
			cobraExpression{expr: value.X, pkg: expression.pkg},
			depth+1, activeVariables,
		)
	case *ast.Ident:
		variable, _ := expressionObject(expression.pkg.TypesInfo, value).(*types.Var)
		return d.resolveVariable(variable, depth+1, activeVariables)
	case *ast.SelectorExpr:
		variable, _ := selectorObject(expression.pkg.TypesInfo, value).(*types.Var)
		return d.resolveVariable(variable, depth+1, activeVariables)
	case *ast.CallExpr:
		return d.constructorDescriptor(
			calledFunction(expression.pkg.TypesInfo, value),
			depth+1,
		)
	}
	return nil
}

func (d *cobraDiscovery) resolveVariable(
	variable *types.Var,
	depth int,
	active map[*types.Var]bool,
) *cobraDescriptor {
	if variable == nil || active[variable] || d.mutated[variable] ||
		len(d.definitions[variable]) != 1 {
		return nil
	}
	active[variable] = true
	defer delete(active, variable)
	return d.resolveExpression(d.definitions[variable][0], depth+1, active)
}

func (d *cobraDiscovery) constructorDescriptor(
	function *types.Func,
	depth int,
) *cobraDescriptor {
	if function == nil || depth > d.limits.resolveDepth {
		return nil
	}
	if d.constructorSeen[function] {
		return d.constructorMemo[function]
	}
	if d.constructorActive[function] {
		return nil
	}
	d.constructorActive[function] = true
	defer delete(d.constructorActive, function)
	d.constructorSeen[function] = true
	info, exists := d.functions[function]
	if !exists || info.body == nil {
		return nil
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Results().Len() != 1 ||
		!isExactCobraCommandType(signature.Results().At(0).Type()) {
		return nil
	}
	var returned *ast.ReturnStmt
	for _, statement := range info.body.List {
		if current, ok := statement.(*ast.ReturnStmt); ok {
			if returned != nil {
				return nil
			}
			returned = current
		}
	}
	if returned == nil || len(returned.Results) != 1 {
		return nil
	}
	descriptor := d.resolveExpression(
		cobraExpression{expr: returned.Results[0], pkg: info.pkg},
		depth+1, make(map[*types.Var]bool),
	)
	d.constructorMemo[function] = descriptor
	return descriptor
}

func (d *cobraDiscovery) derivedDescriptorIdentity(
	descriptor inventoryDescriptor,
) string {
	if descriptor.Constructor.Name != "" {
		name := strings.TrimSuffix(
			strings.TrimPrefix(descriptor.Constructor.Name, "New"),
			"Command",
		)
		name = strings.TrimSuffix(strings.TrimPrefix(name, "new"), "Command")
		if name != "" {
			return strings.ToLower(name[:1]) + name[1:]
		}
	}
	return "command@" + locationKey(descriptor.Location)
}

func (d *cobraDiscovery) handlerInitializer(
	expression cobraExpression,
) (Value, Location, bool) {
	switch value := expression.expr.(type) {
	case *ast.Ident:
		function, _ := expression.pkg.TypesInfo.Uses[value].(*types.Func)
		symbol := d.symbolForTypesFunction(function)
		return knownValue("function", symbol.ID), symbol.Location, symbol.ID != ""
	case *ast.SelectorExpr:
		function, _ := selectorObject(expression.pkg.TypesInfo, value).(*types.Func)
		symbol := d.symbolForTypesFunction(function)
		return knownValue("function", symbol.ID), symbol.Location, symbol.ID != ""
	case *ast.FuncLit:
		location := d.analyzer.location(value.Pos())
		id := expression.pkg.PkgPath + ".func_literal@" + locationKey(location)
		return knownValue("function", id), location, true
	default:
		return Value{}, Location{}, false
	}
}

func (d *cobraDiscovery) symbolForTypesFunction(function *types.Func) Symbol {
	if function == nil {
		return Symbol{}
	}
	if d.analyzer.program != nil {
		if typed := d.analyzer.program.FuncValue(function); typed != nil {
			return d.analyzer.symbol(typed)
		}
	}
	packagePath := ""
	if function.Pkg() != nil {
		packagePath = function.Pkg().Path()
	}
	return Symbol{
		ID: d.typesFunctionID(function), Package: packagePath,
		Name: function.Name(), Location: d.analyzer.location(function.Pos()),
	}
}

func (d *cobraDiscovery) typesFunctionID(function *types.Func) string {
	if function == nil {
		return ""
	}
	packagePath := "synthetic"
	if function.Pkg() != nil {
		packagePath = function.Pkg().Path()
	}
	signature, _ := function.Type().(*types.Signature)
	if receiver := receiverName(signature); receiver != "" {
		return packagePath + ".(" + receiver + ")." + function.Name()
	}
	return packagePath + "." + function.Name()
}

func (d *cobraDiscovery) inventoryProvenance(operation string) Provenance {
	return Provenance{
		Provider: "go_types_ast", Version: AnalyzerVersion, Operation: operation,
		Detail: "exact canonical framework types with shallow local proof only",
	}
}

func (d *cobraDiscovery) recordFrontier(frontier Frontier) {
	location := ""
	if frontier.Location != nil {
		location = locationKey(*frontier.Location)
	}
	key := frontier.Kind + "\x00" + frontier.Detail + "\x00" + location
	if _, duplicate := d.frontierSeen[key]; duplicate {
		return
	}
	if len(d.frontierSeen) >= d.limits.diagnostics {
		d.hitBudget("cobra_diagnostics")
		return
	}
	d.frontierSeen[key] = struct{}{}
	d.analyzer.result.Coverage.UnsupportedDispatch = append(
		d.analyzer.result.Coverage.UnsupportedDispatch,
		frontier,
	)
}

func (d *cobraDiscovery) hitBudget(name string) {
	if name == "" {
		return
	}
	if _, duplicate := d.budgetSeen[name]; duplicate {
		return
	}
	d.budgetSeen[name] = struct{}{}
	d.analyzer.addBudget(name)
}

func (a *analyzer) isRepositoryPackagePath(packagePath string) bool {
	for modulePath := range a.modulePaths {
		if packagePath == modulePath || strings.HasPrefix(packagePath, modulePath+"/") {
			return true
		}
	}
	return false
}

func isExactCobraCommandType(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj() != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == cobraPackagePath &&
		named.Obj().Name() == cobraCommandName
}

func isCobraConstructorFunction(function *types.Func) bool {
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil &&
		signature.Results().Len() == 1 &&
		isExactCobraCommandType(signature.Results().At(0).Type())
}

func isExactCobraMethod(function *types.Func, names ...string) bool {
	if function == nil || function.Pkg() == nil ||
		function.Pkg().Path() != cobraPackagePath {
		return false
	}
	matched := false
	for _, name := range names {
		if function.Name() == name {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Recv() != nil &&
		isExactCobraCommandType(signature.Recv().Type())
}

func calledFunction(info *types.Info, call *ast.CallExpr) *types.Func {
	if info == nil || call == nil {
		return nil
	}
	switch function := call.Fun.(type) {
	case *ast.Ident:
		result, _ := info.Uses[function].(*types.Func)
		return result
	case *ast.SelectorExpr:
		result, _ := selectorObject(info, function).(*types.Func)
		return result
	default:
		return nil
	}
}

func selectorObject(info *types.Info, selector *ast.SelectorExpr) types.Object {
	if info == nil || selector == nil {
		return nil
	}
	if selection := info.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return info.Uses[selector.Sel]
}

func expressionObject(info *types.Info, identifier *ast.Ident) types.Object {
	if info == nil || identifier == nil {
		return nil
	}
	if defined := info.Defs[identifier]; defined != nil {
		return defined
	}
	return info.Uses[identifier]
}

func cobraConstantString(
	info *types.Info,
	expression ast.Expr,
) (string, bool) {
	if info == nil || expression == nil {
		return "", false
	}
	value := info.Types[expression].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}
