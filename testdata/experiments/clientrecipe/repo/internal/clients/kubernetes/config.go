package kubernetes

import "example.com/launchservice/internal/config"

type Config struct {
	Server  string
	Retries int
}

func ConfigFromApplication(value config.KubernetesConfig) Config {
	return Config{Server: value.Server, Retries: value.Retries}
}
