package relay

import (
	"encoding/hex"
	"testing"

	"github.com/gareth/go-nostr-relay/lib/crypto"
	"github.com/gareth/go-nostr-relay/lib/utils"
)

var testLogger = utils.NewLogger("relay.test")

func TestComputeEventID(t *testing.T) {
	testLogger.TestInfo("🧪 Test: ComputeEventID")
	// Create a test event
	event := &Event{
		PubKey:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{{"e", "123456789abcdef"}},
		Content:   "Hello, world!",
	}

	// Convert to crypto.Event
	cryptoEvent := &crypto.Event{
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}

	// Compute the ID
	id, err := crypto.ComputeEventID(cryptoEvent)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		testLogger.TestInfo("✅ Computed event ID successfully")
	}

	// Verify the ID is a valid 32-byte hex string
	if len(id) != 64 {
		t.Errorf("❌ Expected ID length of 64 characters, got %d", len(id))
	} else {
		testLogger.TestInfo("✅ Event ID has correct length (64)")
	}

	// Try to decode the ID as hex
	_, err = hex.DecodeString(id)
	if err != nil {
		t.Errorf("❌ ID is not a valid hex string: %v", err)
	} else {
		testLogger.TestInfo("✅ Event ID is a valid hex string")
	}

	// Set the ID on the event
	event.ID = id
	cryptoEvent.ID = id

	// Compute the ID again and verify it's the same
	id2, err := crypto.ComputeEventID(cryptoEvent)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID second time: %v", err)
	} else {
		testLogger.TestInfo("✅ Computed event ID again successfully")
	}

	if id != id2 {
		t.Errorf("❌ ID computation is not deterministic: %s != %s", id, id2)
	} else {
		testLogger.TestInfo("✅ Event ID computation is deterministic")
	}
}

func TestValidateEvent(t *testing.T) {
	testLogger.TestInfo("🧪 Test: ValidateEvent")
	// Create a private key for testing
	privateKey, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		testLogger.TestInfo("✅ Generated private key successfully")
	}

	// Get the public key
	pubKey := crypto.GetPublicKey(privateKey)

	// Create a test event
	event := &Event{
		PubKey:    pubKey,
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{{"e", "123456789abcdef"}},
		Content:   "Hello, world!",
	}

	// Convert to crypto.Event for ID computation
	cryptoEvent := &crypto.Event{
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}

	// Compute the ID
	id, err := crypto.ComputeEventID(cryptoEvent)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		testLogger.TestInfo("✅ Computed event ID successfully")
	}
	event.ID = id
	cryptoEvent.ID = id

	// Sign the event
	signature, err := crypto.SignEvent(cryptoEvent, privateKey)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		testLogger.TestInfo("✅ Signed event successfully")
	}
	event.Sig = signature
	cryptoEvent.Sig = signature

	// Validate the event
	err = validateEvent(event)
	if err != nil {
		t.Errorf("❌ Event validation failed: %v", err)
	} else {
		testLogger.TestInfo("✅ Event validation succeeded")
	}

	// Test validation with missing fields
	testCases := []struct {
		name        string
		modifyEvent func(*Event)
		expectError bool
	}{
		{
			name: "Valid event",
			modifyEvent: func(e *Event) {
				// No modifications
			},
			expectError: false,
		},
		{
			name: "Missing pubkey",
			modifyEvent: func(e *Event) {
				e.PubKey = ""
			},
			expectError: true,
		},
		{
			name: "Missing created_at",
			modifyEvent: func(e *Event) {
				e.CreatedAt = 0
			},
			expectError: true,
		},
		{
			name: "Missing signature",
			modifyEvent: func(e *Event) {
				e.Sig = ""
			},
			expectError: true,
		},
		{
			name: "Invalid ID",
			modifyEvent: func(e *Event) {
				e.ID = "invalid_id"
			},
			expectError: true,
		},
		{
			name: "Modified content",
			modifyEvent: func(e *Event) {
				e.Content = "Modified content"
				// ID and signature are now invalid
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testLogger.TestInfo("🧪 Test case: %s", tc.name)
			// Create a copy of the valid event
			testEvent := &Event{
				ID:        event.ID,
				PubKey:    event.PubKey,
				CreatedAt: event.CreatedAt,
				Kind:      event.Kind,
				Tags:      event.Tags,
				Content:   event.Content,
				Sig:       event.Sig,
			}

			// Apply the test case modification
			tc.modifyEvent(testEvent)

			// Validate the event
			err := validateEvent(testEvent)
			if tc.expectError && err == nil {
				t.Errorf("❌ Expected validation to fail, but it succeeded")
			} else if !tc.expectError && err != nil {
				t.Errorf("❌ Expected validation to succeed, but it failed: %v", err)
			} else {
				testLogger.TestInfo("✅ Validation result matches expectation")
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	testLogger.TestInfo("🧪 Test: VerifySignature")

	// Generate a private key
	privateKey, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		testLogger.TestInfo("✅ Generated private key successfully")
	}

	// Get the public key
	pubKey := crypto.GetPublicKey(privateKey)

	// Create a test event
	event := &Event{
		PubKey:    pubKey,
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{{"e", "123456789abcdef"}},
		Content:   "Hello, world!",
	}

	// Convert to crypto.Event
	cryptoEvent := &crypto.Event{
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}

	// Compute the event ID
	id, err := crypto.ComputeEventID(cryptoEvent)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		testLogger.TestInfo("✅ Computed event ID successfully")
	}
	event.ID = id
	cryptoEvent.ID = id

	// Sign the event
	signature, err := crypto.SignEvent(cryptoEvent, privateKey)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		testLogger.TestInfo("✅ Signed event successfully")
	}
	event.Sig = signature
	cryptoEvent.Sig = signature

	// Directly use the crypto package for verification
	err = crypto.VerifySignature(cryptoEvent)
	if err != nil {
		t.Errorf("❌ Signature verification failed: %v", err)
	} else {
		testLogger.TestInfo("✅ Signature verification succeeded")
	}

	// Test with invalid signature
	cryptoEvent.Sig = "invalid_signature"
	err = crypto.VerifySignature(cryptoEvent)
	if err == nil {
		t.Errorf("❌ Expected signature verification to fail with invalid signature")
	} else {
		testLogger.TestInfo("✅ Signature verification failed with invalid signature as expected: %v", err)
	}
}
