# leo - build, test, and install helpers.
CARGO ?= cargo
GORELEASER ?= goreleaser
BINARY := leo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX ?= $(HOME)/.local
BINDIR := $(PREFIX)/bin

.DEFAULT_GOAL := build
.PHONY: build install uninstall run test test-race cover vet fmt fmt-check tidy check clean version release-check help

build: ## Build the Rust binary into ./leo
	LEO_VERSION="$(VERSION)" $(CARGO) build --release --locked
	cp target/release/leo $(BINARY)

install: build ## Build and install into $(BINDIR)
	mkdir -p "$(BINDIR)"
	cp $(BINARY) "$(BINDIR)/$(BINARY)"

uninstall: ## Remove leo from $(BINDIR)
	rm -f "$(BINDIR)/$(BINARY)"

run: ## Run leo, e.g. make run ARGS="kv"
	LEO_VERSION="$(VERSION)" $(CARGO) run --locked -- $(ARGS)

test: ## Run Rust tests and Go/Rust CLI compatibility checks
	$(CARGO) test --locked
	cd tests/reference && go test ./...

test-race: test ## Also run the reference's race checks
	cd tests/reference && go test -race ./...

cover: ## Run Rust coverage (requires cargo-llvm-cov)
	$(CARGO) llvm-cov --locked

vet: ## Run Clippy
	$(CARGO) clippy --locked --all-targets --all-features -- -D warnings

fmt: ## Format Rust code
	$(CARGO) fmt

fmt-check: ## Check Rust formatting
	$(CARGO) fmt -- --check

tidy: ## Refresh the Cargo lockfile
	$(CARGO) generate-lockfile

check: fmt-check vet test ## Run formatting, Clippy, tests, and compatibility checks

clean: ## Remove built binaries and Cargo output
	rm -f $(BINARY)
	$(CARGO) clean

version: ## Print the version that would be baked in
	@echo $(VERSION)

release-check: ## Validate tests, GoReleaser configuration, and release archives
	PATH="$(dir $(shell command -v $(GORELEASER) 2>/dev/null)):$$PATH" bash scripts/release-readiness-check.sh

help: ## Show every target
	@awk 'BEGIN {FS=":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
