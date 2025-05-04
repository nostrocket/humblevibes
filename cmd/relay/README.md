# Nostr Relay

A lightweight Nostr relay server with SQLite backend.

## Overview

This program implements a Nostr relay server that validates, stores, and serves Nostr events according to the Nostr protocol specification. It uses SQLite for persistent storage and provides a WebSocket interface for clients to connect and interact with the relay.

## Features

- Full implementation of Nostr protocol (NIP-01)
- SQLite database for efficient event storage and retrieval
- Event validation including signature verification
- Subscription filtering support
- WebSocket server for real-time communication
- Configurable settings for port, database path, and more

## Usage

```
./relay [options]
```

### Options

- `-port`: Set the port for the WebSocket server (default: 8080)
- `-db`: Set the path to the SQLite database (default: nostr.db)
- `-log`: Enable verbose logging
- `-timeout`: Set connection timeout in seconds

## Example

```
./relay -port 9000 -db custom_nostr.db -log
```

## Notes

- The relay follows the reference implementation standards for ID computation and signature verification
- Events are validated before being stored in the database
- The server handles multiple concurrent client connections efficiently
