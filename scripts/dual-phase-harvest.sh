#!/bin/bash
# This script runs the forwarder with 500 relays for 3 minutes,
# then terminates and runs it again with a different discovery approach.

set -e

# Get command line arguments
PUBKEY=$1
PORT=${2:-8899}
DURATION=${3:-180} # 3 minutes in seconds
DB_PATH=${4:-nostr.db}
OUTPUT_FILE=${5:-"${PUBKEY}_harvested.txt"}

if [ -z "$PUBKEY" ]; then
  echo "Usage: $0 <pubkey> [port] [duration_in_seconds] [db_path] [output_file]"
  exit 1
fi

RELAY_BIN=./bin/nostr-relay
FORWARDER_BIN=./bin/nostr-forwarder
EXPORT_BIN=./bin/nostr-export-content

echo "Starting dual-phase content harvesting for pubkey: $PUBKEY"
echo "1. Starting local relay on port $PORT..."

# Start the relay
$RELAY_BIN -port $PORT -db $DB_PATH -v &
RELAY_PID=$!
echo "Relay started with PID: $RELAY_PID"
echo "Waiting 3 seconds for relay to initialize..."
sleep 3

# First phase: Main relay discovery
echo "2. Starting forwarder with primary relay set..."
$FORWARDER_BIN -target "ws://localhost:$PORT/ws" -pubkeys "$PUBKEY" -kinds 1 -discover 250 -log &
FORWARDER_PID=$!
echo "Forwarder started with PID: $FORWARDER_PID"
echo "Harvesting notes for $(($DURATION / 60)) minutes in phase 1..."
sleep $DURATION

# Stop the first forwarder but keep the relay running
echo "3. Stopping first forwarder run..."
kill $FORWARDER_PID || true
echo "Waiting 2 seconds for the forwarder to terminate..."
sleep 2

# Second phase: Additional relay discovery with different settings
echo "4. Starting forwarder with secondary relay set..."
# Using a different method to effectively get a different set of relays
# We use -skip-old false to potentially find different content
$FORWARDER_BIN -target "ws://localhost:$PORT/ws" -pubkeys "$PUBKEY" -kinds 1 -discover 250 -skip-old false -log &
FORWARDER_PID=$!
echo "Second forwarder started with PID: $FORWARDER_PID"
echo "Harvesting notes for $(($DURATION / 60)) minutes in phase 2..."
sleep $DURATION

# Stop everything
echo "5. Stopping forwarder..."
kill $FORWARDER_PID || true
echo "6. Stopping relay..."
kill $RELAY_PID || true
echo "Waiting 2 seconds for processes to terminate..."
sleep 2

# Export the content
echo "7. Exporting content..."
$EXPORT_BIN --pubkey "$PUBKEY" --db "$DB_PATH" --output "$OUTPUT_FILE" --sort asc
echo "Dual-phase harvesting complete! Content saved to $OUTPUT_FILE"
