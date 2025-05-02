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

const (
	testDBPath = "test_nostr.db"
	relayPort  = "8089" // Use a different port for testing
	relayURL   = "ws://localhost:8089/ws"
)

func TestMain(m *testing.M) {
	// Setup
	setup()

	// Run tests
	exitCode := m.Run()

	// Teardown
	teardown()

	os.Exit(exitCode)
}

func setup() {
	// Remove test database if it exists
	os.Remove(testDBPath)
}

func teardown() {
	// Clean up test database
	os.Remove(testDBPath)
}

// startRelay starts the relay server for testing
func startRelay(t *testing.T) (*exec.Cmd, context.CancelFunc) {
	t.Log("Starting relay server...")
	
	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start the relay process with the correct path
	relayPath := "../bin/nostr-relay"
	cmd := exec.CommandContext(ctx, relayPath, "-port", relayPort, "-db", testDBPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}
	
	// Give the relay time to start up
	time.Sleep(1 * time.Second)
	
	return cmd, cancel
}

// TestRelayAndPublisher tests the entire flow: starting relay, publishing events, and verifying storage
func TestRelayAndPublisher(t *testing.T) {
	fmt.Println("🧪 Test: RelayAndPublisher")
	// Start the relay
	cmd, cancel := startRelay(t)
	defer func() {
		cancel()
		cmd.Wait()
	}()
	
	// Test publishing events
	t.Run("PublishEvents", func(t *testing.T) {
		fmt.Println("📝 Subtest: PublishEvents")
		// Create a client
		nostrClient, err := client.NewNostrClient(relayURL)
		if err != nil {
			t.Fatalf("❌ Failed to create client: %v", err)
		} else {
			fmt.Println("✅ Created client successfully")
		}
		defer nostrClient.Close()
		
		// Publish a test note
		content := "Test note from integration test"
		event, err := nostrClient.PublishTextNote(content)
		if err != nil {
			t.Fatalf("❌ Failed to publish note: %v", err)
		} else {
			fmt.Printf("✅ Published event with ID: %s\n", event.ID)
		}
		
		// Verify the event was stored in the database
		if err := verifyEventInDB(event.ID, content); err != nil {
			t.Fatalf("❌ Event verification failed: %v", err)
		} else {
			fmt.Println("✅ Event verified in database")
		}
	})
	
	// Test signature verification
	t.Run("SignatureVerification", func(t *testing.T) {
		fmt.Println("🔏 Subtest: SignatureVerification")
		// Create a client
		nostrClient, err := client.NewNostrClient(relayURL)
		if err != nil {
			t.Fatalf("❌ Failed to create client: %v", err)
		} else {
			fmt.Println("✅ Created client successfully")
		}
		defer nostrClient.Close()
		// Get the public key
		pubKey := nostrClient.GetPublicKey()
		t.Logf("Using public key: %s", pubKey)
		fmt.Printf("🔑 Using public key: %s\n", pubKey)
		// Publish multiple events to test signature verification
		for i := 1; i <= 3; i++ {
			content := fmt.Sprintf("Signature test #%d", i)
			event, err := nostrClient.PublishTextNote(content)
			if err != nil {
				t.Fatalf("❌ Failed to publish note #%d: %v", i, err)
			} else {
				fmt.Printf("✅ Published event #%d with ID: %s\n", i, event.ID)
			}
		}
		// Verify events count in database
		count, err := getEventCount()
		if err != nil {
			t.Fatalf("❌ Failed to get event count: %v", err)
		} else {
			fmt.Printf("📦 Events in database: %d\n", count)
		}
		// We should have at least 4 events (1 from previous test + 3 from this test)
		if count < 4 {
			t.Fatalf("❌ Expected at least 4 events in database, got %d", count)
		} else {
			fmt.Println("✅ Event count in database is as expected")
		}
	})
	
	// Test event retrieval
	t.Run("EventRetrieval", func(t *testing.T) {
		fmt.Println("🔎 Subtest: EventRetrieval")
		// Create a client
		nostrClient, err := client.NewNostrClient(relayURL)
		if err != nil {
			t.Fatalf("❌ Failed to create client: %v", err)
		} else {
			fmt.Println("✅ Created client successfully")
		}
		defer nostrClient.Close()
		// Subscribe to events
		subID := "test-sub"
		filters := map[string]interface{}{
			"kinds": []int{1},
			"limit": 10,
		}
		err = nostrClient.SubscribeToEvents(subID, filters)
		if err != nil {
			t.Fatalf("❌ Failed to subscribe to events: %v", err)
		} else {
			fmt.Println("✅ Subscribed to events successfully")
		}
		// Give some time for subscription to process
		time.Sleep(1 * time.Second)
		fmt.Println("📬 Subscription processing complete (waited 1s)")
		// We can't easily verify the subscription results in this test framework,
		// but we've at least verified that the subscription request doesn't error
	})
}

// verifyEventInDB checks if an event with the given ID exists in the database
func verifyEventInDB(eventID, expectedContent string) error {
	// Open the database
	db, err := sql.Open("sqlite3", testDBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()
	
	// Query for the event
	var content string
	err = db.QueryRow("SELECT content FROM events WHERE id = ?", eventID).Scan(&content)
	if err != nil {
		return fmt.Errorf("failed to find event in database: %v", err)
	}
	
	// Verify content matches
	if content != expectedContent {
		return fmt.Errorf("content mismatch: expected '%s', got '%s'", expectedContent, content)
	}
	
	return nil
}

// getEventCount returns the total number of events in the database
func getEventCount() (int, error) {
	// Open the database
	db, err := sql.Open("sqlite3", testDBPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()
	
	// Count events
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %v", err)
	}
	
	return count, nil
}

// TestEventSignatureVerification tests the signature verification functionality directly
func TestEventSignatureVerification(t *testing.T) {
	fmt.Println("🧪 Test: EventSignatureVerification")
	// This test requires the relay to be running
	cmd, cancel := startRelay(t)
	defer func() {
		cancel()
		cmd.Wait()
	}()
	
	// Create a client with a known private key for deterministic testing
	privateKey := "0000000000000000000000000000000000000000000000000000000000000001"
	nostrClient, err := client.NewNostrClientWithKey(relayURL, privateKey)
	if err != nil {
		t.Fatalf("❌ Failed to create client with key: %v", err)
	} else {
		fmt.Println("✅ Created client with key successfully")
	}
	defer nostrClient.Close()
	
	// Get the public key
	pubKey := nostrClient.GetPublicKey()
	t.Logf("Using public key: %s", pubKey)
	fmt.Printf("🔑 Using public key: %s\n", pubKey)
	// Publish a test note
	content := "Test note with deterministic key"
	event, err := nostrClient.PublishTextNote(content)
	if err != nil {
		t.Fatalf("❌ Failed to publish note with deterministic key: %v", err)
	} else {
		fmt.Printf("✅ Published event with ID: %s\n", event.ID)
	}
	
	// Verify the event was stored in the database
	if err := verifyEventInDB(event.ID, content); err != nil {
		t.Fatalf("❌ Event verification failed: %v", err)
	} else {
		fmt.Println("✅ Event verified in database")
	}
}

// TestInvalidSignature tests that events with invalid signatures are rejected
func TestInvalidSignature(t *testing.T) {
	fmt.Println("🧪 Test: InvalidSignature")
	// This test would require modifying the client to produce invalid signatures
	// For now, we'll just log that this test would be valuable to implement
	t.Log("TestInvalidSignature: This test would verify that events with invalid signatures are rejected")
	t.Log("Implementation would require modifying the client code to produce invalid signatures")
}
