# ==============================================================================
# 👑 Toron Web Server & Edge Gateway — Build Automation Makefile
# ==============================================================================

BINARY_NAME=toron
BUILD_DIR=bin
MAIN_SRC=./cmd/toron

.PHONY: all build dummy run test test-v clean docker-up docker-down graph help

all: build

## build: Compiles the Toron edge gateway binary into bin/
build:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary in $(BUILD_DIR)/$(BINARY_NAME)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_SRC)
	@echo "==> Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

## build-darwin-arm64: Compiles Toron binary for macOS Apple Silicon (ARM64)
build-darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary for macOS ARM64 in $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_SRC)
	@echo "==> macOS ARM64 Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

## build-linux-arm64: Compiles Toron binary for Linux ARM64 (aarch64)
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary for Linux ARM64 in $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_SRC)
	@echo "==> Linux ARM64 Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

## build-linux-amd64: Compiles Toron binary for Linux AMD64 (x86_64)
build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary for Linux AMD64 in $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_SRC)
	@echo "==> Linux AMD64 Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

## build-windows-amd64: Compiles Toron binary for Windows AMD64 (x86_64)
build-windows-amd64:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary for Windows AMD64 in $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_SRC)
	@echo "==> Windows AMD64 Build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

## build-windows-arm64: Compiles Toron binary for Windows ARM64
build-windows-arm64:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building Toron binary for Windows ARM64 in $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe..."
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_SRC)
	@echo "==> Windows ARM64 Build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe"

## build-all: Cross-compiles binaries for macOS, Linux, and Windows (ARM64 & AMD64)
build-all: build-darwin-arm64 build-linux-arm64 build-linux-amd64 build-windows-amd64 build-windows-arm64

## dummy: Compiles the dummy microservice cluster binary into bin/
dummy:
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building dummy microservice binary in $(BUILD_DIR)/dummy..."
	go build -o $(BUILD_DIR)/dummy ./dummy-services
	@echo "==> Build complete: $(BUILD_DIR)/dummy"

## run: Builds and launches Toron server using config.yaml
run: build
	@echo "==> Starting Toron server..."
	./$(BUILD_DIR)/$(BINARY_NAME) -config config.yaml

## test: Runs the full unit test suite across all packages
test:
	@echo "==> Running full unit test suite..."
	go test ./pkg/...

## test-v: Runs the unit test suite with verbose log output
test-v:
	@echo "==> Running verbose unit test suite..."
	go test -v ./pkg/...

## clean: Removes build artifacts and bin directory
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) toron dummy

## docker-up: Starts Docker Compose container microservices
docker-up:
	@echo "==> Launching Docker Compose microservices..."
	docker compose up -d

## docker-down: Stops Docker Compose container microservices
docker-down:
	@echo "==> Stopping Docker Compose microservices..."
	docker compose down

## graph: Re-syncs the AST Knowledge Graph
graph:
	@echo "==> Updating AST Knowledge Graph..."
	graphify update .

## help: Displays available Makefile targets
help:
	@echo "Toron Makefile Targets:"
	@echo "  make build       - Compile Toron binary to bin/toron"
	@echo "  make dummy       - Compile dummy microservices to bin/dummy"
	@echo "  make run         - Build and run Toron server"
	@echo "  make test        - Run all unit tests"
	@echo "  make clean       - Remove bin/ directory and build files"
	@echo "  make docker-up   - Launch Docker Compose containers"
	@echo "  make docker-down - Stop Docker Compose containers"
	@echo "  make graph       - Re-sync graphify AST Knowledge Graph"
