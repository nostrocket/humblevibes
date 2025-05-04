# Nostr Content Export

A tool to export event content from specific authors to text files.

## Overview

The content export program retrieves events from a Nostr database and exports their content to text files. It's particularly useful for backing up notes, creating archives, or migrating content between platforms.

## Features

- Export content from specific authors (by public key or npub)
- Filter by event kinds
- Sort content chronologically
- Format output with timestamps and metadata
- Support for custom output file paths
- Option to include or exclude replies and reposts

## Usage

```
./nostr-export-content [options]
```

### Options

- `-db`: Path to the Nostr SQLite database (default: nostr.db)
- `-pubkey`: Author's public key or npub to export content from
- `-kinds`: Event kinds to export (comma-separated, default: 1)
- `-output`: Output file path (default: <pubkey>_content.txt)
- `-format`: Output format (text, json, markdown)
- `-include-replies`: Include replies in the export
- `-since`: Only export events newer than timestamp
- `-until`: Only export events older than timestamp

## Example

```
./nostr-export-content -db ./nostr.db -pubkey npub1... -kinds 1,30023 -output author_notes.txt
```

## Notes

- The program handles both hex and npub format public keys
- Content is exported in chronological order by default
- The tool preserves event metadata like timestamps and tags in the export
