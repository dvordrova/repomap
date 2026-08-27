package app

import (
	"fmt"

	"example.com/launchservice/internal/clients/clickhouse"
	"example.com/launchservice/internal/clients/kubernetes"
	"example.com/launchservice/internal/clients/notifier"
	"example.com/launchservice/internal/clients/vault"
	"example.com/launchservice/internal/config"
	"example.com/launchservice/internal/launch"
	"example.com/launchservice/internal/observability"
)

func wireClients(configuration config.Config) (*launch.Service, error) {
	metrics := observability.NewMetrics()
	vaultClient, err := vault.NewClient(vault.ConfigFromApplication(configuration.Vault), metrics)
	if err != nil {
		return nil, fmt.Errorf("wire Vault client: %w", err)
	}
	kubernetesClient, err := kubernetes.NewClient(
		kubernetes.ConfigFromApplication(configuration.Kubernetes), metrics,
	)
	if err != nil {
		return nil, fmt.Errorf("wire Kubernetes client: %w", err)
	}
	clickHouseClient, err := clickhouse.NewClient(
		clickhouse.ConfigFromApplication(configuration.ClickHouse), metrics,
	)
	if err != nil {
		return nil, fmt.Errorf("wire ClickHouse client: %w", err)
	}
	notifierClient := notifier.NewClient(notifier.ConfigFromApplication(configuration.Notifier))
	return launch.NewService(vaultClient, kubernetesClient, clickHouseClient, notifierClient), nil
}
