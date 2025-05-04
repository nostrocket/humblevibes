# Nostr ID Converter

A utility for converting between different Nostr ID formats.

## Overview

The ID converter tool provides easy conversion between different Nostr ID formats, including hex, bech32 (npub/nsec), and other encoding formats used in the Nostr ecosystem.

## Features

- Convert between hex and bech32 formats (npub, nsec, note, etc.)
- Validate ID formats and checksums
- Generate key pairs
- Extract public keys from private keys
- Support for NIP-19 encoding formats

## Usage

```
./idconvert [options] <id>
```

### Options

- `-from`: Source format (hex, npub, nsec, note)
- `-to`: Target format (hex, npub, nsec, note)
- `-generate`: Generate a new key pair
- `-extract`: Extract public key from private key

## Examples

### Convert hex to npub
```
./idconvert -from hex -to npub 3bf7c38639c09128ce5d95b3e0e8f8ade1a9f4d2d1534880302eec34c3f5a58e
```

### Convert npub to hex
```
./idconvert -from npub -to hex npub1...
```

### Generate a new key pair
```
./idconvert -generate
```

## Notes

- The tool implements NIP-19 bech32 encoding/decoding
- All conversions are done locally without any network requests
- Private keys (nsec format) should be handled with care
