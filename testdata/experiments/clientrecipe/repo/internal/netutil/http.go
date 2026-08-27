package netutil

import (
	"context"
	"net/http"
)

// DoHealthRequest is a standard-library helper, not a repository-owned external client boundary.
func DoHealthRequest(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}
