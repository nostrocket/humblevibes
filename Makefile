.PHONY: all build clean run-relay run-relay-custom run-relay-timeout run-publish-sample-events run-publish-sample-events-custom run-publish-sample-events-interactive run-publish-sample-events-multi run-fetch-and-publish run-fetch-and-publish-custom run-fetch-and-publish-relay run-export-content run-export-content-custom harvest-and-export harvest-and-export-custom test test-unit test-integration test-all fetch-and-publish publish-sample-events export-content broadcast

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

# Run tests for the relay package only
test:
	@echo "🧪 Running relay tests..."
	@go test -v ./relay > test-output.log 2>&1 || (echo "❌ Some tests failed. See summary below:"; grep -A 3 "FAIL" test-output.log; echo "📝 Full test output saved to test-output.log"; exit 1)
	@echo "✅ All relay tests passed!"
	@rm -f test-output.log
