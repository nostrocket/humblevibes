package client

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
)

func TestComputeEventID(t *testing.T) {
	fmt.Println("🧪 Test: ComputeEventID (client)")
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

func TestSignEvent(t *testing.T) {
	fmt.Println("🧪 Test: SignEvent")
	// Create a private key for testing
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		fmt.Println("✅ Generated private key successfully")
	}

	// Get the public key
	pubKey := getPublicKey(privateKey)

	// Create a test event
	event := &Event{
		PubKey:    pubKey,
		CreatedAt: 1617932400,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "Sign me!",
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
	sig, err := signEvent(event, privateKey)
	if err != nil {
		t.Fatalf("❌ Failed to sign event: %v", err)
	} else {
		fmt.Println("✅ Signed event successfully")
	}
	event.Sig = sig

	// Verify signature is a valid hex string
	_, err = hex.DecodeString(sig)
	if err != nil {
		t.Errorf("❌ Signature is not a valid hex string: %v", err)
	} else {
		fmt.Println("✅ Signature is a valid hex string")
	}
}

func TestGeneratePrivateKey(t *testing.T) {
	fmt.Println("🧪 Test: GeneratePrivateKey")
	priv, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		fmt.Println("✅ Generated private key successfully")
	}
	privBytes := priv.Serialize()
	if len(privBytes) != 32 {
		t.Errorf("❌ Expected private key length of 32 bytes, got %d", len(privBytes))
	} else {
		fmt.Println("✅ Private key has correct length (32 bytes)")
	}
}

func TestGetPublicKey(t *testing.T) {
	fmt.Println("🧪 Test: GetPublicKey")
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("❌ Failed to generate private key: %v", err)
	} else {
		fmt.Println("✅ Generated private key successfully")
	}
	pub := getPublicKey(privateKey)
	if len(pub) != 64 {
		t.Errorf("❌ Expected public key length of 64, got %d", len(pub))
	} else {
		fmt.Println("✅ Public key has correct length (64)")
	}
}
