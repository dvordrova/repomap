package main

import (
	"fmt"
	"net/http"

	"example.com/repomap/cumulative-go-fixture/internal/storefixture"
)

func main() {
	fmt.Println(storefixture.Exercise("fixture"))
	registerLevelConsumer()
	registerProductRoutes()
}

func fetchLevels() {
	_, _ = http.Get("/api/levels")
}

func registerLevelRoute() {
	http.HandleFunc("/api/levels", getLevel)
	_ = http.ListenAndServe(":8080", http.DefaultServeMux)
}

func getLevel(http.ResponseWriter, *http.Request) {}

func Subscribe(_ string, _ func([]byte)) {}

func registerLevelConsumer() {
	Subscribe("levels.requested", consumeLevel)
}

func consumeLevel([]byte) {}

type fixtureRouter struct{}
type fixtureRoute struct{}

func (*fixtureRouter) HandleFunc(_ string, _ func()) *fixtureRoute { return &fixtureRoute{} }
func (route *fixtureRoute) Methods(_ ...string) *fixtureRoute      { return route }

func registerProductRoutes() {
	router := &fixtureRouter{}
	router.HandleFunc("/products", listProductsHandler).Methods("GET")
	router.HandleFunc("/product", createProductHandler).Methods("POST")
	router.HandleFunc("/product/{id}", getProductHandler).Methods("GET")
	router.HandleFunc("/product/{id}", updateProductHandler).Methods("PUT")
	router.HandleFunc("/product/{id}", deleteProductHandler).Methods("DELETE")
}

func listProductsHandler()  { listProducts() }
func createProductHandler() { createProduct() }
func getProductHandler()    { getProduct() }
func updateProductHandler() { updateProduct() }
func deleteProductHandler() { deleteProduct() }

func listProducts()  {}
func createProduct() {}
func getProduct()    {}
func updateProduct() {}
func deleteProduct() {}

// unreachableHandler and unreachableHandlerConsumer preserve a real chained
// receiver shape found in chi. The consumer is still visible to the complete
// external-call scan and declaration-wide direct graph; neither requires the
// local producer call to be reachable from main.
func unreachableHandler() http.HandlerFunc { return http.NotFound }

func unreachableHandlerConsumer(w http.ResponseWriter, r *http.Request) {
	unreachableHandler().ServeHTTP(w, r)
}

// unreachableCallbackFactory mirrors a callback passed from inside a returned
// closure. The exact callback transfer and its owning argument are retained
// even though the local call is outside main reachability.
func unreachableCallbackFactory() func() {
	return func() {
		walkFixture([]string{"fixture"}, func(value string) bool { return value != "" })
	}
}

func walkFixture(values []string, visit func(string) bool) {
	for _, value := range values {
		if visit(value) {
			return
		}
	}
}

func registerAliasedCallbacks() {
	callback := func(value string) bool { return value != "" }
	walkFixture([]string{"fixture"}, callback)
	named := namedCallback
	walkFixture([]string{"fixture"}, named)
}

func namedCallback(value string) bool { return value != "" }

type fixtureMapper struct{}

func (mapper *fixtureMapper) Map(_ func(int) int) *fixtureMapper { return mapper }

func registerChainedCallbacks() {
	mapper := &fixtureMapper{}
	mapper.Map(func(value int) int { return value * 2 }).Map(func(value int) int { return value + 1 })
}

// Go promotes the embedded method; it has no class inheritance.
type applicationMux struct{ *http.ServeMux }
type overriddenMux struct{ *http.ServeMux }

func (*overriddenMux) HandleFunc(string, func(http.ResponseWriter, *http.Request)) {}

func registerEmbeddedRoutes() {
	mux := &applicationMux{http.NewServeMux()}
	mux.HandleFunc("/api/embedded", getLevel)
	local := &overriddenMux{http.NewServeMux()}
	local.HandleFunc("/api/overridden-lookalike", getLevel)
}
