package main

import "os"

// This program names its own functions like a setting read and code
// evaluation. A call to them runs this repository's code, whatever the name:
// it reads no setting and evaluates nothing (Go has no builtin that does).
func eval(expression string) string { return expression }

func Getenv(key string) string { return "database row: " + key }

type statusLedger struct{}

func (statusLedger) exec(statement string) string { return statement }

func readLookalikes() []string {
	return []string{eval("ordinary data"), Getenv("CUSTOMER_ROW"), statusLedger{}.exec("show status")}
}

// The operating system's own environment read.
func readSetting() string {
	return os.Getenv("FIXTURE_LOOKALIKE_SETTING")
}
