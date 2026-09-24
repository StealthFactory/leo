# leo - build, test, and install helpers.
#
# Common usage:
#   make            build ./leo
#   make install    build and install into ~/.local/bin
#   make test       run the tests
#   make check      fmt-check + vet + test (what CI would run)
#   make run ARGS="store list"
#   make help       list every target

GO      ?= go
BINARY  := leo
PKG     := .
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Install location. Override with: make install PREFIX=/usr/local
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the leo binary into ./leo
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

.PHONY: install
install: ## Build and install leo into $(BINDIR)
	@mkdir -p $(BINDIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(BINARY) $(PKG)
	@echo "installed $(BINDIR)/$(BINARY) ($(VERSION))"

.PHONY: uninstall
uninstall: ## Remove leo from $(BINDIR)
	rm -f $(BINDIR)/$(BINARY)

.PHONY: run
run: ## Build and run leo, e.g. make run ARGS="store list"
	$(GO) run -ldflags "$(LDFLAGS)" $(PKG) $(ARGS)

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run tests with the race detector
	$(GO) test -race ./...

.PHONY: cover
cover: ## Run tests and print a coverage summary
	$(GO) test -cover ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go code in place
	gofmt -s -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file needs gofmt
	@out=$$(gofmt -s -l .); if [ -n "$$out" ]; then echo "needs gofmt:"; echo "$$out"; exit 1; fi

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: check
check: fmt-check vet test ## Run fmt-check, vet, and tests

.PHONY: clean
clean: ## Remove the built binary and Go build cache artifacts
	rm -f $(BINARY)
	$(GO) clean

.PHONY: version
version: ## Print the version that would be baked in
	@echo $(VERSION)

.PHONY: formula-sha
formula-sha: ## Print a tag's tarball sha256 for the formula, e.g. make formula-sha TAG=v0.2.0
	@curl -fsSL https://github.com/StealthFactory/leo/archive/refs/tags/$(TAG).tar.gz | shasum -a 256 | awk '{print $$1}'

.PHONY: formula-readiness-check
formula-readiness-check: ## Check the formula is ready to publish to the tap
	bash scripts/formula-readiness-check.sh

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'
