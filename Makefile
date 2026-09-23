SHELL := /bin/bash
.DEFAULT_GOAL := help

GO_MODULES := packages/core packages/binaries packages/presets packages/history packages/doctor packages/ghrelease packages/updater apps/desktop

# `go install` puts wails in GOPATH/bin (or GOBIN), which is frequently not on
# PATH. Resolve it explicitly so the build does not depend on shell setup.
GOBIN := $(shell go env GOBIN)
ifeq ($(strip $(GOBIN)),)
GOBIN := $(shell go env GOPATH)/bin
endif
WAILS := $(shell command -v wails 2>/dev/null || echo $(GOBIN)/wails)

.PHONY: help setup doctor dev build dmg release-assets release-notes test test-go test-js lint lint-go lint-js lint-tokens lint-notices notices fetch-binaries clean

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
	@test -x "$(WAILS)" || { \
		echo "  missing: wails"; \
		echo "     go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		exit 1; }
	@echo "  go    $$(go version | awk '{print $$3}')"
	@echo "  node  $$(node --version)"
	@echo "  pnpm  $$(pnpm --version)"
	@echo "  wails $(WAILS)"
	@command -v wails >/dev/null 2>&1 || \
		echo "        note: wails is not on your PATH; make uses it directly. To run it yourself, add $(GOBIN) to PATH."
	@echo
	@"$(WAILS)" doctor

dev: ## Run the app in development mode (hot reload)
	@test -x "$(WAILS)" || { \
		echo "wails not found at $(WAILS)"; \
		echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		exit 1; }
	cd apps/desktop && "$(WAILS)" dev

build: ## Build Lasso.app into apps/desktop/build/bin
	@test -x "$(WAILS)" || { \
		echo "wails not found at $(WAILS)"; \
		echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		exit 1; }
	cd apps/desktop && "$(WAILS)" build -platform darwin/arm64
	./scripts/make-icon.sh
	./scripts/bundle-binaries.sh
	# Last, and it must stay last: the two steps above modify the bundle after
	# Wails signed it, which leaves a broken seal and an app that downloads as
	# "damaged". sign-app.sh reseals and verifies.
	./scripts/sign-app.sh

dmg: build ## Build Lasso.app and package it into an unsigned DMG
	./scripts/make-dmg.sh

release-assets: release-notes dmg ## Build the DMG, update zip, latest.json and SHA256SUMS (NOTES=notes.md)
	NOTES="$(NOTES)" ./scripts/make-release-assets.sh

# Checked before the build rather than after it, so forgetting the notes costs
# a second rather than the three minutes the build and DMG take.
release-notes:
	@test -n "$(NOTES)" || { echo "set NOTES to the release notes file: make release-assets NOTES=notes.md"; exit 1; }
	@test -s "$(NOTES)" || { echo "NOTES=$(NOTES) is missing or empty"; exit 1; }

test: test-go test-js ## Run all tests

test-go: ## Run Go tests across every workspace module
	@for m in $(GO_MODULES); do \
		echo "==> $$m"; \
		( cd $$m && go test ./... ) || exit 1; \
	done

test-js: ## Run JS tests via Turborepo
	pnpm run test

lint: lint-go lint-js lint-tokens lint-notices ## Lint everything

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

lint-tokens: ## Fail if any component hardcodes a colour
	@node scripts/check-tokens.mjs

lint-notices: ## Fail if THIRD_PARTY_NOTICES.md no longer matches what is built
	@node scripts/third-party-notices.mjs --check

notices: ## Regenerate THIRD_PARTY_NOTICES.md from the app's real dependencies
	@node scripts/third-party-notices.mjs

fetch-binaries: ## Download + verify sidecar binaries for this machine
	./scripts/fetch-binaries.sh

clean: ## Remove build output and fetched binaries (keeps the download cache)
	rm -rf apps/desktop/build/bin apps/desktop/frontend/dist .turbo
	@echo "Cleaned. Run make fetch-binaries to restore the sidecars."
