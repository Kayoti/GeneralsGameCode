// Tests of the display name rules of SERVICE.md section 4.

package accounts

import (
	"errors"
	"strings"
	"testing"
)

func TestChangeDisplayNameAcceptsValidNames(t *testing.T) {
	service, _ := newTestService(t, 0)
	login := loginWithNewAccount(t, service, loopbackAddress)

	for _, name := range []string{"abc", "Brave Fox", "a_b-c", "Player1000", strings.Repeat("x", 20), "Player"} {
		if err := service.ChangeDisplayName(login.UserID, name); err != nil {
			t.Errorf("ChangeDisplayName(%q) returned error: %v", name, err)
		}
	}
	if stored, _, _ := service.DisplayName(login.UserID); stored != "Player" {
		t.Errorf("stored name = %q, want the last accepted name", stored)
	}
}

func TestChangeDisplayNameRefusesBrokenRules(t *testing.T) {
	service, _ := newTestService(t, 0)
	login := loginWithNewAccount(t, service, loopbackAddress)
	cases := map[string]string{
		"too short":          "ab",
		"too long":           strings.Repeat("x", 21),
		"empty":              "",
		"bad character":      "name!",
		"non-ASCII":          "José Smith",
		"control character":  "ab\tc",
		"leading space":      " name",
		"trailing space":     "name ",
		"another default":    "Player1001",
		"Player and digits":  "Player7",
		"long default digit": "Player123456",
	}

	for label, name := range cases {
		err := service.ChangeDisplayName(login.UserID, name)
		var refusal *NameRefusal
		if !errors.As(err, &refusal) {
			t.Errorf("%s: ChangeDisplayName(%q) = %v, want a *NameRefusal", label, name, err)
		}
	}
	if stored, _, _ := service.DisplayName(login.UserID); stored != "Player1000" {
		t.Errorf("stored name = %q, want it unchanged", stored)
	}
}

func TestChangeDisplayNameRefusesANameInUseWithoutRegardToCase(t *testing.T) {
	service, _ := newTestService(t, 0)
	first := loginWithNewAccount(t, service, loopbackAddress)
	second := loginWithNewAccount(t, service, loopbackAddress)
	if err := service.ChangeDisplayName(first.UserID, "Brave Fox"); err != nil {
		t.Fatalf("ChangeDisplayName returned error: %v", err)
	}

	err := service.ChangeDisplayName(second.UserID, "bRaVe fOx")

	var refusal *NameRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("ChangeDisplayName = %v, want a *NameRefusal", err)
	}
}
