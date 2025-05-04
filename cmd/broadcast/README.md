# Nostr Broadcast

A utility to broadcast a Nostr event to multiple relays simultaneously.

## Overview

The broadcast program retrieves an event from the local `nostr.db` database and publishes it to all online relays discovered from nostr.watch. It uses goroutines and waitgroups to connect to all relays simultaneously, ensuring efficient broadcasting.

## Features

- Retrieves events from the local SQLite database by event ID
- Fetches a list of active relays from nostr.watch
- Deduplicates relay URLs to ensure no relay is connected to more than once
- Broadcasts the event concurrently to all relays using goroutines
- Provides real-time feedback with emoji indicators for successful and failed broadcasts
- Reports detailed error information for failed connections

## Usage

```
./nostr-broadcast <event-id>
```

## Example

```
./nostr-broadcast 3bf7c38639c09128ce5d95b3e0e8f8ade1a9f4d2d1534880302eec34c3f5a58e
```

## Output

The program provides visual feedback during the broadcast process:

- ✅ Successful broadcasts are marked with a green checkmark
- ❌ Failed broadcasts are marked with a red X and include error details
- 📊 A summary of successful and failed broadcasts is provided at the end
- 🎉 A celebration emoji indicates when all broadcasts succeed

## Notes

- The program requires a local `nostr.db` SQLite database in the current directory
- The event must exist in the database before broadcasting
- The program validates the event's existence before making any connections to relays
