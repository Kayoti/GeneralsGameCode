// This file builds and signs passes. The service never signs a pass; the
// relay does. This is for the check program and the tests, which stand in for
// the relay (SERVICE.md section 13).

package pass

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// secretKeyHexLength is the length of the key file written by `relay --generate-key`:
// the 64 bytes of the secret key as hexadecimal digits.
const secretKeyHexLength = ed25519.PrivateKeySize * 2

// Fields are the values of a pass that the service reads or that a test must set (PROTOCOL.md 4.3).
type Fields struct {
	Version   uint8
	Region    uint16
	ExeCRC    uint32
	IniCRC    uint32
	IssuedAt  uint32
	ExpiresAt uint32
}

// Sign encodes the fields as a 100-byte pass, signs it with the region's secret
// key and returns it as the 200 lowercase hexadecimal digits of the header.
func Sign(fields Fields, secretKey ed25519.PrivateKey) string {
	data := make([]byte, Size)
	data[versionOffset] = fields.Version
	binary.LittleEndian.PutUint16(data[regionOffset:], fields.Region)
	binary.LittleEndian.PutUint32(data[exeCRCOffset:], fields.ExeCRC)
	binary.LittleEndian.PutUint32(data[iniCRCOffset:], fields.IniCRC)
	binary.LittleEndian.PutUint32(data[issuedOffset:], fields.IssuedAt)
	binary.LittleEndian.PutUint32(data[expiresOffset:], fields.ExpiresAt)
	copy(data[signatureOffset:], ed25519.Sign(secretKey, data[:signedSize]))
	return hex.EncodeToString(data)
}

// LoadSigningKey reads a secret key file as `relay --generate-key` writes it.
func LoadSigningKey(path string) (ed25519.PrivateKey, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read key file %q: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(text))
	if len(trimmed) != secretKeyHexLength {
		return nil, fmt.Errorf("key file %q does not hold a secret key", path)
	}
	key, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("key file %q does not hold a secret key", path)
	}
	return ed25519.PrivateKey(key), nil
}
