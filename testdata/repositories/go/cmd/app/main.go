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
// external-call scan even though target-root traversal does not retain the
// local producer call as a reachable direct edge.
func unreachableHandler() http.HandlerFunc { return http.NotFound }

func unreachableHandlerConsumer(w http.ResponseWriter, r *http.Request) {
	unreachableHandler().ServeHTTP(w, r)
}

// unreachableCallbackFactory mirrors a callback passed from inside a returned
// closure. The exact callback transfer exists program-wide, while its owning
// local call is deliberately outside the target-root direct traversal.
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
