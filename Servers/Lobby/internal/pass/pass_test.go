// Tests of the pass check (PROTOCOL.md 4.3 and 5.2) with passes signed by a
// key pair made in the test.

package pass

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"servers-lobby/internal/config"
)

const (
	testRegion  = 1
	testTag     = "eu"
	testExeCRC  = 0x11223344
	testIniCRC  = 0x55667788
	otherRegion = 2
)

// testNow is the service clock of the tests.
var testNow = time.Unix(1_800_000_000, 0)

// testKeys makes a key pair for a test.
func testKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, secret, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	return public, secret
}

// goodFields are the fields of a pass that is valid at testNow.
func goodFields() Fields {
	return Fields{
		Version:   Version,
		Region:    testRegion,
		ExeCRC:    testExeCRC,
		IniCRC:    testIniCRC,
		IssuedAt:  uint32(testNow.Unix()) - 10,
		ExpiresAt: uint32(testNow.Unix()) + 590,
	}
}

// testVerifier knows region 1, tag eu, and returns it with the key that signs for it.
func testVerifier(t *testing.T) (*Verifier, ed25519.PrivateKey) {
	t.Helper()
	public, secret := testKeys(t)
	verifier := NewVerifier()
	verifier.Add(testRegion, testTag, public)
	return verifier, secret
}

func TestGoodPassIsAcceptedAndNamesItsRegionTag(t *testing.T) {
	verifier, secret := testVerifier(t)

	tag, ok := verifier.Check(Sign(goodFields(), secret), testNow, testExeCRC, testIniCRC)

	if !ok || tag != testTag {
		t.Errorf("Check = %q, %v; want %q, true", tag, ok, testTag)
	}
}

func TestPassRefusals(t *testing.T) {
	verifier, secret := testVerifier(t)
	_, strangerSecret := testKeys(t)
	good := Sign(goodFields(), secret)

	changed := func(change func(*Fields)) string {
		fields := goodFields()
		change(&fields)
		return Sign(fields, secret)
	}
	lastDigit := good[len(good)-1]
	otherDigit := "0"
	if lastDigit == '0' {
		otherDigit = "1"
	}

	cases := []struct {
		name   string
		header string
	}{
		{"signed by another key", Sign(goodFields(), strangerSecret)},
		{"signature changed", good[:len(good)-1] + otherDigit},
		{"expired", changed(func(f *Fields) { f.ExpiresAt = uint32(testNow.Unix()) })},
		{"expired long ago", changed(func(f *Fields) { f.ExpiresAt = 5 })},
		{"region the service does not know", changed(func(f *Fields) { f.Region = otherRegion })},
		{"region zero", changed(func(f *Fields) { f.Region = 0 })},
		{"version 2", changed(func(f *Fields) { f.Version = 2 })},
		{"version 0", changed(func(f *Fields) { f.Version = 0 })},
		{"another exe fingerprint", changed(func(f *Fields) { f.ExeCRC++ })},
		{"another ini fingerprint", changed(func(f *Fields) { f.IniCRC++ })},
		{"empty", ""},
		{"one digit short", good[:HexLength-1]},
		{"one digit long", good + "0"},
		{"two passes", good + good},
		{"upper case", strings.ToUpper(good)},
		{"non-hexadecimal digit", "g" + good[1:]},
		{"white space around", " " + good[1:HexLength-1] + " "},
		{"a newline at the end", good[:HexLength-1] + "\n"},
		{"prefix 0x", "0x" + good[2:]},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if tag, ok := verifier.Check(test.header, testNow, testExeCRC, testIniCRC); ok {
				t.Errorf("Check accepted the pass and gave tag %q", tag)
			}
		})
	}
}

func TestPassExpiresAtTheSecondOfExpiry(t *testing.T) {
	verifier, secret := testVerifier(t)
	header := Sign(goodFields(), secret)
	last := time.Unix(int64(goodFields().ExpiresAt)-1, 0)
	first := time.Unix(int64(goodFields().ExpiresAt), 0)

	if _, ok := verifier.Check(header, last, testExeCRC, testIniCRC); !ok {
		t.Error("a pass one second before expiry was refused")
	}
	if _, ok := verifier.Check(header, first, testExeCRC, testIniCRC); ok {
		t.Error("a pass at its expiry second was accepted")
	}
}

func TestEachRegionIsCheckedWithItsOwnKey(t *testing.T) {
	euPublic, euSecret := testKeys(t)
	usPublic, usSecret := testKeys(t)
	verifier := NewVerifier()
	verifier.Add(testRegion, "eu", euPublic)
	verifier.Add(otherRegion, "us", usPublic)
	usFields := goodFields()
	usFields.Region = otherRegion

	if tag, ok := verifier.Check(Sign(usFields, usSecret), testNow, testExeCRC, testIniCRC); !ok || tag != "us" {
		t.Errorf("a pass of region 2 gave %q, %v; want us, true", tag, ok)
	}
	if _, ok := verifier.Check(Sign(usFields, euSecret), testNow, testExeCRC, testIniCRC); ok {
		t.Error("a pass of region 2 signed with the key of region 1 was accepted")
	}
	if tag, ok := verifier.Check(Sign(goodFields(), euSecret), testNow, testExeCRC, testIniCRC); !ok || tag != "eu" {
		t.Errorf("a pass of region 1 gave %q, %v; want eu, true", tag, ok)
	}
}

func TestMissingVerifierRefusesEveryPass(t *testing.T) {
	_, secret := testKeys(t)
	var verifier *Verifier

	if _, ok := verifier.Check(Sign(goodFields(), secret), testNow, testExeCRC, testIniCRC); ok {
		t.Error("a nil Verifier accepted a pass")
	}
}

// writeKeyFile writes text as a file in a new directory and returns its path.
func writeKeyFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "region.key.pub")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}

func TestLoadReadsAPublicKeyFileAsTheRelayWritesIt(t *testing.T) {
	public, secret := testKeys(t)
	path := writeKeyFile(t, hex.EncodeToString(public)+"\n")

	verifier, err := Load([]config.Region{{Number: testRegion, Tag: testTag, PublicKeyPath: path}})

	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if tag, ok := verifier.Check(Sign(goodFields(), secret), testNow, testExeCRC, testIniCRC); !ok || tag != testTag {
		t.Errorf("Check = %q, %v; want %q, true", tag, ok, testTag)
	}
}

func TestLoadRefusesAKeyFileThatCannotBeUsed(t *testing.T) {
	public, _ := testKeys(t)
	valid := hex.EncodeToString(public)
	cases := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(t.TempDir(), "none.pub")},
		{"empty file", writeKeyFile(t, "")},
		{"too short", writeKeyFile(t, valid[:62]+"\n")},
		{"too long", writeKeyFile(t, valid+"00\n")},
		{"not hexadecimal", writeKeyFile(t, "zz"+valid[2:]+"\n")},
		{"far too large", writeKeyFile(t, strings.Repeat("a", maxKeyFileBytes+1))},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load([]config.Region{{Number: testRegion, Tag: testTag, PublicKeyPath: test.path}})
			if err == nil {
				t.Fatal("Load returned no error")
			}
			if !strings.Contains(err.Error(), "region 1") {
				t.Errorf("error %q does not name the region", err)
			}
		})
	}
}

func TestLoadRefusesARegionNumberThePassCannotHold(t *testing.T) {
	public, _ := testKeys(t)
	path := writeKeyFile(t, hex.EncodeToString(public))

	if _, err := Load([]config.Region{{Number: 65536, Tag: testTag, PublicKeyPath: path}}); err == nil {
		t.Error("Load accepted region number 65536")
	}
}

func TestLoadSigningKeyReadsASecretKeyFile(t *testing.T) {
	public, secret := testKeys(t)
	path := writeKeyFile(t, hex.EncodeToString(secret)+"\n")

	loaded, err := LoadSigningKey(path)

	if err != nil {
		t.Fatalf("LoadSigningKey returned error: %v", err)
	}
	if !loaded.Public().(ed25519.PublicKey).Equal(public) {
		t.Error("the loaded key belongs to another public key")
	}
	if _, err := LoadSigningKey(writeKeyFile(t, "abcd")); err == nil {
		t.Error("LoadSigningKey accepted a short file")
	}
}
