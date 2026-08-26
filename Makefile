SHELL := /usr/bin/env bash

-include .env
export

BIN_DIR ?= .bin
REPO ?= .
RUN_ARGS ?=
CACHE_ARGS ?=
PRODUCT_GO_PACKAGES := ./cmd/... ./internal/...
GO_PACKAGE_PARALLELISM ?= 4
GO_TEST_TIMEOUT ?= 5m

.PHONY: help build verify-package-layout test vet check run cache-clear

help: ## Print available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the repomap binary into .bin/
	@mkdir -p $(BIN_DIR)
	go build -trimpath -o $(BIN_DIR)/repomap ./cmd/repomap

# Benchmark evidence may contain nested source trees and module caches, so the
# product package roots are explicit. Fail if a new root would otherwise be
# silently omitted from test and vet.
verify-package-layout:
	@tracked_go="$$(git ls-files -- '*.go')" || { \
		echo "cannot enumerate tracked Go sources" >&2; \
		exit 1; \
	}; \
	if [[ -z "$$tracked_go" ]]; then \
		echo "tracked Go source inventory is empty" >&2; \
		exit 1; \
	fi; \
	unexpected=""; \
	while IFS= read -r path; do \
		case "$$path" in cmd/*|internal/*|testdata/*) ;; *) unexpected="$$path"; break ;; esac; \
	done <<< "$$tracked_go"; \
	if [[ -n "$$unexpected" ]]; then \
		echo "tracked Go source outside PRODUCT_GO_PACKAGES: $$unexpected" >&2; \
		exit 1; \
	fi

test: verify-package-layout ## Run Go tests
	go test -p $(GO_PACKAGE_PARALLELISM) -timeout $(GO_TEST_TIMEOUT) $(PRODUCT_GO_PACKAGES)

vet: verify-package-layout ## Run go vet
	go vet -p $(GO_PACKAGE_PARALLELISM) $(PRODUCT_GO_PACKAGES)

check: test vet ## Run tests and vet

run: build ## Analyze REPO with the configured model provider
	$(BIN_DIR)/repomap "$(REPO)" $(RUN_ARGS)

cache-clear: build ## Clear repomap's persistent model caches
	$(BIN_DIR)/repomap cache clear $(CACHE_ARGS)
