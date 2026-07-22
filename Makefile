# GoShareIt - build, package and release.
#
# Required environment variables for the signing/notarization targets
# (sign / notarize / release). Set them in your shell before running:
#
#   DEVELOPER_ID_APP   "Developer ID Application: Your Name (TEAMID)"  codesign identity
#   TEAM_ID            Apple Developer Team ID (10-char), e.g. ABCDE12345
#   AC_NOTARY_PROFILE  name of the notarytool keychain profile (see scripts/notarize.sh)
#   BUNDLE_ID          app bundle identifier, default pro.rake.goshareit
#
# VERSION may be overridden:  make release VERSION=1.2.3
#
# Build/test targets do not require any of the above.

# Apple toolchain (cgo) lives on macOS; the Go toolchain path matches the
# project's prescribed environment.
GO          ?= go
VERSION     ?= 0.0.0-dev
BUNDLE_ID   ?= pro.rake.goshareit
DIST        := dist
BIN         := $(DIST)/goshareit
EDITOR_BIN  := $(DIST)/goshareit-editor
APP         := $(DIST)/GoShareIt.app
CMD         := ./cmd/goshareit
EDITOR_CMD  := ./cmd/goshareit-editor

# Host architecture for the darwin build (arm64 on Apple Silicon, amd64 on Intel).
HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# Stamp the version into the binary for the self-updater.
LDFLAGS := -X github.com/Rake-Pro/GoShareIt/internal/core/version.Version=$(VERSION)

export VERSION
export BUNDLE_ID

.PHONY: all test vet fmt-check build-darwin bundle sign notarize release dev dev-run clean help

all: test vet fmt-check ## Run the core checks.

help: ## List targets.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'

test: ## Build/test all packages (cgo on the dev host; CI runs the core CGO-off on linux).
	$(GO) test ./...

vet: ## go vet the module.
	$(GO) vet ./...

fmt-check: ## Fail if any file is not gofmt-clean.
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

build-darwin: ## Build the cgo host + editor binaries for the host arch into dist/.
	@mkdir -p $(DIST)
	CGO_ENABLED=1 GOOS=darwin GOARCH=$(HOST_ARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)
	CGO_ENABLED=1 GOOS=darwin GOARCH=$(HOST_ARCH) $(GO) build -ldflags "$(LDFLAGS)" -o $(EDITOR_BIN) $(EDITOR_CMD)
	@echo "built $(BIN) and $(EDITOR_BIN) (darwin/$(HOST_ARCH))"

bundle: build-darwin ## Assemble dist/GoShareIt.app (host + editor) from the built binaries.
	VERSION=$(VERSION) BUNDLE_ID=$(BUNDLE_ID) BIN=$(BIN) EDITOR_BIN=$(EDITOR_BIN) APP=$(APP) \
		scripts/bundle.sh

sign: ## Codesign the .app (needs DEVELOPER_ID_APP).
	APP=$(APP) scripts/sign.sh

notarize: ## Notarize + staple the .app (needs AC_NOTARY_PROFILE).
	APP=$(APP) scripts/notarize.sh

release: bundle sign notarize ## bundle -> sign -> notarize -> staple.
	@echo "release complete: $(APP) (v$(VERSION))"

dev: ## Local loop: build -> bundle -> sign (if DEVELOPER_ID_APP set).
	scripts/dev-build.sh

dev-run: ## Same as dev, then launch the .app.
	scripts/dev-build.sh --open

clean: ## Remove build artifacts.
	rm -rf $(DIST)
