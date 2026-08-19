# Hedioum Dynamic Pool Tunnel - Build Automation
# This Makefile automates the cross-compilation for Linux environments.

BINARY_NAME=hedioum-tunnel
MAIN_PATH=./cmd/hedioum
BUILD_DIR=bin

# Build metadata injected into the binary for `hedioum-tunnel version`.
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Linker flags:
# -s disables symbol table, -w disables DWARF generation (~50% smaller binary).
# -X injects build metadata (Version stays the AppVersion const used by self-update).
LDFLAGS=-ldflags="-s -w -X main.Commit=$(COMMIT) -X main.Date=$(DATE)"

.PHONY: all build-linux build-linux-arm64 build-linux-armv7 docker docker-multiarch clean fmt deps

all: build-linux build-linux-arm64

build-linux: deps fmt
	@echo "Building highly optimized Linux AMD64 static binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "[✓] AMD64 Build complete! Static binary is ready at: $(BUILD_DIR)/$(BINARY_NAME)"

build-linux-arm64: deps fmt
	@echo "Building highly optimized Linux ARM64 static binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-arm64 $(MAIN_PATH)
	@echo "[✓] ARM64 Build complete! Static binary is ready at: $(BUILD_DIR)/$(BINARY_NAME)-arm64"

build-linux-armv7: deps fmt
	@echo "Building Linux ARMv7 static binary (older ARM MikroTik / SBCs)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-armv7 $(MAIN_PATH)
	@echo "[✓] ARMv7 Build complete! Static binary is ready at: $(BUILD_DIR)/$(BINARY_NAME)-armv7"

# Build a single-arch container image for the host architecture.
docker:
	docker build -t ghcr.io/hedioum/pool-tunnel:latest .

# Build and push a multi-arch image (amd64 + arm64 + armv7). Requires a buildx
# builder and a logged-in registry; override IMAGE to your repo/tag.
IMAGE ?= ghcr.io/hedioum/pool-tunnel:latest
docker-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t $(IMAGE) --push .

clean:
	@echo "Cleaning up build artifacts..."
	rm -rf $(BUILD_DIR)/
	@echo "[✓] Clean complete."

fmt:
	@echo "Formatting Go source code..."
	go fmt ./...

deps:
	@echo "Resolving and downloading Go modules..."
	go mod tidy