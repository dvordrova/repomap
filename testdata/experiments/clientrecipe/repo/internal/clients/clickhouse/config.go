package clickhouse

import "example.com/launchservice/internal/config"

type Config struct {
	DSN string
}

func ConfigFromApplication(value config.ClickHouseConfig) Config {
	return Config{DSN: value.DSN}
}
