# Go Nostr Relay

A simple Nostr relay and client implementation in Go with SQLite storage.

## Components

- **Relay**: WebSocket server that validates and stores Nostr events
- **Publisher**: Command-line tool to publish events to relays
- **Monitor**: Tool to track database changes in real-time

## Quick Start

Build all components:
```bash
make build
```

Run the relay:
```bash
make run-relay
```

Publish a note:
```bash
make run-publisher
```

Monitor database changes:
```bash
make run-monitor
```

## Custom Options

Run relay on a specific port:
```bash
make run-relay-custom PORT=8090
```

Publish multiple notes:
```bash
make run-publisher-custom ARGS="-num 5"
```

Interactive publisher mode:
```bash
make run-publisher-interactive
```

## Testing

Run all tests:
```bash
make test
```

## Project Structure

- `cmd/`: Command-line applications
- `relay/`: Core relay implementation
- `client/`: Client library for Nostr protocol
- `test/`: Integration tests
- `bin/`: Compiled binaries
