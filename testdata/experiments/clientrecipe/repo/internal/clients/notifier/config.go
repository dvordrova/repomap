package notifier

import "example.com/launchservice/internal/config"

type Config struct {
	Endpoint string
}

func ConfigFromApplication(value config.NotifierConfig) Config {
	return Config{Endpoint: value.Endpoint}
}
