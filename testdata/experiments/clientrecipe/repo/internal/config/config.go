package config

import "time"

type Config struct {
	Vault      VaultConfig
	Kubernetes KubernetesConfig
	ClickHouse ClickHouseConfig
	Notifier   NotifierConfig
}

type VaultConfig struct {
	Address string
	Token   string
	Timeout time.Duration
	Retries int
}

type NotifierConfig struct {
	Endpoint string
}

type KubernetesConfig struct {
	Server  string
	Retries int
}

type ClickHouseConfig struct {
	DSN string
}

func FromEnvironment() Config {
	return Config{
		Vault: VaultConfig{
			Address: "http://vault.service.local",
			Token:   "development-placeholder",
			Timeout: 2 * time.Second,
			Retries: 2,
		},
		Kubernetes: KubernetesConfig{Server: "https://kubernetes.service.local", Retries: 1},
		ClickHouse: ClickHouseConfig{DSN: "clickhouse://analytics.service.local/launches"},
		Notifier:   NotifierConfig{Endpoint: "http://notify.service.local"},
	}
}
