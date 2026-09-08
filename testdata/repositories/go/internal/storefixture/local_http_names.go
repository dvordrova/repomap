package storefixture

import http "example.com/repomap/cumulative-go-fixture/internal/localstore"

func ReadLocalHTTPName() string {
	return http.Get("/local-key")
}
