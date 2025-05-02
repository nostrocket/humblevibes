package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gareth/go-nostr-relay/lib/crypto"
	"github.com/gareth/go-nostr-relay/lib/utils"
)

var (
	// Logger
	relayLogger = utils.NewLogger("relay")
	
	// Cache of mismatched event fingerprints to avoid duplicate logging
	mismatchedEvents     = make(map[string]bool)
	mismatchedEventsMutex sync.Mutex
)

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

// handleEvent processes an EVENT message
func (c *Client) handleEvent(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid EVENT message", "")
		return
	}

	// Parse the event
	var event Event
	if err := json.Unmarshal(msg[1], &event); err != nil {
		c.sendError("Invalid event format", "")
		return
	}

	// Get a shortened event ID and author for logging
	shortID := event.ID
	if len(shortID) > 8 {
		shortID = shortID[:8] + "..."
	}
	
	author := event.PubKey
	if len(author) > 8 {
		author = author[:8] + "..."
	}
	
	content := event.Content
	if len(content) > 40 {
		content = content[:37] + "..."
	}

	// First check if the event already exists in the database
	exists, err := c.relay.eventExists(event.ID)
	if err != nil {
		c.sendError(fmt.Sprintf("Failed to check event existence: %v", err), "")
		relayLogger.Error("Database error checking event existence: %v", err)
		return
	}

	if exists {
		// Event already exists, skip validation logs and just accept it
		relayLogger.Debug("⏩ Event already exists (ID: %s)", shortID)
		// Send OK message back to the client
		c.sendResponse([]interface{}{"OK", event.ID, true, "duplicate: already have this event"})
		return
	}

	// Log receipt of new event
	relayLogger.Info("📥 New event received: ID=%s, Kind=%d, Author=%s", 
		shortID, event.Kind, author)

	// Validate the event (only for new events)
	if err := validateEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Invalid event: %v", err), "")
		relayLogger.Error("❌ Event validation failed: %v", err)
		return
	}

	// Store the event in the database
	if err := c.relay.storeEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Failed to store event: %v", err), "")
		relayLogger.Error("❌ Failed to store event: %v", err)
		return
	}

	relayLogger.Info("✅ Event stored and broadcasted: %s (Kind: %d)", shortID, event.Kind)

	// Broadcast the event to all clients with matching subscriptions
	c.relay.broadcastEvent(&event)

	// Send OK message back to the client
	c.sendResponse([]interface{}{"OK", event.ID, true, ""})
}

// validateEvent validates a Nostr event
func validateEvent(event *Event) error {
	// Extra debug logging
	relayLogger.Debug("🧪 Validating event ID: %s", event.ID)
	relayLogger.Debug("  👤 Author: %s", event.PubKey)
	relayLogger.Debug("  🕒 Created: %d", event.CreatedAt)
	relayLogger.Debug("  🏷️  Kind: %d", event.Kind)
	
	// Check required fields
	if event.PubKey == "" {
		relayLogger.Error("❌ Missing pubkey")
		return errors.New("missing pubkey")
	}
	
	if event.CreatedAt == 0 {
		relayLogger.Error("❌ Missing created_at")
		return errors.New("missing created_at")
	}
	
	if event.Sig == "" {
		relayLogger.Error("❌ Missing sig")
		return errors.New("missing sig")
	}
	
	// Compute the event ID
	cryptoEvent := &crypto.Event{
		PubKey:    event.PubKey,
		CreatedAt: event.CreatedAt,
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}
	
	computedID, err := crypto.ComputeEventID(cryptoEvent)
	if err != nil {
		relayLogger.Error("❌ Failed to compute event ID: %v", err)
		return fmt.Errorf("failed to compute event ID: %v", err)
	}
	
	// Verify the event ID
	if computedID != event.ID {
		// Create a unique fingerprint for this event's content
		// This identifies the event regardless of its ID
		fingerprint := fmt.Sprintf("%s:%d:%d:%s", 
			event.PubKey, event.CreatedAt, event.Kind, event.Content)
		
		// Check if we've seen this mismatched event before
		mismatchedEventsMutex.Lock()
		seenBefore := mismatchedEvents[fingerprint]
		if !seenBefore {
			// Mark this event fingerprint as seen
			mismatchedEvents[fingerprint] = true
		}
		mismatchedEventsMutex.Unlock()
		
		// Always log the basic error message
		relayLogger.Error("❌ ID mismatch: computed=%s vs. provided=%s", computedID, event.ID)
		
		// Only log detailed information the first time we see this event content
		if !seenBefore {
			// Print the entire event for debugging
			eventJSON, err := json.MarshalIndent(event, "", "  ")
			if err != nil {
				relayLogger.Error("Failed to marshal event for logging: %v", err)
			} else {
				relayLogger.Error("Invalid event details:\n%s", string(eventJSON))
			}
			
			// Print the input that went into the ID computation
			computeInput := []interface{}{
				0,
				event.PubKey,
				event.CreatedAt,
				event.Kind,
				event.Tags,
				event.Content,
			}
			inputJSON, err := json.MarshalIndent(computeInput, "", "  ")
			if err != nil {
				relayLogger.Error("Failed to marshal computation input for logging: %v", err)
			} else {
				relayLogger.Error("ID computation input:\n%s", string(inputJSON))
			}
		} else {
			relayLogger.Debug("⏩ Skipping detailed logging for previously seen mismatched event (fingerprint: %s)", 
				fingerprint[:32]+"...")
		}
		
		return fmt.Errorf("event ID mismatch")
	}
	relayLogger.Debug("✅ Event ID valid")
	
	// Verify the signature
	cryptoEvent.ID = event.ID
	cryptoEvent.Sig = event.Sig
	if err := crypto.VerifySignature(cryptoEvent); err != nil {
		relayLogger.Error("❌ Signature verification failed: %v", err)
		return fmt.Errorf("signature verification failed: %v", err)
	}
	relayLogger.Debug("✅ Signature valid")
	
	relayLogger.Debug("✅ Event validated successfully")
	return nil
}

// storeEvent stores an event in the database
func (r *Relay) storeEvent(event *Event) error {
	// Convert tags to JSON string
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		return err
	}

	// Insert event into the database
	_, err = r.db.Exec(
		"INSERT OR IGNORE INTO events (id, pubkey, created_at, kind, tags, content, sig) VALUES (?, ?, ?, ?, ?, ?, ?)",
		event.ID, event.PubKey, event.CreatedAt, event.Kind, string(tagsJSON), event.Content, event.Sig,
	)
	return err
}

// eventExists checks if an event with the given ID already exists in the database
func (r *Relay) eventExists(id string) (bool, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// broadcastEvent broadcasts an event to all clients with matching subscriptions
func (r *Relay) broadcastEvent(event *Event) {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()

	for client := range r.clients {
		for subID, sub := range client.subscriptions {
			if eventMatchesFilters(event, sub.Filters) {
				client.sendResponse([]interface{}{"EVENT", subID, event})
			}
		}
	}
}

// eventMatchesFilters checks if an event matches the given filters
func eventMatchesFilters(event *Event, filters []Filter) bool {
	if len(filters) == 0 {
		return false
	}

	// An event matches if it matches any of the filters
	for _, filter := range filters {
		if eventMatchesFilter(event, filter) {
			return true
		}
	}

	return false
}

// eventMatchesFilter checks if an event matches a single filter
func eventMatchesFilter(event *Event, filter Filter) bool {
	// Check IDs filter
	if len(filter.IDs) > 0 {
		found := false
		for _, id := range filter.IDs {
			if id == event.ID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Authors filter
	if len(filter.Authors) > 0 {
		found := false
		for _, author := range filter.Authors {
			if author == event.PubKey {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Kinds filter
	if len(filter.Kinds) > 0 {
		found := false
		for _, kind := range filter.Kinds {
			if kind == event.Kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Since filter
	if filter.Since != nil {
		eventTime := time.Unix(event.CreatedAt, 0)
		if eventTime.Before(*filter.Since) {
			return false
		}
	}

	// Check Until filter
	if filter.Until != nil {
		eventTime := time.Unix(event.CreatedAt, 0)
		if eventTime.After(*filter.Until) {
			return false
		}
	}

	// Check Tags filter
	// This is a simplified implementation
	for tagName, tagValues := range filter.Tags {
		if len(tagValues) == 0 {
			continue
		}

		// Remove the '#' prefix from the tag name
		tagName = tagName[1:]
		
		found := false
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == tagName {
				for _, value := range tagValues {
					if tag[1] == value {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		
		if !found {
			return false
		}
	}

	return true
}
