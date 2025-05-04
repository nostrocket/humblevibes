package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/gorilla/websocket"
)

func main() {
	// Parse command line flags
	privateKeyHex := flag.String("key", "", "Private key in hex (optional, will generate if not provided)")
	relayURL := flag.String("relay", "ws://localhost:8080/ws", "Relay URL")
	message := flag.String("message", "Hello Nostr!", "Message content")
	flag.Parse()

	var privateKey *btcec.PrivateKey
	var err error

	// Either use provided private key or generate a new one
	if *privateKeyHex == "" {
		privateKey, err = btcec.NewPrivateKey()
		if err != nil {
			log.Fatalf("Failed to generate private key: %v", err)
		}
		log.Printf("Generated new private key: %x", privateKey.Serialize())
	} else {
		keyBytes, err := hex.DecodeString(*privateKeyHex)
		if err != nil {
			log.Fatalf("Invalid private key hex: %v", err)
		}
		privateKey, _ = btcec.PrivKeyFromBytes(keyBytes)
	}

	// Get the public key in hex format (without the 0x02 prefix byte)
	pubkey := hex.EncodeToString(privateKey.PubKey().SerializeCompressed()[1:])
	log.Printf("Using pubkey: %s", pubkey)

	// Connect to relay
	log.Printf("Connecting to relay: %s", *relayURL)
	c, _, err := websocket.DefaultDialer.Dial(*relayURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()
	log.Println("Connected to relay!")

	// Create a Nostr event
	createdAt := time.Now().Unix()
	
	event := map[string]interface{}{
		"pubkey":     pubkey,
		"created_at": createdAt,
		"kind":       1, // Text note
		"tags":       [][]string{
			{"t", "nostr"},
			{"t", "test"},
		},
		"content":    *message,
	}

	// Serialize for ID computation
	// [0, <pubkey>, <created_at>, <kind>, <tags>, <content>]
	serializable := []interface{}{
		0,
		event["pubkey"],
		event["created_at"],
		event["kind"],
		event["tags"],
		event["content"],
	}

	// Compute event ID
	serialized, err := json.Marshal(serializable)
	if err != nil {
		log.Fatalf("Failed to serialize event: %v", err)
	}
	
	hash := sha256.Sum256(serialized)
	id := hex.EncodeToString(hash[:])
	event["id"] = id
	
	// Sign the event
	sig, err := schnorr.Sign(privateKey, hash[:])
	if err != nil {
		log.Fatalf("Failed to sign event: %v", err)
	}
	
	event["sig"] = hex.EncodeToString(sig.Serialize())
	
	// Construct the EVENT message
	eventMsg := []interface{}{"EVENT", event}
	bytes, err := json.Marshal(eventMsg)
	if err != nil {
		log.Fatalf("Failed to marshal event message: %v", err)
	}
	
	// Send the event
	log.Printf("Sending event: %s", string(bytes))
	if err := c.WriteMessage(websocket.TextMessage, bytes); err != nil {
		log.Fatalf("Failed to send event: %v", err)
	}
	
	// Wait for OK response
	_, response, err := c.ReadMessage()
	if err != nil {
		log.Fatalf("Failed to receive response: %v", err)
	}
	
	log.Printf("Received response: %s", string(response))
	
	// Parse the OK message
	var msg []json.RawMessage
	if err := json.Unmarshal(response, &msg); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}
	
	// Check if it's an OK message
	var msgType string
	if err := json.Unmarshal(msg[0], &msgType); err != nil {
		log.Fatalf("Failed to parse message type: %v", err)
	}
	
	if msgType == "OK" {
		var eventID string
		var success bool
		var notice string
		
		if len(msg) >= 3 {
			json.Unmarshal(msg[1], &eventID)
			json.Unmarshal(msg[2], &success)
			
			if len(msg) >= 4 {
				json.Unmarshal(msg[3], &notice)
			}
			
			if success {
				log.Printf("Event accepted by relay: %s", eventID)
			} else {
				log.Printf("Event rejected by relay: %s - %s", eventID, notice)
			}
		}
	} else {
		log.Printf("Unexpected response type: %s", msgType)
	}
}
