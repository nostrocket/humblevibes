# Go Nostr Relay

A simple Nostr relay and client implementation in Go with SQLite storage.

## Components

- **Relay**: WebSocket server that validates and stores Nostr events
- **Publisher**: Command-line tool to publish events to relays
- **Monitor**: Tool to track database changes in real-time
- **Forwarder**: Tool to subscribe to external relays and forward events to your local relay
- **Content Export**: Tool to export event content from specific authors to text files

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

Forward events from other relays:
```bash
make run-forwarder-custom ARGS="-sources ws://example.com/ws,ws://another.com/ws -kinds 1,4"
```

Export event content from a pubkey:
```bash
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec
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

## Forwarder Options

The forwarder supports several command-line options:

- `-sources`: Comma-separated list of source relay URLs to subscribe to
- `-target`: Target relay URL to forward events to (default: "ws://localhost:8080/ws")
- `-kinds`: Comma-separated list of event kinds to forward (default: "1")
- `-pubkeys`: Comma-separated list of public keys to filter by (supports both hex and bech32/npub formats)
- `-since`: Only forward events newer than this Unix timestamp (0 = no limit)
- `-until`: Only forward events older than this Unix timestamp (0 = no limit)
- `-limit`: Maximum number of events to request from each source relay (default: 100)
- `-batch`: Number of events to forward in a batch (default: 10)
- `-log`: Log event details when forwarding (default: false)

Examples:

```bash
# Forward events from specific authors (using hex format)
make run-forwarder-custom ARGS="-sources ws://relay.example.com/ws -pubkeys 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

# Forward events from specific authors (using bech32/npub format)
make run-forwarder-custom ARGS="-sources ws://relay.example.com/ws -pubkeys npub1sn0wdenkukak0d9dfczzeacvhkrgz92ak56egt7vdgzn8pv2wfqqhrjdv9"

# Forward recent events (last 24 hours)
make run-forwarder-custom ARGS="-sources ws://relay.example.com/ws -since $(date -v-1d +%s)"

# Forward only reactions and metadata
make run-forwarder-custom ARGS="-sources ws://relay.example.com/ws -kinds 7,0"

# Forward from one relay to another (simplified syntax)
make run-forwarder-relay SOURCE="ws://relay.example.com/ws" TARGET="ws://localhost:9000/ws"

# Forward from multiple relays to a custom target with additional filters
make run-forwarder-relay SOURCE="ws://relay1.com/ws,ws://relay2.com/ws" TARGET="ws://custom-relay.com/ws" ARGS="-kinds 1,4 -log"

## Content Export Options

The content export tool supports several command-line options:

- `--pubkey`: (Required) The public key to filter events by
- `--db`: Path to SQLite database (default: "nostr.db")
- `--output`: Output file path (default: "<shortened_pubkey>.txt")
- `--sort`: Sort order of events: 'asc' or 'desc' by creation time (default: "asc")
- `--separator`: Separator between event contents (default: two newlines)

Examples:

```bash
# Export all content from a specific pubkey in chronological order
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec

# Export all content in reverse chronological order to a specific file
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec ARGS="--sort desc --output my_export.txt"

# Use a custom separator
make run-export-content-custom PUBKEY=3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaef47fec ARGS="--separator '\n---\n'"

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
