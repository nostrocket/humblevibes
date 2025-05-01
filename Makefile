.PHONY: all build clean run-relay run-relay-custom run-publisher run-publisher-custom run-publisher-interactive run-publisher-multi run-monitor test test-unit test-integration test-all

# Binary output directory
BIN_DIR=bin

# Binary output names
RELAY_BIN=$(BIN_DIR)/nostr-relay
PUBLISHER_BIN=$(BIN_DIR)/nostr-publisher
MONITOR_BIN=$(BIN_DIR)/nostr-monitor

# Build all binaries
all: build

# Create bin directory if it doesn't exist
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Build both relay and publisher
build: $(BIN_DIR)
	go build -o $(RELAY_BIN) ./cmd/relay
	go build -o $(PUBLISHER_BIN) ./cmd/publisher
	go build -o $(MONITOR_BIN) ./cmd/monitor

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISHER_BIN) $(MONITOR_BIN)
	rm -f test_nostr.db

# Run the relay server
run-relay: $(RELAY_BIN)
	$(RELAY_BIN) $(ARGS)

# Run the relay server on a specific port
# Usage: make run-relay-custom PORT=8081
run-relay-custom: $(RELAY_BIN)
	$(RELAY_BIN) -port $(PORT)

# Run the publisher with default settings (single message)
run-publisher: $(PUBLISHER_BIN)
	$(PUBLISHER_BIN) $(ARGS)

# Run the publisher with custom arguments
# Usage: make run-publisher-custom ARGS="-relay ws://localhost:8081/ws -num 3"
run-publisher-custom: $(PUBLISHER_BIN)
	$(PUBLISHER_BIN) $(ARGS)

# Run the publisher in interactive mode
run-publisher-interactive: $(PUBLISHER_BIN)
	$(PUBLISHER_BIN) -num 0

# Run the publisher with multiple messages
run-publisher-multi: $(PUBLISHER_BIN)
	$(PUBLISHER_BIN) -num 5

# Run the database monitor
run-monitor: $(MONITOR_BIN)
	$(MONITOR_BIN)

# Run unit tests only
test-unit:
	go test -v ./relay ./client

# Run integration tests only
test-integration: build
	go test -v ./test

# Run all tests
test-all: test-unit test-integration

# Default test target runs all tests
test: test-all

# Install dependencies
deps:
	go mod tidy

# Build and install binaries to GOPATH/bin
install:
	go install ./cmd/relay
	go install ./cmd/publisher
	go install ./cmd/monitor

# Run a full validation test
validate: build test-all
	@echo "All validation tests passed!"

# Help target
help:
	@echo "Available targets:"
	@echo "  all                 - Build all binaries"
	@echo "  build               - Build relay, publisher, and monitor binaries"
	@echo "  clean               - Remove built binaries"
	@echo "  run-relay           - Run the relay server"
	@echo "  run-relay-custom    - Run the relay server on a specific port"
	@echo "  run-publisher       - Run the publisher (single message)"
	@echo "  run-publisher-custom - Run the publisher with custom arguments"
	@echo "  run-publisher-interactive - Run the publisher in interactive mode"
	@echo "  run-publisher-multi - Run the publisher with 5 messages"
	@echo "  run-monitor         - Run the database monitor"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-integration    - Run integration tests only"
	@echo "  test-all            - Run all tests"
	@echo "  test                - Run all tests (alias for test-all)"
	@echo "  validate            - Build and run all tests"
	@echo "  deps                - Install dependencies"
	@echo "  install             - Install binaries to GOPATH/bin"
