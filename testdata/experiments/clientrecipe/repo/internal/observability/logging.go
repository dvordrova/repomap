package observability

import "log"

func ClientFailure(client, operation string, err error) {
	log.Printf("external client %s operation %s failed: %v", client, operation, err)
}
