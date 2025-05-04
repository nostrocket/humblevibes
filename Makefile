.PHONY: all build clean run-relay run-relay-custom run-publisher run-publisher-custom run-publisher-interactive run-publisher-multi run-monitor run-forwarder run-forwarder-custom run-forwarder-relay run-reverse-forwarder run-reverse-forwarder-custom run-reverse-forwarder-discover run-export-content run-export-content-custom harvest-and-export harvest-and-export-custom test test-unit test-integration test-all test-forwarder

# Binary output directory
BIN_DIR=bin

# Binary output names
RELAY_BIN=$(BIN_DIR)/nostr-relay
PUBLISHER_BIN=$(BIN_DIR)/nostr-publisher
MONITOR_BIN=$(BIN_DIR)/nostr-monitor
FORWARDER_BIN=$(BIN_DIR)/nostr-forwarder
REVERSE_FORWARDER_BIN=$(BIN_DIR)/nostr-reverse-forwarder
EXPORT_CONTENT_BIN=$(BIN_DIR)/nostr-export-content

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
	go build -o $(FORWARDER_BIN) ./cmd/forwarder
	go build -o $(REVERSE_FORWARDER_BIN) ./cmd/reverse-forwarder
	go build -o $(EXPORT_CONTENT_BIN) ./cmd/export-content

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISHER_BIN) $(MONITOR_BIN) $(FORWARDER_BIN) $(REVERSE_FORWARDER_BIN) $(EXPORT_CONTENT_BIN)
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

# Run the reverse forwarder with default settings
run-reverse-forwarder: $(REVERSE_FORWARDER_BIN)
	$(REVERSE_FORWARDER_BIN) -source "ws://localhost:8080/ws" $(ARGS)

# Run the reverse forwarder with custom arguments
# Usage: make run-reverse-forwarder-custom ARGS="-source ws://example.com/ws -targets ws://relay1.com/ws,ws://relay2.com/ws"
run-reverse-forwarder-custom: $(REVERSE_FORWARDER_BIN)
	$(REVERSE_FORWARDER_BIN) $(ARGS)

# Run the reverse forwarder with specific source and discover target relays
# Usage: make run-reverse-forwarder-discover SOURCE="ws://example.com/ws" RELAYS=20
run-reverse-forwarder-discover: $(REVERSE_FORWARDER_BIN)
	$(REVERSE_FORWARDER_BIN) -source "$(SOURCE)" -relays $(RELAYS) $(ARGS)

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
	go install ./cmd/reverse-forwarder
	go install ./cmd/export-content

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
	@echo "  run-forwarder       - Run the forwarder with default settings"
	@echo "  run-forwarder-custom - Run the forwarder with custom arguments"
	@echo "  run-forwarder-relay - Run the forwarder with specific source and target relays"
	@echo "  run-reverse-forwarder - Run the reverse forwarder with default settings"
	@echo "  run-reverse-forwarder-custom - Run the reverse forwarder with custom arguments"
	@echo "  run-reverse-forwarder-discover - Run the reverse forwarder with a source relay and discover targets"
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
