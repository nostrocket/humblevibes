.PHONY: all build clean run-relay run-publisher run-monitor test

# Binary output names
RELAY_BIN=nostr-relay
PUBLISHER_BIN=nostr-publisher
MONITOR_BIN=nostr-monitor

# Build all binaries
all: build

# Build both relay and publisher
build:
	go build -o $(RELAY_BIN) ./cmd/relay
	go build -o $(PUBLISHER_BIN) ./cmd/publisher
	go build -o $(MONITOR_BIN) ./cmd/monitor

# Clean up binaries
clean:
	rm -f $(RELAY_BIN) $(PUBLISHER_BIN) $(MONITOR_BIN)

# Run the relay server
run-relay:
	go run ./cmd/relay/main.go

# Run the publisher with default settings (single message)
run-publisher:
	go run ./cmd/publisher/main.go

# Run the publisher in interactive mode
run-publisher-interactive:
	go run ./cmd/publisher/main.go -num 0

# Run the publisher with multiple messages
run-publisher-multi:
	go run ./cmd/publisher/main.go -num 5

# Run the database monitor
run-monitor:
	go run ./cmd/monitor/main.go

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod tidy

# Build and install binaries to GOPATH/bin
install:
	go install ./cmd/relay
	go install ./cmd/publisher
	go install ./cmd/monitor

# Help target
help:
	@echo "Available targets:"
	@echo "  all                 - Build all binaries"
	@echo "  build               - Build relay, publisher, and monitor binaries"
	@echo "  clean               - Remove built binaries"
	@echo "  run-relay           - Run the relay server"
	@echo "  run-publisher       - Run the publisher (single message)"
	@echo "  run-publisher-interactive - Run the publisher in interactive mode"
	@echo "  run-publisher-multi - Run the publisher with 5 messages"
	@echo "  run-monitor         - Run the database monitor"
	@echo "  test                - Run all tests"
	@echo "  deps                - Install dependencies"
	@echo "  install             - Install binaries to GOPATH/bin"
