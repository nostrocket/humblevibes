package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Bech32 character set for encoding
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// convertBits performs the conversion from one power-of-2 number base to another
func convertBits(data []byte, fromBits, toBits uint8, pad bool) []byte {
	acc := uint32(0)
	bits := uint8(0)
	ret := make([]byte, 0, len(data)*int(fromBits)/int(toBits)+1)
	maxv := uint32((1 << toBits) - 1)
	maxAcc := uint32((1 << (fromBits + toBits - 1)) - 1)

	for _, b := range data {
		acc = ((acc << fromBits) | uint32(b)) & maxAcc
		bits += fromBits

		for bits >= toBits {
			bits -= toBits
			ret = append(ret, byte((acc>>bits)&maxv))
		}
	}

	if pad && bits > 0 {
		ret = append(ret, byte((acc<<(toBits-bits))&maxv))
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		// We have too many partial bits or the leftover bits are not zero
		// (for standards compliant encodings)
		// We ignore this as it doesn't affect decoding
	}

	return ret
}

// polymod calculates the checksum for bech32 encoding
func polymod(values []byte) uint32 {
	chk := uint32(1)
	for _, v := range values {
		b := uint32(chk >> 25)
		chk = ((chk & 0x1ffffff) << 5) ^ uint32(v)
		for i := 0; i < 5; i++ {
			if ((b >> i) & 1) == 1 {
				chk ^= 0x3b6a57b2 << uint(i*5)
			}
		}
	}
	return chk
}

// hrpExpand expands the human-readable part for use in checksum calculation
func hrpExpand(hrp string) []byte {
	result := make([]byte, len(hrp)*2+1)
	for i, c := range hrp {
		result[i] = byte(c >> 5)
		result[i+len(hrp)+1] = byte(c & 31)
	}
	return result
}

// bech32Encode encodes data to bech32 format with a human-readable prefix
func bech32Encode(hrp string, data []byte) string {
	// Convert data to 5-bit groups
	converted := convertBits(data, 8, 5, true)

	// Calculate checksum
	values := make([]byte, 0, len(hrp)+len(converted)+7)
	values = append(values, hrpExpand(hrp)...)
	values = append(values, 0)
	values = append(values, converted...)
	
	// Add 6 checksum bytes
	for i := 0; i < 6; i++ {
		values = append(values, 0)
	}
	
	mod := polymod(values) ^ 1
	checksum := make([]byte, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = byte((mod >> (5 * (5 - i))) & 31)
	}
	
	// Build the final string
	result := hrp + "1"
	for _, b := range converted {
		result += string(charset[b])
	}
	for _, b := range checksum {
		result += string(charset[b])
	}
	
	return result
}

// EncodeBech32 encodes a hex string to bech32 format with the given prefix
func EncodeBech32(prefix, hexStr string) (string, error) {
	// Decode hex string
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("invalid hex string: %v", err)
	}
	
	return bech32Encode(prefix, data), nil
}

// ConvertHexToNote converts a hex event ID to note format
func ConvertHexToNote(eventIDHex string) (string, error) {
	// Clean up the hex string
	eventIDHex = strings.TrimPrefix(eventIDHex, "0x")
	
	// Convert to note format
	return EncodeBech32("note", eventIDHex)
}

// ConvertHexToNpub converts a hex public key to npub format
func ConvertHexToNpub(pubkeyHex string) (string, error) {
	// Clean up the hex string
	pubkeyHex = strings.TrimPrefix(pubkeyHex, "0x")
	
	// Convert to npub format
	return EncodeBech32("npub", pubkeyHex)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  idconvert note <event_id_hex>")
		fmt.Println("  idconvert npub <pubkey_hex>")
		os.Exit(1)
	}

	operation := os.Args[1]
	hexStr := os.Args[2]

	var result string
	var err error

	switch operation {
	case "note":
		result, err = ConvertHexToNote(hexStr)
	case "npub":
		result, err = ConvertHexToNpub(hexStr)
	default:
		fmt.Printf("Unknown operation: %s\n", operation)
		fmt.Println("Supported operations: note, npub")
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
