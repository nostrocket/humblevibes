package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
	"context"

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
	mu         sync.Mutex
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
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024 * 16,
		WriteBufferSize:  1024 * 16,
	}

	// Set up connection parameters
	header := http.Header{}
	header.Add("Origin", "nostr-client")

	conn, _, err := dialer.Dial(u.String(), header)
	if err != nil {
		return nil, err
	}

	// Improve connection stability by setting a large read limit and removing read deadline
	conn.SetReadLimit(10 * 1024 * 1024) // 10MB read limit

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
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024 * 16,
		WriteBufferSize:  1024 * 16,
	}

	// Set up connection parameters
	header := http.Header{}
	header.Add("Origin", "nostr-client")

	conn, _, err := dialer.Dial(u.String(), header)
	if err != nil {
		return nil, err
	}

	// Improve connection stability by setting a large read limit and removing read deadline
	conn.SetReadLimit(10 * 1024 * 1024) // 10MB read limit

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
	// Set up dialer with more robust timeout settings
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024 * 16,
		WriteBufferSize:  1024 * 16,
	}

	// Set up connection parameters
	header := http.Header{}
	header.Add("Origin", "nostr-client")

	// Connect to the relay
	conn, _, err := dialer.Dial(c.relayURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.relayURL, err)
	}

	// Improve connection stability by setting a large read limit and removing read deadline
	conn.SetReadLimit(10 * 1024 * 1024) // 10MB read limit
	
	c.conn = conn
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
			// Read the next message
			_, msgBytes, err := c.conn.ReadMessage()
			if err != nil {
				// Notify about connection error but don't terminate the subscription immediately
				// This allows the caller to handle reconnection
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					clientLogger.Warn("WebSocket closed: %v", err)
				} else {
					clientLogger.Error("Error reading from relay: %v", err)
				}
				return
			}

			// Attempt to unmarshal the message
			var msg []json.RawMessage
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				clientLogger.Warn("Error parsing message: %v", err)
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

				// Call the handler with the event
				// handler(&event)

			case "EOSE":
				// Check if message contains subscription ID to avoid panic
				if len(msg) < 2 {
					clientLogger.Warn("Received malformed EOSE message")
					continue
				}
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

	return nil
}

// SubscribeToEventsWithHandler subscribes to events matching the given filters and calls the handler for each event
func (c *NostrClient) SubscribeToEventsWithHandler(subscriptionID string, filters map[string]interface{}, handler func(*Event)) error {
	clientLogger := utils.NewLogger("client")
	
	// Create request as JSON
	reqMsg, err := json.Marshal([]interface{}{"REQ", subscriptionID, filters})
	if err != nil {
		return fmt.Errorf("failed to marshal subscription request: %w", err)
	}
	
	// Write request to server
	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, reqMsg)
	c.mu.Unlock()
	if err != nil {
		clientLogger.Error("Failed to send subscription request: %v", err)
		return fmt.Errorf("failed to send subscription request: %w", err)
	}
	clientLogger.Info("Sent subscription request: %s", string(reqMsg))
	
	// Setup a context with cancellation for coordinated shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Set up a heartbeat ping to keep the connection alive
	heartbeatTicker := time.NewTicker(20 * time.Second)
	defer heartbeatTicker.Stop()
	
	heartbeatID := 0
	
	// Start a goroutine for regular heartbeats
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				heartbeatID++
				pingID := fmt.Sprintf("heartbeat_%s_%d", subscriptionID, heartbeatID)
				if err := c.SendPing(pingID); err != nil {
					clientLogger.Warn("Failed to send heartbeat ping #%d: %v", heartbeatID, err)
				} else {
					clientLogger.Debug("Sent heartbeat ping #%d", heartbeatID)
				}
			}
		}
	}()
	
	// Create a read deadline ticker to periodically reset the deadline
	readDeadlineTicker := time.NewTicker(30 * time.Second)
	defer readDeadlineTicker.Stop()
	
	// Start a goroutine to manage read deadlines
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-readDeadlineTicker.C:
				// Set a read deadline to detect connection issues
				c.mu.Lock()
				err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
				c.mu.Unlock()
				if err != nil {
					clientLogger.Warn("Failed to set read deadline: %v", err)
				}
			}
		}
	}()
	
	// Flag to track if an EOSE burst has been sent
	eosePingsSent := false
	
	// Create a buffer to store incoming messages until they are processed
	for {
		// Read the next message
		c.mu.Lock()
		_, message, err := c.conn.ReadMessage()
		c.mu.Unlock()
		
		if err != nil {
			clientLogger.Error("Error reading from relay: %v", err)
			// If we're past EOSE and still getting errors, this is a true connection issue
			return fmt.Errorf("error reading from relay: %w", err)
		}
		
		// Parse the message
		var msg []json.RawMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			clientLogger.Warn("Failed to parse message: %v", err)
			continue
		}
		
		// Check if the message is empty
		if len(msg) == 0 {
			clientLogger.Warn("Received empty message")
			continue
		}
		
		// Get the message type
		var messageType string
		if err := json.Unmarshal(msg[0], &messageType); err != nil {
			clientLogger.Warn("Failed to parse message type: %v", err)
			continue
		}
		
		switch messageType {
		case "EVENT":
			// Check if the message contains enough elements
			if len(msg) < 3 {
				clientLogger.Warn("Received malformed EVENT message")
				continue
			}
			
			// Check if this event is for our subscription
			var eventSubID string
			if err := json.Unmarshal(msg[1], &eventSubID); err != nil {
				clientLogger.Warn("Failed to parse EVENT subscription ID: %v", err)
				continue
			}
			
			// Skip events not for our subscription
			if eventSubID != subscriptionID {
				continue
			}
			
			// Parse the event
			var event Event
			if err := json.Unmarshal(msg[2], &event); err != nil {
				clientLogger.Warn("Failed to parse event: %v", err)
				continue
			}
			
			// Handle the event
			handler(&event)
			
		case "EOSE":
			// Check if message contains subscription ID to avoid panic
			if len(msg) < 2 {
				clientLogger.Warn("Received malformed EOSE message")
				continue
			}
			// End of stored events
			var eoseSubID string
			if err := json.Unmarshal(msg[1], &eoseSubID); err != nil {
				clientLogger.Warn("Failed to parse EOSE subscription ID: %v", err)
				continue
			}
			if eoseSubID == subscriptionID {
				clientLogger.Info("🏁 End of stored events for subscription: %s", subscriptionID)
				
				// Only send post-EOSE pings once
				if !eosePingsSent {
					eosePingsSent = true
					
					// Send an immediate ping burst after EOSE to ensure the connection stays open
					go func() {
						// Send 3 pings with short delays between them
						for i := 0; i < 3; i++ {
							select {
							case <-ctx.Done():
								return
							case <-time.After(time.Duration(500*i) * time.Millisecond):
								pingID := fmt.Sprintf("eose_ping_%s_%d", subscriptionID, i)
								c.mu.Lock()
								pingErr := c.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("[\"PING\",\"%s\"]", pingID)))
								c.mu.Unlock()
								if pingErr != nil {
									clientLogger.Warn("Failed to send EOSE ping burst #%d: %v", i, pingErr)
								} else {
									clientLogger.Debug("Sent EOSE ping burst #%d", i)
								}
							}
						}
						
						// Increase the heartbeat frequency after EOSE to keep the connection more stable
						heartbeatTicker.Reset(10 * time.Second)
					}()
				}
			}
			
		case "NOTICE":
			if len(msg) >= 2 {
				var notice string
				if err := json.Unmarshal(msg[1], &notice); err == nil {
					clientLogger.Info("Received NOTICE from relay: %s", notice)
				}
			}
			
		case "OK":
			if len(msg) >= 3 {
				var okEventID string
				var okStatus bool
				var okMessage string
				
				if err := json.Unmarshal(msg[1], &okEventID); err == nil {
					if err := json.Unmarshal(msg[2], &okStatus); err == nil {
						if len(msg) >= 4 {
							_ = json.Unmarshal(msg[3], &okMessage)
						}
						
						if okStatus {
							clientLogger.Info("Event %s accepted by relay", okEventID)
						} else {
							clientLogger.Warn("Event %s rejected by relay: %s", okEventID, okMessage)
						}
					}
				}
			}
			
		case "PONG":
			if len(msg) >= 2 {
				var pongID string
				if err := json.Unmarshal(msg[1], &pongID); err == nil {
					clientLogger.Debug("Received PONG: %s", pongID)
				}
			}
			
		default:
			clientLogger.Warn("Received unknown message type: %s", messageType)
		}
		
		// Reset read deadline after successful message processing
		c.mu.Lock()
		c.conn.SetReadDeadline(time.Time{})
		c.mu.Unlock()
	}
}

// SubscribeToEventsWithHandlerAndEOSE subscribes to events based on the provided filters and invokes the handler function for each event
// It also provides a callback when an EOSE message is received, which is useful for validating connections
func (c *NostrClient) SubscribeToEventsWithHandlerAndEOSE(subscriptionID string, filters map[string]interface{}, handler func(*Event), eoseCallback func()) error {
	clientLogger := utils.NewLogger("client")
	
	// Create request as JSON
	reqMsg, err := json.Marshal([]interface{}{"REQ", subscriptionID, filters})
	if err != nil {
		return fmt.Errorf("failed to marshal subscription request: %w", err)
	}
	
	// Write request to server
	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, reqMsg)
	c.mu.Unlock()
	if err != nil {
		clientLogger.Error("Failed to send subscription request: %v", err)
		return fmt.Errorf("failed to send subscription request: %w", err)
	}
	clientLogger.Info("Sent subscription request: %s", string(reqMsg))
	
	// Setup a context with cancellation for coordinated shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Set up a heartbeat ping to keep the connection alive
	heartbeatTicker := time.NewTicker(20 * time.Second)
	defer heartbeatTicker.Stop()
	
	heartbeatID := 0
	
	// Start a goroutine for regular heartbeats
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				heartbeatID++
				pingID := fmt.Sprintf("heartbeat_%s_%d", subscriptionID, heartbeatID)
				if err := c.SendPing(pingID); err != nil {
					clientLogger.Warn("Failed to send heartbeat ping #%d: %v", heartbeatID, err)
				} else {
					clientLogger.Debug("Sent heartbeat ping #%d", heartbeatID)
				}
			}
		}
	}()
	
	// Create a read deadline ticker to periodically reset the deadline
	readDeadlineTicker := time.NewTicker(30 * time.Second)
	defer readDeadlineTicker.Stop()
	
	// Start a goroutine to manage read deadlines
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-readDeadlineTicker.C:
				// Set a read deadline to detect connection issues
				c.mu.Lock()
				err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
				c.mu.Unlock()
				if err != nil {
					clientLogger.Warn("Failed to set read deadline: %v", err)
				}
			}
		}
	}()
	
	// Flag to track if an EOSE burst has been sent
	eosePingsSent := false
	
	// Create a buffer to store incoming messages until they are processed
	for {
		// Read the next message
		c.mu.Lock()
		_, message, err := c.conn.ReadMessage()
		c.mu.Unlock()
		
		if err != nil {
			clientLogger.Error("Error reading from relay: %v", err)
			// If we're past EOSE and still getting errors, this is a true connection issue
			return fmt.Errorf("error reading from relay: %w", err)
		}
		
		// Parse the message
		var msg []json.RawMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			clientLogger.Warn("Failed to parse message: %v", err)
			continue
		}
		
		// Check if the message is empty
		if len(msg) == 0 {
			clientLogger.Warn("Received empty message")
			continue
		}
		
		// Get the message type
		var messageType string
		if err := json.Unmarshal(msg[0], &messageType); err != nil {
			clientLogger.Warn("Failed to parse message type: %v", err)
			continue
		}
		
		switch messageType {
		case "EVENT":
			// Check if the message contains enough elements
			if len(msg) < 3 {
				clientLogger.Warn("Received malformed EVENT message")
				continue
			}
			
			// Check if this event is for our subscription
			var eventSubID string
			if err := json.Unmarshal(msg[1], &eventSubID); err != nil {
				clientLogger.Warn("Failed to parse EVENT subscription ID: %v", err)
				continue
			}
			
			// Skip events not for our subscription
			if eventSubID != subscriptionID {
				continue
			}
			
			// Parse the event
			var event Event
			if err := json.Unmarshal(msg[2], &event); err != nil {
				clientLogger.Warn("Failed to parse event: %v", err)
				continue
			}
			
			// Handle the event
			handler(&event)
			
		case "EOSE":
			// Check if message contains subscription ID to avoid panic
			if len(msg) < 2 {
				clientLogger.Warn("Received malformed EOSE message")
				continue
			}
			// End of stored events
			var eoseSubID string
			if err := json.Unmarshal(msg[1], &eoseSubID); err != nil {
				clientLogger.Warn("Failed to parse EOSE subscription ID: %v", err)
				continue
			}
			if eoseSubID == subscriptionID {
				clientLogger.Info("🏁 End of stored events for subscription: %s", subscriptionID)
				
				// Call the EOSE callback if provided
				if eoseCallback != nil {
					eoseCallback()
				}
				
				// Only send post-EOSE pings once
				if !eosePingsSent {
					eosePingsSent = true
					
					// Send an immediate ping burst after EOSE to ensure the connection stays open
					go func() {
						// Send 3 pings with short delays between them
						for i := 0; i < 3; i++ {
							select {
							case <-ctx.Done():
								return
							case <-time.After(time.Duration(500*i) * time.Millisecond):
								pingID := fmt.Sprintf("eose_ping_%s_%d", subscriptionID, i)
								c.mu.Lock()
								pingErr := c.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("[\"PING\",\"%s\"]", pingID)))
								c.mu.Unlock()
								if pingErr != nil {
									clientLogger.Warn("Failed to send EOSE ping burst #%d: %v", i, pingErr)
								} else {
									clientLogger.Debug("Sent EOSE ping burst #%d", i)
								}
							}
						}
						
						// Increase the heartbeat frequency after EOSE to keep the connection more stable
						heartbeatTicker.Reset(10 * time.Second)
					}()
				}
			}
			
		case "NOTICE":
			if len(msg) >= 2 {
				var notice string
				if err := json.Unmarshal(msg[1], &notice); err == nil {
					clientLogger.Info("Received NOTICE from relay: %s", notice)
				}
			}
			
		case "OK":
			if len(msg) >= 3 {
				var okEventID string
				var okStatus bool
				var okMessage string
				
				if err := json.Unmarshal(msg[1], &okEventID); err == nil {
					if err := json.Unmarshal(msg[2], &okStatus); err == nil {
						if len(msg) >= 4 {
							_ = json.Unmarshal(msg[3], &okMessage)
						}
						
						if okStatus {
							clientLogger.Info("Event %s accepted by relay", okEventID)
						} else {
							clientLogger.Warn("Event %s rejected by relay: %s", okEventID, okMessage)
						}
					}
				}
			}
			
		case "PONG":
			if len(msg) >= 2 {
				var pongID string
				if err := json.Unmarshal(msg[1], &pongID); err == nil {
					clientLogger.Debug("Received PONG: %s", pongID)
				}
			}
			
		default:
			clientLogger.Warn("Received unknown message type: %s", messageType)
		}
		
		// Reset read deadline after successful message processing
		c.mu.Lock()
		c.conn.SetReadDeadline(time.Time{})
		c.mu.Unlock()
	}
}

// SendPing sends a lightweight ping message to keep the WebSocket connection alive
// It uses a "PING" message
func (c *NostrClient) SendPing(pingID string) error {
	// Send a PING message as per Nostr protocol
	message := []interface{}{"PING", pingID}
	clientLogger.Debug("Sending PING: %s", pingID)
	// Set write deadline for ping
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		clientLogger.Warn("Failed to set write deadline for PING: %v", err)
	}
	// Send the PING message
	if err := c.conn.WriteJSON(message); err != nil {
		clientLogger.Error("Failed to send PING: %v", err)
		return err
	}
	// Reset write deadline
	if err := c.conn.SetWriteDeadline(time.Time{}); err != nil {
		clientLogger.Warn("Failed to reset write deadline after PING: %v", err)
	}
	return nil
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
				// Notify about connection error but don't terminate the subscription immediately
				// This allows the caller to handle reconnection
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
				// Check if message contains subscription ID to avoid panic
				if len(msg) < 2 {
					clientLogger.Warn("Received malformed EOSE message")
					continue
				}
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

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length]
}
