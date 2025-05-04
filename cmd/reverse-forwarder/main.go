package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gareth/go-nostr-relay/client"
	"github.com/gareth/go-nostr-relay/lib/utils"
)

var logger = utils.NewLogger("reverse-forwarder")

// Config holds the reverse forwarder configuration
type Config struct {
	SourceRelay  string              // Single source relay to subscribe to
	TargetRelays []string            // Multiple target relays to forward events to
	Filters      map[string]interface{}
	BatchSize    int
	LogEvents    bool
	PrintEvents  bool
	SkipOld      bool
	MaxRelays    int // Maximum number of relays to discover
}

// ReverseForwarder handles subscribing to a source relay and forwarding events to multiple target relays
type ReverseForwarder struct {
	config       Config
	sourceClient *client.NostrClient
	targetClients []*client.NostrClient
	events       chan *client.Event
	wg           sync.WaitGroup
	stopChan     chan struct{}
}

// NewReverseForwarder creates a new reverse forwarder with the given configuration
func NewReverseForwarder(config Config) *ReverseForwarder {
	return &ReverseForwarder{
		config:   config,
		events:   make(chan *client.Event, 1000),
		stopChan: make(chan struct{}),
	}
}

// fetchRelaysFromNostrWatch fetches active relays from nostr.watch
func fetchRelaysFromNostrWatch(maxRelays int) ([]string, error) {
	logger.Info("Discovering relays using nostr.watch...")
	
	// Make HTTP request to nostr.watch API
	resp, err := http.Get("https://api.nostr.watch/v1/online")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nostr.watch: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nostr.watch API returned status code %d", resp.StatusCode)
	}
	
	// Read and parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	
	// The response is a JSON array of relay URLs
	var relayURLs []string
	if err := json.Unmarshal(body, &relayURLs); err != nil {
		return nil, fmt.Errorf("failed to parse relay list: %v", err)
	}
	
	// Filter and limit relays
	var discoveredRelays []string
	for i, relayURL := range relayURLs {
		if i >= maxRelays {
			break
		}
		
		// Skip relays without wss:// protocol
		if !strings.HasPrefix(relayURL, "wss://") {
			continue
		}
		
		discoveredRelays = append(discoveredRelays, relayURL)
		logger.Info("Discovered relay: %s", relayURL)
	}
	
	if len(discoveredRelays) > 0 {
		logger.Info("Discovered %d relays", len(discoveredRelays))
	} else {
		return nil, fmt.Errorf("no valid relays discovered")
	}
	
	return discoveredRelays, nil
}

// Start connects to relays and begins forwarding events
func (rf *ReverseForwarder) Start() error {
	// Connect to the source relay first
	logger.Info("Connecting to source relay: %s", rf.config.SourceRelay)
	sourceClient, err := client.NewNostrClient(rf.config.SourceRelay)
	if err != nil {
		return fmt.Errorf("failed to connect to source relay %s: %v", rf.config.SourceRelay, err)
	}
	rf.sourceClient = sourceClient

	// Validate that we can subscribe to the source relay before proceeding
	if err := rf.validateSourceRelay(); err != nil {
		rf.sourceClient.Close()
		return fmt.Errorf("source relay validation failed for %s: %v", rf.config.SourceRelay, err)
	}
	
	logger.Info("Successfully validated connection to source relay: %s", rf.config.SourceRelay)

	// Now connect to all target relays
	for _, relayURL := range rf.config.TargetRelays {
		logger.Info("Connecting to target relay: %s", relayURL)
		targetClient, err := client.NewNostrClient(relayURL)
		if err != nil {
			logger.Warn("Failed to connect to target relay %s: %v", relayURL, err)
			continue
		}
		rf.targetClients = append(rf.targetClients, targetClient)
	}
	
	if len(rf.targetClients) == 0 {
		rf.sourceClient.Close()
		return fmt.Errorf("failed to connect to any target relays")
	}
	
	logger.Info("Connected to %d target relays", len(rf.targetClients))

	// Start the event processor
	rf.wg.Add(1)
	go rf.processEvents()

	// Subscribe to the source relay
	rf.wg.Add(1)
	go rf.subscribeToSourceRelay()

	return nil
}

// Stop disconnects from relays and stops forwarding
func (rf *ReverseForwarder) Stop() {
	logger.Info("Stopping reverse forwarder...")
	close(rf.stopChan)
	
	// Wait for all goroutines to finish
	rf.wg.Wait()
	
	// Close all connections
	if rf.sourceClient != nil {
		rf.sourceClient.Close()
	}
	
	for _, client := range rf.targetClients {
		client.Close()
	}
	
	logger.Info("Reverse forwarder stopped")
}

// subscribeToSourceRelay subscribes to events from the source relay
func (rf *ReverseForwarder) subscribeToSourceRelay() {
	defer rf.wg.Done()
	
	logger.Info("Subscribing to events from %s", rf.config.SourceRelay)
	
	// Generate a subscription ID
	subID := fmt.Sprintf("sub_%d", time.Now().Unix())
	
	// Subscribe to events
	err := rf.sourceClient.SubscribeToEventsWithHandler(subID, rf.config.Filters, func(event *client.Event) {
		// Check if we should skip old events
		if rf.config.SkipOld && time.Now().Unix()-event.CreatedAt > 86400 {
			return
		}
		
		// Log or print the event if requested
		if rf.config.LogEvents {
			logger.Info("Received event %s from %s", truncateString(event.ID, 8), rf.config.SourceRelay)
		}
		if rf.config.PrintEvents {
			eventJSON, _ := json.Marshal(event)
			fmt.Println(string(eventJSON))
		}
		
		select {
		case rf.events <- event:
			// Event added to queue
		case <-rf.stopChan:
			// Forwarder is stopping
			return
		}
	})
	
	if err != nil {
		logger.Error("Failed to subscribe to events from %s: %v", rf.config.SourceRelay, err)
		return
	}
	
	// Keep the subscription active until stopped
	<-rf.stopChan
}

// processEvents processes events from the queue and forwards them to the target relays
func (rf *ReverseForwarder) processEvents() {
	defer rf.wg.Done()
	
	batch := make([]*client.Event, 0, rf.config.BatchSize)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case event := <-rf.events:
			// Add the event to the current batch
			batch = append(batch, event)
			
			// If we've reached the batch size, forward the events
			if len(batch) >= rf.config.BatchSize {
				rf.forwardEvents(batch)
				batch = make([]*client.Event, 0, rf.config.BatchSize)
			}
			
		case <-ticker.C:
			// Forward any events in the batch every 500ms
			if len(batch) > 0 {
				rf.forwardEvents(batch)
				batch = make([]*client.Event, 0, rf.config.BatchSize)
			}
			
		case <-rf.stopChan:
			// Forward any remaining events
			if len(batch) > 0 {
				rf.forwardEvents(batch)
			}
			return
		}
	}
}

// forwardEvents forwards events to all target relays
func (rf *ReverseForwarder) forwardEvents(events []*client.Event) {
	if len(events) == 0 {
		return
	}
	
	// Create a waitgroup to track completion
	var wg sync.WaitGroup
	
	// Forward to each target relay in parallel
	for _, targetClient := range rf.targetClients {
		wg.Add(1)
		go func(client *client.NostrClient, eventBatch []*client.Event) {
			defer wg.Done()
			
			for _, event := range eventBatch {
				success, message, err := client.PublishExistingEvent(event)
				
				if err != nil {
					logger.Error("Failed to publish event %s to relay: %v", truncateString(event.ID, 8), err)
				} else if !success {
					logger.Warn("Relay rejected event %s: %s", truncateString(event.ID, 8), message)
				} else if rf.config.LogEvents {
					logger.Info("Successfully forwarded event %s", truncateString(event.ID, 8))
				}
			}
		}(targetClient, events)
	}
	
	// Wait for all forwards to complete
	wg.Wait()
	
	logger.Info("Forwarded %d events to %d relays", len(events), len(rf.targetClients))
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// validateSourceRelay attempts a test subscription to verify that the source relay is responsive
func (rf *ReverseForwarder) validateSourceRelay() error {
	logger.Info("Validating source relay connection...")
	
	// Create a validation channel to receive the validation result
	validationChan := make(chan error, 1)
	
	// Generate a test subscription ID
	testSubID := fmt.Sprintf("test_sub_%d", time.Now().Unix())
	
	// Create minimal filter for validation - request just 1 event
	testFilter := map[string]interface{}{
		"limit": 1,
	}
	
	// Set a timeout for validation
	timeout := time.After(5 * time.Second)
	
	// Start a goroutine to test the subscription
	go func() {
		// Subscribe with a simple handler that just signals success
		err := rf.sourceClient.SubscribeToEventsWithHandler(testSubID, testFilter, 
			func(event *client.Event) {
				// Successfully received an event, signal success
				select {
				case validationChan <- nil:
					// Sent success signal
				default:
					// Channel already has a value, do nothing
				}
			})
		
		if err != nil {
			// Failed to subscribe
			validationChan <- err
			return
		}
		
		// If we get here without error but don't receive an event within the timeout,
		// the timeout will handle it
	}()
	
	// Wait for either validation success, error, or timeout
	select {
	case err := <-validationChan:
		// We can't unsubscribe directly since we can't access the WebSocket connection
		// But we'll consider the validation complete
		return err
		
	case <-timeout:
		// Even if we timeout, consider it a success if we at least could submit the subscription
		// Many relays might not have events matching our criteria, but still be valid
		return nil
	}
}

func main() {
	// Parse command-line flags
	sourceRelay := flag.String("source", "", "Source relay URL to subscribe to")
	targetRelays := flag.String("targets", "", "Comma-separated list of target relay URLs (optional, uses nostr.watch if not provided)")
	kinds := flag.String("kinds", "1", "Comma-separated list of event kinds to forward (e.g., '1,4,7')")
	pubkeys := flag.String("pubkeys", "", "Comma-separated list of public keys to filter by (hex format)")
	since := flag.Int64("since", 0, "Only forward events newer than this Unix timestamp (0 = no limit)")
	until := flag.Int64("until", 0, "Only forward events older than this Unix timestamp (0 = no limit)")
	limit := flag.Int("limit", 100, "Maximum number of events to request from the source relay")
	batchSize := flag.Int("batch", 10, "Number of events to forward in a batch")
	logEvents := flag.Bool("log", false, "Log event details when forwarding")
	printEvents := flag.Bool("print-events", false, "Print each event received before forwarding")
	flag.BoolVar(printEvents, "p", false, "Print each event received before forwarding (shorthand)")
	skipOld := flag.Bool("skip-old", false, "Skip events older than 24 hours")
	maxRelays := flag.Int("relays", 10, "Number of relays to auto-discover from nostr.watch")
	flag.IntVar(maxRelays, "n", 10, "Number of relays to auto-discover (shorthand)")
	
	flag.Parse()
	
	// Ensure a source relay is provided
	if *sourceRelay == "" {
		fmt.Println("Error: Source relay is required")
		fmt.Println("Usage: reverse-forwarder -source <relay-url> [options]")
		flag.PrintDefaults()
		os.Exit(1)
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
				logger.Warn("Warning: Invalid pubkey format for %s: %v", pk, err)
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
	
	// Set up target relays - either from command line or discovery
	var targetRelayList []string
	
	if *targetRelays != "" {
		// Use explicitly provided target relays
		targetRelayList = strings.Split(*targetRelays, ",")
		for i, relay := range targetRelayList {
			targetRelayList[i] = strings.TrimSpace(relay)
		}
		logger.Info("Using %d user-specified target relays", len(targetRelayList))
	} else {
		// Discover relays from nostr.watch
		discoveredRelays, err := fetchRelaysFromNostrWatch(*maxRelays)
		if err != nil {
			logger.Error("Failed to discover relays: %v", err)
			os.Exit(1)
		}
		targetRelayList = discoveredRelays
	}
	
	// Create and start the reverse forwarder
	config := Config{
		SourceRelay:  *sourceRelay,
		TargetRelays: targetRelayList,
		Filters:      filters,
		BatchSize:    *batchSize,
		LogEvents:    *logEvents,
		PrintEvents:  *printEvents,
		SkipOld:      *skipOld,
		MaxRelays:    *maxRelays,
	}
	
	reverseForwarder := NewReverseForwarder(config)
	
	// Start the forwarder
	if err := reverseForwarder.Start(); err != nil {
		logger.Error("Failed to start reverse forwarder: %v", err)
		os.Exit(1)
	}
	
	logger.Info("Reverse forwarder started - subscribing to %s and forwarding to %d relays", 
		*sourceRelay, len(targetRelayList))
	
	// Handle interrupt signal to gracefully shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	// Stop the forwarder
	reverseForwarder.Stop()
}
