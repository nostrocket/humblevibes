package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gareth/go-nostr-relay/relay"
)

func main() {
	// Parse command line flags
	port := flag.String("port", "8080", "Port to run the relay on")
	dbPath := flag.String("db", "nostr.db", "Path to SQLite database")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of received events")
	flag.BoolVar(verbose, "v", false, "Enable verbose logging of received events (shorthand)")
	timeout := flag.Int("timeout", 0, "WebSocket connection timeout in seconds (0 for no timeout)")
	flag.Parse()

	// Initialize the relay with options
	opts := []relay.Option{relay.WithVerboseLogging(*verbose)}
	
	// Always add timeout option, even when it's 0, to ensure proper configuration
	opts = append(opts, relay.WithConnectionTimeouts(time.Duration(*timeout)*time.Second))
	
	r, err := relay.NewRelay(*dbPath, opts...)
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
