# Nostr Test Publisher

A specialized tool for testing event publication to Nostr relays.

## Overview

The test publisher is designed specifically for testing and benchmarking the event publication process to Nostr relays. It allows for controlled, high-volume event generation and publication to measure relay performance and reliability.

## Features

- Generate and publish test events in bulk
- Configurable event parameters (kind, content, tags)
- Measure publication success rates and latency
- Support for concurrent publishing
- Detailed reporting of publication results
- Customizable event signing with test keys

## Usage

```
./testpublisher [options]
```

### Options

- `-relay`: URL of the relay to publish to
- `-count`: Number of test events to publish
- `-key`: Private key for signing events (hex format)
- `-kind`: Event kind (default: 1)
- `-content`: Content template for events
- `-concurrent`: Number of concurrent publishing threads
- `-report`: Generate a detailed publication report

## Examples

### Publish 10 test events
```
./testpublisher -relay ws://localhost:8080/ws -count 10 -key <private-key>
```

### Benchmark with concurrent publishing
```
./testpublisher -relay ws://localhost:8080/ws -count 100 -concurrent 10 -report
```

## Notes

- The test publisher is primarily for development, testing, and benchmarking
- It can generate significant load on relays when used with high event counts
- The tool provides detailed metrics on publication performance and reliability
- Test events are properly signed but contain generated content
