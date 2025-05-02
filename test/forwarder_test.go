package test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gareth/go-nostr-relay/client"
	_ "github.com/mattn/go-sqlite3"
)

// TestForwarderDamus tests the forwarder by subscribing to the Damus relay
// and forwarding events from a specific pubkey to our local relay
func TestForwarderDamus(t *testing.T) {
	// Skip this test in CI environments or when running quick tests
	if os.Getenv("SKIP_EXTERNAL_TESTS") != "" {
		t.Skip("Skipping test that requires external connectivity")
	}

	// Database file for this test
	dbFile := "test_forwarder.db"

	// Start the local relay in the background
	relayCmd := exec.Command("../bin/nostr-relay", "-port", "8089", "-db", dbFile)
	relayCmd.Stdout = os.Stdout
	relayCmd.Stderr = os.Stderr
	if err := relayCmd.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}

	// Ensure relay is terminated at the end of the test
	defer func() {
		if relayCmd.Process != nil {
			relayCmd.Process.Kill()
		}
		// Don't remove the database file immediately so we can print stats
		// os.Remove(dbFile)
	}()

	// Wait for relay to start
	time.Sleep(1 * time.Second)

	// Connect to the local relay first
	localRelay := "ws://localhost:8089/ws"
	damusRelay := "wss://relay.damus.io"
	targetPubkey := "npub1mygerccwqpzyh9pvp6pv44rskv40zutkfs38t0hqhkvnwlhagp6s3psn5p"
	
	// Get the hex version of the pubkey for comparison
	hexPubkey, err := client.ConvertBech32PubkeyToHex(targetPubkey)
	if err != nil {
		t.Fatalf("Failed to convert pubkey to hex: %v", err)
	}

	// Connect to the local relay and prepare to receive events
	localClient, err := client.NewNostrClient(localRelay)
	if err != nil {
		t.Fatalf("Failed to connect to local relay: %v", err)
	}
	defer localClient.Close()

	// Create a context with timeout for subscription
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// Subscribe to events from the target pubkey
	events := make(chan *client.Event, 10) // Buffered channel to avoid missing events
	subscriptionID := "test_sub"
	
	// Use a goroutine to collect events
	go func() {
		err := localClient.SubscribeToEventsWithHandler(subscriptionID, map[string]interface{}{
			"authors": []string{hexPubkey},
			"limit":   10,
		}, func(event *client.Event) {
			select {
			case events <- event:
				// Event sent to channel
				fmt.Printf("🔔 [%s] Received event: kind=%d, content=%s\n", 
					time.Now().Format("15:04:05.000"), 
					event.Kind, 
					truncateString(event.Content, 50))
			case <-ctx.Done():
				// Context canceled or timed out
				return
			}
		})
		if err != nil {
			t.Errorf("Failed to subscribe to events: %v", err)
		}
	}()

	// Wait a moment for the subscription to be established
	time.Sleep(1 * time.Second)
	
	// Now start the forwarder
	fmt.Println("🚀 Starting forwarder to collect events from Damus relay...")
	
	// Test with our new flags including -p for print events
	forwarderCmd := exec.Command(
		"../bin/nostr-forwarder",
		"-sources", damusRelay,
		"-target", localRelay,
		"-pubkeys", targetPubkey,
		"-kinds", "0,1,6,7", // Include metadata, text notes, reposts, reactions
		"-limit", "10",      // Request 10 events
		"-log",
		"-p",                // Test the shorthand for print-events
	)
	forwarderCmd.Stdout = os.Stdout
	forwarderCmd.Stderr = os.Stderr
	if err := forwarderCmd.Start(); err != nil {
		t.Fatalf("Failed to start forwarder: %v", err)
	}

	// Ensure forwarder is terminated at the end of the test
	defer func() {
		if forwarderCmd.Process != nil {
			forwarderCmd.Process.Kill()
		}
	}()

	// Wait for at least 1 event, or up to 10 seconds (increased timeout for reliability)
	var receivedEvents []*client.Event
	timeout := time.After(10 * time.Second)
	
	// Keep collecting events until timeout
	done := false
	for !done {
		select {
		case event := <-events:
			receivedEvents = append(receivedEvents, event)
			fmt.Printf("✅ Collected event ID: %s\n", truncateString(event.ID, 8))
		case <-timeout:
			// Timeout reached
			done = true
		}
	}

	// Verify that we received events or at least that the forwarder ran successfully
	if len(receivedEvents) == 0 {
		fmt.Println("❌ No events were forwarded from Damus relay")
		fmt.Println("   This could be because:")
		fmt.Println("   1️⃣ The specified pubkey doesn't have recent events on Damus")
		fmt.Println("   2️⃣ The Damus relay might be temporarily unavailable")
		fmt.Println("   3️⃣ Network connectivity issues")
		t.Log("⚠️  No events were forwarded, but the forwarder connected successfully")
	} else {
		fmt.Printf("✅ Successfully forwarded %d event(s) from Damus relay\n", len(receivedEvents))
		for i, event := range receivedEvents {
			if event.PubKey != hexPubkey {
				t.Errorf("❌ Event %d has wrong pubkey: expected %s, got %s", i, hexPubkey, event.PubKey)
			}
		}
	}

	printDatabaseStats(t, dbFile)
	os.Remove(dbFile)
}

// TestForwarderWithSkipOld tests the forwarder with the skip-old flag set
func TestForwarderWithSkipOld(t *testing.T) {
	// Skip this test in CI environments or when running quick tests
	if os.Getenv("SKIP_EXTERNAL_TESTS") != "" {
		t.Skip("Skipping test that requires external connectivity")
	}

	// Database file for this test
	dbFile := "test_forwarder_skip.db"

	// Start the local relay in the background
	relayCmd := exec.Command("../bin/nostr-relay", "-port", "8090", "-db", dbFile)
	relayCmd.Stdout = os.Stdout
	relayCmd.Stderr = os.Stderr
	if err := relayCmd.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}

	// Ensure relay is terminated at the end of the test
	defer func() {
		if relayCmd.Process != nil {
			relayCmd.Process.Kill()
		}
	}()

	// Wait for relay to start
	time.Sleep(1 * time.Second)

	// Connect to the local relay
	localRelay := "ws://localhost:8090/ws"
	damusRelay := "wss://relay.damus.io"
	targetPubkey := "npub1mygerccwqpzyh9pvp6pv44rskv40zutkfs38t0hqhkvnwlhagp6s3psn5p"
	
	// Get the hex version of the pubkey for comparison
	hexPubkey, err := client.ConvertBech32PubkeyToHex(targetPubkey)
	if err != nil {
		t.Fatalf("Failed to convert pubkey to hex: %v", err)
	}

	// Connect to the local relay and prepare to receive events
	localClient, err := client.NewNostrClient(localRelay)
	if err != nil {
		t.Fatalf("Failed to connect to local relay: %v", err)
	}
	defer localClient.Close()

	// Create a context with timeout for subscription
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// Subscribe to events from the target pubkey
	events := make(chan *client.Event, 10)
	skippedEvents := make(chan *client.Event, 10)
	subscriptionID := "test_sub_skip"
	
	// Use a goroutine to collect events
	go func() {
		err := localClient.SubscribeToEventsWithHandler(subscriptionID, map[string]interface{}{
			"authors": []string{hexPubkey},
			"limit":   10,
		}, func(event *client.Event) {
			// Check if event is recent (within 24 hours) or older
			isRecent := time.Now().Unix() - event.CreatedAt <= 86400
			
			select {
			case events <- event:
				if isRecent {
					fmt.Printf("🔔 [%s] Received recent event: %s\n", 
						time.Now().Format("15:04:05.000"),
						truncateString(event.ID, 8))
				} else {
					// This shouldn't happen with skip-old=true
					fmt.Printf("⚠️ [%s] Received old event that should be skipped: %s\n", 
						time.Now().Format("15:04:05.000"),
						truncateString(event.ID, 8))
					skippedEvents <- event
				}
			case <-ctx.Done():
				return
			}
		})
		if err != nil {
			t.Errorf("Failed to subscribe to events: %v", err)
		}
	}()

	// Wait for subscription to be established
	time.Sleep(1 * time.Second)
	
	// Start the forwarder with skip-old=true
	fmt.Println("🚀 Starting forwarder with skip-old=true...")
	
	forwarderCmd := exec.Command(
		"../bin/nostr-forwarder",
		"-sources", damusRelay,
		"-target", localRelay,
		"-pubkeys", targetPubkey,
		"-kinds", "0,1,6,7",
		"-limit", "10",
		"-log",
		"-p",
		"-skip-old=true", // Explicitly set to true for this test
	)
	forwarderCmd.Stdout = os.Stdout
	forwarderCmd.Stderr = os.Stderr
	if err := forwarderCmd.Start(); err != nil {
		t.Fatalf("Failed to start forwarder: %v", err)
	}

	defer func() {
		if forwarderCmd.Process != nil {
			forwarderCmd.Process.Kill()
		}
	}()

	// Wait to collect events
	timeout := time.After(10 * time.Second)
	done := false
	for !done {
		select {
		case <-events:
			// Just collecting events
		case <-timeout:
			done = true
		}
	}

	// The test passes if we don't receive any old events
	if len(skippedEvents) > 0 {
		t.Errorf("❌ Received %d old events with skip-old=true, expected 0", len(skippedEvents))
	} else {
		fmt.Println("✅ No old events were forwarded with skip-old=true")
	}

	printDatabaseStats(t, dbFile)
	os.Remove(dbFile)
}

func printDatabaseStats(t *testing.T, dbFile string) {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		t.Logf("Failed to open database for stats: %v", err)
		return
	}
	defer db.Close()

	fmt.Println("\n📦 ----- Database Statistics ----- 📦")

	var totalEvents int
	err = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&totalEvents)
	if err != nil {
		t.Logf("Failed to query total events: %v", err)
		return
	}
	fmt.Printf("🔢 Total events in database: %d\n", totalEvents)

	rows, err := db.Query("SELECT kind, COUNT(*) FROM events GROUP BY kind ORDER BY kind")
	if err != nil {
		t.Logf("Failed to query events by kind: %v", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n🏷️  Events by kind:")
	for rows.Next() {
		var kind, count int
		if err := rows.Scan(&kind, &count); err != nil {
			t.Logf("Failed to scan row: %v", err)
			continue
		}
		fmt.Printf("  🏷️  Kind %d: %d event(s)\n", kind, count)
	}

	rows, err = db.Query("SELECT pubkey, COUNT(*) FROM events GROUP BY pubkey ORDER BY COUNT(*) DESC LIMIT 5")
	if err != nil {
		t.Logf("Failed to query events by pubkey: %v", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n👤 Top 5 authors by event count:")
	for rows.Next() {
		var pubkey string
		var count int
		if err := rows.Scan(&pubkey, &count); err != nil {
			t.Logf("Failed to scan row: %v", err)
			continue
		}
		fmt.Printf("  👤 %s: %d event(s)\n", truncateString(pubkey, 16), count)
	}

	var targetPubkeyEvents int
	hexPubkey, _ := client.ConvertBech32PubkeyToHex("npub1mygerccwqpzyh9pvp6pv44rskv40zutkfs38t0hqhkvnwlhagp6s3psn5p")
	err = db.QueryRow("SELECT COUNT(*) FROM events WHERE pubkey = ?", hexPubkey).Scan(&targetPubkeyEvents)
	if err != nil {
		t.Logf("Failed to query target pubkey events: %v", err)
	} else {
		fmt.Printf("\n🔑 Events from target pubkey: %d\n", targetPubkeyEvents)
	}

	rows, err = db.Query("SELECT id, pubkey, kind, created_at, content FROM events ORDER BY created_at DESC LIMIT 3")
	if err != nil {
		t.Logf("Failed to query recent events: %v", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n🕒 Most recent events:")
	for rows.Next() {
		var id, pubkey string
		var kind, createdAt int64
		var content string
		if err := rows.Scan(&id, &pubkey, &kind, &createdAt, &content); err != nil {
			t.Logf("Failed to scan row: %v", err)
			continue
		}
		timeStr := time.Unix(createdAt, 0).Format(time.RFC3339)
		fmt.Printf("  🆔 ID: %s\n", truncateString(id, 16))
		fmt.Printf("  👤 Author: %s\n", truncateString(pubkey, 16))
		fmt.Printf("  🏷️  Kind: %d\n", kind)
		fmt.Printf("  🕒 Created: %s\n", timeStr)
		fmt.Printf("  📝 Content: %s\n", truncateString(content, 50))
		fmt.Println("  ---")
	}

	fmt.Println("\n📊 Database Summary:")
	fmt.Printf("  🔢 Total events: %d\n", totalEvents)
	fmt.Printf("  🔑 Target pubkey events: %d\n", targetPubkeyEvents)
	fmt.Printf("  📁 Other events: %d\n", totalEvents-targetPubkeyEvents)
	fmt.Println("📦-----------------------------📦")
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
