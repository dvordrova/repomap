package storefixture

import (
	"os"
	"os/signal"
)

// fileStoreTestBundle intentionally mirrors the private receiver involved in
// a ProgramIndex/DirectCallIndex ownership regression.
type fileStoreTestBundle struct {
	root string
}

// recreateStore is deliberately unused. It must still be a valid callable in
// the typed index, without claiming a direct-call node that was not retained.
func (bundle *fileStoreTestBundle) recreateStore() string {
	return bundle.root + "/recreated"
}

// Exercise keeps the receiver type in the reachable package without calling
// recreateStore.
func Exercise(root string) string {
	bundle := &fileStoreTestBundle{root: root}
	if root == "__repomap_boundary_fixture__" {
		events := make(chan os.Signal, 1)
		registerSignalConsumer(events)
		_, _ = createFixtureState()
	}
	return bundle.root
}

// registerSignalConsumer is an exact standard-library event registration. It
// gives the shared pattern classifier a language-neutral inbound-event row
// without relying on a framework allowlist or a local-name convention.
func registerSignalConsumer(events chan<- os.Signal) {
	signal.Notify(events, os.Interrupt)
}

// createFixtureState is an exact standard-library durable-store operation. It
// does not need a third-party driver or a live service during analysis.
func createFixtureState() (*os.File, error) {
	return os.Create("fixture-state.db")
}

// Interface-field forwarding preserves evidence without assuming that two
// instances share a receiver or that every compatible type is ever installed.
type FieldStore interface{ Put(string) }
type storedEngine struct{}
type alternateEngine struct{}
type neverStoredEngine struct{}

func (*storedEngine) Put(string)      {}
func (*alternateEngine) Put(string)   {}
func (*neverStoredEngine) Put(string) {}

type fieldFacade struct{ store FieldStore }

func NewFieldFacade(engine *storedEngine) FieldStore        { return &fieldFacade{store: engine} }
func NewAlternateFacade(engine *alternateEngine) FieldStore { return &fieldFacade{store: engine} }
func (f *fieldFacade) Put(key string)                       { f.store.Put(key) }

type outerFacade struct{ inner FieldStore }

func NewOuterFacade(engine *storedEngine) FieldStore {
	return &outerFacade{inner: NewFieldFacade(engine)}
}
func (f *outerFacade) Put(key string) { f.inner.Put(key) }

type unrelatedFacade struct{ store FieldStore }

func NewUnrelatedFacade(engine *neverStoredEngine) FieldStore { return &unrelatedFacade{store: engine} }
func (f *unrelatedFacade) Put(key string)                     { f.store.Put(key) }

type unknownFacade struct{ store FieldStore }

func NewUnknownFacade(engine FieldStore) FieldStore { return &unknownFacade{store: engine} }
func (f *unknownFacade) Put(key string)             { f.store.Put(key) }

func directInterfaceInvoke() { var store FieldStore = &storedEngine{}; store.Put("key") }

// Neutral registration fields: two instances in one factory must not share
// literal metadata, and later/conditional assignments remain observations.
type actionSpec struct {
	Label   string
	Summary string
	Enabled bool
	Count   int
	Extra   any
	Run     func()
}

func BuildActionPair(alternate bool) (*actionSpec, *actionSpec) {
	first := &actionSpec{Label: "restore [file]", Summary: "Restore a saved state", Enabled: true, Count: 3, Extra: "fixture", Run: restoreAction}
	second := &actionSpec{Label: "inspect", Summary: "Inspect saved state", Run: func() {}}
	if alternate {
		first.Label = "recover [file]"
	}
	return first, second
}

func restoreAction() {}

// A framework can receive an object through an interface instead of receiving
// a function. Only the methods supplied by that interface belong to the joint.
type serviceRequest struct{ Value string }
type serviceResponse struct{ Value string }
type serviceContract interface {
	Apply(serviceRequest) serviceResponse
}
type serviceLayer struct{}
type alternateServiceLayer struct{}
type unregisteredServiceLayer struct{}

func (*serviceLayer) Apply(r serviceRequest) serviceResponse { return serviceResponse{r.Value} }
func (*serviceLayer) InternalHelper()                        {}
func (*alternateServiceLayer) Apply(r serviceRequest) serviceResponse {
	return serviceResponse{r.Value}
}
func (*unregisteredServiceLayer) Apply(r serviceRequest) serviceResponse {
	return serviceResponse{r.Value}
}

func newServiceLayer() serviceContract       { return &serviceLayer{} }
func installService(string, serviceContract) {}

func RegisterServices(alternate bool, unknown serviceContract) {
	installService("primary", newServiceLayer())
	var selected serviceContract = &serviceLayer{}
	if alternate {
		selected = &alternateServiceLayer{}
	}
	installService("alternative", selected)
	installService("unknown", unknown)
}

// TicketContract manages pending work through an abstract interface.
type TicketContract[T any] interface {
	// Cancel revokes the ticket. It removes its pending jobs from the queue.
	Cancel(id T) error
	// Status reports the ticket's current state.
	Status(id T) string
}

// Embedding and aliases do not redeclare the original methods.
type EmbeddedTicket interface{ TicketContract[string] }
type TicketAlias = TicketContract[string]
