package relay

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

// Relay represents a Nostr relay server
type Relay struct {
	db        *sql.DB
	clients   map[*Client]bool
	clientsMu sync.Mutex
	upgrader  websocket.Upgrader
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
func NewRelay(dbPath string) (*Relay, error) {
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
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
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
	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := &Client{
		conn:         conn,
		relay:        r,
		subscriptions: make(map[string]*Subscription),
	}

	// Register client
	r.clientsMu.Lock()
	r.clients[client] = true
	r.clientsMu.Unlock()

	// Handle client messages
	go client.readPump()
}

// readPump handles incoming messages from a client
func (c *Client) readPump() {
	defer func() {
		c.relay.clientsMu.Lock()
		delete(c.relay.clients, c)
		c.relay.clientsMu.Unlock()
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Process the message
		c.handleMessage(message)
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
		c.handleCloseSubscription(msg)
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
		log.Printf("Failed to send response: %v", err)
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
