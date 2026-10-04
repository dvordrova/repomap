package main

import (
	"database/sql"
	"os"
)

// settingOrDefault reads a setting as a configuration helper often does:
// from the environment, else a word it keeps for two of its keys only.
func settingOrDefault(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	res := ""
	if key == "staticBaseUrl" {
		res = "https://cdn.example/static"
	} else if key == "logConfig" {
		res = "logs/app.log"
	}
	return res
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
