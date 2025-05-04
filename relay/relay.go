package relay

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	
	"github.com/gareth/go-nostr-relay/lib/utils"
)

// Use the logger from event.go
var _ = utils.NewLogger("relay") // Just to ensure utils is used

// Relay represents a Nostr relay server
type Relay struct {
	db        *sql.DB
	clients   map[*Client]bool
	clientsMu sync.Mutex
	upgrader  websocket.Upgrader
	verbose   bool
	pongWait       time.Duration
	pingPeriod     time.Duration
	writeWait      time.Duration
	timeout        time.Duration
}

// Option is a functional option for configuring a Relay
type Option func(*Relay)

// WithVerboseLogging enables verbose logging of events
func WithVerboseLogging(verbose bool) Option {
	return func(r *Relay) {
		r.verbose = verbose
	}
}

// WithConnectionTimeouts sets custom timeouts for WebSocket connections
func WithConnectionTimeouts(pongWait time.Duration) Option {
	return func(r *Relay) {
		r.pongWait = pongWait
		
		if pongWait > 0 {
			// pingPeriod must be less than pongWait to ensure pings are sent before the pong wait expires
			r.pingPeriod = (pongWait * 9) / 10
			r.writeWait = 10 * time.Second
		} else {
			// If pongWait is 0, we want to disable the timeout mechanism completely
			// Set pingPeriod to a very long duration (essentially disabling pings)
			r.pingPeriod = 24 * time.Hour // Just a very long time
			r.writeWait = 10 * time.Second
		}
	}
}

// WithTimeout sets the timeout for WebSocket connections
func WithTimeout(timeout time.Duration) Option {
	return func(r *Relay) {
		r.timeout = timeout
	}
}

// Client represents a connected WebSocket client
type Client struct {
	conn        *websocket.Conn
	relay       *Relay
	subscriptions map[string]*Subscription
	sendMu      sync.Mutex
}

// Subscription represents a client subscription
type Subscription struct {
	ID     string
	Filters []Filter
}

// Filter represents a subscription filter
type Filter struct {
	IDs        []string          `json:"ids,omitempty"`
	Authors    []string          `json:"authors,omitempty"`
	Kinds      []int             `json:"kinds,omitempty"`
	Tags       map[string][]string `json:"#e,omitempty"`
	Since      *time.Time        `json:"since,omitempty"`
	Until      *time.Time        `json:"until,omitempty"`
	Limit      int               `json:"limit,omitempty"`
}

// NewRelay creates a new Nostr relay with the given database path
func NewRelay(dbPath string, opts ...Option) (*Relay, error) {
	// Open SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Initialize database schema
	if err := initDB(db); err != nil {
		db.Close()
		return nil, err
	}

	relay := &Relay{
		db:      db,
		clients: make(map[*Client]bool),
		clientsMu: sync.Mutex{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
		// Default timeouts
		pongWait:   60 * time.Second, // 60 seconds inactivity timeout
		pingPeriod: 54 * time.Second, // 9/10 of pongWait
		writeWait:  10 * time.Second,
	}

	// Apply options
	for _, opt := range opts {
		opt(relay)
	}

	return relay, nil
}

// Close closes the relay and its database connection
func (r *Relay) Close() error {
	r.clientsMu.Lock()
	for client := range r.clients {
		client.conn.Close()
	}
	r.clientsMu.Unlock()
	return r.db.Close()
}

// HandleWebSocket handles WebSocket connections
func (r *Relay) HandleWebSocket(w http.ResponseWriter, req *http.Request) {
	clientIP := req.RemoteAddr
	utils.NewLogger("relay").Info("🔌 New connection request from %s", clientIP)
	
	// Upgrade HTTP connection to WebSocket
	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		utils.NewLogger("relay").Warn("❌ Failed to upgrade connection: %v", err)
		return
	}

	utils.NewLogger("relay").Info("✅ Connection established with %s", conn.RemoteAddr().String())

	// Configure WebSocket connection
	conn.SetReadLimit(512 * 1024) // 512KB max message size
	
	// Only set read deadline if pongWait > 0
	if r.pongWait > 0 {
		conn.SetReadDeadline(time.Now().Add(r.pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(r.pongWait))
			return nil
		})
		
		if r.verbose {
			utils.NewLogger("relay").Info("⏱️ Connection configured with %v timeout for %s", r.pongWait, conn.RemoteAddr().String())
		}
	} else if r.verbose {
		utils.NewLogger("relay").Info("⏱️ Connection configured with no timeout for %s", conn.RemoteAddr().String())
	}

	client := &Client{
		conn:         conn,
		relay:        r,
		subscriptions: make(map[string]*Subscription),
	}

	// Register client
	r.clientsMu.Lock()
	r.clients[client] = true
	clientCount := len(r.clients)
	r.clientsMu.Unlock()
	
	utils.NewLogger("relay").Info("👥 Client connected: %s (Total clients: %d)", conn.RemoteAddr().String(), clientCount)

	// Start ping/pong handler if pongWait > 0
	if r.pongWait > 0 {
		go client.writePump()
	}
	
	// Handle client messages
	go client.readPump()
}

func (r *Relay) handleWebSocket(c *websocket.Conn) {
	// Create a new client for this connection
	client := &Client{
		conn:          c,
		relay:         r,
		subscriptions: make(map[string]*Subscription),
	}

	// Set up the WebSocket connection
	// For zero timeout (infinite), use SetReadDeadline(time.Time{}) - empty time means no deadline
	if r.timeout == 0 {
		c.SetReadDeadline(time.Time{}) // No timeout - connection stays open indefinitely
		utils.NewLogger("relay").Info("WebSocket connection established with no timeout")
	} else {
		c.SetReadDeadline(time.Now().Add(r.timeout))
		utils.NewLogger("relay").Info("WebSocket connection established with timeout: %v", r.timeout)
	}

	// Enable pong handler to refresh read deadline
	c.SetPongHandler(func(string) error {
		if r.timeout > 0 {
			c.SetReadDeadline(time.Now().Add(r.timeout))
		}
		return nil
	})

	// Create a channel to signal when the connection is closed
	done := make(chan struct{})

	// Handle pings from the client to keep the connection alive
	go func() {
		pingTicker := time.NewTicker(50 * time.Second)
		defer pingTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-pingTicker.C:
				// Send ping to client
				if err := c.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					utils.NewLogger("relay").Warn("Failed to send ping to client: %v", err)
					return
				}
			}
		}
	}()

	// Handle incoming WebSocket messages
	for {
		// Read message from the client
		messageType, message, err := c.ReadMessage()
		
		if err != nil {
			// Check if the connection was closed
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				utils.NewLogger("relay").Info("WebSocket connection closed normally")
			} else if websocket.IsUnexpectedCloseError(err) {
				utils.NewLogger("relay").Warn("WebSocket connection closed unexpectedly: %v", err)
			} else {
				utils.NewLogger("relay").Error("Error reading WebSocket message: %v", err)
			}
			
			close(done) // Signal the ping goroutine to exit
			break
		}

		// If read timeout is set, reset the deadline after each message
		if r.timeout > 0 {
			c.SetReadDeadline(time.Now().Add(r.timeout))
		}

		// Handle the message based on its type
		switch messageType {
		case websocket.TextMessage:
			// Parse the message as JSON
			var msg []json.RawMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				client.sendError("Invalid message format", "")
				continue
			}

			// Check if the message is well-formed
			if len(msg) < 1 {
				client.sendError("Empty message", "")
				continue
			}

			// Get the message type
			var msgType string
			if err := json.Unmarshal(msg[0], &msgType); err != nil {
				client.sendError("Invalid message type", "")
				continue
			}

			// Handle different message types
			switch msgType {
			case "EVENT":
				client.handleEvent(msg)
			case "REQ":
				client.handleSubscription(msg)
			case "CLOSE":
				client.handleCloseRequest(msg)
			default:
				client.sendError("Unknown message type: "+msgType, "")
			}

		default:
			// Ignore non-text messages
			utils.NewLogger("relay").Warn("Received non-text message type: %d", messageType)
		}
	}
}

// readPump handles incoming messages from a client
func (c *Client) readPump() {
	defer func() {
		c.relay.clientsMu.Lock()
		delete(c.relay.clients, c)
		remainingClients := len(c.relay.clients)
		c.relay.clientsMu.Unlock()
		
		// Log the disconnection with client address and remaining count
		utils.NewLogger("relay").Info("🔌 Client disconnected: %s (Remaining clients: %d)", 
			c.conn.RemoteAddr().String(), remainingClients)
		
		c.conn.Close()
	}()

	// Set read deadline based on pongWait only if it's greater than 0
	if c.relay.pongWait > 0 {
		c.conn.SetReadDeadline(time.Now().Add(c.relay.pongWait))
		c.conn.SetPongHandler(func(string) error {
			c.conn.SetReadDeadline(time.Now().Add(c.relay.pongWait))
			return nil
		})
	}

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				utils.NewLogger("relay").Warn("🚫 WebSocket error: %v from client %s", 
					err, c.conn.RemoteAddr().String())
			} else {
				utils.NewLogger("relay").Debug("🔌 Normal connection closure from client %s", 
					c.conn.RemoteAddr().String())
			}
			break
		}

		// Process the message
		c.handleMessage(message)
	}
}

// writePump sends ping messages to the client to keep the connection alive
func (c *Client) writePump() {
	ticker := time.NewTicker(c.relay.pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-ticker.C:
			// Set write deadline
			c.conn.SetWriteDeadline(time.Now().Add(c.relay.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes an incoming message from a client
func (c *Client) handleMessage(message []byte) {
	// Nostr messages are JSON arrays
	var msg []json.RawMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		c.sendError("Invalid message format", "")
		return
	}

	if len(msg) < 2 {
		c.sendError("Invalid message: too few elements", "")
		return
	}

	// Extract message type
	var msgType string
	if err := json.Unmarshal(msg[0], &msgType); err != nil {
		c.sendError("Invalid message type", "")
		return
	}

	switch msgType {
	case "EVENT":
		c.handleEvent(msg)
	case "REQ":
		c.handleSubscription(msg)
	case "CLOSE":
		c.handleCloseRequest(msg)
	default:
		c.sendError("Unknown message type", "")
	}
}

// sendError sends an error message to the client
func (c *Client) sendError(message, subscriptionID string) {
	response := []interface{}{"ERROR", subscriptionID, message}
	c.sendResponse(response)
}

// sendResponse sends a response to the client
func (c *Client) sendResponse(response interface{}) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if err := c.conn.WriteJSON(response); err != nil {
		utils.NewLogger("relay").Warn("Failed to send response: %v", err)
	}
}

// initDB initializes the database schema
func initDB(db *sql.DB) error {
	// Create events table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			pubkey TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			kind INTEGER NOT NULL,
			tags TEXT NOT NULL,
			content TEXT NOT NULL,
			sig TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_pubkey ON events(pubkey);
		CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
		CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
	`)
	return err
}

// handleCloseRequest processes a CLOSE message
func (c *Client) handleCloseRequest(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid CLOSE message", "")
		return
	}

	// Extract subscription ID
	var subscriptionID string
	if err := json.Unmarshal(msg[1], &subscriptionID); err != nil {
		c.sendError("Invalid subscription ID", "")
		return
	}

	// Check if the subscription exists
	_, exists := c.subscriptions[subscriptionID]
	if !exists {
		c.sendError("Subscription not found", subscriptionID)
		return
	}

	// Close the subscription
	utils.NewLogger("relay").Info("Closing subscription: %s", subscriptionID)
	delete(c.subscriptions, subscriptionID)

	// Note: We explicitly do NOT close the WebSocket connection here
	// This allows the client to maintain other subscriptions or create new ones
}
