// Package pass checks the entry pass that a client sends as the HTTP header
// X-Servers-Pass when it creates or joins a server lobby (PROTOCOL.md 4.3 and
// 5.2, SERVICE.md section 10). The relay signs the pass with Ed25519 using the
// key of its region; the service holds the public key of each region.
package pass

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"servers-lobby/internal/config"
)

// Header is the name of the HTTP header that carries the pass (PROTOCOL.md 5.2).
const Header = "X-Servers-Pass"

// Layout of the pass (PROTOCOL.md 4.3). Integers are little-endian.
const (
	// Size is the length of a pass in bytes, and HexLength the length of its header value.
	Size      = 100
	HexLength = Size * 2

	signedSize      = 36
	versionOffset   = 0
	regionOffset    = 2
	exeCRCOffset    = 8
	iniCRCOffset    = 12
	issuedOffset    = 20
	expiresOffset   = 24
	signatureOffset = signedSize

	// Version is the only pass version the service knows.
	Version = 1
)

// maxKeyFileBytes is the largest public key file the service reads. The file
// written by the relay is 65 bytes.
const maxKeyFileBytes = 1024

// maxRegionNumber is the highest region number; the pass holds it in 16 bits (PROTOCOL.md 7.1).
const maxRegionNumber = 65535

// region is a configured region: its tag and the public key of its relay.
type region struct {
	tag string
	key ed25519.PublicKey
}

// Verifier checks passes against the configured regions. It is safe for
// concurrent use, because it is never changed after it is made.
type Verifier struct {
	regions map[uint16]region
}

// NewVerifier returns a Verifier that knows no region. Add gives it regions.
func NewVerifier() *Verifier {
	return &Verifier{regions: make(map[uint16]region)}
}

// Add makes the region number known, with its tag and public key.
func (verifier *Verifier) Add(number uint16, tag string, key ed25519.PublicKey) {
	verifier.regions[number] = region{tag: tag, key: key}
}

// Load reads the public key file of every configured region. It returns an
// error that names the region and the file when one cannot be read or does
// not hold a public key as `relay --generate-key` writes it: 64 hexadecimal
// digits of the 32-byte key, with white space around them.
func Load(regions []config.Region) (*Verifier, error) {
	verifier := NewVerifier()
	for _, configured := range regions {
		if configured.Number < 1 || configured.Number > maxRegionNumber {
			return nil, fmt.Errorf("region %d: the number must be 1 to %d", configured.Number, maxRegionNumber)
		}
		key, err := readPublicKey(configured.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("region %d: %w", configured.Number, err)
		}
		verifier.Add(uint16(configured.Number), configured.Tag, key)
	}
	return verifier, nil
}

// readPublicKey reads one public key file.
func readPublicKey(path string) (ed25519.PublicKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read public key file %q: %w", path, err)
	}
	if info.Size() > maxKeyFileBytes {
		return nil, fmt.Errorf("public key file %q is too large", path)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read public key file %q: %w", path, err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(text)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key file %q does not hold a public key", path)
	}
	return ed25519.PublicKey(key), nil
}

// Check judges the value of an X-Servers-Pass header at now by PROTOCOL.md 5.2.
// It accepts the pass when the value is 200 lowercase hexadecimal digits, the
// signature is valid for the key of the region the pass names, the version is
// 1, the pass has not expired, and its build fingerprints equal exeCRC and
// iniCRC. It returns the tag of the pass's region. Any other input, however
// malformed, is refused.
func (verifier *Verifier) Check(header string, now time.Time, exeCRC uint32, iniCRC uint32) (string, bool) {
	if verifier == nil {
		return "", false
	}
	data, ok := decodeLowerHex(header)
	if !ok {
		return "", false
	}
	known, found := verifier.regions[binary.LittleEndian.Uint16(data[regionOffset:])]
	if !found || !ed25519.Verify(known.key, data[:signedSize], data[signatureOffset:]) {
		return "", false
	}
	if data[versionOffset] != Version {
		return "", false
	}
	if now.Unix() >= int64(binary.LittleEndian.Uint32(data[expiresOffset:])) {
		return "", false
	}
	if binary.LittleEndian.Uint32(data[exeCRCOffset:]) != exeCRC || binary.LittleEndian.Uint32(data[iniCRCOffset:]) != iniCRC {
		return "", false
	}
	return known.tag, true
}

// decodeLowerHex decodes a pass header value. It returns false unless the
// value is exactly HexLength lowercase hexadecimal digits.
func decodeLowerHex(text string) ([]byte, bool) {
	if len(text) != HexLength {
		return nil, false
	}
	for index := 0; index < len(text); index++ {
		character := text[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return nil, false
		}
	}
	data, err := hex.DecodeString(text)
	if err != nil || len(data) != Size {
		return nil, false
	}
	return data, true
}
