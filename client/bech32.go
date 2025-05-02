package client

import (
	"encoding/hex"
	"errors"
	"strings"
)

// Bech32 character set for encoding
const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// Bech32 decode errors
var (
	ErrInvalidLength      = errors.New("invalid bech32 string length")
	ErrInvalidCharacter   = errors.New("invalid character in bech32 string")
	ErrInvalidChecksum    = errors.New("invalid bech32 checksum")
	ErrInvalidHRP         = errors.New("invalid human-readable part")
	ErrInvalidSeparator   = errors.New("invalid separator")
	ErrUnsupportedPrefix  = errors.New("unsupported bech32 prefix")
	ErrInvalidDataLength  = errors.New("invalid data length")
	ErrInvalidPubkeyHex   = errors.New("invalid public key hex string")
)

// DecodeBech32 decodes a bech32 encoded string and returns the human-readable part and the data part
func DecodeBech32(bech string) (string, []byte, error) {
	// The maximum length of a bech32 string
	if len(bech) > 90 {
		return "", nil, ErrInvalidLength
	}

	// Check for mixed case
	lower := strings.ToLower(bech)
	upper := strings.ToUpper(bech)
	if bech != lower && bech != upper {
		return "", nil, ErrInvalidCharacter
	}

	// Normalize to lowercase
	bech = lower

	// Find the last '1' separator
	pos := strings.LastIndexByte(bech, '1')
	if pos < 1 || pos+7 > len(bech) {
		return "", nil, ErrInvalidSeparator
	}

	// Extract the human-readable part (hrp) and data part
	hrp := bech[:pos]
	data := bech[pos+1:]

	// Check for invalid characters
	for i := 0; i < len(data); i++ {
		if strings.IndexByte(charset, data[i]) == -1 {
			return "", nil, ErrInvalidCharacter
		}
	}

	// Convert from bech32 to 5-bit data
	decoded := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		decoded = append(decoded, byte(strings.IndexByte(charset, data[i])))
	}

	// Convert from 5-bit to 8-bit data (skip checksum)
	converted := convertBits(decoded[:len(decoded)-6], 5, 8, false)
	if converted == nil {
		return "", nil, ErrInvalidDataLength
	}

	return hrp, converted, nil
}

// ConvertBech32PubkeyToHex converts a bech32 public key (npub) to hex format
func ConvertBech32PubkeyToHex(pubkey string) (string, error) {
	// Check if already in hex format
	if len(pubkey) == 64 {
		// Try to decode as hex to validate
		_, err := hex.DecodeString(pubkey)
		if err == nil {
			return pubkey, nil
		}
	}

	// Handle npub prefix
	if !strings.HasPrefix(pubkey, "npub1") {
		return "", ErrUnsupportedPrefix
	}

	// Decode bech32
	hrp, data, err := DecodeBech32(pubkey)
	if err != nil {
		return "", err
	}

	// Verify HRP
	if hrp != "npub" {
		return "", ErrInvalidHRP
	}

	// Convert to hex
	return hex.EncodeToString(data), nil
}

// convertBits performs the conversion from one power-of-2 number base to another
func convertBits(data []byte, fromBits, toBits uint8, pad bool) []byte {
	if fromBits < 1 || fromBits > 8 || toBits < 1 || toBits > 8 {
		return nil
	}

	// Calculate the number of bits in the input data
	// and determine the maximum value in the output base
	maxValue := (1 << toBits) - 1

	// Calculate the size of the output array
	size := len(data)*int(fromBits)/int(toBits)
	if pad && len(data)*int(fromBits)%int(toBits) != 0 {
		size++
	}

	// Create the output array
	result := make([]byte, 0, size)
	
	// Initialize the accumulator
	acc := uint32(0)
	bits := uint8(0)
	
	// Process each input byte
	for _, b := range data {
		// Shift the accumulator and add the input byte
		acc = (acc << fromBits) | uint32(b)
		bits += fromBits
		
		// While we have enough bits to create an output byte
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((acc>>bits)&uint32(maxValue)))
		}
	}
	
	// If we need to pad the output
	if pad && bits > 0 {
		result = append(result, byte((acc<<(toBits-bits))&uint32(maxValue)))
	}
	
	return result
}
