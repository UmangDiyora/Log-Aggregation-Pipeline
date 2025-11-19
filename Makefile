# Variables
BINARY_DIR=bin
AGENT_BINARY=$(BINARY_DIR)/logagent
SERVER_BINARY=$(BINARY_DIR)/logserver
CLI_BINARY=$(BINARY_DIR)/logcli
UI_BINARY=$(BINARY_DIR)/logui

GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

# Default target
.PHONY: all
all: build

# Create bin directory
$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

# Build all components
.PHONY: build
build: build-agent build-server build-cli build-ui

# Build agent
.PHONY: build-agent
build-agent: $(BINARY_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(AGENT_BINARY) ./cmd/agent

# Build server
.PHONY: build-server
build-server: $(BINARY_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/server

# Build CLI
.PHONY: build-cli
build-cli: $(BINARY_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(CLI_BINARY) ./cmd/cli

# Build UI
.PHONY: build-ui
build-ui: $(BINARY_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(UI_BINARY) ./cmd/ui

# Run tests
.PHONY: test
test:
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage: test
	$(GO) tool cover -html=coverage.txt -o coverage.html

# Run benchmarks
.PHONY: bench
bench:
	$(GO) test -bench=. -benchmem ./...

# Format code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Lint code
.PHONY: lint
lint:
	golangci-lint run ./...

# Vet code
.PHONY: vet
vet:
	$(GO) vet ./...

# Tidy dependencies
.PHONY: tidy
tidy:
	$(GO) mod tidy

# Download dependencies
.PHONY: deps
deps:
	$(GO) mod download

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BINARY_DIR)
	rm -f coverage.txt coverage.html

# Install tools
.PHONY: install-tools
install-tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Docker build
.PHONY: docker-build
docker-build:
	docker build -t logpipeline/agent:latest -f deployments/Dockerfile.agent .
	docker build -t logpipeline/server:latest -f deployments/Dockerfile.server .

# Run agent locally
.PHONY: run-agent
run-agent: build-agent
	./$(AGENT_BINARY) -config configs/agent.yaml

# Run server locally
.PHONY: run-server
run-server: build-server
	./$(SERVER_BINARY) -config configs/server.yaml

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make build          - Build all components"
	@echo "  make build-agent    - Build agent only"
	@echo "  make build-server   - Build server only"
	@echo "  make build-cli      - Build CLI only"
	@echo "  make build-ui       - Build UI only"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage"
	@echo "  make bench          - Run benchmarks"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code"
	@echo "  make vet            - Vet code"
	@echo "  make tidy           - Tidy dependencies"
	@echo "  make deps           - Download dependencies"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make docker-build   - Build Docker images"
	@echo "  make run-agent      - Run agent locally"
	@echo "  make run-server     - Run server locally"
