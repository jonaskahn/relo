# relo development, build, and CI targets. `make` prints them all.

BINARY   := relo
BACKEND  := daemon
CONSOLE := console
NPM      := $(CURDIR)/packaging/npm
VERSION ?= 0.1.0
MACOS_MIN_VERSION ?= 12.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X github.com/jonaskahn/relo/internal/cli.version=$(VERSION) -X github.com/jonaskahn/relo/internal/cli.commit=$(COMMIT)

PLATFORMS := linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64
GOLANGCI_LINT_VERSION := v2.13.2

# The dev targets below share one state directory, so the daemon and the
# desktop tray read the same database and neither collides with ~/.relo.
DEV_HOME ?= $(CURDIR)/.dev/relo
DEV_PORT ?= 10201
DEV_URL  ?= http://127.0.0.1:$(DEV_PORT)

.PHONY: help \
	build build-console build-backend build-all \
	sync-assets sync-console icons icons-check \
	run \
	dev-setup dev-backend dev-console \
	fmt fmt-check vet lint lint-backend lint-console tidy \
	test test-backend test-console test-race cover \
	ci ci-backend ci-console ci-assets ci-coverage ci-cross-build ci-package-linux \
	package-linux package-macos package-windows package-all package-clean \
	test-npm npm-pack ci-npm \
	clean

# === help: every target, what it does ===

.DEFAULT_GOAL := help

## print this list
help:
	@if [ -t 1 ] && [ -z "$$NO_COLOR" ] && [ "$$TERM" != dumb ]; then c=1; else c=0; fi; \
	awk -v c=$$c -f scripts/make-help.awk $(firstword $(MAKEFILE_LIST))

# === build: the console is compiled and synced before the daemon embeds it ===

## the full binary
build: build-console sync-console build-backend

## compile the console
build-console: sync-assets
	./scripts/build-console.sh

## compile the daemon
build-backend:
	cd $(BACKEND) && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../$(BINARY) ./cmd/relo

## one binary per platform
build-all: build-console sync-console
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "building dist/$(BINARY)-$$os-$$arch$$ext"; \
		(cd $(BACKEND) && GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o ../dist/$(BINARY)-$$os-$$arch$$ext ./cmd/relo) || exit 1; \
		done

## copy assets/ into the console
sync-assets:
	./scripts/sync-assets.sh

## copy the console build into the daemon
sync-console:
	./scripts/sync-console.sh

## regenerate the icons and the Windows resource object from assets/
icons: sync-assets
	cd $(BACKEND) && go run ./tools/genicons
	./scripts/gen-syso.sh

## fail when the artwork is stale
icons-check:
	./scripts/check-icons.sh

# === run: the built binary, from the state directory RELO_HOME names ===

## run the built binary from RELO_HOME
run: build
	./$(BINARY) daemon run

# === dev: the state directory the editor's launch configurations also read ===

## write the dev home's config once
dev-setup:
	RELO_HOME=$(DEV_HOME) ./scripts/dev-setup.sh $(DEV_PORT)

## daemon against the dev home
dev-backend: dev-setup
	cd $(BACKEND) && RELO_HOME=$(DEV_HOME) go run ./cmd/relo daemon run

## console dev server
dev-console:
	cd $(CONSOLE) && ([ -d node_modules ] || npm ci) && RELO_BACKEND_URL=$(DEV_URL) npm run dev

# === check: format, vet, lint, tests, coverage ===

## gofmt the Go sources
fmt:
	cd $(BACKEND) && gofmt -w $$(git ls-files '*.go')

## fail when Go sources need gofmt
fmt-check:
	@cd $(BACKEND) && unformatted=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed on:"; echo "$$unformatted"; exit 1; fi

## go vet
vet:
	cd $(BACKEND) && go vet ./...

## lint the Go and the console code
lint: lint-backend lint-console

## golangci-lint
lint-backend:
	@cd $(BACKEND) && if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run; \
	fi

## svelte-check, eslint, and prettier
lint-console:
	cd $(CONSOLE) && ([ -d node_modules ] || npm ci) && npm run check && npm run lint && npm run format:check

## go mod tidy
tidy:
	cd $(BACKEND) && go mod tidy

## backend and console tests
test: test-backend test-console

## Go tests with the race detector
test-backend:
	cd $(BACKEND) && CGO_ENABLED=1 go test -race -count=1 ./...

## svelte-check, vitest, and the coverage floor
test-console:
	cd $(CONSOLE) && ([ -d node_modules ] || npm ci) && npm run check && npm run test:coverage

## Go tests, race detector only
test-race:
	cd $(BACKEND) && go test -race ./...

## per-package coverage gate
cover:
	./scripts/check-coverage.sh

# === ci: the gate before a push; packaging needs nfpm and stays out of ci ===

## the gate before a push
ci: ci-backend ci-console ci-assets ci-coverage ci-cross-build

## deps, build, fmt, vet, lint, tests
ci-backend:
	cd $(BACKEND) && go mod verify && go build ./...
	$(MAKE) fmt-check vet lint-backend test-backend

## checks, lint, tests, console build
ci-console: lint-console test-console build-console

## artwork drift check
ci-assets: icons-check

## the coverage gate
ci-coverage: cover

## every platform
ci-cross-build: build-all

## .deb and .rpm, needs nfpm
ci-package-linux: package-linux

# === package: installers and the npm tarball, all pure and offline ===

## remove every derived artifact before packaging
package-clean: clean

## .deb and .rpm for the Relo desktop app
package-linux: build-console sync-console
	./scripts/package-linux.sh

## the macOS bundle
package-macos: build-console sync-console
	MACOS_MIN_VERSION=$(MACOS_MIN_VERSION) ./scripts/package-macos.sh
	MACOS_MIN_VERSION=$(MACOS_MIN_VERSION) ./scripts/check-macos-target.sh

## the Windows installer (Relo-Setup)
package-windows: build-console sync-console
	./scripts/package-windows.sh

## every platform package, plus the npm daemon binaries
package-all: package-clean build-all package-windows package-macos package-linux

## the installer's own tests
test-npm:
	@cd $(NPM) && node --test

## dry-run the npm tarball
npm-pack:
	@cd $(NPM) && npm pack --dry-run --cache $(NPM)/.npm-cache

## installer tests and the tarball
ci-npm: test-npm npm-pack

# === clean: every derived artifact ===

## remove every derived artifact
clean:
	@rm -f $(BINARY) $(BINARY).exe
	@rm -rf dist
	@rm -rf $(CONSOLE)/build $(CONSOLE)/.svelte-kit
	@find $(BACKEND)/internal/dashboard/dist -mindepth 1 -not -name .gitkeep -delete
