// Tests of the configuration file reader (SERVICE.md section 11).

package config

import (
	"strings"
	"testing"
)

// fullConfig holds every key of SERVICE.md section 11 with a valid value.
var fullConfig = map[string]string{
	"listen":                        "127.0.0.1:9000",
	"database":                      "lobby.db",
	"public_ws_url":                 "ws://127.0.0.1:9000/ws",
	"motd":                          "Welcome",
	"rooms":                         "Lobby, Practice",
	"turn_secret":                   "file-secret",
	"accounts_per_address_per_hour": "5",
	"trust_forwarded_for":           "off",
	"region.1.tag":                  "eu",
	"region.1.public_key":           "keys/region.key.pub",
}

// configText writes the given keys as key = value lines, in a fixed order.
func configText(values map[string]string) string {
	var builder strings.Builder
	for key, value := range values {
		builder.WriteString(key + " = " + value + "\n")
	}
	return builder.String()
}

// withValues returns a copy of fullConfig with the given keys changed. A value
// of "-" removes the key.
func withValues(changes map[string]string) map[string]string {
	values := make(map[string]string)
	for key, value := range fullConfig {
		values[key] = value
	}
	for key, value := range changes {
		if value == "-" {
			delete(values, key)
			continue
		}
		values[key] = value
	}
	return values
}

func noEnv(string) (string, bool) {
	return "", false
}

func parseValues(t *testing.T, values map[string]string, lookupEnv func(string) (string, bool)) (Config, error) {
	t.Helper()
	return Parse(strings.NewReader(configText(values)), lookupEnv)
}

func TestParseReadsEveryKey(t *testing.T) {
	cfg, err := parseValues(t, fullConfig, noEnv)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Listen != "127.0.0.1:9000" || cfg.Database != "lobby.db" {
		t.Errorf("listen or database not read: %+v", cfg)
	}
	if cfg.PublicWSURL != "ws://127.0.0.1:9000/ws" || cfg.MOTD != "Welcome" {
		t.Errorf("public_ws_url or motd not read: %+v", cfg)
	}
	if len(cfg.Rooms) != 2 || cfg.Rooms[0] != "Lobby" || cfg.Rooms[1] != "Practice" {
		t.Errorf("rooms not split and trimmed: %q", cfg.Rooms)
	}
	if cfg.TurnSecret != "file-secret" || cfg.AccountsPerAddressPerHour != 5 || cfg.TrustForwardedFor {
		t.Errorf("turn_secret, accounts or trust_forwarded_for not read: %+v", cfg)
	}
	if len(cfg.Regions) != 1 || cfg.Regions[0] != (Region{Number: 1, Tag: "eu", PublicKeyPath: "keys/region.key.pub"}) {
		t.Errorf("region not read: %+v", cfg.Regions)
	}
}

func TestParseIgnoresBlankLinesAndComments(t *testing.T) {
	text := "# comment\n\n" + configText(fullConfig) + "   \n# end\n"
	if _, err := Parse(strings.NewReader(text), noEnv); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
}

func TestParseAllowsEmptyMOTDAndTurnSecret(t *testing.T) {
	values := withValues(map[string]string{"motd": "", "turn_secret": ""})
	cfg, err := parseValues(t, values, noEnv)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.MOTD != "" || cfg.TurnSecret != "" {
		t.Errorf("empty motd or turn_secret not kept empty: %+v", cfg)
	}
}

func TestParseAllowsNoRegions(t *testing.T) {
	cfg, err := parseValues(t, withValues(map[string]string{"region.1.tag": "-", "region.1.public_key": "-"}), noEnv)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(cfg.Regions) != 0 {
		t.Errorf("expected no regions, got %+v", cfg.Regions)
	}
}

func TestParseNamesEveryMissingRequiredKey(t *testing.T) {
	required := []string{
		"listen", "database", "public_ws_url", "rooms",
		"accounts_per_address_per_hour", "trust_forwarded_for",
	}
	for _, key := range required {
		t.Run(key, func(t *testing.T) {
			_, err := parseValues(t, withValues(map[string]string{key: "-"}), noEnv)
			if err == nil || !strings.Contains(err.Error(), `"`+key+`"`) {
				t.Fatalf("expected error naming %q, got %v", key, err)
			}
		})
	}
}

func TestParseRefusesEmptyRequiredValue(t *testing.T) {
	_, err := parseValues(t, withValues(map[string]string{"database": ""}), noEnv)
	if err == nil || !strings.Contains(err.Error(), `"database"`) {
		t.Fatalf("expected error naming database, got %v", err)
	}
}

func TestParseRefusesInvalidValues(t *testing.T) {
	cases := []struct {
		name    string
		changes map[string]string
		wantKey string
	}{
		{"listen without port", map[string]string{"listen": "127.0.0.1"}, "listen"},
		{"listen port zero", map[string]string{"listen": "127.0.0.1:0"}, "listen"},
		{"listen port too large", map[string]string{"listen": "127.0.0.1:70000"}, "listen"},
		{"ws url over http", map[string]string{"public_ws_url": "http://127.0.0.1:9000/ws"}, "public_ws_url"},
		{"ws url without host", map[string]string{"public_ws_url": "ws:///ws"}, "public_ws_url"},
		{"negative account limit", map[string]string{"accounts_per_address_per_hour": "-1"}, "accounts_per_address_per_hour"},
		{"account limit not a number", map[string]string{"accounts_per_address_per_hour": "five"}, "accounts_per_address_per_hour"},
		{"trust flag not on or off", map[string]string{"trust_forwarded_for": "yes"}, "trust_forwarded_for"},
		{"room name empty", map[string]string{"rooms": "Lobby,,Practice"}, "rooms"},
		{"region without public key", map[string]string{"region.1.public_key": "-"}, "region.1.public_key"},
		{"region without tag", map[string]string{"region.1.tag": "-"}, "region.1.tag"},
		{"region number zero", map[string]string{"region.0.tag": "xx", "region.0.public_key": "k"}, "region.0."},
		{"region key with unknown suffix", map[string]string{"region.1.name": "x"}, "region.1.name"},
		{"unknown key", map[string]string{"listn": "127.0.0.1:9000"}, "listn"},
		{"duplicate region tag", map[string]string{"region.2.tag": "eu", "region.2.public_key": "k2"}, "region tag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseValues(t, withValues(tc.changes), noEnv)
			if err == nil || !strings.Contains(err.Error(), tc.wantKey) {
				t.Fatalf("expected error naming %q, got %v", tc.wantKey, err)
			}
		})
	}
}

func TestParseRefusesMalformedLines(t *testing.T) {
	cases := map[string]string{
		"line without equals sign": "listen 127.0.0.1:9000\n",
		"line with empty key":      " = value\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(text), noEnv); err == nil {
				t.Fatal("expected error for malformed line")
			}
		})
	}
}

func TestParseRefusesRepeatedKey(t *testing.T) {
	text := configText(fullConfig) + "listen = 127.0.0.1:9001\n"
	_, err := Parse(strings.NewReader(text), noEnv)
	if err == nil || !strings.Contains(err.Error(), `"listen"`) {
		t.Fatalf("expected error naming listen, got %v", err)
	}
}

func TestTurnSecretFromEnvironmentReplacesFile(t *testing.T) {
	env := func(key string) (string, bool) {
		if key == "LOBBY_TURN_SECRET" {
			return "env-secret", true
		}
		return "", false
	}
	cfg, err := parseValues(t, fullConfig, env)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.TurnSecret != "env-secret" {
		t.Errorf("TurnSecret = %q, want the environment value", cfg.TurnSecret)
	}
}

func TestEmptyTurnSecretFromEnvironmentKeepsFileValue(t *testing.T) {
	env := func(key string) (string, bool) {
		if key == "LOBBY_TURN_SECRET" {
			return "", true
		}
		return "", false
	}
	cfg, err := parseValues(t, fullConfig, env)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.TurnSecret != "file-secret" {
		t.Errorf("TurnSecret = %q, want the file value", cfg.TurnSecret)
	}
}

func TestErrorDoesNotContainTurnSecret(t *testing.T) {
	values := withValues(map[string]string{"listen": "bad", "turn_secret": "very-secret-value"})
	_, err := parseValues(t, values, noEnv)
	if err == nil {
		t.Fatal("expected error for invalid listen")
	}
	if strings.Contains(err.Error(), "very-secret-value") {
		t.Errorf("error text contains the turn secret: %v", err)
	}
}
