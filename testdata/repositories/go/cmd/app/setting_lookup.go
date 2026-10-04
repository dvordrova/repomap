package main

import (
	"database/sql"
	"net/http"
	"os"
)

// settingOrDefault reads a setting as a configuration helper often does:
// from the environment, else its file, else a word it keeps for two of its
// keys only.
func settingOrDefault(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	res, _ := settingFile(key)
	if res == "" {
		if key == "staticBaseUrl" {
			res = "https://cdn.example/static"
		} else if key == "logConfig" {
			res = "logs/app.log"
		}
	}
	return res
}

// settingFile reads a setting's file; the words it holds are not code's.
func settingFile(key string) (string, error) {
	data, err := os.ReadFile(key + ".conf")
	return string(data), err
}

// languageOf returns a word as written in each branch.
func languageOf(tag string) string {
	switch tag {
	case "ru":
		return "russian"
	case "en":
		return "english"
	}
	return "unknown"
}

// openStore asks for the data source: neither the static URL nor the log
// file is a value settingOrDefault gives this key.
func openStore() (*sql.DB, error) {
	return sql.Open("sqlite", settingOrDefault("dataSourceName"))
}

// greeting is the one word languageOf returns for ru.
func greeting() string {
	return languageOf("ru")
}

// fetchStatic asks for the static base: the URL settingOrDefault keeps for
// this key is its value.
func fetchStatic() {
	_, _ = http.Get(settingOrDefault("staticBaseUrl"))
}

// pick writes both of its branches and its default on one line: what
// pick("y") returns is the default alone.
func pick(key string) string { if key == "x" { return "https://x.example" } else if key == "z" { return "https://z.example" }; return "https://default.example" }

// fetchPicked asks pick for a key neither branch compares with.
func fetchPicked() {
	_, _ = http.Get(pick("y"))
}
