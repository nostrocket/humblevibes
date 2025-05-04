# Nostr Test Client

A simple client for testing and debugging Nostr relay connections.

## Overview

The test client provides a lightweight, command-line interface for interacting with Nostr relays. It's designed for developers to test relay functionality, debug connections, and verify event handling without needing a full Nostr client application.

## Features

- Connect to any Nostr relay
- Subscribe to events with customizable filters
- Publish test events with various kinds and content
- Monitor real-time relay responses
- Test NIP compliance
- Simulate client behaviors

## Usage

```
./testclient [options]
```

### Options

- `-relay`: URL of the relay to connect to
- `-subscribe`: Subscribe to events (with optional filter)
- `-publish`: Publish a test event
- `-key`: Private key for signing events (hex format)
- `-content`: Content for published events
- `-kind`: Event kind for published events
- `-interactive`: Run in interactive mode
- `-timeout`: Connection timeout in seconds

## Examples

### Subscribe to all events
```
./testclient -relay ws://localhost:8080/ws -subscribe
```

### Publish a test event
```
./testclient -relay ws://localhost:8080/ws -publish -key <private-key> -content "Test message" -kind 1
```

### Interactive mode
```
./testclient -relay ws://localhost:8080/ws -interactive
```

## Notes

- The test client is primarily for development and debugging purposes
- It provides raw access to relay messages for detailed inspection
- The interactive mode allows for dynamic testing of relay functionality
