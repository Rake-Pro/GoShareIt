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
APP         := $(DIST)/GoShareIt.app
CMD         := ./cmd/goshareit

# Host architecture for the darwin build (arm64 on Apple Silicon, amd64 on Intel).
HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

export VERSION
export BUNDLE_ID

.PHONY: all test vet fmt-check build-darwin bundle sign notarize release clean help

all: test vet fmt-check ## Run the core checks.

help: ## List targets.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'

test: ## Build/test the pure-Go core with cgo OFF.
	CGO_ENABLED=0 $(GO) test ./...

vet: ## go vet the module.
	$(GO) vet ./...

fmt-check: ## Fail if any file is not gofmt-clean.
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

build-darwin: ## Build the cgo binary for the host arch into dist/goshareit.
	@mkdir -p $(DIST)
	CGO_ENABLED=1 GOOS=darwin GOARCH=$(HOST_ARCH) \
		$(GO) build -o $(BIN) $(CMD)
	@echo "built $(BIN) (darwin/$(HOST_ARCH))"

bundle: build-darwin ## Assemble dist/GoShareIt.app from the built binary.
	VERSION=$(VERSION) BUNDLE_ID=$(BUNDLE_ID) BIN=$(BIN) APP=$(APP) \
		scripts/bundle.sh

sign: ## Codesign the .app (needs DEVELOPER_ID_APP).
	APP=$(APP) scripts/sign.sh

notarize: ## Notarize + staple the .app (needs AC_NOTARY_PROFILE).
	APP=$(APP) scripts/notarize.sh

release: bundle sign notarize ## bundle -> sign -> notarize -> staple.
	@echo "release complete: $(APP) (v$(VERSION))"

clean: ## Remove build artifacts.
	rm -rf $(DIST)
