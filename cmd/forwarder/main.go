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
	kinds := flag.String("kinds", "1", "Comma-separated list of event kinds to forward (e.g., '1,4,7')")
	pubkeys := flag.String("pubkeys", "", "Comma-separated list of public keys to filter by (hex format)")
	since := flag.Int64("since", 0, "Only forward events newer than this Unix timestamp (0 = no limit)")
	until := flag.Int64("until", 0, "Only forward events older than this Unix timestamp (0 = no limit)")
	limit := flag.Int("limit", 100, "Maximum number of events to request from each source relay")
	batchSize := flag.Int("batch", 10, "Number of events to forward in a batch")
	logEvents := flag.Bool("log", false, "Log event details when forwarding")
	printEvents := flag.Bool("print-events", false, "Print each event received before forwarding")
	flag.BoolVar(printEvents, "p", false, "Print each event received before forwarding (shorthand)")
	skipOld := flag.Bool("skip-old", false, "Skip events older than 24 hours")
	useDiscovery := flag.Bool("discover", false, "Discover relays using nostr.watch API when no source relays are specified")
	
	// Define relayCount with both long and short forms
	relayCount := flag.Int("relays", 10, "Number of relays to auto-discover when using -discover")
	flag.IntVar(relayCount, "n", 10, "Number of relays to auto-discover (shorthand)")
	
	// Keep max-relays for backward compatibility
	flag.IntVar(relayCount, "max-relays", 10, "Maximum number of relays to discover (legacy, use -relays instead)")
	
	flag.Parse()

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
		if *useDiscovery {
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
		} else {
			forwarderLogger.Error("No source relays specified. Use -sources flag to provide relay URLs or -discover flag to automatically discover relays.")
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

	// Use a mutex to protect concurrent access to sourceClients
	var sourceClientsMutex sync.Mutex
	
	// Create a wait group to know when all connection attempts are done
	var connectWg sync.WaitGroup
	
	// Connect to source relays concurrently
	connectWg.Add(len(f.config.SourceRelays))
	for _, relayURL := range f.config.SourceRelays {
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
	forwarderLogger.Info("Connected to %d out of %d source relays", 
		len(f.sourceClients), len(f.config.SourceRelays))

	// Start event processor
	f.wg.Add(1)
	go f.processEvents()

	forwarderLogger.Info("Forwarder started with %d source relays", len(f.sourceClients))
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

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
