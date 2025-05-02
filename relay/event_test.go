package relay

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func TestComputeEventID(t *testing.T) {
	fmt.Println("🧪 Test: ComputeEventID")
	// Create a test event
	event := &Event{
		PubKey:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{{"e", "123456789abcdef"}},
		Content:   "Hello, world!",
	}

	// Compute the ID
	id, err := computeEventID(event)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		fmt.Println("✅ Computed event ID successfully")
	}

	// Verify the ID is a valid 32-byte hex string
	if len(id) != 64 {
		t.Errorf("❌ Expected ID length of 64 characters, got %d", len(id))
	} else {
		fmt.Println("✅ Event ID has correct length (64)")
	}

	// Try to decode the ID as hex
	_, err = hex.DecodeString(id)
	if err != nil {
		t.Errorf("❌ ID is not a valid hex string: %v", err)
	} else {
		fmt.Println("✅ Event ID is a valid hex string")
	}

	// Set the ID on the event
	event.ID = id

	// Compute the ID again and verify it's the same
	id2, err := computeEventID(event)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID second time: %v", err)
	} else {
		fmt.Println("✅ Computed event ID again successfully")
	}

	if id != id2 {
		t.Errorf("❌ ID computation is not deterministic: %s != %s", id, id2)
	} else {
		fmt.Println("✅ Event ID computation is deterministic")
	}
}

func TestValidateEvent(t *testing.T) {
	fmt.Println("🧪 Test: ValidateEvent")
	// Create a private key for testing
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		fmt.Println("✅ Generated private key successfully")
	}

	// Get the public key
	pubKey := hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])

	// Create a test event
	event := &Event{
		PubKey:    pubKey,
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{{"e", "123456789abcdef"}},
		Content:   "Hello, world!",
	}

	// Compute the ID
	id, err := computeEventID(event)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		fmt.Println("✅ Computed event ID successfully")
	}
	event.ID = id

	// Sign the event
	idBytes, err := hex.DecodeString(id)
	if err != nil {
		t.Fatalf("❌ Failed to decode ID: %v", err)
	} else {
		fmt.Println("✅ Decoded event ID successfully")
	}
	sig, err := schnorr.Sign(privateKey, idBytes)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		fmt.Println("✅ Signed event successfully")
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	// Validate the event
	err = validateEvent(event)
	if err != nil {
		t.Errorf("❌ Event validation failed: %v", err)
	} else {
		fmt.Println("✅ Event validation succeeded")
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
			fmt.Printf("🧪 Test case: %s\n", tc.name)
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
				fmt.Println("✅ Validation result matches expectation")
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	fmt.Println("🧪 Test: VerifySignature")
	// Create a private key for testing
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		fmt.Println("✅ Generated private key successfully")
	}

	// Get the public key
	pubKey := hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])

	// Create a test event
	event := &Event{
		PubKey:    pubKey,
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "Hello, world!",
	}

	// Compute the ID
	id, err := computeEventID(event)
	if err != nil {
		t.Fatalf("❌ Failed to compute event ID: %v", err)
	} else {
		fmt.Println("✅ Computed event ID successfully")
	}
	event.ID = id

	// Sign the event
	idBytes, err := hex.DecodeString(id)
	if err != nil {
		t.Fatalf("❌ Failed to decode ID: %v", err)
	} else {
		fmt.Println("✅ Decoded event ID successfully")
	}
	sig, err := schnorr.Sign(privateKey, idBytes)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		fmt.Println("✅ Signed event successfully")
	}
	event.Sig = hex.EncodeToString(sig.Serialize())

	// Verify the signature
	err = verifySignature(event)
	if err != nil {
		t.Errorf("❌ Signature verification failed: %v", err)
	} else {
		fmt.Println("✅ Signature verification succeeded")
	}

	// Test with invalid signature
	event.Sig = "0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	err = verifySignature(event)
	if err == nil {
		t.Errorf("❌ Expected signature verification to fail with invalid signature, but it succeeded")
	} else {
		fmt.Println("✅ Signature verification failed with invalid signature")
	}
}
