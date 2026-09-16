SHELL := /bin/bash
.DEFAULT_GOAL := help

GO_MODULES := packages/core packages/binaries packages/presets apps/desktop

.PHONY: help setup doctor dev build test test-go test-js lint lint-go lint-js fetch-binaries clean

help: ## Show available targets
	@echo "Lasso — make targets"
	@echo
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk -F':.*?## ' '{printf "  \033[1m%-16s\033[0m %s\n", $$1, $$2}'
	@echo

setup: doctor ## Check the toolchain, install JS deps, fetch sidecar binaries
	pnpm install
	./scripts/fetch-binaries.sh
	@echo
	@echo "Setup complete. Next: make dev"

doctor: ## Verify Go, Node, pnpm and Wails are present
	@echo "Checking toolchain..."
	@command -v go   >/dev/null 2>&1 || { echo "  missing: go — install from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "  missing: node — install Node 20+"; exit 1; }
	@command -v pnpm >/dev/null 2>&1 || { echo "  missing: pnpm — npm install -g pnpm"; exit 1; }
	@command -v wails >/dev/null 2>&1 || { \
		echo "  missing: wails"; \
		echo "     go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		echo "     (then make sure \$$(go env GOPATH)/bin is on your PATH)"; \
		exit 1; }
	@echo "  go    $$(go version | awk '{print $$3}')"
	@echo "  node  $$(node --version)"
	@echo "  pnpm  $$(pnpm --version)"
	@echo
	@wails doctor

dev: ## Run the app in development mode (hot reload)
	@test -f apps/desktop/wails.json || { \
		echo "apps/desktop/wails.json does not exist yet — the Wails app is wired up in phase D."; \
		exit 1; }
	cd apps/desktop && wails dev

build: ## Build Lasso.app into apps/desktop/build/bin
	@test -f apps/desktop/wails.json || { \
		echo "apps/desktop/wails.json does not exist yet — packaging lands in phase F."; \
		exit 1; }
	cd apps/desktop && wails build -platform darwin/arm64
	./scripts/bundle-binaries.sh

test: test-go test-js ## Run all tests

test-go: ## Run Go tests across every workspace module
	@for m in $(GO_MODULES); do \
		echo "==> $$m"; \
		( cd $$m && go test ./... ) || exit 1; \
	done

test-js: ## Run JS tests via Turborepo
	pnpm run test

lint: lint-go lint-js ## Lint everything

lint-go: ## go vet + gofmt across every workspace module
	@for m in $(GO_MODULES); do \
		( cd $$m && go vet ./... ) || exit 1; \
	done
	@unformatted=$$(gofmt -l $(GO_MODULES)); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi
	@echo "go: vet and gofmt clean"

lint-js: ## Lint JS via Turborepo
	pnpm run lint

fetch-binaries: ## Download + verify sidecar binaries for this machine
	./scripts/fetch-binaries.sh

clean: ## Remove build output and fetched binaries (keeps the download cache)
	rm -rf apps/desktop/build/bin apps/desktop/frontend/dist .turbo
	@echo "Cleaned. Run make fetch-binaries to restore the sidecars."
