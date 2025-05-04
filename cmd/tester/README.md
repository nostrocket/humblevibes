# Nostr Tester

A comprehensive testing utility for Nostr relays and protocol implementations.

## Overview

The tester program provides a suite of tests to verify the functionality and compliance of Nostr relays. It helps developers ensure their relay implementations follow the Nostr protocol specifications and handle various edge cases correctly.

## Features

- Comprehensive protocol compliance testing
- Performance and load testing capabilities
- Event validation and signature verification tests
- Subscription filter testing
- Connection handling and timeout tests
- Detailed reporting of test results

## Usage

```
./nostr-tester [options]
```

### Options

- `-relay`: URL of the relay to test
- `-tests`: Specific tests to run (comma-separated)
- `-timeout`: Test timeout in seconds
- `-verbose`: Enable detailed logging of test operations
- `-report`: Generate a detailed test report (format: text, json, html)

## Example

```
./nostr-tester -relay ws://localhost:8080/ws -tests basic,subscription,performance -verbose
```

## Test Categories

- **Basic**: Tests basic relay functionality (connect, publish, subscribe)
- **Protocol**: Tests compliance with NIPs and protocol specifications
- **Performance**: Tests relay performance under load
- **Edge Cases**: Tests handling of malformed events, invalid signatures, etc.
- **Subscription**: Tests subscription filtering and event delivery

## Notes

- The tester can be used for both development and production relay verification
- Test results include detailed information about failures and compliance issues
- The tool is regularly updated to include tests for new NIPs and protocol features
