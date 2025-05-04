# Nostr Monitor

A real-time monitoring tool for tracking events in a Nostr database.

## Overview

The monitor program connects to a Nostr database and provides real-time updates as new events are added. It's useful for debugging, development, and monitoring relay activity without needing to use a full Nostr client.

## Features

- Real-time monitoring of database changes
- Filtering options for specific event kinds, authors, or content
- Formatted display of event details including timestamps, authors, and content
- Support for monitoring remote databases
- Configurable refresh rates and display options

## Usage

```
./nostr-monitor [options]
```

### Options

- `-db`: Path to the Nostr SQLite database (default: nostr.db)
- `-kinds`: Filter by event kinds (comma-separated)
- `-authors`: Filter by author public keys (comma-separated)
- `-search`: Filter by content search term
- `-refresh`: Refresh rate in seconds (default: 1)
- `-format`: Output format (text, json)

## Example

```
./nostr-monitor -db ./nostr.db -kinds 1,4 -search "nostr" -refresh 2
```

## Notes

- The monitor uses efficient SQL queries to minimize database load
- It can be run alongside an active relay without performance impact
- The tool is particularly useful for development and debugging
