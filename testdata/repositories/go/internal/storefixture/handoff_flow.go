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

// compatibleOnlyEngine is never assigned to a FieldStore: its implements facts are method-set
// authority, not runtime binding; only unknownFacade.Put, whose value no flow gives, runs it.
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

// A registration function no repository code calls hands the server it is
// given to the handler it registers, as a generated gateway's
// RegisterElectionHandlerServer does: no observed flow gives the value the
// handler calls, so the call runs the method of each repository type
// implementing the interface, never an unknown (owner, 2026-09-16 and
// 2026-09-30). A type that only embeds an implementation runs that one's
// method, a type embedding the interface implements nothing of its own, and
// a method of another signature is no implementation.
type ticketServer interface {
	IssueTicket(string) error
}

type ticketDesk struct{}
type ticketKiosk struct{}
type renamedTicketDesk struct{ ticketDesk }
type ticketGateway struct{ ticketServer }
type ticketPrinter struct{}

func (*ticketDesk) IssueTicket(string) error  { return nil }
func (*ticketKiosk) IssueTicket(string) error { return nil }
func (*ticketPrinter) IssueTicket(int) error  { return nil }

func RegisterTicketHandlerServer(mux map[string]func(string) error, server ticketServer) {
	mux["issue"] = func(name string) error { return server.IssueTicket(name) }
}

// One repository type implements receiptServer: the call is exact, its
// target known by the implementation all the same.
type receiptServer interface{ PrintReceipt(string) }

type receiptDesk struct{}

func (receiptDesk) PrintReceipt(string) {}

func RegisterReceiptHandlerServer(mux map[string]func(string), server receiptServer) {
	mux["print"] = func(name string) { server.PrintReceipt(name) }
}

// A call's guard (GO): the arm a non-nil error takes, what panic is handed
// and an arm ending in panic run only on a failing path; any other arm is a
// branch; the rest runs unguarded.
func CheckedStore(key string, err error) {
	if err != nil {
		reportStoreFailure(key)
		return
	}
	if key == "" {
		panic(describeStore(key))
	}
	if len(key) > 64 {
		reportLongKey(key)
	}
	countStore(key)
}

func reportStoreFailure(string)       {}
func describeStore(key string) string { return key }
func reportLongKey(string)            {}
func countStore(string)               {}

// An arm with a return before its panic does not only fail: it is a branch.
func CheckedLongKey(key string) {
	if len(key) > 128 {
		reportLongKey(key)
		if key[0] == 'o' {
			return
		}
		panic(key)
	}
}
