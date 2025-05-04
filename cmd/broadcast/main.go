package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/gorilla/websocket"

	relaypkg "github.com/gareth/go-nostr-relay/relay"
)

// fetchRelays fetches the list of relays from nostr.watch
func fetchRelays() ([]string, error) {
	resp, err := http.Get("https://api.nostr.watch/v1/online")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch relay list: %w", err)
	}
	defer resp.Body.Close()

	var relays []string
	if err := json.NewDecoder(resp.Body).Decode(&relays); err != nil {
		return nil, fmt.Errorf("failed to decode relay list: %w", err)
	}
	return relays, nil
}

// getEventByID returns the relay.Event by ID from the given db
func getEventByID(db *sql.DB, id string) (*relaypkg.Event, error) {
	row := db.QueryRow("SELECT id, pubkey, created_at, kind, tags, content, sig FROM events WHERE id = ?", id)
	var event relaypkg.Event
	var tagsJSON string
	if err := row.Scan(&event.ID, &event.PubKey, &event.CreatedAt, &event.Kind, &tagsJSON, &event.Content, &event.Sig); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("event ID not found: %s", id)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
		return nil, fmt.Errorf("failed to parse tags: %w", err)
	}
	return &event, nil
}

// broadcastToRelay connects, publishes the event, then disconnects
func broadcastToRelay(relayURL string, event *relaypkg.Event, wg *sync.WaitGroup, mu *sync.Mutex, errors *[]string, successes *[]string) {
	defer wg.Done()

	c, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		mu.Lock()
		*errors = append(*errors, fmt.Sprintf("❌ %s: connect error: %v", relayURL, err))
		mu.Unlock()
		return
	}
	defer c.Close()

	// Send ["EVENT", event]
	msg := []interface{}{"EVENT", event}
	if err := c.WriteJSON(msg); err != nil {
		mu.Lock()
		*errors = append(*errors, fmt.Sprintf("❌ %s: send error: %v", relayURL, err))
		mu.Unlock()
		return
	}

	// Wait for OK or ERROR
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, resp, err := c.ReadMessage()
	if err != nil {
		mu.Lock()
		*errors = append(*errors, fmt.Sprintf("❌ %s: read error: %v", relayURL, err))
		mu.Unlock()
		return
	}
	
	var arr []interface{}
	if err := json.Unmarshal(resp, &arr); err != nil {
		mu.Lock()
		*errors = append(*errors, fmt.Sprintf("❌ %s: decode error: %v", relayURL, err))
		mu.Unlock()
		return
	}
	
	if len(arr) < 2 || arr[0] != "OK" {
		mu.Lock()
		*errors = append(*errors, fmt.Sprintf("❌ %s: unexpected response: %s", relayURL, string(resp)))
		mu.Unlock()
		return
	}
	
	// Success
	mu.Lock()
	*successes = append(*successes, fmt.Sprintf("✅ %s: published successfully", relayURL))
	mu.Unlock()
	
	// Log success immediately
	log.Printf("✅ Published to %s", relayURL)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <event-id>\n", os.Args[0])
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	eventID := flag.Arg(0)

	db, err := sql.Open("sqlite3", "./nostr.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Validate that we have the event before making any connections
	event, err := getEventByID(db, eventID)
	if err != nil {
		log.Fatalf("%v", err)
	}

	log.Printf("Found event ID %s (kind: %d) from %s", event.ID, event.Kind, event.PubKey)

	relays, err := fetchRelays()
	if err != nil {
		log.Fatalf("Failed to fetch relays: %v", err)
	}
	if len(relays) == 0 {
		log.Fatalf("No relays discovered from nostr.watch")
	}

	log.Printf("Discovered %d relays from nostr.watch", len(relays))

	// Ensure no relay is connected to more than once
	relaySet := make(map[string]struct{})
	for _, r := range relays {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if !strings.HasPrefix(r, "ws://") && !strings.HasPrefix(r, "wss://") {
			continue
		}
		relaySet[r] = struct{}{}
	}
	uniqueRelays := make([]string, 0, len(relaySet))
	for r := range relaySet {
		uniqueRelays = append(uniqueRelays, r)
	}
	sort.Strings(uniqueRelays)

	log.Printf("Broadcasting to %d unique relays", len(uniqueRelays))

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]string, 0)
	successes := make([]string, 0)

	// Connect simultaneously to all relays using goroutines
	for _, relayURL := range uniqueRelays {
		wg.Add(1)
		go broadcastToRelay(relayURL, event, &wg, &mu, &errors, &successes)
	}
	wg.Wait()

	// Print summary
	log.Printf("📊 Broadcast summary: %d successful, %d failed", len(successes), len(errors))
	
	// Print errors
	if len(errors) > 0 {
		log.Printf("❌ Errors:")
		for _, e := range errors {
			log.Printf("  %s", e)
		}
	}
	
	// Final status
	if len(errors) == 0 {
		log.Printf("🎉 Broadcast completed successfully to all %d relays!", len(uniqueRelays))
	} else if len(successes) > 0 {
		log.Printf("🎭 Broadcast completed with mixed results: %d successes, %d failures", 
			len(successes), len(errors))
	} else {
		log.Printf("💔 Broadcast failed to all relays")
	}
}
