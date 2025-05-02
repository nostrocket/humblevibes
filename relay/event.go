package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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

	// Validate the event
	if err := validateEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Invalid event: %v", err), "")
		return
	}

	// Store the event in the database
	if err := c.relay.storeEvent(&event); err != nil {
		c.sendError(fmt.Sprintf("Failed to store event: %v", err), "")
		return
	}

	// Broadcast the event to all clients with matching subscriptions
	c.relay.broadcastEvent(&event)

	// Send OK message back to the client
	c.sendResponse([]interface{}{"OK", event.ID, true, ""})
}

// validateEvent validates a Nostr event
func validateEvent(event *Event) error {
	// Check required fields
	if event.PubKey == "" {
		return errors.New("missing pubkey")
	}
	if event.CreatedAt == 0 {
		return errors.New("missing created_at")
	}
	if event.Sig == "" {
		return errors.New("missing sig")
	}

	// Validate event ID
	computedID, err := computeEventID(event)
	if err != nil {
		return fmt.Errorf("failed to compute event ID: %v", err)
	}
	if computedID != event.ID {
		return errors.New("invalid event ID")
	}

	// Verify the signature
	if err := verifySignature(event); err != nil {
		return fmt.Errorf("invalid signature: %v", err)
	}

	return nil
}

// verifySignature verifies the signature of an event
func verifySignature(event *Event) error {
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

	// Parse the public key - Nostr uses compressed secp256k1 public keys without prefix byte
	// We need to add the 0x02 prefix byte for parsing
	pubKey, err := btcec.ParsePubKey(append([]byte{0x02}, pubKeyBytes...))
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
