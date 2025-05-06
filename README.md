# Go Nostr Relay

A simple Nostr relay and client implementation in Go with SQLite storage.

## Components

- **Relay**: The main Nostr relay server
- **Publisher**: Tool to publish events to relays
- **Monitor**: Tool to track database changes in real-time
- **Forwarder**: Tool to subscribe to external relays and forward events to your local relay
- **Content Export**: Tool to export event content from specific authors to text files
- **Broadcast**: Tool to broadcast a Nostr event to all online relays listed on nostr.watch

## Quick Start

Build all components:
```bash
make all
```

Run the relay:
```bash
make run-relay
# or directly:
./bin/relay
```

Publish a note:
```bash
make run-publish-sample-events
# or with custom options:
./bin/publish-sample-events -relay ws://localhost:8080/ws -content "Hello, Nostr!" -kind 1
```

Monitor database changes:
```bash
make run-monitor
```

Forward events from other relays:
```bash
make run-fetch-and-publish-custom ARGS="-sources wss://relay.damus.io -target ws://localhost:8080/ws -kinds 1,4"
# or directly:
./bin/fetch-and-publish -sources wss://relay.damus.io -target ws://localhost:8080/ws -kinds 1,4
```

Export event content from a pubkey:
```bash
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec
```

Broadcast an event to multiple relays:
```bash
make run-broadcast EVENT_ID=<event-id>
# or directly:
./bin/broadcast <event-id>
```

## Relay Options

The relay supports several command-line options:

- `-port`: Port to run the relay on (default: 8080)
- `-db`: Path to SQLite database (default: "nostr.db")
- `-verbose`: Enable verbose logging of received events
- `-v`: Short form for verbose logging
- `-timeout`: WebSocket connection timeout in seconds (default: 0, no timeout)

Examples:

```bash
# Run with default settings
make run-relay

# Run with custom port
make run-relay-custom ARGS="-port 9000"

# Run with verbose logging
make run-relay-custom ARGS="-v"

# Run with custom database
make run-relay-custom ARGS="-db custom_nostr.db"
```

## Forwarder Options

The forwarder supports several command-line options:

- `-sources`: Comma-separated list of source relay URLs to subscribe to
- `-target`: Target relay URL to forward events to (default: "ws://localhost:8080/ws")
- `-kinds`: Comma-separated list of event kinds to forward
- `-pubkeys`: Comma-separated list of public keys to filter by (supports both hex and bech32/npub formats)
- `-since`: Only forward events newer than this Unix timestamp (0 = no limit)
- `-until`: Only forward events older than this Unix timestamp (0 = no limit)
- `-limit`: Maximum number of events to request from each source relay
- `-batch`: Number of events to forward in a batch (default: 10)
- `-log`: Log event details when forwarding
- `-discover`: Automatically discover relays using nostr.watch API
- `-relays`: Number of relays to auto-discover (default: 10)
- `-nip65`: Discover user's preferred relays from their NIP-65 (kind 10002) events

Examples:

```bash
# Forward events from specific authors
make run-fetch-and-publish-custom ARGS="-sources wss://relay.damus.io -pubkeys 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

# Forward events using NIP-65 relay discovery
make run-fetch-and-publish-custom ARGS="-nip65 -pubkeys npub1sn0wdenkukak0d9dfczzeacvhkrgz92ak56egt7vdgzn8pv2wfqqhrjdv9"

# Forward recent events (last 24 hours)
make run-fetch-and-publish-custom ARGS="-sources wss://relay.damus.io -since $(date -v-1d +%s)"

# Forward only reactions and metadata
make run-fetch-and-publish-custom ARGS="-sources wss://relay.damus.io -kinds 7,0"

# Forward from multiple relays with auto-discovery
make run-fetch-and-publish-custom ARGS="-discover -relays 20 -kinds 1,4 -log"
```

## Broadcast Tool

The broadcast tool sends a Nostr event to all online relays listed on nostr.watch.

```bash
# Broadcast an event by ID
make run-broadcast EVENT_ID=<event-id>

# Or directly
./bin/broadcast <event-id>
```

The tool:
1. Retrieves the event from the local database by ID
2. Fetches the list of online relays from nostr.watch
3. Broadcasts the event concurrently to all relays
4. Reports success/failure statistics

## Content Export Options

The content export tool supports several command-line options:

- `--pubkey`: (Required) The public key to filter events by
- `--db`: Path to SQLite database (default: "nostr.db")
- `--output`: Output file path (default: "<shortened_pubkey>.txt")
- `--sort`: Sort order of events: 'asc' or 'desc' by creation time (default: "asc")
- `--separator`: Separator between event contents (default: two newlines)

Examples:

```bash
# Export all content from a specific pubkey
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec

# Export in reverse chronological order to a specific file
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec ARGS="--sort desc --output my_export.txt"
```

## Monitor Tool

The monitor tool tracks changes to the database in real-time:

```bash
# Run the monitor
make run-monitor

# Run with custom options
make run-monitor-custom ARGS="-db custom_nostr.db -kinds 1,4"
```

## Testing

Run the tests:
```bash
# Run core relay tests
make test

# Run all tests including integration tests
make test-all
```

## Project Structure

- `cmd/`: Command-line applications
  - `relay/`: Main relay server
  - `broadcast/`: Event broadcasting tool
  - `export-content/`: Content export tool
  - `fetch-and-publish/`: Event forwarding tool
  - `monitor/`: Database monitoring tool
  - `publish-sample-events/`: Event publishing tool
- `relay/`: Core relay implementation
- `client/`: Client library for Nostr protocol
- `lib/`: Shared libraries
  - `crypto/`: Cryptographic operations
  - `utils/`: Utility functions
- `test/`: Integration tests
