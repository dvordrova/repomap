SHELL := /usr/bin/env bash

-include .env
export

BIN_DIR ?= .bin
REPO ?= .
RUN_ARGS ?=
CACHE_ARGS ?=

.PHONY: help build test vet check run cache-clear

help: ## Print available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the repomap binary into .bin/
	@mkdir -p $(BIN_DIR)
	go build -trimpath -o $(BIN_DIR)/repomap ./cmd/repomap

test: ## Run Go tests
	go test ./...

vet: ## Run go vet
	go vet ./...

check: test vet ## Run tests and vet

run: build ## Analyze REPO with the configured model provider
	$(BIN_DIR)/repomap "$(REPO)" $(RUN_ARGS)

cache-clear: build ## Clear repomap's persistent model caches
	$(BIN_DIR)/repomap cache clear $(CACHE_ARGS)
