.PHONY: all build clean run-relay run-relay-custom run-relay-timeout run-publisher run-publisher-custom run-publisher-interactive run-publisher-multi run-monitor run-forwarder run-forwarder-custom run-forwarder-relay run-export-content run-export-content-custom harvest-and-export harvest-and-export-custom test test-unit test-integration test-all test-forwarder relay publisher monitor forwarder export-content tester broadcast

# Binary output directory
BIN_DIR=bin

# Binary output names
RELAY_BIN=$(BIN_DIR)/nostr-relay
PUBLISHER_BIN=$(BIN_DIR)/nostr-publisher
MONITOR_BIN=$(BIN_DIR)/nostr-monitor
FORWARDER_BIN=$(BIN_DIR)/nostr-forwarder
EXPORT_CONTENT_BIN=$(BIN_DIR)/nostr-export-content
TESTER_BIN=$(BIN_DIR)/nostr-tester
BROADCAST_BIN=$(BIN_DIR)/nostr-broadcast

# Create bin directory if it doesn't exist
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Build all binaries
all: $(BIN_DIR) relay publisher monitor forwarder export-content tester broadcast

relay:
	go build -o $(RELAY_BIN) ./cmd/relay

publisher:
	go build -o $(PUBLISHER_BIN) ./cmd/publisher

monitor:
	go build -o $(MONITOR_BIN) ./cmd/monitor

forwarder:
	go build -o $(FORWARDER_BIN) ./cmd/forwarder

export-content:
	go build -o $(EXPORT_CONTENT_BIN) ./cmd/export-content

tester:
	go build -o $(TESTER_BIN) ./cmd/tester

broadcast:
	go build -o $(BROADCAST_BIN) ./cmd/broadcast

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISHER_BIN) $(MONITOR_BIN) $(FORWARDER_BIN) $(EXPORT_CONTENT_BIN) $(TESTER_BIN) $(BROADCAST_BIN)
	rm -f test_nostr.db

# Run the relay server
run-relay: $(RELAY_BIN)
	$(RELAY_BIN) $(ARGS)

# Run the relay server on a specific port
# Usage: make run-relay-custom PORT=8081
run-relay-custom: $(RELAY_BIN)
	$(RELAY_BIN) -port $(PORT)

# Run the relay server with a specific timeout
# Usage: make run-relay-timeout TIMEOUT=60
run-relay-timeout: $(RELAY_BIN)
	$(RELAY_BIN) -timeout $(TIMEOUT)

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
	$(MONITOR_BIN) $(ARGS)

# Run the forwarder with default settings
run-forwarder: $(FORWARDER_BIN)
	$(FORWARDER_BIN) -sources "ws://localhost:8080/ws" $(ARGS)

# Run the forwarder with custom arguments
# Usage: make run-forwarder-custom ARGS="-sources ws://example.com/ws,ws://another.com/ws -kinds 1,4"
run-forwarder-custom: $(FORWARDER_BIN)
	$(FORWARDER_BIN) $(ARGS)

# Run the forwarder with specific source and target relays
# Usage: make run-forwarder-relay SOURCE="ws://example.com/ws" TARGET="ws://localhost:9000/ws"
run-forwarder-relay: $(FORWARDER_BIN)
	$(FORWARDER_BIN) -sources "$(SOURCE)" -target "$(TARGET)" $(ARGS)

# Run the content export tool
run-export-content: $(EXPORT_CONTENT_BIN)
	$(EXPORT_CONTENT_BIN) $(ARGS)

# Run the content export tool with custom arguments
# Usage: make run-export-content-custom PUBKEY=<pubkey> ARGS="-sort desc -output custom.txt"
run-export-content-custom: $(EXPORT_CONTENT_BIN)
	$(EXPORT_CONTENT_BIN) --pubkey $(PUBKEY) $(ARGS)

# Harvest content from 500 relays for a specific pubkey, then export it (using default database)
# Uses a dual-phase approach: runs forwarder twice with different relay discovery settings
# Usage: make harvest-and-export PUBKEY=<pubkey or npub>
harvest-and-export: $(RELAY_BIN) $(FORWARDER_BIN) $(EXPORT_CONTENT_BIN)
	@./scripts/dual-phase-harvest.sh "$(PUBKEY)" 8899 180 nostr.db "$(PUBKEY)_harvested.txt"

# Harvest content from 500 relays for a specific pubkey using a dedicated database, then export it
# Uses a dual-phase approach: runs forwarder twice with different relay discovery settings
# Usage: make harvest-and-export-custom PUBKEY=<pubkey or npub>
harvest-and-export-custom: $(RELAY_BIN) $(FORWARDER_BIN) $(EXPORT_CONTENT_BIN)
	@./scripts/dual-phase-harvest.sh "$(PUBKEY)" 8899 180 "harvest_$(PUBKEY).db" "$(PUBKEY)_harvested.txt"

# Run unit tests only
test-unit:
	go test -v ./relay ./client

# Run integration tests only
test-integration:
	go test -v ./test

# Run forwarder test with Damus relay
test-forwarder:
	go test -v ./test -run TestForwarderDamus

# Run all tests
test-all: test-unit test-integration test-forwarder

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
	go install ./cmd/forwarder
	go install ./cmd/export-content
	go install ./cmd/tester
	go install ./cmd/broadcast

# Run a full validation test
validate: build test-all
	@echo "All validation tests passed!"

# Help target
help:
	@echo "Available targets:"
	@echo "  all                 - Build all binaries"
	@echo "  build               - Build relay, publisher, and monitor binaries"
	@echo "  clean               - Remove built binaries"
	@echo "  run-relay           - Run the relay server (no connection timeout by default)"
	@echo "  run-relay-custom    - Run the relay server on a specific port"
	@echo "  run-relay-timeout   - Run the relay server with a specific timeout"
	@echo "  run-publisher       - Run the publisher (single message)"
	@echo "  run-publisher-custom - Run the publisher with custom arguments"
	@echo "  run-publisher-interactive - Run the publisher in interactive mode"
	@echo "  run-publisher-multi - Run the publisher with 5 messages"
	@echo "  run-monitor         - Run the database monitor"
	@echo "  run-forwarder       - Run the forwarder with default settings"
	@echo "  run-forwarder-custom - Run the forwarder with custom arguments"
	@echo "  run-forwarder-relay - Run the forwarder with specific source and target relays"
	@echo "  run-export-content  - Run the content export tool"
	@echo "  run-export-content-custom - Run the content export tool with specific pubkey and options"
	@echo "  harvest-and-export  - Harvest content using the existing database with dual-phase relay discovery"
	@echo "  harvest-and-export-custom - Harvest content using a dedicated database with dual-phase relay discovery"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-integration    - Run integration tests only"
	@echo "  test-all            - Run all tests"
	@echo "  test                - Run all tests (alias for test-all)"
	@echo "  test-forwarder      - Run forwarder test with Damus relay"
	@echo "  validate            - Build and run all tests"
	@echo "  deps                - Install dependencies"
	@echo "  install             - Install binaries to GOPATH/bin"
