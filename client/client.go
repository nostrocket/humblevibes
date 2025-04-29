package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gorilla/websocket"
)

// NostrClient represents a client for interacting with a Nostr relay
type NostrClient struct {
	conn       *websocket.Conn
	privateKey *btcec.PrivateKey
	publicKey  string
}

// Event represents a Nostr event
type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// EventJSON is used for computing the event ID
type EventJSON struct {
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
}

// NewNostrClient creates a new Nostr client
func NewNostrClient(relayURL string) (*NostrClient, error) {
	// Parse the relay URL
	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, err
	}

	// Ensure the URL uses the WebSocket scheme
	switch u.Scheme {
	case "ws", "wss":
		// Already using WebSocket scheme
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}

	// Connect to the relay
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Generate a new private key
	privateKey, err := generatePrivateKey()
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Get the public key
	publicKey := getPublicKey(privateKey)

	return &NostrClient{
		conn:       conn,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// NewNostrClientWithKey creates a new Nostr client with a specific private key
func NewNostrClientWithKey(relayURL string, hexPrivateKey string) (*NostrClient, error) {
	// Parse the relay URL
	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, err
	}

	// Ensure the URL uses the WebSocket scheme
	switch u.Scheme {
	case "ws", "wss":
		// Already using WebSocket scheme
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}

	// Connect to the relay
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Parse the private key
	privateKeyBytes, err := hex.DecodeString(hexPrivateKey)
	if err != nil {
		conn.Close()
		return nil, err
	}

	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)
	publicKey := getPublicKey(privateKey)

	return &NostrClient{
		conn:       conn,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// Close closes the connection to the relay
func (c *NostrClient) Close() error {
	return c.conn.Close()
}

// PublishTextNote publishes a text note (kind 1) to the relay
func (c *NostrClient) PublishTextNote(content string) (*Event, error) {
	// Create the event
	event := &Event{
		PubKey:    c.publicKey,
		CreatedAt: time.Now().Unix(),
		Kind:      1, // Text note
		Tags:      [][]string{},
		Content:   content,
	}

	// Compute the event ID
	id, err := computeEventID(event)
	if err != nil {
		return nil, err
	}
	event.ID = id

	// Sign the event
	sig, err := signEvent(event, c.privateKey)
	if err != nil {
		return nil, err
	}
	event.Sig = sig

	// Publish the event to the relay
	if err := c.publishEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

// PublishEvent publishes a custom event to the relay
func (c *NostrClient) PublishEvent(kind int, content string, tags [][]string) (*Event, error) {
	// Create the event
	event := &Event{
		PubKey:    c.publicKey,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}

	// Compute the event ID
	id, err := computeEventID(event)
	if err != nil {
		return nil, err
	}
	event.ID = id

	// Sign the event
	sig, err := signEvent(event, c.privateKey)
	if err != nil {
		return nil, err
	}
	event.Sig = sig

	// Publish the event to the relay
	if err := c.publishEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

// publishEvent sends an event to the relay
func (c *NostrClient) publishEvent(event *Event) error {
	// Create the EVENT message
	message := []interface{}{"EVENT", event}

	// Send the message to the relay
	if err := c.conn.WriteJSON(message); err != nil {
		return err
	}

	// Wait for the response
	_, messageBytes, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}

	// Parse the response
	var response []json.RawMessage
	if err := json.Unmarshal(messageBytes, &response); err != nil {
		return err
	}

	// Check if the response is an OK message
	if len(response) < 2 {
		return errors.New("invalid response from relay")
	}

	var messageType string
	if err := json.Unmarshal(response[0], &messageType); err != nil {
		return err
	}

	if messageType != "OK" {
		var errorMessage string
		if len(response) >= 3 {
			json.Unmarshal(response[2], &errorMessage)
		}
		return fmt.Errorf("relay rejected event: %s", errorMessage)
	}

	return nil
}

// GetPublicKey returns the client's public key
func (c *NostrClient) GetPublicKey() string {
	return c.publicKey
}

// generatePrivateKey generates a new random private key
func generatePrivateKey() (*btcec.PrivateKey, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	// Convert to a private key
	privateKey, _ := btcec.PrivKeyFromBytes(b)
	return privateKey, nil
}

// getPublicKey returns the hex-encoded public key for a private key
func getPublicKey(privateKey *btcec.PrivateKey) string {
	return hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])
}

// computeEventID computes the ID of an event according to the Nostr protocol
func computeEventID(event *Event) (string, error) {
	// Create the event serialization
	eventJSON := EventJSON{
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}

	// Serialize to JSON
	serialized, err := json.Marshal(eventJSON)
	if err != nil {
		return "", err
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(serialized)
	return hex.EncodeToString(hash[:]), nil
}

// signEvent signs an event with the given private key
func signEvent(event *Event, privateKey *btcec.PrivateKey) (string, error) {
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

// SubscribeToEvents subscribes to events matching the given filters
func (c *NostrClient) SubscribeToEvents(subscriptionID string, filters map[string]interface{}) error {
	// Create the REQ message
	message := []interface{}{"REQ", subscriptionID, filters}

	// Send the message to the relay
	if err := c.conn.WriteJSON(message); err != nil {
		return err
	}

	// Start a goroutine to handle incoming events
	go func() {
		for {
			_, messageBytes, err := c.conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading from relay: %v", err)
				return
			}

			// Parse the message
			var response []json.RawMessage
			if err := json.Unmarshal(messageBytes, &response); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}

			// Check if the message is an EVENT message
			if len(response) < 3 {
				continue
			}

			var messageType string
			if err := json.Unmarshal(response[0], &messageType); err != nil {
				continue
			}

			if messageType == "EVENT" {
				var subID string
				if err := json.Unmarshal(response[1], &subID); err != nil {
					continue
				}

				var event Event
				if err := json.Unmarshal(response[2], &event); err != nil {
					continue
				}

				// Process the event
				log.Printf("Received event: %+v", event)
			}
		}
	}()

	return nil
}
