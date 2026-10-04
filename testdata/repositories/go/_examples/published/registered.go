package main

// The same package names driver here and reads what it registered: the
// blank import in main.go still imports it for its effect (its init runs
// once for the program), as casdoor's package object imports its mysql
// driver both `_` (ormer.go) and by name (syncer_database.go).
import "example.com/repomap/cumulative-go-fixture/driver"

func registeredDrivers() []string {
	return driver.Registered
}
