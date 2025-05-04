# Nostr Publisher

A command-line tool to create and publish Nostr events to relays.

## Overview

The publisher program allows users to create, sign, and publish Nostr events to one or more relays. It supports various event types (kinds) and provides both interactive and batch publishing modes.

## Features

- Create and sign Nostr events with proper cryptographic signatures
- Support for multiple event kinds (text notes, metadata, contacts, etc.)
- Publish to multiple relays simultaneously
- Interactive mode for guided event creation
- Support for private key management (generate, import, export)
- Verification of successful event publication

## Usage

```
./publisher [options]
```

### Options

- `-relay`: Specify relay URL(s) to publish to (comma-separated)
- `-key`: Private key in hex format
- `-content`: Event content
- `-kind`: Event kind (1=text note, 0=metadata, etc.)
- `-tags`: Event tags in JSON format
- `-interactive`: Run in interactive mode

## Examples

### Publish a text note
```
./publisher -relay wss://relay.example.com/ws -key <private-key> -content "Hello Nostr!" -kind 1
```

### Interactive mode
```
./publisher -interactive
```

## Notes

- Private keys should be kept secure and not shared
- The program verifies relay responses to confirm successful publication
- In interactive mode, the program guides you through the event creation process
