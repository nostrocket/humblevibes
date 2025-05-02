package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gareth/go-nostr-relay/lib/encoding"
	_ "github.com/mattn/go-sqlite3"
)

// Event represents a minimal version of a Nostr event for content export
type Event struct {
	ID        string
	PubKey    string
	CreatedAt int64
	Content   string
}

func main() {
	// Parse command line flags
	dbPath := flag.String("db", "nostr.db", "Path to SQLite database")
	pubkey := flag.String("pubkey", "", "Public key to filter events by (required, supports both hex and bech32/npub format)")
	outputPath := flag.String("output", "", "Output file path (defaults to pubkey.txt)")
	sortOrder := flag.String("sort", "asc", "Sort order of events: 'asc' or 'desc' by creation time")
	separator := flag.String("separator", "\n\n", "Separator between event contents")
	flag.Parse()

	// Validate required parameters
	if *pubkey == "" {
		log.Fatal("Error: --pubkey is required")
	}

	// Convert pubkey from bech32/npub format to hex if needed
	hexPubkey := *pubkey
	if strings.HasPrefix(*pubkey, "npub1") {
		var err error
		hexPubkey, err = encoding.ConvertBech32PubkeyToHex(*pubkey)
		if err != nil {
			log.Fatalf("Error converting bech32 pubkey to hex: %v", err)
		}
		log.Printf("Converted npub to hex pubkey: %s", hexPubkey)
	}

	// Set default output path if not specified
	if *outputPath == "" {
		// Create a shortened version of the pubkey for the filename
		shortPubkey := hexPubkey
		if len(shortPubkey) > 12 {
			shortPubkey = shortPubkey[:12]
		}
		*outputPath = fmt.Sprintf("%s.txt", shortPubkey)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(*outputPath)
	if outputDir != "." && outputDir != "/" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Validate sort order
	*sortOrder = strings.ToLower(*sortOrder)
	if *sortOrder != "asc" && *sortOrder != "desc" {
		log.Fatal("Error: sort must be either 'asc' or 'desc'")
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Build query with appropriate sort order
	orderClause := "ASC"
	if *sortOrder == "desc" {
		orderClause = "DESC"
	}
	
	query := fmt.Sprintf(
		"SELECT id, pubkey, created_at, content FROM events WHERE pubkey = ? ORDER BY created_at %s",
		orderClause,
	)

	// Query events by pubkey
	log.Printf("Fetching events for pubkey: %s", hexPubkey)
	rows, err := db.Query(query, hexPubkey)
	if err != nil {
		log.Fatalf("Failed to query events: %v", err)
	}
	defer rows.Close()

	// Read all events
	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.PubKey, &event.CreatedAt, &event.Content); err != nil {
			log.Fatalf("Failed to read event: %v", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error during row iteration: %v", err)
	}

	// Check if any events were found
	if len(events) == 0 {
		log.Printf("No events found for pubkey: %s", *pubkey)
		return
	}

	log.Printf("Found %d events for pubkey: %s", len(events), *pubkey)

	// Create output file
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	// Write content of each event to the file
	var contentParts []string
	for _, event := range events {
		// Add event content if it's not empty
		if len(strings.TrimSpace(event.Content)) > 0 {
			contentParts = append(contentParts, event.Content)
		}
	}

	// Join all content parts with the specified separator
	fullContent := strings.Join(contentParts, *separator)
	
	// Write to file
	if _, err := outputFile.WriteString(fullContent); err != nil {
		log.Fatalf("Failed to write to output file: %v", err)
	}

	log.Printf("Successfully exported content to: %s", *outputPath)
	
	// Print some stats about the export
	contentSize := len(fullContent)
	t := time.Unix(events[len(events)-1].CreatedAt, 0)
	lastEventTime := t.Format("2006-01-02 15:04:05")
	
	log.Printf("Export statistics:")
	log.Printf("- Events processed: %d", len(events))
	log.Printf("- Content size: %d bytes", contentSize)
	log.Printf("- Latest event time: %s", lastEventTime)
}
