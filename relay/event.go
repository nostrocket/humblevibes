package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gareth/go-nostr-relay/lib/crypto"
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

	// Log event if verbose mode is enabled
	if c.relay.verbose {
		author := event.PubKey
		if len(author) > 8 {
			author = author[:8] + "..."
		}
		content := event.Content
		if len(content) > 40 {
			content = content[:37] + "..."
		}
		log.Printf("📩 Received event: ID=%s, Kind=%d, Author=%s, Content=%s", 
			event.ID[:8]+"...", event.Kind, author, content)
	}

	// Validate the event
	if err := validateEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Invalid event: %v", err), "")
		if c.relay.verbose {
			log.Printf("❌ Event validation failed: %v", err)
		}
		return
	}

	// Store the event in the database
	if err := c.relay.storeEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Failed to store event: %v", err), "")
		if c.relay.verbose {
			log.Printf("❌ Failed to store event: %v", err)
		}
		return
	}

	if c.relay.verbose {
		log.Printf("✅ Event stored successfully: %s", event.ID[:8]+"...")
	}

	// Broadcast the event to all clients with matching subscriptions
	c.relay.broadcastEvent(&event)

	// Send OK message back to the client
	c.sendResponse([]interface{}{"OK", event.ID, true, ""})
}

// validateEvent validates a Nostr event
func validateEvent(event *Event) error {
	// Extra debug logging
	fmt.Printf("🧪 Validating event ID: %s\n", event.ID)
	fmt.Printf("  👤 Author: %s\n", event.PubKey)
	fmt.Printf("  🕒 Created: %d\n", event.CreatedAt)
	fmt.Printf("  🏷️  Kind: %d\n", event.Kind)
	
	// Check required fields
	if event.PubKey == "" {
		fmt.Println("❌ Missing pubkey")
		return errors.New("missing pubkey")
	}
	
	if event.CreatedAt == 0 {
		fmt.Println("❌ Missing created_at")
		return errors.New("missing created_at")
	}
	
	if event.Sig == "" {
		fmt.Println("❌ Missing sig")
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
		fmt.Printf("❌ Failed to compute event ID: %v\n", err)
		return fmt.Errorf("failed to compute event ID: %v", err)
	}
	
	// Verify the event ID
	if computedID != event.ID {
		fmt.Printf("❌ ID mismatch: computed=%s vs. provided=%s\n", computedID, event.ID)
		return fmt.Errorf("event ID mismatch")
	}
	fmt.Println("✅ Event ID valid")
	
	// Verify the signature
	cryptoEvent.ID = event.ID
	cryptoEvent.Sig = event.Sig
	if err := crypto.VerifySignature(cryptoEvent); err != nil {
		fmt.Printf("❌ Signature verification failed: %v\n", err)
		return fmt.Errorf("signature verification failed: %v", err)
	}
	fmt.Println("✅ Signature valid")
	
	fmt.Println("✅ Event validated successfully")
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
