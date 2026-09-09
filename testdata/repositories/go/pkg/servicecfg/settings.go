// Package servicecfg holds settings shared by two services in this repository.
package servicecfg

func Address() string          { return "127.0.0.1:8112" }
func PollIntervalSeconds() int { return 60 }
