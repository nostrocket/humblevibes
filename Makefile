.PHONY: all build clean run-relay run-relay-custom run-relay-timeout run-publish-sample-events run-publish-sample-events-custom run-publish-sample-events-interactive run-publish-sample-events-multi run-fetch-and-publish run-fetch-and-publish-custom run-fetch-and-publish-relay run-export-content run-export-content-custom harvest-and-export harvest-and-export-custom test test-unit test-integration test-all test-fetch-and-publish fetch-and-publish publish-sample-events export-content broadcast

# Binary output directory
BIN_DIR=bin

# Binary output names
RELAY_BIN=$(BIN_DIR)/relay
PUBLISH_SAMPLE_EVENTS_BIN=$(BIN_DIR)/publish-sample-events
FETCH_AND_PUBLISH_BIN=$(BIN_DIR)/fetch-and-publish
EXPORT_CONTENT_BIN=$(BIN_DIR)/export-content
BROADCAST_BIN=$(BIN_DIR)/broadcast

# Create bin directory if it doesn't exist
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Build all binaries
all: $(BIN_DIR) relay publish-sample-events fetch-and-publish export-content broadcast

relay:
	go build -o $(RELAY_BIN) ./cmd/relay

publish-sample-events:
	go build -o $(PUBLISH_SAMPLE_EVENTS_BIN) ./cmd/publish-sample-events

fetch-and-publish:
	go build -o $(FETCH_AND_PUBLISH_BIN) ./cmd/fetch-and-publish

export-content:
	go build -o $(EXPORT_CONTENT_BIN) ./cmd/export-content

broadcast:
	go build -o $(BROADCAST_BIN) ./cmd/broadcast

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISH_SAMPLE_EVENTS_BIN) $(FETCH_AND_PUBLISH_BIN) $(EXPORT_CONTENT_BIN) $(BROADCAST_BIN)
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

# Run the publish-sample-events with default settings (single message)
run-publish-sample-events: $(PUBLISH_SAMPLE_EVENTS_BIN)
	$(PUBLISH_SAMPLE_EVENTS_BIN) $(ARGS)

# Run the publish-sample-events with custom arguments
# Usage: make run-publish-sample-events-custom ARGS="-relay ws://localhost:8081/ws -num 3"
run-publish-sample-events-custom: $(PUBLISH_SAMPLE_EVENTS_BIN)
	$(PUBLISH_SAMPLE_EVENTS_BIN) $(ARGS)

# Run the publish-sample-events in interactive mode
run-publish-sample-events-interactive: $(PUBLISH_SAMPLE_EVENTS_BIN)
	$(PUBLISH_SAMPLE_EVENTS_BIN) -num 0

# Run the publish-sample-events with multiple messages
run-publish-sample-events-multi: $(PUBLISH_SAMPLE_EVENTS_BIN)
	$(PUBLISH_SAMPLE_EVENTS_BIN) -num 5

# Run the fetch-and-publish with default settings
run-fetch-and-publish: $(FETCH_AND_PUBLISH_BIN)
	$(FETCH_AND_PUBLISH_BIN) -sources "ws://localhost:8080/ws" $(ARGS)

# Run the fetch-and-publish with custom arguments
# Usage: make run-fetch-and-publish-custom ARGS="-sources ws://example.com/ws,ws://another.com/ws -kinds 1,4"
run-fetch-and-publish-custom: $(FETCH_AND_PUBLISH_BIN)
	$(FETCH_AND_PUBLISH_BIN) $(ARGS)

# Run the fetch-and-publish with specific source and target relays
# Usage: make run-fetch-and-publish-relay SOURCE="ws://example.com/ws" TARGET="ws://localhost:9000/ws"
run-fetch-and-publish-relay: $(FETCH_AND_PUBLISH_BIN)
	$(FETCH_AND_PUBLISH_BIN) -sources "$(SOURCE)" -target "$(TARGET)" $(ARGS)

# Run the content export tool
run-export-content: $(EXPORT_CONTENT_BIN)
	$(EXPORT_CONTENT_BIN) $(ARGS)

# Run the content export tool with custom arguments
# Usage: make run-export-content-custom PUBKEY=<pubkey> ARGS="-sort desc -output custom.txt"
run-export-content-custom: $(EXPORT_CONTENT_BIN)
	$(EXPORT_CONTENT_BIN) --pubkey $(PUBKEY) $(ARGS)

# Harvest content from 500 relays for a specific pubkey, then export it (using default database)
# Uses a dual-phase approach: runs fetch-and-publish twice with different relay discovery settings
# Usage: make harvest-and-export PUBKEY=<pubkey or npub>
harvest-and-export: $(RELAY_BIN) $(FETCH_AND_PUBLISH_BIN) $(EXPORT_CONTENT_BIN)
	@./scripts/dual-phase-harvest.sh "$(PUBKEY)" 8899 180 nostr.db "$(PUBKEY)_harvested.txt"

# Harvest content from 500 relays for a specific pubkey using a dedicated database, then export it
# Uses a dual-phase approach: runs fetch-and-publish twice with different relay discovery settings
# Usage: make harvest-and-export-custom PUBKEY=<pubkey or npub>
harvest-and-export-custom: $(RELAY_BIN) $(FETCH_AND_PUBLISH_BIN) $(EXPORT_CONTENT_BIN)
	@./scripts/dual-phase-harvest.sh "$(PUBKEY)" 8899 180 "harvest_$(PUBKEY).db" "$(PUBKEY)_harvested.txt"

# Run unit tests only
test-unit:
	@echo "🧪 Running unit tests..."
	@go test -v ./relay ./client > test-output.log 2>&1 || (echo "❌ Some tests failed. See summary below:"; grep -A 3 "FAIL" test-output.log; echo "📝 Full test output saved to test-output.log"; exit 1)
	@echo "✅ All unit tests passed!"
	@rm -f test-output.log

# Run integration tests
test-integration:
	@echo "🧪 Running integration tests..."
	@go test -v ./test > test-integration-output.log 2>&1 || (echo "❌ Some integration tests failed. See summary below:"; grep -A 3 "FAIL" test-integration-output.log; echo "📝 Full test output saved to test-integration-output.log"; exit 1)
	@echo "✅ All integration tests passed!"
	@rm -f test-integration-output.log

# Run fetch-and-publish test with Damus relay
test-fetch-and-publish:
	@echo "🧪 Running fetch-and-publish tests with Damus relay..."
	@go test -v ./test -run TestFetchAndPublishDamus > test-fetch-output.log 2>&1 || (echo "❌ Fetch-and-publish test failed. See summary below:"; grep -A 3 "FAIL" test-fetch-output.log; echo "📝 Full test output saved to test-fetch-output.log"; exit 1)
	@echo "✅ Fetch-and-publish test passed!"
	@rm -f test-fetch-output.log

# Run all tests
test-all: 
	@echo "🧪 Running all tests..."
	@$(MAKE) -s test-unit || UNIT_FAILED=1
	@$(MAKE) -s test-integration || INTEGRATION_FAILED=1
	@$(MAKE) -s test-fetch-and-publish || FETCH_FAILED=1
	@if [ "$$UNIT_FAILED" = "1" ] || [ "$$INTEGRATION_FAILED" = "1" ] || [ "$$FETCH_FAILED" = "1" ]; then \
		echo "❌ Some tests failed. See above for details."; \
		exit 1; \
	fi
	@echo "✅ All tests passed successfully!"

# Default test target runs all tests
test: test-all

# Install dependencies
deps:
	go mod tidy

# Build and install binaries to GOPATH/bin
install:
	go install ./cmd/relay
	go install ./cmd/publish-sample-events
	go install ./cmd/fetch-and-publish
	go install ./cmd/export-content
	go install ./cmd/broadcast

# Run a full validation test
validate: build test-all
	@echo "All validation tests passed!"

# Help target
help:
	@echo "Available targets:"
	@echo "  all                 - Build all binaries"
	@echo "  build               - Build relay, publish-sample-events, and fetch-and-publish binaries"
	@echo "  clean               - Remove built binaries"
	@echo "  run-relay           - Run the relay server (no connection timeout by default)"
	@echo "  run-relay-custom    - Run the relay server on a specific port"
	@echo "  run-relay-timeout   - Run the relay server with a specific timeout"
	@echo "  run-publish-sample-events       - Run the publish-sample-events (single message)"
	@echo "  run-publish-sample-events-custom - Run the publish-sample-events with custom arguments"
	@echo "  run-publish-sample-events-interactive - Run the publish-sample-events in interactive mode"
	@echo "  run-publish-sample-events-multi - Run the publish-sample-events with 5 messages"
	@echo "  run-fetch-and-publish       - Run the fetch-and-publish with default settings"
	@echo "  run-fetch-and-publish-custom - Run the fetch-and-publish with custom arguments"
	@echo "  run-fetch-and-publish-relay - Run the fetch-and-publish with specific source and target relays"
	@echo "  run-export-content  - Run the content export tool"
	@echo "  run-export-content-custom - Run the content export tool with specific pubkey and options"
	@echo "  harvest-and-export  - Harvest content using the existing database with dual-phase relay discovery"
	@echo "  harvest-and-export-custom - Harvest content using a dedicated database with dual-phase relay discovery"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-integration    - Run integration tests only"
	@echo "  test-all            - Run all tests"
	@echo "  test                - Run all tests (alias for test-all)"
	@echo "  test-fetch-and-publish      - Run fetch-and-publish test with Damus relay"
	@echo "  validate            - Build and run all tests"
	@echo "  deps                - Install dependencies"
	@echo "  install             - Install binaries to GOPATH/bin"
