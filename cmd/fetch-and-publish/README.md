# Fetch and Publish

A utility to subscribe to external relays and forward events to a target relay.

## Overview

The fetch-and-publish program connects to one or more source relays, subscribes to events matching specified filters, and forwards those events to a target relay. This is useful for relay federation, event harvesting, and creating specialized relay networks.

## Features

- Connect to multiple source relays simultaneously
- Filter events by kind, author, tags, or other criteria
- Forward events to a specified target relay
- Deduplicate events to prevent redundant forwarding
- Support for continuous operation with automatic reconnection
- Detailed logging of forwarding activity

## Usage

```
./fetch-and-publish [options]
```

### Options

- `-sources`: Source relay URLs (comma-separated)
- `-target`: Target relay URL
- `-kinds`: Event kinds to forward (comma-separated)
- `-authors`: Author public keys to forward (comma-separated)
- `-since`: Only forward events newer than timestamp
- `-until`: Only forward events older than timestamp
- `-log`: Enable verbose logging

## Example

```
./fetch-and-publish -sources wss://relay1.com/ws,wss://relay2.com/ws -target ws://localhost:8080/ws -kinds 1,4 -log
```

## Notes

- The program validates events before forwarding them
- It maintains persistent connections to both source and target relays
- The program handles network interruptions gracefully with automatic reconnection
