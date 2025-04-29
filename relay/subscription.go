package relay

import (
	"encoding/json"
	"fmt"
	"time"
)

// handleSubscription processes a REQ message (subscription request)
func (c *Client) handleSubscription(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid REQ message", "")
		return
	}

	// Extract subscription ID
	var subID string
	if err := json.Unmarshal(msg[1], &subID); err != nil {
		c.sendError("Invalid subscription ID", "")
		return
	}

	// Parse filters
	filters := make([]Filter, 0, len(msg)-2)
	for i := 2; i < len(msg); i++ {
		var filter Filter
		if err := json.Unmarshal(msg[i], &filter); err != nil {
			c.sendError(fmt.Sprintf("Invalid filter: %v", err), subID)
			return
		}
		filters = append(filters, filter)
	}

	// Create or update subscription
	c.subscriptions[subID] = &Subscription{
		ID:      subID,
		Filters: filters,
	}

	// Query database for matching events
	events, err := c.relay.queryEvents(filters)
	if err != nil {
		c.sendError(fmt.Sprintf("Failed to query events: %v", err), subID)
		return
	}

	// Send matching events to the client
	for _, event := range events {
		c.sendResponse([]interface{}{"EVENT", subID, event})
	}

	// Send EOSE (End of Stored Events) message
	c.sendResponse([]interface{}{"EOSE", subID})
}

// handleCloseSubscription processes a CLOSE message
func (c *Client) handleCloseSubscription(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid CLOSE message", "")
		return
	}

	// Extract subscription ID
	var subID string
	if err := json.Unmarshal(msg[1], &subID); err != nil {
		c.sendError("Invalid subscription ID", "")
		return
	}

	// Remove subscription
	delete(c.subscriptions, subID)
}

// queryEvents queries the database for events matching the given filters
func (r *Relay) queryEvents(filters []Filter) ([]*Event, error) {
	if len(filters) == 0 {
		return nil, nil
	}

	// For simplicity, we'll implement a basic query that doesn't handle all filter types
	// In a production relay, you would build more complex SQL queries based on the filters
	// and use SQL parameters to prevent SQL injection

	// Get all events from the database
	rows, err := r.db.Query("SELECT id, pubkey, created_at, kind, tags, content, sig FROM events ORDER BY created_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var tagsJSON string
		if err := rows.Scan(&event.ID, &event.PubKey, &event.CreatedAt, &event.Kind, &tagsJSON, &event.Content, &event.Sig); err != nil {
			return nil, err
		}

		// Parse tags JSON
		if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
			return nil, err
		}

		// Check if the event matches any of the filters
		for _, filter := range filters {
			if eventMatchesFilter(&event, filter) {
				events = append(events, &event)
				break
			}
		}
	}

	return events, nil
}

// parseTimeFilter parses a time filter from a JSON value
func parseTimeFilter(value json.RawMessage) (*time.Time, error) {
	var unixTime int64
	if err := json.Unmarshal(value, &unixTime); err != nil {
		return nil, err
	}
	t := time.Unix(unixTime, 0)
	return &t, nil
}
