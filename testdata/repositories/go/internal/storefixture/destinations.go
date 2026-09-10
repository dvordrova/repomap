package storefixture

import (
	"context"
	"flag"
	"net/http"
)

// DestinationRequest is a shared transport adapter, not a separate service.
func DestinationRequest(ctx context.Context, method, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// DestinationHandler captures the constructor argument in its callback.
func DestinationHandler(base string) func(context.Context) {
	return func(ctx context.Context) {
		_, _ = DestinationRequest(ctx, http.MethodGet, base+"/시세")
	}
}

func DestinationApplication() {
	endpoint := flag.String("price-endpoint", "", "price service")
	handler := DestinationHandler(*endpoint)
	handler(context.Background())
	_, _ = DestinationRequest(context.Background(), http.MethodPost, "https://audit.example/events")
}

// RequestConstructionOnly prepares a request but performs no exchange.
func RequestConstructionOnly() *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "https://unused.example", nil)
	return req
}

type DestinationClient struct{ Base string }

func NewDestinationClient(base string) *DestinationClient {
	return &DestinationClient{Base: base}
}

func (client *DestinationClient) Send(ctx context.Context) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, client.Base+"/account", nil)
	return http.DefaultClient.Do(req)
}

func BuildDestinationRequest(endpoint string) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	return req
}

func DestinationFactories(ctx context.Context) {
	client := NewDestinationClient("https://client.example")
	_, _ = client.Send(ctx)
	req := BuildDestinationRequest("https://factory.example/events")
	_, _ = http.DefaultClient.Do(req.WithContext(ctx))
}

// DestinationStoresAfterCall writes a different endpoint after the exchange.
// Returning the object keeps its allocation and both writes in native SSA.
func DestinationStoresAfterCall() *DestinationClient {
	client := &DestinationClient{Base: "https://before.example"}
	_, _ = http.Get(client.Base)
	client.Base = "https://after.example"
	return client
}

func DestinationSingleStoreAfterCall() *DestinationClient {
	client := new(DestinationClient)
	_, _ = http.Get(client.Base)
	client.Base = "https://after-only.example"
	return client
}

func DestinationReturnedStore() {
	client := DestinationSingleStoreAfterCall()
	_, _ = http.Get(client.Base)
}
