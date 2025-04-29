package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gareth/go-nostr-relay/client"
)

func main() {
	// Parse command line flags
	relayURL := flag.String("relay", "ws://localhost:8080/ws", "URL of the Nostr relay")
	privateKey := flag.String("key", "", "Private key in hex format (optional, will generate if not provided)")
	numEvents := flag.Int("num", 1, "Number of events to publish (default 1, 0 for interactive mode)")
	flag.Parse()

	var nostrClient *client.NostrClient
	var err error

	// Create a new Nostr client
	if *privateKey != "" {
		nostrClient, err = client.NewNostrClientWithKey(*relayURL, *privateKey)
	} else {
		nostrClient, err = client.NewNostrClient(*relayURL)
	}

	if err != nil {
		log.Fatalf("Failed to create Nostr client: %v", err)
	}
	defer nostrClient.Close()

	// Display the public key
	fmt.Printf("Using public key: %s\n", nostrClient.GetPublicKey())

	// Interactive mode
	if *numEvents == 0 {
		fmt.Println("Interactive mode. Enter text notes to publish (empty line to quit):")
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}
			content := scanner.Text()
			if content == "" {
				break
			}

			// Publish the note
			event, err := nostrClient.PublishTextNote(content)
			if err != nil {
				log.Printf("Failed to publish note: %v", err)
				continue
			}

			fmt.Printf("Published note with ID: %s\n", event.ID)
		}
	} else {
		// Publish a fixed number of events
		for i := 1; i <= *numEvents; i++ {
			content := fmt.Sprintf("Test note #%d at %s", i, time.Now().Format(time.RFC3339))
			
			// Publish the note
			event, err := nostrClient.PublishTextNote(content)
			if err != nil {
				log.Printf("Failed to publish note #%d: %v", i, err)
				continue
			}

			fmt.Printf("Published note #%d with ID: %s\n", i, event.ID)
			
			// Small delay between events
			if i < *numEvents {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}
