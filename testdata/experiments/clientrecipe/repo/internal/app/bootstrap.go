package app

import (
	"fmt"

	"example.com/launchservice/internal/config"
	"example.com/launchservice/internal/launch"
)

func Bootstrap(configuration config.Config) (*launch.Service, error) {
	service, err := wireClients(configuration)
	if err != nil {
		return nil, fmt.Errorf("bootstrap launch service: %w", err)
	}
	return service, nil
}
