package storefixture

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os/exec"
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

// verbose is a command-line option the package declares where it is
// initialized: flag.Bool is given the option's words, as flag.String is in
// DestinationApplication.
var verbose = flag.Bool("verbose", false, "log every destination request")

// Verbose reports the option.
func Verbose() bool { return *verbose }

// Revision starts another program, git, with its subcommand and reads what
// it prints: the call names the program by its first word, and Output on
// the command it built is that same program, not another one.
func Revision(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	return string(out), err
}

// RunHook starts the program a setting names: no word of the call names it.
func RunHook(hook string, args ...string) error {
	return exec.Command(hook, args...).Run()
}

// RevisionOf starts git on either branch and reads what it prints: the
// command is built by one of two calls, both naming git, and CombinedOutput
// on it is the program either call started, not another one.
func RevisionOf(ctx context.Context, ref string) (string, error) {
	var cmd *exec.Cmd
	if ref != "" {
		cmd = exec.CommandContext(ctx, "git", "rev-parse", ref)
	} else {
		cmd = exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// destinationBase fails with the zero value beside its error, as Go
// functions do: its caller uses the value only when the error is nil, so
// the empty string is no destination and no address is only "/items".
func destinationBase(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("no base")
	}
	return raw + "/v1", nil
}

func DestinationThroughAFailingHelper() {
	base, err := destinationBase("https://versioned.example")
	if err != nil {
		return
	}
	_, _ = http.Get(base + "/items")
}
