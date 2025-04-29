package client

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
)

func TestComputeEventID(t *testing.T) {
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
		t.Fatalf("Failed to compute event ID: %v", err)
	}

	// Verify the ID is a valid 32-byte hex string
	if len(id) != 64 {
		t.Errorf("Expected ID length of 64 characters, got %d", len(id))
	}

	// Try to decode the ID as hex
	_, err = hex.DecodeString(id)
	if err != nil {
		t.Errorf("ID is not a valid hex string: %v", err)
	}

	// Set the ID on the event
	event.ID = id

	// Compute the ID again and verify it's the same
	id2, err := computeEventID(event)
	if err != nil {
		t.Fatalf("Failed to compute event ID second time: %v", err)
	}

	if id != id2 {
		t.Errorf("ID computation is not deterministic: %s != %s", id, id2)
	}
}

func TestSignEvent(t *testing.T) {
	// Create a private key for testing
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Get the public key
	pubKey := getPublicKey(privateKey)

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
		t.Fatalf("Failed to compute event ID: %v", err)
	}
	event.ID = id

	// Sign the event
	sig, err := signEvent(event, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign event: %v", err)
	}

	// Verify the signature is a valid 64-byte hex string
	if len(sig) != 128 {
		t.Errorf("Expected signature length of 128 characters, got %d", len(sig))
	}

	// Try to decode the signature as hex
	_, err = hex.DecodeString(sig)
	if err != nil {
		t.Errorf("Signature is not a valid hex string: %v", err)
	}
}

func TestGeneratePrivateKey(t *testing.T) {
	// Generate a private key
	privateKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Verify the private key is not nil
	if privateKey == nil {
		t.Errorf("Generated private key is nil")
	}

	// Generate another private key and verify it's different
	privateKey2, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate second private key: %v", err)
	}

	// Compare the private keys
	if hex.EncodeToString(privateKey.Serialize()) == hex.EncodeToString(privateKey2.Serialize()) {
		t.Errorf("Generated private keys are identical")
	}
}

func TestGetPublicKey(t *testing.T) {
	// Create a private key for testing
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Get the public key
	pubKey := getPublicKey(privateKey)

	// Verify the public key is a valid 32-byte hex string
	if len(pubKey) != 64 {
		t.Errorf("Expected public key length of 64 characters, got %d", len(pubKey))
	}

	// Try to decode the public key as hex
	_, err = hex.DecodeString(pubKey)
	if err != nil {
		t.Errorf("Public key is not a valid hex string: %v", err)
	}

	// Verify the public key matches the expected format
	expectedPubKey := hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])
	if pubKey != expectedPubKey {
		t.Errorf("Public key does not match expected format: %s != %s", pubKey, expectedPubKey)
	}
}
