// Package servicecfg holds settings shared by two services in this repository.
package servicecfg

import "strconv"

func Address() string          { return "127.0.0.1:8112" }
func PollIntervalSeconds() int { return 60 }

// Limits are the request limits both services apply.
type Limits struct{ MaxBody int }

// Describe says the limits for a log line; another package calls it on a
// Limits value.
func (l Limits) Describe() string { return "max body " + strconv.Itoa(l.MaxBody) }

// parsed is a setting as read, before it is checked. Its method's name is
// exported, but no other package can name the type to call it.
type parsed struct{ raw string }

func (p parsed) Raw() string { return p.raw }

func parse(raw string) parsed { return parsed{raw: raw} }
