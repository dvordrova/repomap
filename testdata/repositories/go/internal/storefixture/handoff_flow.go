package storefixture

import (
	"context"
	"database/sql"
)

// Shared branches stay distinct paths even when they reach one callback.
type flowCallback func()
type routedCallback func()
type savedCallback func()

func flowAction() {}

func SharedCallbackFlow(flag bool, unknown flowCallback) {
	callback := flowCallback(flowAction)
	if flag {
		callback = unknown
	}
	if flag {
		callback = flowCallback(routedCallback(callback))
	} else {
		callback = flowCallback(savedCallback(callback))
	}
	if flag {
		callback = flowCallback(routedCallback(callback))
	} else {
		callback = flowCallback(savedCallback(callback))
	}
	callback()
}

func CyclicCallbackFlow(remaining int) {
	callback := flowCallback(flowAction)
	for remaining > 0 {
		callback = flowCallback(routedCallback(callback))
		remaining--
	}
	callback()
}

// Reusing a value must not merge its two exact assignment locations.
func SharedInterfaceStores(first, second *fieldFacade) {
	value := FieldStore(&storedEngine{})
	first.store = value
	second.store = value
}

func flowStoreBase() FieldStore { return &storedEngine{} }
func flowStoreBranch(flag bool) FieldStore {
	if flag {
		return flowStoreBase()
	}
	return flowStoreBase()
}
func flowStoreDiamond(flag bool) FieldStore {
	if flag {
		return flowStoreBranch(flag)
	}
	return flowStoreBranch(flag)
}
func recursiveFlowStore(flag bool) FieldStore {
	if flag {
		return &storedEngine{}
	}
	return recursiveFlowStore(flag)
}
func flowStorePair() (FieldStore, FieldStore) {
	return &storedEngine{}, &alternateEngine{}
}
func InvokeInterfaceFlows(flag bool) {
	flowStoreDiamond(flag).Put("diamond")
	recursiveFlowStore(flag).Put("cycle")
	first, second := flowStorePair()
	first.Put("first")
	second.Put("second")
}

type constructorInjectedFacade struct{ store FieldStore }

// compatibleOnlyEngine is never assigned to a FieldStore. It exists to prove
// that library interface matching is method-set authority, not runtime binding.
type compatibleOnlyEngine struct{}

func (*compatibleOnlyEngine) Put(string) {}

func newConstructorInjectedFacade(store FieldStore) *constructorInjectedFacade {
	return &constructorInjectedFacade{store: store}
}

func (facade *constructorInjectedFacade) Put(key string) { facade.store.Put(key) }

func InvokeConstructorInjectedStore() {
	newConstructorInjectedFacade(&storedEngine{}).Put("constructor")
}

// A repository interface filled by a standard-library value: the call through
// the field is the external method of that value, with its SQL argument.
type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type userRows struct{ db rowQuerier }

func newUserRows(db rowQuerier) *userRows { return &userRows{db: db} }

func (rows *userRows) Name(ctx context.Context, id int64) error {
	var name string
	return rows.db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = $1", id).Scan(&name)
}

func OpenUserRows() (*userRows, error) {
	db, err := sql.Open("postgres", "")
	if err != nil {
		return nil, err
	}
	return newUserRows(db), nil
}
