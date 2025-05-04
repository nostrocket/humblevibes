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

var forwarderLogger = utils.NewLogger("forwarder")

// Config holds the forwarder configuration
type Config struct {
	SourceRelays []string
	TargetRelay  string
	Filters      map[string]interface{}
	BatchSize    int
	LogEvents    bool
	PrintEvents  bool
	SkipOld      bool
	UseDiscovery bool // Flag to indicate if we should use relay discovery
	MaxRelays    int  // Maximum number of relays to discover
	NIP65        bool // Use NIP-65 (kind 10002) events to find user's preferred relays
}

// NostrWatchRelay represents a relay from the nostr.watch API response
type NostrWatchRelay struct {
	URL      string  `json:"url"`
	Count    int     `json:"count"`
	Uptime   float64 `json:"uptime"`
	Country  string  `json:"country"`
	Software string  `json:"software"`
}

// fetchPopularRelays fetches the most popular relays from nostr.watch
func fetchPopularRelays(maxRelays int) ([]string, error) {
	forwarderLogger.Info("Discovering relays using nostr.watch...")
	
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
		forwarderLogger.Info("Discovered relay: %s", relayURL)
	}
	
	if len(discoveredRelays) > 0 {
		forwarderLogger.Info("Discovered %d relays", len(discoveredRelays))
	} else {
		return nil, fmt.Errorf("no valid relays discovered")
	}
	
	return discoveredRelays, nil
}

func main() {
	// Parse command-line flags
	targetRelay := flag.String("target", "ws://localhost:8080/ws", "Target relay URL to forward events to")
	sourceRelays := flag.String("sources", "", "Comma-separated list of source relay URLs to subscribe to")
	kinds := flag.String("kinds", "", "Comma-separated list of event kinds to forward (default all kinds)")
	pubkeys := flag.String("pubkeys", "", "Comma-separated list of public keys to filter by (hex format)")
	since := flag.Int64("since", 0, "Only forward events newer than this Unix timestamp (0 = no limit)")
	until := flag.Int64("until", 0, "Only forward events older than this Unix timestamp (0 = no limit)")
	limit := flag.Int("limit", 0, "Maximum number of events to request from each source relay (0 = no limit)")
	batchSize := flag.Int("batch", 10, "Number of events to forward in a batch")
	logEvents := flag.Bool("log", false, "Log event details when forwarding")
	printEvents := flag.Bool("print-events", false, "Print each event received before forwarding")
	flag.BoolVar(printEvents, "p", false, "Print each event received before forwarding (shorthand)")
	skipOld := flag.Bool("skip-old", false, "Skip events older than 24 hours")
	useDiscovery := flag.Bool("discover", false, "Discover relays using nostr.watch API when no source relays are specified")
	
	// Define relayCount with both long and short forms
	relayCount := flag.Int("relays", 10, "Number of relays to auto-discover when using -discover")
	flag.IntVar(relayCount, "n", 10, "Number of relays to auto-discover (shorthand)")
	
	// Add NIP-65 relay discovery option
	useNIP65 := flag.Bool("nip65", false, "Discover user's preferred relays from their NIP-65 (kind 10002) events")
	
	// Keep max-relays for backward compatibility
	flag.IntVar(relayCount, "max-relays", 10, "Maximum number of relays to discover (legacy, use -relays instead)")
	
	flag.Parse()

	// Parse kinds (empty = all kinds)
	var kindsList []int
	if *kinds != "" {
		for _, k := range strings.Split(*kinds, ",") {
			var kind int
			fmt.Sscanf(k, "%d", &kind)
			kindsList = append(kindsList, kind)
		}
	}

	// Create filters
	filters := map[string]interface{}{}
	if *limit > 0 {
		filters["limit"] = *limit
	}

	// Add kinds filter if specified
	if len(kindsList) > 0 {
		filters["kinds"] = kindsList
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
				forwarderLogger.Warn("Warning: Invalid pubkey format for %s: %v", pk, err)
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

	// Set up source relays - either from command line or discovery
	var relayList []string
	
	// If source relays are empty and discovery is enabled, fetch from nostr.watch
	if *sourceRelays == "" {
		if *useNIP65 && *pubkeys != "" {
			// First, check if there's a pubkey specified since we need it for NIP-65
			pubkeysList := strings.Split(*pubkeys, ",")
			if len(pubkeysList) > 0 {
				// Use the first pubkey for NIP-65 discovery
				pubkey := strings.TrimSpace(pubkeysList[0])
				
				// Convert to hex if it's a bech32 key
				hexPk, err := client.ConvertBech32PubkeyToHex(pubkey)
				if err != nil {
					forwarderLogger.Error("Invalid pubkey format for NIP-65 discovery: %v", err)
					os.Exit(1)
				}
				
				// Create a forwarder with default config just for NIP-65 discovery
				tempConfig := Config{
					MaxRelays: *relayCount,
				}
				tempForwarder := NewForwarder(tempConfig)
				
				// Find relays from NIP-65
				nip65Relays, err := tempForwarder.FindRelaysFromNIP65(hexPk)
				if err != nil {
					forwarderLogger.Warn("NIP-65 discovery failed: %v. Falling back to nostr.watch discovery.", err)
				} else if len(nip65Relays) > 0 {
					forwarderLogger.Info("Using %d relays from NIP-65 event", len(nip65Relays))
					relayList = nip65Relays
				}
			}
		}
		
		// If we didn't get relays from NIP-65, fall back to nostr.watch discovery
		if len(relayList) == 0 && *useDiscovery {
			discoveredRelays, err := fetchPopularRelays(*relayCount)
			if err != nil {
				forwarderLogger.Error("Failed to discover relays: %v", err)
				os.Exit(1)
			}
			if len(discoveredRelays) == 0 {
				forwarderLogger.Error("No relays discovered. Please specify source relays manually.")
				flag.Usage()
				os.Exit(1)
			}
			relayList = discoveredRelays
		}
		
		// If we still have no relays, show an error
		if len(relayList) == 0 {
			forwarderLogger.Error("No source relays specified. Use -sources flag to provide relay URLs, -discover flag to automatically discover relays, or -nip65 to use the author's preferred relays.")
			flag.Usage()
			os.Exit(1)
		}
	} else {
		// Use manually specified relays
		relayList = strings.Split(*sourceRelays, ",")
	}

	// Create configuration
	config := Config{
		SourceRelays: relayList,
		TargetRelay:  *targetRelay,
		Filters:      filters,
		BatchSize:    *batchSize,
		LogEvents:    *logEvents,
		PrintEvents:  *printEvents,
		SkipOld:      *skipOld,
		UseDiscovery: *useDiscovery,
		MaxRelays:    *relayCount,
		NIP65:        *useNIP65,
	}

	// Print configuration
	forwarderLogger.Info("Nostr Event Forwarder")
	forwarderLogger.Info("Target relay: %s", config.TargetRelay)
	forwarderLogger.Info("Source relays: %v", config.SourceRelays)
	
	if config.UseDiscovery && len(config.SourceRelays) > 0 {
		forwarderLogger.Info("Using %d auto-discovered relays (max: %d)", 
			len(config.SourceRelays), config.MaxRelays)
	}
	
	if kinds, ok := config.Filters["kinds"].([]int); ok {
		forwarderLogger.Info("Event kinds: %v", kinds)
	}
	if authors, ok := config.Filters["authors"].([]string); ok {
		forwarderLogger.Info("Filtering by authors: %v", authors)
	}
	if since, ok := config.Filters["since"].(int64); ok {
		forwarderLogger.Info("Since: %v (%s)", since, time.Unix(since, 0).Format(time.RFC3339))
	}
	if until, ok := config.Filters["until"].(int64); ok {
		forwarderLogger.Info("Until: %v (%s)", until, time.Unix(until, 0).Format(time.RFC3339))
	}
	forwarderLogger.Info("Batch size: %d", config.BatchSize)
	forwarderLogger.Info("Skip old events: %v", config.SkipOld)

	// Start the forwarder
	forwarder := NewForwarder(config)
	forwarder.Start()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Shutdown
	forwarderLogger.Info("Shutting down forwarder...")
	forwarder.Stop()
	forwarderLogger.Info("Forwarder shutdown complete")
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
		forwarderLogger.Error("Failed to connect to target relay: %v", err)
		os.Exit(1)
	}

	// Deduplicate source relays to avoid multiple REQs to the same relay
	seen := make(map[string]struct{})
	uniqueRelays := make([]string, 0, len(f.config.SourceRelays))
	for _, url := range f.config.SourceRelays {
		if _, exists := seen[url]; !exists {
			seen[url] = struct{}{}
			uniqueRelays = append(uniqueRelays, url)
		}
	}
	if len(uniqueRelays) < len(f.config.SourceRelays) {
		forwarderLogger.Info("Removed %d duplicate source relays", len(f.config.SourceRelays)-len(uniqueRelays))
	}
	
	// Use a mutex to protect concurrent access to sourceClients
	var sourceClientsMutex sync.Mutex
	
	// Create a wait group to know when all connection attempts are done
	var connectWg sync.WaitGroup
	
	// Connect to source relays concurrently
	connectWg.Add(len(uniqueRelays))
	for _, relayURL := range uniqueRelays {
		// Capture the relayURL for the goroutine
		relayURL := relayURL
		
		// Start a goroutine to connect to this relay
		go func() {
			defer connectWg.Done()
			
			// Connect to the relay
			sourceClient, err := client.NewNostrClient(relayURL)
			if err != nil {
				forwarderLogger.Warn("Failed to connect to source relay %s: %v", relayURL, err)
				return
			}
			
			// Safely add to the source clients slice
			sourceClientsMutex.Lock()
			f.sourceClients = append(f.sourceClients, sourceClient)
			sourceClientsMutex.Unlock()
			
			// Start subscription in a new goroutine
			f.wg.Add(1)
			go f.subscribeToRelay(sourceClient, relayURL)
		}()
	}
	
	// Wait for all connection attempts to complete
	connectWg.Wait()
	
	// Log the result
	forwarderLogger.Info("Connected to %d out of %d unique source relays", len(f.sourceClients), len(uniqueRelays))

	// Start event processor
	f.wg.Add(1)
	go f.processEvents()

	forwarderLogger.Info("Forwarder started with %d source relays", len(f.sourceClients))
}

// Stop disconnects from relays and stops forwarding
func (f *Forwarder) Stop() {
	// Signal all goroutines to stop
	close(f.stopChan)

	// Close connections to trigger read errors and exit subscriptions
	for _, client := range f.sourceClients {
		client.Close()
	}
	if f.targetClient != nil {
		f.targetClient.Close()
	}

	// Wait for all goroutines to finish
	f.wg.Wait()
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
		forwarderLogger.Warn("Failed to subscribe to events from %s: %v", relayURL, err)
		return
	}

	forwarderLogger.Info("Subscribed to events from %s", relayURL)

	// Wait for stop signal
	<-f.stopChan
	forwarderLogger.Info("Unsubscribing from %s", relayURL)
}

// processEvents processes events from the queue and forwards them to the target relay
func (f *Forwarder) processEvents() {
	defer f.wg.Done()
	
	// Create a buffer to accumulate events for batch forwarding
	batch := make([]*client.Event, 0, f.config.BatchSize)
	lastForwardTime := time.Now()
	
	for {
		select {
		case <-f.stopChan:
			// Flush any remaining events before stopping
			if len(batch) > 0 {
				f.forwardEvents(batch)
			}
			return
			
		case event := <-f.events:
			// Handle a new event
			if f.config.PrintEvents {
				content := event.Content
				if len(content) > 40 {
					content = content[:40] + "..."
				}
				forwarderLogger.Info("🔔 Received event: kind=%d, content=%s", event.Kind, content)
			}
			
			// Log event ID
			forwarderLogger.Info("✅ Collected event ID: %s...", truncateString(event.ID, 5))
			
			// Add to the current batch
			batch = append(batch, event)
			
			// Forward events when batch is full or timeout occurred
			if len(batch) >= f.config.BatchSize || time.Since(lastForwardTime) > 5*time.Second {
				f.forwardEvents(batch)
				batch = batch[:0] // Clear the batch
				lastForwardTime = time.Now()
			}
		}
	}
}

// forwardEvents forwards a batch of events to the target relay
func (f *Forwarder) forwardEvents(events []*client.Event) {
	if len(events) == 0 {
		return
	}

	forwarderLogger.Info("Forwarding %d events to target relay", len(events))

	for _, event := range events {
		// Skip events that are too old (older than 24 hours) if configured to do so
		if f.config.SkipOld && time.Now().Unix()-event.CreatedAt > 86400 {
			forwarderLogger.Info("Skipping old event from %s", 
				time.Unix(event.CreatedAt, 0).Format(time.RFC3339))
			continue
		}

		// Print the event details if configured
		if f.config.PrintEvents || f.config.LogEvents {
			content := event.Content
			if len(content) > 40 {
				content = content[:40] + "..."
			}
			forwarderLogger.Info("Event: kind=%d, author=%s, content=%s", 
				event.Kind, 
				event.PubKey[:8], 
				content)
		}

		// Forward the event to the target relay
		success, errMsg, err := f.targetClient.PublishExistingEvent(event)
		if err != nil {
			forwarderLogger.Error("Failed to forward event: %v", err)
		} else if !success {
			forwarderLogger.Error("Relay rejected event: %s", errMsg) 
		} else {
			forwarderLogger.Info("✅ Successfully forwarded event ID: %s", 
				truncateString(event.ID, 16))
		}
		
		// Add a small delay between events to prevent overwhelming the relay
		time.Sleep(50 * time.Millisecond)
	}
}

// FindRelaysFromNIP65 discovers relays from a user's NIP-65 (kind 10002) event
func (f *Forwarder) FindRelaysFromNIP65(pubkey string) ([]string, error) {
	forwarderLogger.Info("Looking for NIP-65 relay list for pubkey %s...", pubkey)
	
	// First, we need to discover initial relays to find the NIP-65 event
	initialRelays, err := fetchPopularRelays(f.config.MaxRelays)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch initial relays: %v", err)
	}
	
	// Store the discovered NIP-65 relays
	var nip65Relays []string
	var nip65EventFound = false
	var nip65ConnectMutex sync.Mutex
	
	// Connect to initial relays and look for the NIP-65 event
	var nip65Wg sync.WaitGroup
	nip65Wg.Add(len(initialRelays))
	
	for _, relayURL := range initialRelays {
		// Capture the relayURL for the goroutine
		relayURL := relayURL
		
		// Start a goroutine to connect to this relay and look for NIP-65 events
		go func() {
			defer nip65Wg.Done()
			
			// Connect to the relay
			sourceClient, err := client.NewNostrClient(relayURL)
			if err != nil {
				forwarderLogger.Warn("Failed to connect to relay %s for NIP-65 discovery: %v", relayURL, err)
				return
			}
			defer sourceClient.Close()
			
			// Create NIP-65 filter
			nip65Filter := map[string]interface{}{
				"kinds":   []int{10002}, // NIP-65 relay list event
				"authors": []string{pubkey},
				"limit":   1, // We only need the most recent one
			}
			
			// Create a channel for events and a channel for completion signals
			eventChan := make(chan *client.Event, 1)
			completeChan := make(chan struct{})
			
			// Set up handler for NIP-65 events
			handler := func(event *client.Event) {
				// We're only interested in the first event we receive
				select {
				case eventChan <- event:
				default:
					// Channel is full, ignoring additional events
				}
			}
			
			// Subscribe to NIP-65 events
			subID := fmt.Sprintf("nip65_%x", time.Now().UnixNano())
			err = sourceClient.SubscribeToEventsWithHandler(subID, nip65Filter, handler)
			if err != nil {
				forwarderLogger.Warn("Failed to subscribe to NIP-65 events from %s: %v", relayURL, err)
				return
			}
			
			// Set a timeout to limit how long we search for NIP-65 events
			timeout := time.After(5 * time.Second)
			
			// Wait for event or timeout
			select {
			case event := <-eventChan:
				// Process NIP-65 event to extract relay URLs
				relays := extractRelaysFromNIP65Event(event)
				if len(relays) > 0 {
					nip65ConnectMutex.Lock()
					if !nip65EventFound {
						// This is the first NIP-65 event we found
						nip65EventFound = true
						nip65Relays = relays
						forwarderLogger.Info("Found NIP-65 event with %d relay URLs from %s", len(relays), relayURL)
					}
					nip65ConnectMutex.Unlock()
				}
				
			case <-timeout:
				// Timeout occurred
				forwarderLogger.Debug("Timeout waiting for NIP-65 events from %s", relayURL)
			}
			
			// Cancel the subscription
			close(completeChan)
		}()
	}
	
	// Wait for all relays to be checked
	nip65Wg.Wait()
	
	// Return the discovered relays from NIP-65 event
	if nip65EventFound {
		return nip65Relays, nil
	}
	
	return nil, fmt.Errorf("no NIP-65 events found for pubkey %s", pubkey)
}

// extractRelaysFromNIP65Event extracts relay URLs from a NIP-65 (kind 10002) event
func extractRelaysFromNIP65Event(event *client.Event) []string {
	var relays []string
	
	// NIP-65 uses 'r' tags for relay URLs
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			// The relay URL is in the second position (index 1)
			relayURL := tag[1]
			
			// Optional read/write value is in third position (index 2)
			// We include relays with no marker or with 'read' or 'write'
			// We need both for our purpose (read to fetch events, write to forward them)
			if len(tag) < 3 || tag[2] == "read" || tag[2] == "write" {
				// In a NIP-65 event, relays are often listed like this:
				// ["r", "wss://relay.example.com", "read"]
				// ["r", "wss://another.relay.example", "write"]
				// We also need to ensure the URL begins with ws:// or wss://
				if strings.HasPrefix(relayURL, "wss://") || strings.HasPrefix(relayURL, "ws://") {
					relays = append(relays, relayURL)
				}
			}
		}
	}
	
	return relays
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
