package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DBStats holds statistics about the database
type DBStats struct {
	EventCount     int
	AuthorCount    int
	KindCounts     map[int]int
	LatestEventID  string
	LatestContent  string
	LatestAuthor   string
	LatestCreated  int64
	LastUpdateTime time.Time
}

func main() {
	// Parse command line flags
	dbPath := flag.String("db", "nostr.db", "Path to SQLite database")
	interval := flag.Int("interval", 10, "Interval in seconds between checks")
	flag.Parse()

	// Open SQLite database
	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Check if the database connection is valid
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Printf("Monitoring Nostr database at %s (checking every %d seconds)\n", *dbPath, *interval)
	fmt.Println("Press Ctrl+C to stop monitoring")

	// Get initial stats
	var lastStats DBStats
	lastStats, err = getDBStats(db)
	if err != nil {
		log.Fatalf("Failed to get initial stats: %v", err)
	}

	// Print initial stats
	printDBStats(lastStats, true)

	// Monitor for changes
	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		currentStats, err := getDBStats(db)
		if err != nil {
			log.Printf("Error getting stats: %v", err)
			continue
		}

		// Check if anything has changed
		if hasChanged(lastStats, currentStats) {
			printDBStats(currentStats, false)
			lastStats = currentStats
		}
	}
}

// getDBStats retrieves statistics from the database
func getDBStats(db *sql.DB) (DBStats, error) {
	stats := DBStats{
		KindCounts:     make(map[int]int),
		LastUpdateTime: time.Now(),
	}

	// Get total event count
	err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&stats.EventCount)
	if err != nil {
		return stats, fmt.Errorf("failed to get event count: %v", err)
	}

	// Get unique author count
	err = db.QueryRow("SELECT COUNT(DISTINCT pubkey) FROM events").Scan(&stats.AuthorCount)
	if err != nil {
		return stats, fmt.Errorf("failed to get author count: %v", err)
	}

	// Get counts by kind
	rows, err := db.Query("SELECT kind, COUNT(*) FROM events GROUP BY kind")
	if err != nil {
		return stats, fmt.Errorf("failed to get kind counts: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind, count int
		if err := rows.Scan(&kind, &count); err != nil {
			return stats, fmt.Errorf("failed to scan kind count: %v", err)
		}
		stats.KindCounts[kind] = count
	}

	// Get latest event
	row := db.QueryRow(`
		SELECT id, pubkey, created_at, content 
		FROM events 
		ORDER BY created_at DESC 
		LIMIT 1
	`)

	err = row.Scan(
		&stats.LatestEventID,
		&stats.LatestAuthor,
		&stats.LatestCreated,
		&stats.LatestContent,
	)
	if err != nil && err != sql.ErrNoRows {
		return stats, fmt.Errorf("failed to get latest event: %v", err)
	}

	return stats, nil
}

// hasChanged checks if the database stats have changed
func hasChanged(old, new DBStats) bool {
	if old.EventCount != new.EventCount {
		return true
	}
	if old.AuthorCount != new.AuthorCount {
		return true
	}
	if old.LatestEventID != new.LatestEventID {
		return true
	}
	
	// Check if kind counts have changed
	if len(old.KindCounts) != len(new.KindCounts) {
		return true
	}
	for kind, count := range old.KindCounts {
		if new.KindCounts[kind] != count {
			return true
		}
	}
	
	return false
}

// printDBStats prints the database statistics
func printDBStats(stats DBStats, isInitial bool) {
	var header string
	if isInitial {
		header = "INITIAL DATABASE STATE"
	} else {
		header = "DATABASE CHANGED"
	}
	
	fmt.Printf("\n=== %s AT %s ===\n", header, time.Now().Format(time.RFC3339))
	fmt.Printf("Total Events: %d\n", stats.EventCount)
	fmt.Printf("Unique Authors: %d\n", stats.AuthorCount)
	
	fmt.Println("Events by Kind:")
	for kind, count := range stats.KindCounts {
		var kindName string
		switch kind {
		case 0:
			kindName = "Metadata"
		case 1:
			kindName = "Text Note"
		case 2:
			kindName = "Recommend Relay"
		case 3:
			kindName = "Contacts"
		case 4:
			kindName = "Encrypted Direct Message"
		case 5:
			kindName = "Event Deletion"
		case 6:
			kindName = "Repost"
		case 7:
			kindName = "Reaction"
		default:
			kindName = fmt.Sprintf("Kind %d", kind)
		}
		fmt.Printf("  - %s: %d\n", kindName, count)
	}
	
	if stats.LatestEventID != "" {
		fmt.Println("\nLatest Event:")
		fmt.Printf("  ID: %s\n", stats.LatestEventID)
		fmt.Printf("  Author: %s\n", stats.LatestAuthor)
		fmt.Printf("  Created: %s\n", time.Unix(stats.LatestCreated, 0).Format(time.RFC3339))
		
		// Truncate content if it's too long
		content := stats.LatestContent
		if len(content) > 100 {
			content = content[:97] + "..."
		}
		fmt.Printf("  Content: %s\n", content)
	} else {
		fmt.Println("\nNo events in database")
	}
	
	fmt.Println(strings.Repeat("-", 50))
}
