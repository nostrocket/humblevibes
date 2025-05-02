package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gareth/go-nostr-relay/client"
)

// Config holds the forwarder configuration
type Config struct {
	SourceRelays []string
	TargetRelay  string
	Filters      map[string]interface{}
	BatchSize    int
	LogEvents    bool
}

func main() {
	// Parse command line flags
	targetRelay := flag.String("target", "ws://localhost:8080/ws", "Target relay URL to forward events to")
	sourceRelays := flag.String("sources", "", "Comma-separated list of source relay URLs to subscribe to")
	kinds := flag.String("kinds", "1", "Comma-separated list of event kinds to forward (e.g., '1,4,7')")
	pubkeys := flag.String("pubkeys", "", "Comma-separated list of public keys to filter by (hex format)")
	since := flag.Int64("since", 0, "Only forward events newer than this Unix timestamp (0 = no limit)")
	until := flag.Int64("until", 0, "Only forward events older than this Unix timestamp (0 = no limit)")
	limit := flag.Int("limit", 100, "Maximum number of events to request from each source relay")
	batchSize := flag.Int("batch", 10, "Number of events to forward in a batch")
	logEvents := flag.Bool("log", false, "Log event details when forwarding")
	flag.Parse()

	// Validate source relays
	if *sourceRelays == "" {
		log.Fatal("No source relays specified. Use -sources flag to provide relay URLs.")
	}

	// Parse kinds
	kindsList := []int{}
	for _, k := range strings.Split(*kinds, ",") {
		var kind int
		fmt.Sscanf(k, "%d", &kind)
		kindsList = append(kindsList, kind)
	}

	// Create filters
	filters := map[string]interface{}{
		"kinds": kindsList,
		"limit": *limit,
	}

	// Add pubkeys filter if specified
	if *pubkeys != "" {
		pubkeysList := strings.Split(*pubkeys, ",")
		// Convert each pubkey to hex format (handles both hex and bech32 formats)
		hexPubkeys := make([]string, 0, len(pubkeysList))
		for _, pk := range pubkeysList {
			pk = strings.TrimSpace(pk)
			if pk == "" {
				continue
			}
			
			// Convert to hex if it's a bech32 key
			hexPk, err := client.ConvertBech32PubkeyToHex(pk)
			if err != nil {
				log.Printf("Warning: Invalid pubkey format for %s: %v", pk, err)
				continue
			}
			
			hexPubkeys = append(hexPubkeys, hexPk)
		}
		
		if len(hexPubkeys) > 0 {
			filters["authors"] = hexPubkeys
		}
	}

	// Add time filters if specified
	if *since > 0 {
		filters["since"] = *since
	}
	if *until > 0 {
		filters["until"] = *until
	}

	// Create configuration
	config := Config{
		SourceRelays: strings.Split(*sourceRelays, ","),
		TargetRelay:  *targetRelay,
		Filters:      filters,
		BatchSize:    *batchSize,
		LogEvents:    *logEvents,
	}

	// Print configuration
	log.Printf("Nostr Event Forwarder")
	log.Printf("Target relay: %s", config.TargetRelay)
	log.Printf("Source relays: %v", config.SourceRelays)
	log.Printf("Event kinds: %v", kindsList)
	if *pubkeys != "" {
		if authors, ok := config.Filters["authors"].([]string); ok && len(authors) > 0 {
			log.Printf("Filtering by authors: %v", authors)
		}
	}
	if *since > 0 {
		log.Printf("Since: %v (%s)", *since, time.Unix(*since, 0).Format(time.RFC3339))
	}
	if *until > 0 {
		log.Printf("Until: %v (%s)", *until, time.Unix(*until, 0).Format(time.RFC3339))
	}
	log.Printf("Batch size: %d", config.BatchSize)

	// Start the forwarder
	forwarder := NewForwarder(config)
	forwarder.Start()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Shutdown
	log.Println("Shutting down forwarder...")
	forwarder.Stop()
	log.Println("Forwarder shutdown complete")
}

// Forwarder handles subscribing to source relays and forwarding events to the target relay
type Forwarder struct {
	config        Config
	sourceClients []*client.NostrClient
	targetClient  *client.NostrClient
	events        chan *client.Event
	wg            sync.WaitGroup
	stopChan      chan struct{}
}

// NewForwarder creates a new forwarder with the given configuration
func NewForwarder(config Config) *Forwarder {
	return &Forwarder{
		config:   config,
		events:   make(chan *client.Event, 1000),
		stopChan: make(chan struct{}),
	}
}

// Start connects to relays and begins forwarding events
func (f *Forwarder) Start() {
	// Connect to target relay
	var err error
	f.targetClient, err = client.NewNostrClient(f.config.TargetRelay)
	if err != nil {
		log.Fatalf("Failed to connect to target relay: %v", err)
	}

	// Connect to source relays
	for _, relayURL := range f.config.SourceRelays {
		sourceClient, err := client.NewNostrClient(relayURL)
		if err != nil {
			log.Printf("Failed to connect to source relay %s: %v", relayURL, err)
			continue
		}
		f.sourceClients = append(f.sourceClients, sourceClient)

		// Subscribe to events
		f.wg.Add(1)
		go f.subscribeToRelay(sourceClient, relayURL)
	}

	// Start event processor
	f.wg.Add(1)
	go f.processEvents()

	log.Printf("Forwarder started with %d source relays", len(f.sourceClients))
}

// Stop disconnects from relays and stops forwarding
func (f *Forwarder) Stop() {
	// Signal all goroutines to stop
	close(f.stopChan)

	// Wait for all goroutines to finish
	f.wg.Wait()

	// Close connections
	if f.targetClient != nil {
		f.targetClient.Close()
	}
	for _, client := range f.sourceClients {
		client.Close()
	}
}

// subscribeToRelay subscribes to events from a source relay
func (f *Forwarder) subscribeToRelay(sourceClient *client.NostrClient, relayURL string) {
	defer f.wg.Done()

	// Create a subscription ID based on the relay URL
	subID := fmt.Sprintf("sub_%x", time.Now().UnixNano())

	// Set up event handler
	eventHandler := func(event *client.Event) {
		select {
		case f.events <- event:
			// Event queued for processing
		case <-f.stopChan:
			// Forwarder is stopping
			return
		}
	}

	// Subscribe to events
	err := sourceClient.SubscribeToEventsWithHandler(subID, f.config.Filters, eventHandler)
	if err != nil {
		log.Printf("Failed to subscribe to events from %s: %v", relayURL, err)
		return
	}

	log.Printf("Subscribed to events from %s", relayURL)

	// Wait for stop signal
	<-f.stopChan
	log.Printf("Unsubscribing from %s", relayURL)
}

// processEvents processes events from the queue and forwards them to the target relay
func (f *Forwarder) processEvents() {
	defer f.wg.Done()

	batch := make([]*client.Event, 0, f.config.BatchSize)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-f.events:
			// Add event to batch
			batch = append(batch, event)

			// Process batch if it's full
			if len(batch) >= f.config.BatchSize {
				f.forwardEvents(batch)
				batch = make([]*client.Event, 0, f.config.BatchSize)
			}

		case <-ticker.C:
			// Process any remaining events in the batch
			if len(batch) > 0 {
				f.forwardEvents(batch)
				batch = make([]*client.Event, 0, f.config.BatchSize)
			}

		case <-f.stopChan:
			// Process any remaining events before stopping
			if len(batch) > 0 {
				f.forwardEvents(batch)
			}
			return
		}
	}
}

// forwardEvents forwards a batch of events to the target relay
func (f *Forwarder) forwardEvents(events []*client.Event) {
	log.Printf("[%s] Forwarding %d events to target relay", time.Now().Format("15:04:05.000"), len(events))

	for _, event := range events {
		// Skip events that are too old
		if time.Now().Unix()-event.CreatedAt > 86400 {
			continue
		}

		if f.config.LogEvents {
			// Log event details
			content := event.Content
			if len(content) > 50 {
				content = content[:47] + "..."
			}
			log.Printf("[%s] Event: kind=%d, author=%s, content=%s", 
				time.Now().Format("15:04:05.000"),
				event.Kind, 
				event.PubKey[:8], 
				content)
		}

		// Forward the event to the target relay
		err := f.targetClient.PublishExistingEvent(event)
		if err != nil {
			log.Printf("[%s] Failed to forward event: %v", 
				time.Now().Format("15:04:05.000"), 
				err)
		} else {
			log.Printf("[%s] Successfully forwarded event ID: %s", 
				time.Now().Format("15:04:05.000"), 
				truncateString(event.ID, 16))
		}
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
