package relay

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gareth/go-nostr-relay/lib/utils"
)

// HandleSubscription processes a subscription request
func (c *Client) handleSubscription(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid subscription: missing ID", "")
		return
	}

	// Extract subscription ID
	var subscriptionID string
	if err := json.Unmarshal(msg[1], &subscriptionID); err != nil {
		c.sendError("Invalid subscription ID", "")
		return
	}

	// Create a new subscription
	subscription := &Subscription{
		ID:      subscriptionID,
		Filters: []Filter{},
	}

	// Parse filters
	for i := 2; i < len(msg); i++ {
		var filter Filter
		if err := json.Unmarshal(msg[i], &filter); err != nil {
			c.sendError("Invalid filter format", subscriptionID)
			return
		}
		subscription.Filters = append(subscription.Filters, filter)
	}

	// Log the new subscription
	filtersJSON, _ := json.Marshal(subscription.Filters)
	utils.NewLogger("relay").Info(" New subscription: ID=%s, Filters=%s, Client=%s", 
		subscriptionID, string(filtersJSON), c.conn.RemoteAddr().String())

	// Store the subscription
	c.subscriptions[subscriptionID] = subscription

	// Query matching events
	events, err := c.relay.queryEvents(subscription.Filters)
	if err != nil {
		utils.NewLogger("relay").Error("Error querying events for subscription %s: %v", subscriptionID, err)
		c.sendError("Error querying events: "+err.Error(), subscriptionID)
		// Don't return here - send EOSE even if there's an error to maintain the connection
	}

	// Send matching events if we have any
	if events != nil {
		utils.NewLogger("relay").Info(" Sending %d matching events for subscription %s", len(events), subscriptionID)
		for _, event := range events {
			// Wrap event sending in a try-catch to prevent connection closure on error
			func() {
				defer func() {
					if r := recover(); r != nil {
						utils.NewLogger("relay").Error("Recovered from panic while sending event: %v", r)
					}
				}()
				
				// Convert the *Event to a map for proper event sending
				eventMap := convertEventToMap(event)
				c.sendEvent(subscriptionID, eventMap)
			}()
		}
	}

	// Send EOSE (End of Stored Events) message
	c.sendEOSE(subscriptionID)
}

// handleCloseSubscription processes a CLOSE message
func (c *Client) handleCloseSubscription(msg []json.RawMessage) {
	if len(msg) < 2 {
		c.sendError("Invalid close request: missing ID", "")
		return
	}

	// Extract subscription ID
	var subscriptionID string
	if err := json.Unmarshal(msg[1], &subscriptionID); err != nil {
		c.sendError("Invalid subscription ID", "")
		return
	}

	// Remove the subscription
	delete(c.subscriptions, subscriptionID)
}

// sendEvent sends an event to the client for a specific subscription
func (c *Client) sendEvent(subscriptionID string, event map[string]interface{}) {
	// Log that we're sending an event to a subscriber
	eventID, _ := event["id"].(string)
	shortID := eventID
	if len(shortID) > 8 {
		shortID = shortID[:8] + "..."
	}
	
	utils.NewLogger("relay").Debug(" Sending event %s to subscription %s", shortID, subscriptionID)
	
	response := []interface{}{"EVENT", subscriptionID, event}
	c.sendResponse(response)
}

// sendEOSE sends an EOSE (End of Stored Events) message to the client
func (c *Client) sendEOSE(subscriptionID string) {
	response := []interface{}{"EOSE", subscriptionID}
	c.sendResponse(response)
	
	// It's critical that we DO NOT close the connection after sending EOSE
	// The connection should remain open for real-time updates
	utils.NewLogger("relay").Debug("Sent EOSE for subscription: %s, connection remains open", subscriptionID)
}

// queryEvents queries the database for events matching the given filters
func (r *Relay) queryEvents(filters []Filter) ([]*Event, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	
	utils.NewLogger("relay").Debug("Querying events with %d filters", len(filters))

	// Start with an empty result set
	var allEvents []*Event
	
	// Process each filter separately
	for _, filter := range filters {
		// Build the base query
		query := "SELECT id, pubkey, created_at, kind, tags, content, sig FROM events WHERE 1=1"
		var args []interface{}
		
		// Add filter conditions based on the filter attributes
		if len(filter.Authors) > 0 {
			// Special case for single author for efficiency
			if len(filter.Authors) == 1 {
				query += " AND pubkey = ?"
				args = append(args, filter.Authors[0])
			} else {
				// Multiple authors case
				placeholders := make([]string, len(filter.Authors))
				for i := range placeholders {
					placeholders[i] = "?"
				}
				query += fmt.Sprintf(" AND pubkey IN (%s)", strings.Join(placeholders, ","))
				for _, author := range filter.Authors {
					args = append(args, author)
				}
			}
		}
		
		// Add kinds filter
		if len(filter.Kinds) > 0 {
			// Special case for single kind for efficiency
			if len(filter.Kinds) == 1 {
				query += " AND kind = ?"
				args = append(args, filter.Kinds[0])
			} else {
				// Multiple kinds case
				placeholders := make([]string, len(filter.Kinds))
				for i := range placeholders {
					placeholders[i] = "?"
				}
				query += fmt.Sprintf(" AND kind IN (%s)", strings.Join(placeholders, ","))
				for _, kind := range filter.Kinds {
					args = append(args, kind)
				}
			}
		}
		
		// Add IDs filter
		if len(filter.IDs) > 0 {
			// Special case for single ID for efficiency
			if len(filter.IDs) == 1 {
				query += " AND id = ?"
				args = append(args, filter.IDs[0])
			} else {
				// Multiple IDs case
				placeholders := make([]string, len(filter.IDs))
				for i := range placeholders {
					placeholders[i] = "?"
				}
				query += fmt.Sprintf(" AND id IN (%s)", strings.Join(placeholders, ","))
				for _, id := range filter.IDs {
					args = append(args, id)
				}
			}
		}
		
		// Add since and until time filters
		if filter.Since != nil {
			query += " AND created_at >= ?"
			args = append(args, filter.Since.Unix())
		}
		
		if filter.Until != nil {
			query += " AND created_at <= ?"
			args = append(args, filter.Until.Unix())
		}
		
		// Add limit only if specified in the filter
		if filter.Limit > 0 {
			query += " ORDER BY created_at DESC LIMIT ?"
			args = append(args, filter.Limit)
		} else {
			// No limit, just order by timestamp
			query += " ORDER BY created_at DESC"
		}
		
		// Execute the query
		utils.NewLogger("relay").Debug("Executing SQL query: %s with %d args", query, len(args))
		rows, err := r.db.Query(query, args...)
		if err != nil {
			utils.NewLogger("relay").Error("Database query error: %v", err)
			// Continue to next filter instead of failing the entire query
			continue
		}
		
		// Process results
		events, err := processQueryResults(rows)
		if err != nil {
			utils.NewLogger("relay").Error("Error processing query results: %v", err)
			// Continue to next filter instead of failing the entire query
			continue
		}
		
		// Merge results with deduplication
		allEvents = mergeEventLists(allEvents, events)
		
		// Close rows
		rows.Close()
	}

	utils.NewLogger("relay").Debug("Found %d matching events across all filters", len(allEvents))
	return allEvents, nil
}

// processQueryResults processes SQL query results into Event objects
func processQueryResults(rows *sql.Rows) ([]*Event, error) {
	var events []*Event
	
	for rows.Next() {
		var event Event
		var tagsJSON string
		if err := rows.Scan(&event.ID, &event.PubKey, &event.CreatedAt, &event.Kind, &tagsJSON, &event.Content, &event.Sig); err != nil {
			utils.NewLogger("relay").Error("Error scanning row: %v", err)
			// Continue to next row instead of failing the entire result set
			continue
		}

		// Parse tags JSON
		err := func() error {
			defer func() {
				if r := recover(); r != nil {
					utils.NewLogger("relay").Error("Recovered from panic while parsing tags: %v", r)
				}
			}()
			
			return json.Unmarshal([]byte(tagsJSON), &event.Tags)
		}()
		
		if err != nil {
			utils.NewLogger("relay").Error("Failed to parse tags JSON for event %s: %v", event.ID, err)
			// Skip this event but continue processing others
			continue
		}
		
		events = append(events, &event)
	}
	
	if err := rows.Err(); err != nil {
		utils.NewLogger("relay").Error("Error during row iteration: %v", err)
		// Return events collected so far instead of failing entirely
	}
	
	return events, nil
}

// mergeEventLists merges two event lists with deduplication
func mergeEventLists(list1, list2 []*Event) []*Event {
	// Use a map for deduplication
	eventMap := make(map[string]*Event)
	
	// Add events from list1
	for _, event := range list1 {
		eventMap[event.ID] = event
	}
	
	// Add events from list2
	for _, event := range list2 {
		eventMap[event.ID] = event
	}
	
	// Convert map back to slice
	var merged []*Event
	for _, event := range eventMap {
		merged = append(merged, event)
	}
	
	return merged
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

// matchesEvent checks if an event matches a subscription filter
// This is used for real-time filtering of new events
func matchesEvent(eventMap map[string]interface{}, filter Filter) bool {
	// Convert map to Event struct to use the existing eventMatchesFilter
	event := &Event{}
	
	// Extract ID
	if id, ok := eventMap["id"].(string); ok {
		event.ID = id
	} else {
		return false
	}
	
	// Extract PubKey
	if pubkey, ok := eventMap["pubkey"].(string); ok {
		event.PubKey = pubkey
	} else {
		return false
	}
	
	// Extract CreatedAt
	if createdAt, ok := eventMap["created_at"].(float64); ok {
		event.CreatedAt = int64(createdAt)
	} else {
		return false
	}
	
	// Extract Kind
	if kind, ok := eventMap["kind"].(float64); ok {
		event.Kind = int(kind)
	} else {
		return false
	}
	
	// Extract Content
	if content, ok := eventMap["content"].(string); ok {
		event.Content = content
	}
	
	// Extract Tags
	if tags, ok := eventMap["tags"].([]interface{}); ok {
		for _, tagArray := range tags {
			if tagStrings, ok := tagArray.([]interface{}); ok {
				var stringTag []string
				for _, t := range tagStrings {
					if str, ok := t.(string); ok {
						stringTag = append(stringTag, str)
					}
				}
				event.Tags = append(event.Tags, stringTag)
			}
		}
	}
	
	// Extract Sig
	if sig, ok := eventMap["sig"].(string); ok {
		event.Sig = sig
	}
	
	// Use the existing filter logic
	return eventMatchesFilter(event, filter)
}

// Helper function to convert an Event to a map[string]interface{}
func convertEventToMap(event *Event) map[string]interface{} {
	return map[string]interface{}{
		"id":         event.ID,
		"pubkey":     event.PubKey,
		"created_at": event.CreatedAt,
		"kind":       event.Kind,
		"tags":       event.Tags,
		"content":    event.Content,
		"sig":        event.Sig,
	}
}
