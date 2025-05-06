.PHONY: all build clean run-relay run-relay-custom run-relay-timeout run-publish-sample-events run-publish-sample-events-custom run-publish-sample-events-interactive run-publish-sample-events-multi run-fetch-and-publish run-fetch-and-publish-custom run-fetch-and-publish-relay run-export-content run-export-content-custom harvest-and-export harvest-and-export-custom test test-unit test-integration test-all fetch-and-publish publish-sample-events export-content broadcast monitor run-monitor run-monitor-custom

# Binary output directory
BIN_DIR=bin

# Binary output names
RELAY_BIN=$(BIN_DIR)/relay
PUBLISH_SAMPLE_EVENTS_BIN=$(BIN_DIR)/publish-sample-events
FETCH_AND_PUBLISH_BIN=$(BIN_DIR)/fetch-and-publish
EXPORT_CONTENT_BIN=$(BIN_DIR)/export-content
BROADCAST_BIN=$(BIN_DIR)/broadcast
MONITOR_BIN=$(BIN_DIR)/monitor

# Create bin directory if it doesn't exist
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Build all binaries
all: $(BIN_DIR)
	go build -o $(RELAY_BIN) ./cmd/relay
	go build -o $(PUBLISH_SAMPLE_EVENTS_BIN) ./cmd/publish-sample-events
	go build -o $(FETCH_AND_PUBLISH_BIN) ./cmd/fetch-and-publish
	go build -o $(EXPORT_CONTENT_BIN) ./cmd/export-content
	go build -o $(BROADCAST_BIN) ./cmd/broadcast
	go build -o $(MONITOR_BIN) ./cmd/monitor

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

monitor:
	go build -o $(MONITOR_BIN) ./cmd/monitor

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISH_SAMPLE_EVENTS_BIN) $(FETCH_AND_PUBLISH_BIN) $(EXPORT_CONTENT_BIN) $(BROADCAST_BIN) $(MONITOR_BIN)
	rm -f test_nostr.db

# Run tests for the relay package only
test:
	@echo "🧪 Running relay tests..."
	@go test -v ./relay > test-output.log 2>&1 || (echo "❌ Some tests failed. See summary below:"; grep -A 3 "FAIL" test-output.log; echo "📝 Full test output saved to test-output.log"; exit 1)
	@echo "✅ All relay tests passed!"
	@rm -f test-output.log

# Run all tests including integration tests
test-all:
	@echo "🧪 Running all tests..."
	@go test -v ./... > test-output.log 2>&1 || (echo "❌ Some tests failed. See summary below:"; grep -A 3 "FAIL" test-output.log; echo "📝 Full test output saved to test-output.log"; exit 1)
	@echo "✅ All tests passed!"
	@rm -f test-output.log

# Run the relay
run-relay:
	./$(RELAY_BIN)

# Run the relay with custom options
run-relay-custom:
	./$(RELAY_BIN) $(ARGS)

# Run the relay with timeout
run-relay-timeout:
	./$(RELAY_BIN) -timeout 60

# Run the monitor
run-monitor:
	./$(MONITOR_BIN)

# Run the monitor with custom options
run-monitor-custom:
	./$(MONITOR_BIN) $(ARGS)

# Run the publish-sample-events tool
run-publish-sample-events:
	./$(PUBLISH_SAMPLE_EVENTS_BIN)

# Run the publish-sample-events tool with custom options
run-publish-sample-events-custom:
	./$(PUBLISH_SAMPLE_EVENTS_BIN) $(ARGS)

# Run the publish-sample-events tool in interactive mode
run-publish-sample-events-interactive:
	./$(PUBLISH_SAMPLE_EVENTS_BIN) -interactive

# Run the publish-sample-events tool with multiple events
run-publish-sample-events-multi:
	./$(PUBLISH_SAMPLE_EVENTS_BIN) -num 5

# Run the fetch-and-publish tool
run-fetch-and-publish:
	./$(FETCH_AND_PUBLISH_BIN)

# Run the fetch-and-publish tool with custom options
run-fetch-and-publish-custom:
	./$(FETCH_AND_PUBLISH_BIN) $(ARGS)

# Run the fetch-and-publish tool with relay options
run-fetch-and-publish-relay:
	./$(FETCH_AND_PUBLISH_BIN) -sources $(SOURCE) -target $(TARGET) $(ARGS)

# Run the export-content tool
run-export-content:
	./$(EXPORT_CONTENT_BIN)

# Run the export-content tool with custom options
run-export-content-custom:
	./$(EXPORT_CONTENT_BIN) --pubkey $(PUBKEY) $(ARGS)

# Run the broadcast tool
run-broadcast:
	./$(BROADCAST_BIN) $(EVENT_ID)
