# Slatewave CLI — common dev targets.
# Run `make` (or `make help`) for the full list.

BINARY  := slatewave
GO      := go
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X github.com/kevinlangleyjr/slatewave-cli/cmd.Version=$(VERSION)"

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN { FS = ":.*## "; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n" } \
	  /^[a-zA-Z0-9_-]+:.*## / { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo

.PHONY: build
build: ## Compile slatewave into the repo root, with version baked in.
	$(GO) build $(LDFLAGS) -o $(BINARY) .

.PHONY: install
install: ## Install slatewave to $$GOPATH/bin (~/go/bin by default).
	$(GO) install $(LDFLAGS) .

.PHONY: run
run: ## Run the CLI without building (e.g., `make run ARGS="list"`).
	$(GO) run . $(ARGS)

.PHONY: test
test: ## Run all tests.
	$(GO) test ./...

.PHONY: test-v
test-v: ## Run all tests verbosely.
	$(GO) test -v ./...

.PHONY: test-race
test-race: ## Run all tests with the race detector.
	$(GO) test -race ./...

.PHONY: cover
cover: ## Generate coverage.out and open the HTML report in your browser.
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format every Go file in place.
	$(GO) fmt ./...

.PHONY: tidy
tidy: ## Tidy + verify go.mod / go.sum.
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: check
check: fmt vet test ## Format, vet, and test — the gate before committing.

.PHONY: smoke
smoke: build ## Dry-run install for every embedded theme — exercises all install patterns.
	@for slug in vscode bat btop delta oh-my-posh; do \
		./$(BINARY) install $$slug --dry-run; \
		echo; \
	done

.PHONY: clean
clean: ## Remove the built binary and coverage artifacts.
	rm -f $(BINARY) coverage.out
