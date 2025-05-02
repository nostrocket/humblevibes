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
	"github.com/gareth/go-nostr-relay/lib/utils"
	"github.com/gorilla/websocket"
)

var clientLogger = utils.NewLogger("client")

// NostrClient represents a client for interacting with a Nostr relay
type NostrClient struct {
	conn       *websocket.Conn
	privateKey *btcec.PrivateKey
	publicKey  string
	relayURL   string
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
		relayURL:   relayURL,
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
		relayURL:   relayURL,
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
func (c *NostrClient) PublishExistingEvent(event *Event) (bool, string, error) {
	// Construct the JSON-RPC message for publishing an event
	message := []interface{}{"EVENT", event}
	
	// Marshal the message
	data, err := json.Marshal(message)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal message: %v", err)
	}
	
	clientLogger.Debug("🔍 DEBUG: Sending event to relay: %s", string(data))
	
	// Send the message
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return false, "", fmt.Errorf("failed to send message: %v", err)
	}
	
	// Wait for response (we expect an "OK" message or an "ERROR" message)
	responseType := ""
	success := false
	errorMessage := ""
	
	// Define a timeout for waiting for a response
	const responseTimeout = 5 * time.Second
	c.conn.SetReadDeadline(time.Now().Add(responseTimeout))
	
	// Read and process messages until we get OK or ERROR
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return false, "timeout waiting for relay response", nil
			}
			return false, "", fmt.Errorf("failed to read response: %v", err)
		}
		
		clientLogger.Debug("🔍 DEBUG: Received relay response: %s", string(msg))
		
		// Parse the response
		var response []json.RawMessage
		if err := json.Unmarshal(msg, &response); err != nil {
			clientLogger.Warn("Failed to parse relay response: %v", err)
			continue
		}
		
		// Check if it's too short to be a valid response
		if len(response) < 2 {
			clientLogger.Warn("Invalid response format (too short)")
			continue
		}
		
		// Extract the message type
		var msgType string
		if err := json.Unmarshal(response[0], &msgType); err != nil {
			clientLogger.Warn("Failed to parse message type: %v", err)
			continue
		}
		
		clientLogger.Debug("🔍 DEBUG: Message type: %s", msgType)
		
		// Process different message types
		switch msgType {
		case "OK":
			// OK message format: ["OK", <event_id>, <success>, <message>]
			if len(response) < 4 {
				clientLogger.Warn("Invalid OK response format")
				continue
			}
			
			// Extract the event ID
			var eventID string
			if err := json.Unmarshal(response[1], &eventID); err != nil {
				clientLogger.Warn("Failed to parse event ID: %v", err)
				continue
			}
			
			// Check if it matches our event
			if eventID != event.ID {
				clientLogger.Warn("Event ID mismatch: %s vs %s", eventID, event.ID)
				continue
			}
			
			// Extract the success status
			if err := json.Unmarshal(response[2], &success); err != nil {
				clientLogger.Warn("Failed to parse success status: %v", err)
				continue
			}
			
			// Extract the message if there is one
			if len(response) >= 4 {
				if err := json.Unmarshal(response[3], &errorMessage); err != nil {
					clientLogger.Warn("Failed to parse error message: %v", err)
				}
			}
			
			responseType = "OK"
			break
			
		case "EVENT":
			// This is an event from the relay, not a response to our publish
			clientLogger.Debug("Received EVENT message, continuing to wait for OK/ERROR")
			continue
			
		case "NOTICE":
			// This is a notice from the relay
			if len(response) >= 2 {
				var notice string
				if err := json.Unmarshal(response[1], &notice); err != nil {
					clientLogger.Warn("Failed to parse notice: %v", err)
				} else {
					clientLogger.Info("Received NOTICE: %s", notice)
				}
			}
			continue
			
		case "ERROR":
			// ERROR message format: ["ERROR", <error_message>]
			if len(response) >= 2 {
				if err := json.Unmarshal(response[1], &errorMessage); err != nil {
					clientLogger.Warn("Failed to parse error message: %v", err)
				}
			}
			success = false
			responseType = "ERROR"
			break
			
		default:
			// Unknown message type
			clientLogger.Warn("Unknown message type: %s", msgType)
			continue
		}
		
		// If we got a proper response, break out of the loop
		if responseType == "OK" || responseType == "ERROR" {
			break
		}
	}
	
	// Reset the read deadline
	c.conn.SetReadDeadline(time.Time{})
	
	if responseType == "OK" && success {
		clientLogger.Info("✅ Event %s was accepted by relay", event.ID)
		return true, "", nil
	} else {
		if errorMessage == "" {
			errorMessage = "relay returned non-success status"
		}
		clientLogger.Warn("❌ Event %s was rejected by relay: %s", event.ID, errorMessage)
		return false, errorMessage, nil
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

// Connect establishes a WebSocket connection to the Nostr relay
func (c *NostrClient) Connect() error {
	clientLogger.Info("🔌 Connecting to relay: %s", c.relayURL)
	
	// Parse the relay URL
	u, err := url.Parse(c.relayURL)
	if err != nil {
		return fmt.Errorf("invalid relay URL: %w", err)
	}
	
	// Ensure the WebSocket scheme is used
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// Already using WebSocket scheme
	default:
		return fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
	
	// Connect to the WebSocket endpoint
	clientLogger.Debug("Dialing WebSocket: %s", u.String())
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	
	c.conn = conn
	clientLogger.Info("✅ Connected to relay: %s", c.relayURL)
	
	return nil
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

// Subscribe subscribes to events from the relay based on the provided filters
func (c *NostrClient) Subscribe(filters []map[string]interface{}, eventHandler func(*Event)) (string, error) {
	// Generate a random subscription ID
	subscriptionID := generateSubscriptionID()
	clientLogger.Info("📩 Subscribing to events with ID: %s", subscriptionID)
	
	// Construct the REQ message
	message := append([]interface{}{"REQ", subscriptionID}, filtersToInterfaces(filters)...)
	
	// Marshal the message to JSON for debug logging
	jsonMsg, _ := json.Marshal(message)
	clientLogger.Debug("Sending subscription: %s", string(jsonMsg))
	
	// Send the REQ message
	if err := c.conn.WriteJSON(message); err != nil {
		return "", fmt.Errorf("failed to send subscription request: %w", err)
	}
	
	// Start a goroutine to handle incoming events for this subscription
	go func() {
		for {
			// Read the next message
			_, msgBytes, err := c.conn.ReadMessage()
			if err != nil {
				clientLogger.Error("Error reading from websocket: %v", err)
				return
			}
			
			// Attempt to unmarshal the message
			var msg []json.RawMessage
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				clientLogger.Warn("Failed to parse message: %v", err)
				continue
			}
			
			// Check if the message is well-formed
			if len(msg) < 2 {
				clientLogger.Warn("Received malformed message with too few elements")
				continue
			}
			
			// Get the message type
			var msgType string
			if err := json.Unmarshal(msg[0], &msgType); err != nil {
				clientLogger.Warn("Failed to parse message type: %v", err)
				continue
			}
			
			// Handle different message types
			switch msgType {
			case "EVENT":
				// Check if this is for our subscription
				var eventSubID string
				if err := json.Unmarshal(msg[1], &eventSubID); err != nil {
					clientLogger.Warn("Failed to parse subscription ID: %v", err)
					continue
				}
				
				if eventSubID != subscriptionID {
					// Not for our subscription
					continue
				}
				
				if len(msg) < 3 {
					clientLogger.Warn("Malformed EVENT message, missing event data")
					continue
				}
				
				// Parse the event
				var event Event
				if err := json.Unmarshal(msg[2], &event); err != nil {
					clientLogger.Warn("Failed to parse event: %v", err)
					continue
				}
				
				clientLogger.Debug("📦 Received event: %s", event.ID)
				
				// Call the event handler
				eventHandler(&event)
				
			case "EOSE":
				// End of stored events
				var eoseSubID string
				if err := json.Unmarshal(msg[1], &eoseSubID); err != nil {
					clientLogger.Warn("Failed to parse EOSE subscription ID: %v", err)
					continue
				}
				
				if eoseSubID == subscriptionID {
					clientLogger.Info("🏁 End of stored events for subscription: %s", subscriptionID)
				}
				
			case "NOTICE":
				if len(msg) >= 2 {
					var notice string
					if err := json.Unmarshal(msg[1], &notice); err != nil {
						clientLogger.Warn("Failed to parse notice: %v", err)
					} else {
						clientLogger.Info("📢 NOTICE from relay: %s", notice)
					}
				}
				
			default:
				// Unknown message type
				clientLogger.Debug("Received unknown message type: %s", msgType)
			}
		}
	}()
	
	return subscriptionID, nil
}

// Helper function to convert filter maps to interfaces for the REQ message
func filtersToInterfaces(filters []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(filters))
	for i, filter := range filters {
		result[i] = filter
	}
	return result
}

// Helper function to generate a random subscription ID
func generateSubscriptionID() string {
	// Generate 16 random bytes
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
