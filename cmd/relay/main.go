package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gareth/go-nostr-relay/relay"
)

func main() {
	// Parse command line flags
	port := flag.String("port", "8080", "Port to run the relay on")
	dbPath := flag.String("db", "nostr.db", "Path to SQLite database")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of received events")
	flag.BoolVar(verbose, "v", false, "Enable verbose logging of received events (shorthand)")
	flag.Parse()

	// Initialize the relay
	r, err := relay.NewRelay(*dbPath, relay.WithVerboseLogging(*verbose))
	if err != nil {
		log.Fatalf("Failed to initialize relay: %v", err)
	}
	defer r.Close()

	// Setup HTTP server with WebSocket handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Nostr Relay is running. Connect to the WebSocket endpoint at /ws"))
	})
	http.HandleFunc("/ws", r.HandleWebSocket)

	// Start the server
	log.Printf("Starting Nostr relay on port %s", *port)
	server := &http.Server{
		Addr: ":" + *port,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down relay...")
		server.Close()
	}()

	// Start the server
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
	log.Println("Relay shutdown complete")
}
