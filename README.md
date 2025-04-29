# Go Nostr Relay

A basic Nostr relay implementation written in Go with SQLite as the database backend. This relay is built from scratch without using any existing Nostr libraries.

## Features

- WebSocket server for client connections
- SQLite database for event storage
- Basic Nostr protocol implementation:
  - Event validation and storage
  - Subscription handling
  - Event broadcasting
- Client library for publishing events to relays
- Publisher program for generating and publishing text notes

## Requirements

- Go 1.16 or later
- SQLite3

## Installation

1. Clone the repository:

```bash
git clone https://github.com/yourusername/go-nostr-relay.git
cd go-nostr-relay
```

2. Install dependencies:

```bash
go mod tidy
```

## Usage

### Running the Relay

Run the relay with default settings:

```bash
go run cmd/relay/main.go
```

This will start the relay on port 8080 and create a SQLite database file named `nostr.db` in the current directory.

#### Command-line options for the relay

- `-port`: Port to run the relay on (default: "8080")
- `-db`: Path to SQLite database (default: "nostr.db")

Example:

```bash
go run cmd/relay/main.go -port 9000 -db /path/to/custom.db
```

### Publishing Events

The publisher program allows you to generate and publish Nostr events of kind 1 (text notes) to a relay:

```bash
go run cmd/publisher/main.go
```

This will generate a random key pair and publish a single test note to the local relay.

#### Command-line options for the publisher

- `-relay`: URL of the Nostr relay (default: "ws://localhost:8080/ws")
- `-key`: Private key in hex format (optional, will generate if not provided)
- `-num`: Number of events to publish (default: 1, use 0 for interactive mode)

Examples:

```bash
# Publish 5 test notes
go run cmd/publisher/main.go -num 5

# Interactive mode
go run cmd/publisher/main.go -num 0

# Connect to a different relay
go run cmd/publisher/main.go -relay ws://example.com/nostr
```

## Connecting to the Relay

The relay exposes a WebSocket endpoint at `/ws`. You can connect to it using any Nostr client by adding the URL:

```
ws://localhost:8080/ws
```

## Project Structure

- `cmd/relay/`: Relay server implementation
- `cmd/publisher/`: Event publisher program
- `relay/`: Relay core functionality
- `client/`: Client library for interacting with Nostr relays

## Limitations

This is a basic implementation and has the following limitations:

- Limited filter support
- No authentication or rate limiting
- No event deletion (NIP-09) support
- No event replacement (NIP-16) support

## License

MIT
