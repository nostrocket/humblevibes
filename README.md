# Go Nostr Relay

A simple Nostr relay and client implementation in Go with SQLite storage.

## Components

- **Relay**: Nostr relay server with SQLite backend
- **Publisher**: Tool to publish events to relays
- **Monitor**: Tool to track database changes in real-time
- **Forwarder**: Tool to subscribe to external relays and forward events to your local relay
- **Content Export**: Tool to export event content from specific authors to text files
- **nostr-broadcast**: Tool to broadcast a Nostr event to all online relays listed on nostr.watch

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

## nostr-broadcast

Broadcast a Nostr event to all online relays listed on nostr.watch.

### Build

```
make bin/nostr-broadcast
```

### Usage

```
./bin/nostr-broadcast <event-id>
```

- Retrieves the event from `nostr.db` by event ID.
- Fetches all online relays from nostr.watch.
- Broadcasts the event concurrently to all relays.
- Reports any errors or failures per relay.

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

## Automated Content Harvesting

The project includes automated workflows to harvest content from a specific pubkey across multiple relays and export it to a text file:

```bash
# Use the existing database (default)
make harvest-and-export PUBKEY=npub1mygerccwqpzyh9pvp6pv44rskv40zutkfs38t0hqhkvnwlhagp6s3psn5p

# Use a dedicated database just for this harvesting operation
make harvest-and-export-custom PUBKEY=npub1mygerccwqpzyh9pvp6pv44rskv40zutkfs38t0hqhkvnwlhagp6s3psn5p
```

These commands perform the following steps automatically:

1. Starts a local relay on port 8899 (with either the default database or a dedicated one)
2. Runs a dual-phase discovery process:
   - First phase: Connects to 250 relays and collects content for 3 minutes
   - Second phase: Connects to 250 different relays and collects content for 3 minutes
3. Stops all processes and exports the collected content in chronological order

The dual-phase approach ensures more comprehensive relay coverage by connecting to different relay sets in each phase. This helps discover content that might only be available on certain relays.

The exported content will be saved to a file named `<PUBKEY>_harvested.txt`.

Use the default database (`harvest-and-export`) when you want to:
- Add harvested content to your main database
- Keep all events in one place for future access
- Build on existing content you've already collected

Use a dedicated database (`harvest-and-export-custom`) when you want to:
- Create an isolated collection for a specific purpose
- Avoid affecting your main database
- Export content from just one harvesting session

This is useful for quickly collecting and aggregating all content from a specific author across the Nostr network.

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
