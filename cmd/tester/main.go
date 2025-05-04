package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gareth/go-nostr-relay/client"
	"github.com/gareth/go-nostr-relay/lib/utils"
)

var logger = utils.NewLogger("tester")

func main() {
	// Parse command line flags
	relayURL := flag.String("relay", "ws://localhost:8080/ws", "URL of the relay to test")
	contentText := flag.String("content", "Test note from relay tester", "Content of the test note")
	timeout := flag.Int("timeout", 10, "Timeout in seconds for the test")
	flag.Parse()

	logger.Info("Starting relay test against: %s", *relayURL)

	// 1. Generate a keypair
	privateKey, publicKey, err := generateKeypair()
	if err != nil {
		logger.Error("Failed to generate keypair: %v", err)
		os.Exit(1)
	}
	logger.Info("Generated keypair: Public key: %s", publicKey)

	// Create a context with timeout for the entire test
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Connect to the relay
	nostrClient, err := client.NewNostrClient(*relayURL)
	if err != nil {
		logger.Error("Failed to connect to relay: %v", err)
		os.Exit(1)
	}
	defer nostrClient.Close()
	logger.Info("Connected to relay: %s", *relayURL)

	// Channel to signal when we've received the published note
	receivedNote := make(chan *client.Event, 1)
	var receivedNoteMu sync.Mutex
	noteReceived := false

	// 2. Subscribe to the relay to receive events with our pubkey
	subscriptionID := fmt.Sprintf("test_sub_%d", time.Now().Unix())
	filters := map[string]interface{}{
		"authors": []string{publicKey},
		"kinds":  []int{1},
		"limit":  10,
	}

	// Event handler that validates if we receive our own note
	eventHandler := func(event *client.Event) {
		logger.Info("Received event: %s, author: %s", event.ID, event.PubKey)
		
		receivedNoteMu.Lock()
		defer receivedNoteMu.Unlock()
		
		if event.PubKey == publicKey && !noteReceived {
			logger.Info("✅ Successfully received our published note! Content: %s", event.Content)
			noteReceived = true
			receivedNote <- event
		}
	}

	// Subscribe with EOSE callback to know when we've received all historic events
	subscriptionReady := make(chan struct{}, 1)
	
	// Set up a goroutine to handle the subscription
	go func() {
		err := nostrClient.SubscribeToEventsWithHandlerAndEOSE(
			subscriptionID,
			filters,
			eventHandler,
			func() {
				logger.Info("🏁 Subscription established (EOSE received)")
				subscriptionReady <- struct{}{}
			},
		)
		if err != nil {
			logger.Error("Subscription ended with error: %v", err)
		}
	}()

	// Wait for subscription to be established (EOSE received)
	select {
	case <-subscriptionReady:
		logger.Info("👂 Subscription ready, listening for events")
	case <-ctx.Done():
		logger.Error("Timed out waiting for subscription to be established")
		os.Exit(1)
	}

	// 3. Create a kind 1 note and sign it
	event := &client.Event{
		Kind:      1,
		CreatedAt: time.Now().Unix(),
		Tags:      [][]string{},
		Content:   *contentText,
		PubKey:    publicKey,
	}

	// Compute event ID (following the correct serialization approach)
	if err := computeEventID(event); err != nil {
		logger.Error("Failed to compute event ID: %v", err)
		os.Exit(1)
	}

	// Sign the event using the private key
	if err := signEvent(event, privateKey); err != nil {
		logger.Error("Failed to sign event: %v", err)
		os.Exit(1)
	}

	// Verify the event signature is valid
	if ok, err := verifyEventSignature(event); !ok {
		logger.Error("Event signature verification failed: %v", err)
		os.Exit(1)
	}
	logger.Info("Created and signed event: %s", event.ID)

	// 4. Publish the note to the relay
	logger.Info("📤 Publishing note to relay: %s", *contentText)
	success, message, err := nostrClient.PublishExistingEvent(event)
	if err != nil {
		logger.Error("Failed to publish event: %v", err)
		os.Exit(1)
	}
	
	// Some relays may report success correctly, others might return errors even when they accept the event
	// so we'll log the response but continue with the test regardless
	if !success {
		logger.Warn("Relay returned non-success status for our event: %s", message)
		logger.Info("Continuing test to check if the event was actually accepted...")
	} else {
		logger.Info("✅ Note published successfully according to relay response")
	}

	// 5. Wait to see if we receive the note back through our subscription
	// This is the ultimate test of whether the relay accepted our event
	select {
	case receivedEvent := <-receivedNote:
		logger.Info("🎉 Test PASSED: Note published and received successfully")
		logger.Info("Note ID: %s", receivedEvent.ID)
		logger.Info("Note content: %s", receivedEvent.Content)
	case <-ctx.Done():
		logger.Error("❌ Test FAILED: Timed out waiting to receive our published note")
		os.Exit(1)
	}

	// 6. Close the connection and exit
	nostrClient.Close()
	logger.Info("Test completed successfully")
}

// generateKeypair creates a new keypair for testing
func generateKeypair() (privateKey *btcec.PrivateKey, publicKeyHex string, err error) {
	// Generate a private key
	privateKey, err = btcec.NewPrivateKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Get the corresponding public key
	publicKey := privateKey.PubKey()
	
	// Convert the public key to hexadecimal (Nostr format without the prefix)
	publicKeyBytes := publicKey.SerializeCompressed()[1:] // Remove the prefix byte
	publicKeyHex = hex.EncodeToString(publicKeyBytes)

	return privateKey, publicKeyHex, nil
}

// computeEventID computes the ID of the event according to the Nostr spec
func computeEventID(event *client.Event) error {
	// Create event array for serialization exactly as specified in the Nostr protocol
	eventArray := []interface{}{
		0,                 // version: 0
		event.PubKey,      // pubkey
		event.CreatedAt,   // created_at
		event.Kind,        // kind
		event.Tags,        // tags
		event.Content,     // content
	}

	// Serialize the event array
	serializedEvent, err := json.Marshal(eventArray)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Compute the SHA-256 hash of the serialized event
	h := sha256.Sum256(serializedEvent)
	
	// Set the event ID
	event.ID = hex.EncodeToString(h[:])
	
	return nil
}

// signEvent signs the event with the provided private key
func signEvent(event *client.Event, privateKey *btcec.PrivateKey) error {
	// Decode the event ID from hex to bytes
	eventIDBytes, err := hex.DecodeString(event.ID)
	if err != nil {
		return fmt.Errorf("failed to decode event ID: %w", err)
	}

	// Sign the event ID with the private key using Schnorr signatures
	sig, err := schnorr.Sign(privateKey, eventIDBytes)
	if err != nil {
		return fmt.Errorf("failed to sign event: %w", err)
	}

	// Set the event signature
	event.Sig = hex.EncodeToString(sig.Serialize())
	
	return nil
}

// verifyEventSignature verifies the signature of the event
func verifyEventSignature(event *client.Event) (bool, error) {
	// Decode the signature from hex to bytes
	sigBytes, err := hex.DecodeString(event.Sig)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	// Parse the signature
	signature, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse signature: %w", err)
	}

	// Decode the event ID from hex to bytes
	eventIDBytes, err := hex.DecodeString(event.ID)
	if err != nil {
		return false, fmt.Errorf("failed to decode event ID: %w", err)
	}

	// Decode the public key from hex to bytes
	pubKeyBytes, err := hex.DecodeString(event.PubKey)
	if err != nil {
		return false, fmt.Errorf("failed to decode public key: %w", err)
	}

	// Prepend the compression prefix byte (0x02 or 0x03)
	// For secp256k1 public keys, we can use 0x02 as a prefix
	fullPubKeyBytes := append([]byte{0x02}, pubKeyBytes...)

	// Parse the public key
	pubKey, err := btcec.ParsePubKey(fullPubKeyBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse public key: %w", err)
	}

	// Verify the signature
	return signature.Verify(eventIDBytes, pubKey), nil
}
