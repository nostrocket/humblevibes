package client

import (
	"encoding/hex"
	"testing"

	"github.com/gareth/go-nostr-relay/lib/utils"
)

var testLogger = utils.NewLogger("client.test")

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

	// Compute the ID
	id, err := computeEventID(event)
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

	// Compute the ID again and verify it's the same
	id2, err := computeEventID(event)
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

func TestSignEvent(t *testing.T) {
	testLogger.TestInfo("🧪 Test: SignEvent")
	// Create a private key for testing
	privateKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		testLogger.TestInfo("✅ Generated private key successfully")
	}

	// Create a test event
	event := &Event{
		PubKey:    getPublicKey(privateKey),
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
		testLogger.TestInfo("✅ Computed event ID successfully")
	}
	event.ID = id

	// Sign the event
	signature, err := signEvent(event, privateKey)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		testLogger.TestInfo("✅ Signed event successfully")
	}

	// Verify the signature is a valid hex string
	if len(signature) != 128 {
		t.Errorf("❌ Signature has invalid length: %d", len(signature))
	}

	// Try to decode the signature as hex
	_, err = hex.DecodeString(signature)
	if err != nil {
		t.Errorf("❌ Signature is not a valid hex string: %v", err)
	} else {
		testLogger.TestInfo("✅ Signature is a valid hex string")
	}
}

func TestGeneratePrivateKey(t *testing.T) {
	testLogger.TestInfo("🧪 Test: GeneratePrivateKey")
	// Generate a private key
	privateKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		testLogger.TestInfo("✅ Generated private key successfully")
	}

	// Verify the private key is not nil
	if privateKey == nil {
		t.Errorf("❌ Private key is nil")
	}

	// Verify the private key bytes have the correct length
	privateKeyBytes := privateKey.Serialize()
	if len(privateKeyBytes) != 32 {
		t.Errorf("❌ Private key has invalid length: %d", len(privateKeyBytes))
	} else {
		testLogger.TestInfo("✅ Private key has correct length (32 bytes)")
	}
}

func TestGetPublicKey(t *testing.T) {
	testLogger.TestInfo("🧪 Test: GetPublicKey")
	// Generate a private key
	privateKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		testLogger.TestInfo("✅ Generated private key successfully")
	}

	// Get the public key
	publicKey := getPublicKey(privateKey)

	// Verify the public key is not empty
	if publicKey == "" {
		t.Errorf("❌ Public key is empty")
	}

	// Verify the public key has the correct length
	if len(publicKey) != 64 {
		t.Errorf("❌ Public key has invalid length: %d", len(publicKey))
	} else {
		testLogger.TestInfo("✅ Public key has correct length (64)")
	}
}
