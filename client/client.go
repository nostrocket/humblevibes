package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
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

// PublishExistingEvent publishes an existing event to the relay
func (c *NostrClient) PublishExistingEvent(event *Event) error {
	// Create the EVENT message - make sure it's properly formatted for the Nostr protocol
	message := []interface{}{"EVENT", event}

	// Debug format of the event
	eventJSON, _ := json.Marshal(event)
	fmt.Printf("🔍 DEBUG: Sending event to relay: %s\n", string(eventJSON))

	// Send the message to the relay
	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send event to relay: %w", err)
	}

	// Set read timeout to ensure we don't hang indefinitely
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{}) // Reset deadline

	// Wait for and process responses from the relay
	// We need to keep checking for messages until we get an OK or an error for our event
	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return fmt.Errorf("connection closed by relay: %w", err)
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return fmt.Errorf("timeout waiting for relay response: %w", err)
			}
			return fmt.Errorf("error reading relay response: %w", err)
		}

		fmt.Printf("🔍 DEBUG: Received relay response: %s\n", string(messageBytes))

		// Parse the response
		var response []json.RawMessage
		if err := json.Unmarshal(messageBytes, &response); err != nil {
			// This might not be a response to our event, try to continue
			fmt.Printf("🔍 DEBUG: Error parsing response: %v\n", err)
			continue
		}

		// Check if message is properly formatted
		if len(response) < 2 {
			fmt.Printf("🔍 DEBUG: Skipping response with too few elements\n")
			continue // Not our response, continue waiting
		}

		// Get the message type (NOTICE, OK, EVENT, etc.)
		var messageType string
		if err := json.Unmarshal(response[0], &messageType); err != nil {
			fmt.Printf("🔍 DEBUG: Error parsing message type: %v\n", err)
			continue // Not a valid message, continue waiting
		}

		// Debug the message type
		fmt.Printf("🔍 DEBUG: Message type: %s\n", messageType)

		// If it's a NOTICE, just log it
		if messageType == "NOTICE" {
			var notice string
			if len(response) >= 2 {
				json.Unmarshal(response[1], &notice)
				fmt.Printf("🔍 NOTICE from relay: %s\n", notice)
			}
			continue // Keep waiting for OK
		}

		// If it's OK, check if it's for our event
		if messageType == "OK" {
			// Check if it has the event ID we sent
			if len(response) >= 2 {
				var respEventID string
				if err := json.Unmarshal(response[1], &respEventID); err == nil {
					if respEventID == event.ID {
						fmt.Printf("✅ Event %s was accepted by relay\n", event.ID)
						return nil // Success!
					}
				}
			}
			// Not our event, continue waiting
			continue
		}

		// If we get an error related to our event
		if messageType == "OK" && len(response) >= 3 {
			var respEventID string
			var success bool
			var reason string
			
			if err := json.Unmarshal(response[1], &respEventID); err == nil {
				if respEventID == event.ID {
					json.Unmarshal(response[2], &success)
					if !success && len(response) >= 4 {
						json.Unmarshal(response[3], &reason)
						return fmt.Errorf("relay rejected event: %s", reason)
					}
				}
			}
		}

		// If we've been waiting too long, abort (this is a safety measure)
		// Continue looping to wait for more messages
	}
}

// publishEvent sends an event to the relay
func (c *NostrClient) publishEvent(event *Event) error {
	// Create the EVENT message
	message := []interface{}{"EVENT", event}

	// Send the message to the relay
	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send event to relay: %w", err)
	}

	// Set read timeout to ensure we don't hang indefinitely
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{}) // Reset deadline

	// Wait for the response
	_, messageBytes, err := c.conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			return fmt.Errorf("connection closed by relay: %w", err)
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("timeout waiting for relay response: %w", err)
		}
		return fmt.Errorf("error reading relay response: %w", err)
	}

	// Parse the response
	var response []json.RawMessage
	if err := json.Unmarshal(messageBytes, &response); err != nil {
		return fmt.Errorf("invalid JSON response from relay: %w", err)
	}

	// Check if the response is an OK message
	if len(response) < 2 {
		return errors.New("invalid response format from relay (too few elements)")
	}

	var messageType string
	if err := json.Unmarshal(response[0], &messageType); err != nil {
		return fmt.Errorf("failed to parse message type: %w", err)
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
	// Create a JSON array with the required fields in the specific order
	// [0, <pubkey>, <created_at>, <kind>, <tags>, <content>]
	eventArray := []interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	}

	// Serialize to JSON
	serialized, err := json.Marshal(eventArray)
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

// EventHandler is a function that handles received events
type EventHandler func(*Event)

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

// SubscribeToEventsWithHandler subscribes to events matching the given filters and calls the handler for each event
func (c *NostrClient) SubscribeToEventsWithHandler(subscriptionID string, filters map[string]interface{}, handler EventHandler) error {
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

				// Only process events for this subscription
				if subID != subscriptionID {
					continue
				}

				var event Event
				if err := json.Unmarshal(response[2], &event); err != nil {
					continue
				}

				// Call the handler with the event
				handler(&event)
			} else if messageType == "EOSE" {
				// End of stored events
				var subID string
				if err := json.Unmarshal(response[1], &subID); err != nil {
					continue
				}
				
				if subID == subscriptionID {
					log.Printf("End of stored events for subscription %s", subscriptionID)
				}
			}
		}
	}()

	return nil
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length]
}
