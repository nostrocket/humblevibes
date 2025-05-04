package main

import (
	"context"
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
	KeepAlive    bool // Whether to use ping/pong to keep connections alive
	PingInterval time.Duration // Interval between pings (for keep-alive)
}

// ReverseForwarder handles subscribing to a source relay and forwarding events to multiple target relays
type ReverseForwarder struct {
	config       Config
	sourceClient *client.NostrClient
	targetClients []*client.NostrClient
	events       chan *client.Event
	wg           sync.WaitGroup
	stopChan     chan struct{}
	rateLimiters []*time.Ticker
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
	// First, discover target relays if needed
	if len(rf.config.TargetRelays) == 0 {
		logger.Info("No target relays specified, discovering relays from nostr.watch...")
		discoveredRelays, err := fetchRelaysFromNostrWatch(rf.config.MaxRelays)
		if err != nil {
			return fmt.Errorf("failed to discover target relays: %v", err)
		}
		rf.config.TargetRelays = discoveredRelays
	}

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

	// Now connect to all target relays concurrently
	var targetMutex sync.Mutex
	var connectWg sync.WaitGroup
	
	// Create a channel for clients as they're connected
	relayCount := len(rf.config.TargetRelays)
	connectWg.Add(relayCount)
	
	logger.Info("Connecting to %d target relays concurrently...", relayCount)
	
	for _, relayURL := range rf.config.TargetRelays {
		// Launch a goroutine for each relay connection
		go func(url string) {
			defer connectWg.Done()
			
			logger.Info("Connecting to target relay: %s", url)
			targetClient, err := client.NewNostrClient(url)
			if err != nil {
				logger.Warn("Failed to connect to target relay %s: %v", url, err)
				return
			}
			
			// Safely append to the slice of target clients
			targetMutex.Lock()
			rf.targetClients = append(rf.targetClients, targetClient)
			// Create a rate limiter for this relay (1 event per second)
			rf.rateLimiters = append(rf.rateLimiters, time.NewTicker(1*time.Second))
			targetMutex.Unlock()
			
			logger.Info("Successfully connected to target relay: %s", url)
		}(relayURL)
	}
	
	// Wait for all connection attempts to complete
	connectWg.Wait()
	
	if len(rf.targetClients) == 0 {
		rf.sourceClient.Close()
		return fmt.Errorf("failed to connect to any target relays")
	}
	
	logger.Info("Connected to %d target relays", len(rf.targetClients))

	// Start the keep-alive pings for source and target relays if enabled
	if rf.config.KeepAlive {
		// Start a goroutine to send periodic pings to the source relay
		rf.wg.Add(1)
		go rf.keepConnectionsAlive()
	}

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
	
	// Stop the keep-alive mechanism
	if rf.config.KeepAlive {
		logger.Info("Stopping keep-alive mechanism")
		close(rf.stopChan)
	}
	
	// Close the source client
	if rf.sourceClient != nil {
		rf.sourceClient.Close()
	}
	
	// Close all target clients
	for _, client := range rf.targetClients {
		client.Close()
	}
	
	// Stop all rate limiters
	for _, rateLimiter := range rf.rateLimiters {
		rateLimiter.Stop()
	}
	
	// Wait for all goroutines to finish
	rf.wg.Wait()
	
	logger.Info("Reverse forwarder stopped")
}

// subscribeToSourceRelay subscribes to events from the source relay
func (rf *ReverseForwarder) subscribeToSourceRelay() {
	defer rf.wg.Done()
	
	logger.Info("Subscribing to events from %s", rf.config.SourceRelay)
	
	// Add a count of events we've received and forwarded for logging
	eventCount := 0
	
	// Set up a more robust context-based approach with retry logic
	for {
		select {
		case <-rf.stopChan:
			logger.Info("Stopping source relay subscription")
			return
		default:
			// Subscribe to events with the user's specified filters
			logger.Info("Creating subscription with filters: %v", rf.config.Filters)
			
			// Create a subscription context that can be canceled
			ctx, cancel := context.WithCancel(context.Background())
			
			// Create a channel for subscription errors
			errorChan := make(chan error, 1)
			
			// Start a goroutine to monitor the connection
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
						// Send a ping to verify the connection
						err := rf.sourceClient.SendPing("ping_" + time.Now().Format(time.RFC3339))
						if err != nil {
							logger.Warn("Failed to send ping to source relay: %v", err)
							errorChan <- fmt.Errorf("ping failed: %w", err)
							return
						}
					}
				}
			}()
			
			// Start a goroutine to handle subscription notifications
			go func() {
				// Generate a subscription ID with timestamp
				subID := fmt.Sprintf("sub_%d", time.Now().Unix())
				
				// Set up a channel to detect when the context is done
				ctxDone := make(chan struct{})
				
				// Monitor the context in a separate goroutine
				go func() {
					<-ctx.Done()
					close(ctxDone)
				}()
				
				// Use the existing method but wrap it with our own context handling
				err := rf.sourceClient.SubscribeToEventsWithHandler(subID, rf.config.Filters, func(event *client.Event) {
					// Check if we should skip old events
					if rf.config.SkipOld && time.Now().Unix()-event.CreatedAt > 86400 {
						return
					}
					
					// Increment event counter
					eventCount++
					
					// Log or print the event if requested
					if rf.config.LogEvents {
						logger.Info("Received event %s from %s (#%d)", truncateString(event.ID, 8), rf.config.SourceRelay, eventCount)
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
						cancel()
						return
					}
				})
				
				// This point is reached when the subscription ends
				select {
				case <-ctxDone:
					// Context was canceled, no need to report error
					return
				default:
					// Subscription ended for some other reason
					if err != nil {
						errorChan <- err
					} else {
						// If the subscription ended without an error, it still means the connection was closed
						errorChan <- fmt.Errorf("subscription ended")
					}
				}
			}()
			
			// Wait for either a stop signal or an error
			select {
			case <-rf.stopChan:
				// The forwarder is stopping
				cancel()
				return
				
			case err := <-errorChan:
				// The subscription ended with an error
				cancel()
				
				if err != nil {
					logger.Warn("Source relay connection closed: %v", err)
				} else {
					logger.Warn("Source relay connection closed")
				}
				
				// Wait before attempting to reconnect
				select {
				case <-rf.stopChan:
					return
				case <-time.After(5 * time.Second):
					logger.Warn("Attempting to reconnect to source relay...")
					
					// Close the old connection
					if rf.sourceClient != nil {
						rf.sourceClient.Close()
					}
					
					// Try to establish a new connection
					newClient, err := client.NewNostrClient(rf.config.SourceRelay)
					if err != nil {
						logger.Error("Failed to reconnect to source relay: %v", err)
						// Try again in the next loop iteration
						continue
					}
					
					// Update the client
					rf.sourceClient = newClient
					logger.Info("Successfully reconnected to source relay")
				}
			}
		}
	}
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
	
	// Count successful forwards
	successCount := 0
	var countMutex sync.Mutex
	
	// Forward to each target relay in parallel
	for i, targetClient := range rf.targetClients {
		wg.Add(1)
		go func(idx int, clientInstance *client.NostrClient, eventBatch []*client.Event) {
			defer wg.Done()
			
			// Track failures for this relay
			failures := 0
			localSuccess := 0
			
			for _, event := range eventBatch {
				// Wait for the rate limiter ticker to ensure we don't exceed one event per second
				if idx < len(rf.rateLimiters) {
					<-rf.rateLimiters[idx].C
				} else {
					// Fallback sleep if rate limiter isn't available for some reason
					time.Sleep(1 * time.Second)
				}
				
				success, message, err := clientInstance.PublishExistingEvent(event)
				
				if err != nil {
					failures++
					logger.Error("Failed to publish event %s to relay: %v", truncateString(event.ID, 8), err)
				} else if !success {
					failures++
					logger.Warn("Relay rejected event %s: %s", truncateString(event.ID, 8), message)
				} else {
					localSuccess++
					if rf.config.LogEvents {
						logger.Info("Successfully forwarded event %s", truncateString(event.ID, 8))
					}
				}
				
				// If we have too many consecutive failures, try to reconnect
				if failures >= 5 {
					logger.Warn("Too many failures with relay %d, attempting to reconnect...", idx)
					
					// Close the old connection
					clientInstance.Close()
					
					// Try to reconnect using the package function (not the client instance)
					newClient, err := client.NewNostrClient(rf.config.TargetRelays[idx])
					if err != nil {
						logger.Error("Failed to reconnect to target relay %d: %v", idx, err)
						return
					}
					
					// Update the client
					rf.targetClients[idx] = newClient
					clientInstance = newClient
					logger.Info("Successfully reconnected to target relay %d", idx)
					
					// Reset failure counter
					failures = 0
				}
			}
			
			// Add successful forwards to the counter
			countMutex.Lock()
			successCount += localSuccess
			countMutex.Unlock()
		}(i, targetClient, events)
	}
	
	// Wait for all forwards to complete
	wg.Wait()
	
	logger.Info("Forwarded %d events to %d relays (%d successful publishes)", len(events), len(rf.targetClients), successCount)
}

// keepConnectionsAlive sends periodic pings to keep WebSocket connections active
func (rf *ReverseForwarder) keepConnectionsAlive() {
	defer rf.wg.Done()
	
	logger.Info("Starting keep-alive mechanism with ping interval: %v", rf.config.PingInterval)
	
	ticker := time.NewTicker(rf.config.PingInterval)
	defer ticker.Stop()
	
	// Counter for ping messages to make each one unique
	pingCounter := 0
	
	for {
		select {
		case <-ticker.C:
			// Increment counter
			pingCounter++
			
			// Create a ping message (REQ with no filters)
			// This keeps the connection active without actually requesting data
			pingID := fmt.Sprintf("ping_%d", pingCounter)
			
			// Try to send a ping message to the source relay
			if rf.sourceClient != nil {
				// Send a dummy REQ that won't match anything (limit:0)
				err := rf.sourceClient.SendPing(pingID)
				if err != nil {
					logger.Warn("Failed to send ping to source relay: %v", err)
				} else {
					logger.Debug("Sent keep-alive ping to source relay: %s", pingID)
				}
			}
			
			// Try to send a ping message to each target relay
			for i, client := range rf.targetClients {
				if client != nil {
					// Use a unique ID for each target
					targetPingID := fmt.Sprintf("%s_target%d", pingID, i)
					err := client.SendPing(targetPingID)
					if err != nil {
						logger.Warn("Failed to send ping to target relay %d: %v", i, err)
					} else {
						logger.Debug("Sent keep-alive ping to target relay %d: %s", i, targetPingID)
					}
				}
			}
			
		case <-rf.stopChan:
			logger.Info("Stopping keep-alive mechanism")
			return
		}
	}
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
	
	// Create channels to receive the validation result
	validationChan := make(chan error, 1)
	successChan := make(chan struct{}, 1)
	
	// Generate a test subscription ID
	testSubID := fmt.Sprintf("test_sub_%d", time.Now().Unix())
	
	// Create a very specific filter for validation - only request one random event
	testFilter := map[string]interface{}{
		"limit": 1,
		// Add a random dummy author to make sure we don't consume all events
		// This won't match any real events but will test the subscription mechanism
		"authors": []string{"0000000000000000000000000000000000000000000000000000000000000000"},
	}
	
	// Start the subscription using the NostrClient's exported method with EOSE callback
	go func() {
		logger.Info("Sending validation subscription to source relay...")
		
		// Define a handler for events (unlikely to receive any with our filter, but just in case)
		eventHandler := func(event *client.Event) {
			logger.Info("Received event during validation: %s", truncateString(event.ID, 8))
			select {
			case successChan <- struct{}{}:
			default:
			}
		}
		
		// Define an EOSE callback to signal successful validation when we get an EOSE
		eoseCallback := func() {
			logger.Info("Received EOSE during validation - connection validated")
			// Signal validation success
			select {
			case successChan <- struct{}{}:
			default:
			}
		}
		
		// Subscribe with both event handler and EOSE callback
		err := rf.sourceClient.SubscribeToEventsWithHandlerAndEOSE(
			testSubID, 
			testFilter, 
			eventHandler, 
			eoseCallback,
		)
		
		if err != nil {
			validationChan <- fmt.Errorf("subscription error: %v", err)
			return
		}
	}()
	
	// Wait for either a successful subscription response, an error, or a timeout
	select {
	case <-successChan:
		// We received either an event or an EOSE, which means the subscription is working
		logger.Info("Source relay validation successful")
		return nil
		
	case err := <-validationChan:
		// An error occurred during validation
		return err
		
	case <-time.After(5 * time.Second):
		// Timed out waiting for a response
		return fmt.Errorf("timed out waiting for subscription response from relay")
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
	keepAlive := flag.Bool("keep-alive", true, "Use ping mechanism to keep connections alive")
	pingInterval := flag.Int("ping-interval", 20, "Seconds between ping messages for keep-alive (default: 20)")
	
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
		KeepAlive:    *keepAlive,
		PingInterval: time.Duration(*pingInterval) * time.Second,
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
