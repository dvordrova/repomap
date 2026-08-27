package vault

import (
	"time"

	"example.com/launchservice/internal/config"
)

type Config struct {
	Address string
	Token   string
	Timeout time.Duration
	Retries int
}

func ConfigFromApplication(value config.VaultConfig) Config {
	return Config{Address: value.Address, Token: value.Token, Timeout: value.Timeout, Retries: value.Retries}
}
