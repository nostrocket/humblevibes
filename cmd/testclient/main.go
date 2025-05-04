package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// Set up logging
	log.SetPrefix("TEST CLIENT: ")
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("Starting test client")

	// Connect to relay
	url := "ws://localhost:8080/ws"
	log.Printf("Connecting to %s", url)
	
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()
	
	log.Println("Connected to relay!")
	
	// Set up interrupt handler
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	
	// Create subscription
	subscriptionID := fmt.Sprintf("sub_%d", time.Now().Unix())
	filter := map[string]interface{}{
		"kinds": []int{1},
		"limit": 100,
	}
	
	// Create REQ message
	reqMsg := []interface{}{"REQ", subscriptionID, filter}
	reqBytes, err := json.Marshal(reqMsg)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}
	
	log.Printf("Sending subscription request: %s", string(reqBytes))
	err = c.WriteMessage(websocket.TextMessage, reqBytes)
	if err != nil {
		log.Fatalf("Failed to send subscription: %v", err)
	}
	
	// Read messages
	done := make(chan struct{})
	
	go func() {
		defer close(done)
		
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}
			
			// Try to parse the message
			var msg []json.RawMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Failed to parse message: %v", err)
				log.Printf("Raw message: %s", string(message))
				continue
			}
			
			// Need at least message type
			if len(msg) < 1 {
				log.Printf("Invalid message format (too short): %s", string(message))
				continue
			}
			
			// Extract message type
			var msgType string
			if err := json.Unmarshal(msg[0], &msgType); err != nil {
				log.Printf("Failed to parse message type: %v", err)
				continue
			}
			
			switch msgType {
			case "EVENT":
				// Handle event message
				if len(msg) < 3 {
					log.Printf("Invalid EVENT message format: %s", string(message))
					continue
				}
				
				// Extract subscription ID
				var subID string
				if err := json.Unmarshal(msg[1], &subID); err != nil {
					log.Printf("Failed to parse subscription ID: %v", err)
					continue
				}
				
				// Extract event data
				var event map[string]interface{}
				if err := json.Unmarshal(msg[2], &event); err != nil {
					log.Printf("Failed to parse event data: %v", err)
					continue
				}
				
				// Pretty print the note
				fmt.Println("\n------ NOTE RECEIVED ------")
				fmt.Printf("Subscription: %s\n", subID)
				
				// Print basic metadata
				if id, ok := event["id"].(string); ok {
					fmt.Printf("ID: %s\n", id)
				}
				
				if pubkey, ok := event["pubkey"].(string); ok {
					fmt.Printf("Author: %s\n", pubkey)
				}
				
				if kind, ok := event["kind"].(float64); ok {
					fmt.Printf("Kind: %.0f\n", kind)
				}
				
				if createdAt, ok := event["created_at"].(float64); ok {
					timestamp := time.Unix(int64(createdAt), 0)
					fmt.Printf("Created: %s\n", timestamp.Format(time.RFC3339))
				}
				
				// Print content
				if content, ok := event["content"].(string); ok {
					fmt.Println("\nContent:")
					fmt.Printf("%s\n", content)
				}
				
				// Print tags if present
				if tags, ok := event["tags"].([]interface{}); ok && len(tags) > 0 {
					fmt.Println("\nTags:")
					for _, tag := range tags {
						if tagArray, ok := tag.([]interface{}); ok && len(tagArray) > 0 {
							// Print tag elements
							tagStr := "  "
							for i, elem := range tagArray {
								if i > 0 {
									tagStr += " "
								}
								if str, ok := elem.(string); ok {
									tagStr += str
								}
							}
							fmt.Println(tagStr)
						}
					}
				}
				fmt.Println("-------------------------\n")
				
			case "EOSE":
				if len(msg) >= 2 {
					var subID string
					if err := json.Unmarshal(msg[1], &subID); err == nil {
						log.Printf("End of stored events for subscription: %s", subID)
					} else {
						log.Printf("Received EOSE message: %s", string(message))
					}
				} else {
					log.Printf("Received EOSE message: %s", string(message))
				}
				
			case "OK":
				// Format: ["OK", <event_id>, <success>, <message>]
				if len(msg) >= 4 {
					var eventID string
					var success bool
					var messageText string
					
					json.Unmarshal(msg[1], &eventID)
					json.Unmarshal(msg[2], &success)
					json.Unmarshal(msg[3], &messageText)
					
					if success {
						log.Printf("Event accepted: %s", eventID)
					} else {
						log.Printf("Event rejected: %s - %s", eventID, messageText)
					}
				} else {
					log.Printf("Received OK message: %s", string(message))
				}
				
			case "NOTICE":
				if len(msg) >= 2 {
					var notice string
					if err := json.Unmarshal(msg[1], &notice); err == nil {
						log.Printf("Notice from relay: %s", notice)
					} else {
						log.Printf("Received NOTICE message: %s", string(message))
					}
				} else {
					log.Printf("Received NOTICE message: %s", string(message))
				}
				
			default:
				log.Printf("Received message: %s", string(message))
			}
		}
	}()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	// Keep connection alive for 5 minutes
	timeout := time.After(5 * time.Minute)
	
	for {
		select {
		case <-done:
			log.Println("Connection closed by server")
			return
		case <-ticker.C:
			log.Println("Connection is still alive after 30 seconds")
		case <-interrupt:
			log.Println("Interrupted by user, closing connection...")
			
			// Close the WebSocket connection properly
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Printf("Error sending close message: %v", err)
			}
			
			// Wait for the server to close the connection
			select {
			case <-done:
			case <-time.After(1 * time.Second):
			}
			return
		case <-timeout:
			log.Println("Test completed after 5 minutes")
			return
		}
	}
}
