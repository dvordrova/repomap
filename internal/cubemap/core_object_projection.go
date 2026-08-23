package cubemap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/token"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/gocoreobject"
)

const CoreObjectProjectionVersion = 1

type CoreObjectBindingRole string

const (
	CoreObjectRepresentativeCallable CoreObjectBindingRole = "representative_callable"
	CoreObjectReceiverType           CoreObjectBindingRole = "receiver_type"
)

func (role CoreObjectBindingRole) valid() bool {
	return role == CoreObjectRepresentativeCallable || role == CoreObjectReceiverType
}

// CoreObjectBinding joins one model-named responsibility to an exact local Go
// declaration. ObjectID names either a Callables or ReceiverTypes row. The
// projection never asks a model to reproduce either identity.
type CoreObjectBinding struct {
	CoreBlockID string                `json:"core_block_id"`
	ObjectID    string                `json:"object_id"`
	Role        CoreObjectBindingRole `json:"role"`
}

// CoreObjectProjectionCoverage makes every deliberately absent join visible.
// RepresentativeSymbolClaims counts block-local claims, while
// RepresentativeNodesObserved and all projected-object counts are unique.
type CoreObjectProjectionCoverage struct {
	CoreBlocksObserved             int `json:"core_blocks_observed"`
	RepresentativeSymbolClaims     int `json:"representative_symbol_claims"`
	RepresentativeNodesObserved    int `json:"representative_nodes_observed"`
	RepresentativeCallablesMatched int `json:"representative_callables_matched"`
	RepresentativeNodesUnmatched   int `json:"representative_nodes_unmatched"`
	CallableBindings               int `json:"callable_bindings"`
	ReceiverMethodsObserved        int `json:"receiver_methods_observed"`
	ReceiverTypesMatched           int `json:"receiver_types_matched"`
	ReceiverMethodsOmitted         int `json:"receiver_methods_omitted"`
	GenericReceiverMethodsOmitted  int `json:"generic_receiver_methods_omitted"`
	ReceiverTypeBindings           int `json:"receiver_type_bindings"`
}

// CoreObjectProjection is the small browser-safe subset of the exact
// target-scoped Go object index used by the current CoreMap. It deliberately
// does not inline packages or declarations that the map did not select.
type CoreObjectProjection struct {
	Version               int                                `json:"version"`
	CoreObjectIndexSHA256 string                             `json:"core_object_index_sha256"`
	Callables             []gocoreobject.CallableDeclaration `json:"callables"`
	ReceiverTypes         []gocoreobject.TypeDeclaration     `json:"receiver_types"`
	Bindings              []CoreObjectBinding                `json:"bindings"`
	Coverage              CoreObjectProjectionCoverage       `json:"coverage"`
	SHA256                string                             `json:"sha256"`
}

// compileCoreObjectProjection performs exact identity joins only. It neither
// reads source nor invokes a model. A representative symbol without a unique
// DirectCallNodeID match remains visible in coverage and is not repaired by
// name or path.
func compileCoreObjectProjection(core coremap.Result, index gocoreobject.Index) (CoreObjectProjection, error) {
	if err := core.Validate(); err != nil {
		return CoreObjectProjection{}, fmt.Errorf("cubemap: core object projection: core map: %w", err)
	}
	if err := index.Validate(); err != nil {
		return CoreObjectProjection{}, fmt.Errorf("cubemap: core object projection: index: %w", err)
	}
	if core.Target.Ref != index.Scope.TargetRef {
		return CoreObjectProjection{}, fmt.Errorf("cubemap: core object projection: target authority mismatch")
	}
	if core.CoreObjectSHA256 != index.SHA256 {
		return CoreObjectProjection{}, fmt.Errorf("cubemap: core object projection: source authority mismatch")
	}
	return projectCoreObjects(core.Refined, index)
}

func projectCoreObjects(blocks []coremap.Block, index gocoreobject.Index) (CoreObjectProjection, error) {
	callableByNode := make(map[string]gocoreobject.CallableDeclaration, len(index.Callables))
	ambiguousNodes := make(map[string]struct{})
	for _, callable := range index.Callables {
		if callable.DirectCallNodeID == "" {
			continue
		}
		if _, exists := callableByNode[callable.DirectCallNodeID]; exists {
			delete(callableByNode, callable.DirectCallNodeID)
			ambiguousNodes[callable.DirectCallNodeID] = struct{}{}
			continue
		}
		if _, ambiguous := ambiguousNodes[callable.DirectCallNodeID]; !ambiguous {
			callableByNode[callable.DirectCallNodeID] = callable
		}
	}
	typesByQualifiedName := make(map[string][]gocoreobject.TypeDeclaration, len(index.Types))
	for _, declaration := range index.Types {
		key := declaration.Package + "\x00" + declaration.Name
		typesByQualifiedName[key] = append(typesByQualifiedName[key], declaration)
	}

	projection := CoreObjectProjection{
		Version: CoreObjectProjectionVersion, CoreObjectIndexSHA256: index.SHA256,
		Callables: []gocoreobject.CallableDeclaration{}, ReceiverTypes: []gocoreobject.TypeDeclaration{},
		Bindings: []CoreObjectBinding{},
	}
	projection.Coverage.CoreBlocksObserved = len(blocks)
	seenNodes := make(map[string]struct{})
	selectedCallables := make(map[string]gocoreobject.CallableDeclaration)
	callableBlocks := make(map[string]map[string]struct{})
	for _, block := range blocks {
		for _, symbol := range block.Symbols {
			projection.Coverage.RepresentativeSymbolClaims++
			seenNodes[symbol.NodeID] = struct{}{}
			callable, matched := callableByNode[symbol.NodeID]
			if !matched {
				continue
			}
			if callable.Package != symbol.Package || callable.Name != symbol.Symbol.Name ||
				callable.Location.Path != symbol.Declaration.Path || callable.Location.Line != symbol.Declaration.Line ||
				callable.Location.Column != symbol.Declaration.Column {
				return CoreObjectProjection{}, fmt.Errorf(
					"cubemap: core object projection: representative %q does not match its callable", symbol.NodeID,
				)
			}
			selectedCallables[callable.ID] = callable
			if callableBlocks[callable.ID] == nil {
				callableBlocks[callable.ID] = make(map[string]struct{})
			}
			callableBlocks[callable.ID][block.ID] = struct{}{}
		}
	}
	projection.Coverage.RepresentativeNodesObserved = len(seenNodes)
	projection.Coverage.RepresentativeCallablesMatched = len(selectedCallables)
	projection.Coverage.RepresentativeNodesUnmatched = len(seenNodes) - len(selectedCallables)

	for _, callable := range index.Callables {
		selected, exists := selectedCallables[callable.ID]
		if !exists {
			continue
		}
		projection.Callables = append(projection.Callables, selected)
		blockIDs := sortedSet(callableBlocks[callable.ID])
		for _, blockID := range blockIDs {
			projection.Bindings = append(projection.Bindings, CoreObjectBinding{
				CoreBlockID: blockID, ObjectID: callable.ID, Role: CoreObjectRepresentativeCallable,
			})
			projection.Coverage.CallableBindings++
		}
	}

	selectedTypes := make(map[string]gocoreobject.TypeDeclaration)
	typeBlocks := make(map[string]map[string]struct{})
	for _, callable := range projection.Callables {
		if callable.Kind != gocoreobject.CallableMethod {
			continue
		}
		projection.Coverage.ReceiverMethodsObserved++
		packagePath, typeName, generic, ok := exactReceiverName(callable.Receiver)
		if generic {
			projection.Coverage.GenericReceiverMethodsOmitted++
		}
		if !ok || packagePath != callable.Package {
			projection.Coverage.ReceiverMethodsOmitted++
			continue
		}
		matches := typesByQualifiedName[packagePath+"\x00"+typeName]
		if len(matches) != 1 {
			projection.Coverage.ReceiverMethodsOmitted++
			continue
		}
		declaration := matches[0]
		selectedTypes[declaration.ID] = declaration
		if typeBlocks[declaration.ID] == nil {
			typeBlocks[declaration.ID] = make(map[string]struct{})
		}
		for blockID := range callableBlocks[callable.ID] {
			typeBlocks[declaration.ID][blockID] = struct{}{}
		}
	}
	projection.Coverage.ReceiverTypesMatched = len(selectedTypes)
	for _, declaration := range index.Types {
		selected, exists := selectedTypes[declaration.ID]
		if !exists {
			continue
		}
		projection.ReceiverTypes = append(projection.ReceiverTypes, selected)
		for _, blockID := range sortedSet(typeBlocks[declaration.ID]) {
			projection.Bindings = append(projection.Bindings, CoreObjectBinding{
				CoreBlockID: blockID, ObjectID: declaration.ID, Role: CoreObjectReceiverType,
			})
			projection.Coverage.ReceiverTypeBindings++
		}
	}
	sort.Slice(projection.Bindings, func(i, j int) bool {
		return coreObjectBindingKey(projection.Bindings[i]) < coreObjectBindingKey(projection.Bindings[j])
	})
	digest, err := coreObjectProjectionDigest(projection)
	if err != nil {
		return CoreObjectProjection{}, err
	}
	projection.SHA256 = digest
	if err := projection.Validate(); err != nil {
		return CoreObjectProjection{}, err
	}
	return projection, nil
}

// Validate checks the persisted canonical shape without trusting a live
// producer. ValidateAgainst below additionally recomputes the complete exact
// projection from the named authorities.
func (projection CoreObjectProjection) Validate() error {
	if projection.Version != CoreObjectProjectionVersion || !validProjectionSHA(projection.CoreObjectIndexSHA256) ||
		!validProjectionSHA(projection.SHA256) {
		return fmt.Errorf("cubemap: invalid core object projection identity")
	}
	if len(projection.Callables) > gocoreobject.MaxCallables || len(projection.ReceiverTypes) > gocoreobject.MaxTypes ||
		len(projection.Bindings) > gocoreobject.MaxCallables*2 {
		return fmt.Errorf("cubemap: core object projection exceeds collection bounds")
	}
	objectRoles := make(map[string]CoreObjectBindingRole, len(projection.Callables)+len(projection.ReceiverTypes))
	for position, callable := range projection.Callables {
		if !validProjectedCallable(callable) || position > 0 && projectedCallableKey(projection.Callables[position-1]) >= projectedCallableKey(callable) {
			return fmt.Errorf("cubemap: invalid projected callable")
		}
		if _, duplicate := objectRoles[callable.ID]; duplicate {
			return fmt.Errorf("cubemap: duplicate projected object")
		}
		objectRoles[callable.ID] = CoreObjectRepresentativeCallable
	}
	for position, declaration := range projection.ReceiverTypes {
		if !validProjectedType(declaration) || position > 0 && projectedTypeKey(projection.ReceiverTypes[position-1]) >= projectedTypeKey(declaration) {
			return fmt.Errorf("cubemap: invalid projected receiver type")
		}
		if _, duplicate := objectRoles[declaration.ID]; duplicate {
			return fmt.Errorf("cubemap: duplicate projected object")
		}
		objectRoles[declaration.ID] = CoreObjectReceiverType
	}
	callableBindings := 0
	typeBindings := 0
	for position, binding := range projection.Bindings {
		role, exists := objectRoles[binding.ObjectID]
		if !exists || role != binding.Role || !binding.Role.valid() || !validProjectionText(binding.CoreBlockID) ||
			position > 0 && coreObjectBindingKey(projection.Bindings[position-1]) >= coreObjectBindingKey(binding) {
			return fmt.Errorf("cubemap: invalid core object binding")
		}
		if binding.Role == CoreObjectRepresentativeCallable {
			callableBindings++
		} else {
			typeBindings++
		}
	}
	coverage := projection.Coverage
	if coverage.CoreBlocksObserved < 0 || coverage.RepresentativeSymbolClaims < 0 ||
		coverage.RepresentativeNodesObserved < 0 || coverage.RepresentativeCallablesMatched != len(projection.Callables) ||
		coverage.RepresentativeNodesUnmatched < 0 ||
		coverage.RepresentativeCallablesMatched+coverage.RepresentativeNodesUnmatched != coverage.RepresentativeNodesObserved ||
		coverage.CallableBindings != callableBindings || coverage.ReceiverMethodsObserved < 0 ||
		coverage.ReceiverTypesMatched != len(projection.ReceiverTypes) || coverage.ReceiverMethodsOmitted < 0 ||
		coverage.GenericReceiverMethodsOmitted < 0 || coverage.GenericReceiverMethodsOmitted > coverage.ReceiverMethodsOmitted ||
		coverage.ReceiverTypeBindings != typeBindings {
		return fmt.Errorf("cubemap: invalid core object projection coverage")
	}
	methodCount := 0
	for _, callable := range projection.Callables {
		if callable.Kind == gocoreobject.CallableMethod {
			methodCount++
		}
	}
	if coverage.ReceiverMethodsObserved != methodCount ||
		coverage.ReceiverMethodsObserved-coverage.ReceiverMethodsOmitted < coverage.ReceiverTypesMatched {
		return fmt.Errorf("cubemap: invalid core object receiver coverage")
	}
	digest, err := coreObjectProjectionDigest(projection)
	if err != nil {
		return err
	}
	if digest != projection.SHA256 {
		return fmt.Errorf("cubemap: core object projection sha256 mismatch")
	}
	return nil
}

func (projection CoreObjectProjection) ValidateAgainst(core coremap.Result, index gocoreobject.Index) error {
	if err := projection.Validate(); err != nil {
		return err
	}
	exact, err := compileCoreObjectProjection(core, index)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(projection, exact) {
		return fmt.Errorf("cubemap: core object projection authority mismatch")
	}
	return nil
}

func exactReceiverName(receiver string) (packagePath string, typeName string, generic bool, ok bool) {
	receiver = strings.TrimSpace(receiver)
	if strings.ContainsAny(receiver, "[]") {
		return "", "", true, false
	}
	receiver = strings.TrimPrefix(receiver, "*")
	separator := strings.LastIndexByte(receiver, '.')
	if separator <= 0 || separator == len(receiver)-1 {
		return "", "", false, false
	}
	packagePath, typeName = receiver[:separator], receiver[separator+1:]
	if strings.TrimSpace(packagePath) != packagePath || !token.IsIdentifier(typeName) || typeName == "_" {
		return "", "", false, false
	}
	return packagePath, typeName, false, true
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validProjectedCallable(value gocoreobject.CallableDeclaration) bool {
	return validProjectionText(value.ID) && value.Kind.Valid() && validProjectionText(value.Package) &&
		token.IsIdentifier(value.Name) && value.Name != "_" && validProjectionText(value.Signature) &&
		validProjectionLocation(value.Location) && validProjectionText(value.DirectCallNodeID) &&
		(value.Kind != gocoreobject.CallableMethod || validProjectionText(value.Receiver)) &&
		(value.Kind != gocoreobject.CallableFunction || value.Receiver == "")
}

func validProjectedType(value gocoreobject.TypeDeclaration) bool {
	return validProjectionText(value.ID) && value.Kind.Valid() && validProjectionText(value.Package) &&
		token.IsIdentifier(value.Name) && value.Name != "_" && validProjectionLocation(value.Location)
}

func validProjectionLocation(value gocoreobject.Location) bool {
	return fs.ValidPath(value.Path) && strings.HasSuffix(value.Path, ".go") && value.Line > 0 && value.Column > 0
}

func validProjectionText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > gocoreobject.MaxTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func projectedCallableKey(value gocoreobject.CallableDeclaration) string {
	return fmt.Sprintf("%s\x00%s:%09d:%09d\x00%s\x00%s\x00%s\x00%s", value.Package,
		value.Location.Path, value.Location.Line, value.Location.Column, value.Receiver, value.Name, value.Kind, value.ID)
}

func projectedTypeKey(value gocoreobject.TypeDeclaration) string {
	return fmt.Sprintf("%s\x00%s:%09d:%09d\x00%s\x00%s\x00%s", value.Package,
		value.Location.Path, value.Location.Line, value.Location.Column, value.Name, value.Kind, value.ID)
}

func coreObjectBindingKey(value CoreObjectBinding) string {
	return value.CoreBlockID + "\x00" + string(value.Role) + "\x00" + value.ObjectID
}

func validProjectionSHA(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func coreObjectProjectionDigest(value CoreObjectProjection) (string, error) {
	value.SHA256 = ""
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("cubemap: encode core object projection digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
