package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// Event represents the structure of a Nostr event for cryptographic operations
type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// ComputeEventID computes the ID of an event according to the Nostr protocol
func ComputeEventID(event *Event) (string, error) {
	// Serialize outputs a byte array that can be hashed to produce the event ID
	serialized := serializeEvent(event)
	
	// Compute SHA256 hash
	hash := sha256.Sum256(serialized)
	return hex.EncodeToString(hash[:]), nil
}

// serializeEvent outputs a byte array that can be hashed to produce the event ID
func serializeEvent(evt *Event) []byte {
	// Create a JSON array with the required fields in the specific order
	// [0, <pubkey>, <created_at>, <kind>, <tags>, <content>]
	eventArray := []interface{}{
		0,
		evt.PubKey,
		evt.CreatedAt,
		evt.Kind,
		evt.Tags,
		evt.Content,
	}

	// Serialize to JSON
	serialized, err := json.Marshal(eventArray)
	if err != nil {
		// This should never happen for valid events
		return []byte{}
	}

	return serialized
}

// VerifySignature verifies the signature of an event
func VerifySignature(event *Event) error {
	// Decode the public key
	pubKeyBytes, err := hex.DecodeString(event.PubKey)
	if err != nil {
		return fmt.Errorf("invalid public key format: %v", err)
	}

	// Decode the signature
	sigBytes, err := hex.DecodeString(event.Sig)
	if err != nil {
		return fmt.Errorf("invalid signature format: %v", err)
	}

	// Decode the event ID (which is what was signed)
	idBytes, err := hex.DecodeString(event.ID)
	if err != nil {
		return fmt.Errorf("invalid ID format: %v", err)
	}

	// Parse the public key
	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	// Parse the signature
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %v", err)
	}

	// Verify the signature
	if !sig.Verify(idBytes, pubKey) {
		return errors.New("signature verification failed")
	}

	return nil
}

// SignEvent signs an event with the given private key
func SignEvent(event *Event, privateKey *btcec.PrivateKey) (string, error) {
	// Compute the event ID bytes
	idBytes, err := hex.DecodeString(event.ID)
	if err != nil {
		return "", err
	}

	// Sign the event ID
	sig, err := schnorr.Sign(privateKey, idBytes)
	if err != nil {
		return "", err
	}

	// Return the hex-encoded signature
	return hex.EncodeToString(sig.Serialize()), nil
}

// GeneratePrivateKey generates a new random private key
func GeneratePrivateKey() (*btcec.PrivateKey, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// GetPublicKey returns the hex-encoded public key for a private key
func GetPublicKey(privateKey *btcec.PrivateKey) string {
	return hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])
}

// ParsePubKey parses a public key using btcec library
func ParsePubKey(data []byte) (*btcec.PublicKey, error) {
	return btcec.ParsePubKey(data)
}

// ParseSignature parses a signature using schnorr library
func ParseSignature(data []byte) (*schnorr.Signature, error) {
	return schnorr.ParseSignature(data)
}

// Sha256 computes the SHA256 hash of the given data
func Sha256(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// Additional utility functions for encoding/decoding

// DecodePubKey decodes a hex-encoded public key
func DecodePubKey(pubkey string) ([]byte, error) {
	return hex.DecodeString(pubkey)
}

// DecodeSignature decodes a hex-encoded signature
func DecodeSignature(signature string) ([]byte, error) {
	return hex.DecodeString(signature)
}

// DecodeID decodes a hex-encoded event ID
func DecodeID(id string) ([]byte, error) {
	return hex.DecodeString(id)
}

// EncodeID encodes a byte slice as a hex string
func EncodeID(data []byte) string {
	return hex.EncodeToString(data)
}
