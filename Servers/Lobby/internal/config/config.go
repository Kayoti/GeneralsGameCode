// Package config reads the lobby service's configuration file (SERVICE.md section 11).
//
// The file holds one `key = value` pair per line. Blank lines and lines that
// start with `#` are ignored. Every key is checked when the file is read, and
// the error names the key that is missing or invalid.
package config

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Key names of SERVICE.md section 11.
const (
	keyListen                    = "listen"
	keyDatabase                  = "database"
	keyPublicWSURL               = "public_ws_url"
	keyMOTD                      = "motd"
	keyRooms                     = "rooms"
	keyTurnSecret                = "turn_secret"
	keyAccountsPerAddressPerHour = "accounts_per_address_per_hour"
	keyTrustForwardedFor         = "trust_forwarded_for"
	keyRegionPrefix              = "region."
	keyRegionTagSuffix           = ".tag"
	keyRegionPublicKeySuffix     = ".public_key"
)

// envTurnSecret is the environment variable that may hold turn_secret (SERVICE.md section 1).
const envTurnSecret = "LOBBY_TURN_SECRET"

// Values of trust_forwarded_for.
const (
	valueOn  = "on"
	valueOff = "off"
)

// maxListenPort is the highest TCP port number.
const maxListenPort = 65535

// Config holds every value of the configuration file.
type Config struct {
	Listen                    string
	Database                  string
	PublicWSURL               string
	MOTD                      string
	Rooms                     []string
	TurnSecret                string
	AccountsPerAddressPerHour int
	TrustForwardedFor         bool
	Regions                   []Region
}

// Region is one server region: the tag written into a server lobby and the
// file that holds the region's public key.
type Region struct {
	Number        int
	Tag           string
	PublicKeyPath string
}

// Load reads the configuration file at path. The turn secret may also come
// from the environment variable LOBBY_TURN_SECRET; when that variable is set
// and not empty it replaces the value in the file.
func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open configuration file: %w", err)
	}
	defer file.Close()
	return Parse(file, os.LookupEnv)
}

// Parse reads configuration text from r. lookupEnv reads the environment; it
// is a parameter so that tests need no real environment.
func Parse(r io.Reader, lookupEnv func(key string) (string, bool)) (Config, error) {
	values, err := readPairs(r)
	if err != nil {
		return Config{}, err
	}
	cfg, err := buildConfig(values)
	if err != nil {
		return Config{}, err
	}
	if secret, ok := lookupEnv(envTurnSecret); ok && secret != "" {
		cfg.TurnSecret = secret
	}
	return cfg, nil
}

// readPairs reads every key = value line into a map. A key that appears twice
// is an error, so that a copied line cannot silently replace an earlier one.
func readPairs(r io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("configuration line %d: expected key = value", lineNumber)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("configuration line %d: key %q is given twice", lineNumber, key)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	return values, nil
}

// buildConfig checks every known key and refuses keys it does not know, so a
// misspelt key is reported rather than ignored.
func buildConfig(values map[string]string) (Config, error) {
	var cfg Config
	var err error
	if cfg.Listen, err = requiredListen(values); err != nil {
		return Config{}, err
	}
	if cfg.Database, err = requiredText(values, keyDatabase); err != nil {
		return Config{}, err
	}
	if cfg.PublicWSURL, err = requiredWSURL(values); err != nil {
		return Config{}, err
	}
	if cfg.Rooms, err = requiredRooms(values); err != nil {
		return Config{}, err
	}
	if cfg.AccountsPerAddressPerHour, err = requiredCount(values); err != nil {
		return Config{}, err
	}
	if cfg.TrustForwardedFor, err = requiredOnOff(values); err != nil {
		return Config{}, err
	}
	cfg.MOTD = values[keyMOTD]
	cfg.TurnSecret = values[keyTurnSecret]
	if cfg.Regions, err = readRegions(values); err != nil {
		return Config{}, err
	}
	if err := refuseUnknownKeys(values); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func requiredText(values map[string]string, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("required key %q is missing", key)
	}
	if value == "" {
		return "", fmt.Errorf("required key %q is empty", key)
	}
	return value, nil
}

func requiredListen(values map[string]string) (string, error) {
	address, err := requiredText(values, keyListen)
	if err != nil {
		return "", err
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("key %q must be host:port: %w", keyListen, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > maxListenPort {
		return "", fmt.Errorf("key %q has port %q outside 1 to %d", keyListen, port, maxListenPort)
	}
	return address, nil
}

func requiredWSURL(values map[string]string) (string, error) {
	raw, err := requiredText(values, keyPublicWSURL)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("key %q is not a valid URL: %w", keyPublicWSURL, err)
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return "", fmt.Errorf("key %q must start with ws:// or wss://", keyPublicWSURL)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("key %q has no host name", keyPublicWSURL)
	}
	return raw, nil
}

func requiredRooms(values map[string]string) ([]string, error) {
	raw, err := requiredText(values, keyRooms)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("key %q has an empty room name", keyRooms)
		}
		names = append(names, name)
	}
	return names, nil
}

func requiredCount(values map[string]string) (int, error) {
	raw, err := requiredText(values, keyAccountsPerAddressPerHour)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("key %q must be a whole number of 0 or more, 0 meaning no limit", keyAccountsPerAddressPerHour)
	}
	return count, nil
}

func requiredOnOff(values map[string]string) (bool, error) {
	raw, err := requiredText(values, keyTrustForwardedFor)
	if err != nil {
		return false, err
	}
	switch raw {
	case valueOn:
		return true, nil
	case valueOff:
		return false, nil
	}
	return false, fmt.Errorf("key %q must be %q or %q", keyTrustForwardedFor, valueOn, valueOff)
}

// readRegions reads region.<number>.tag and region.<number>.public_key. A
// region needs both keys. The public key file is read by the code that uses it.
// Regions are returned in ascending number order.
func readRegions(values map[string]string) ([]Region, error) {
	numbers, err := regionNumbers(values)
	if err != nil {
		return nil, err
	}
	var regions []Region
	for _, number := range numbers {
		region, err := readRegion(values, number)
		if err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	if err := refuseDuplicateTags(regions); err != nil {
		return nil, err
	}
	return regions, nil
}

// regionNumbers returns the sorted region numbers that appear in any region key.
// A region key must be region.<number>.tag or region.<number>.public_key.
func regionNumbers(values map[string]string) ([]int, error) {
	seen := make(map[int]bool)
	var numbers []int
	for key := range values {
		if !strings.HasPrefix(key, keyRegionPrefix) {
			continue
		}
		number, suffix, err := splitRegionKey(key)
		if err != nil {
			return nil, err
		}
		if suffix != keyRegionTagSuffix && suffix != keyRegionPublicKeySuffix {
			return nil, fmt.Errorf("unknown key %q", key)
		}
		if !seen[number] {
			seen[number] = true
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	return numbers, nil
}

// splitRegionKey splits region.<number>.<suffix> into its number and suffix.
func splitRegionKey(key string) (int, string, error) {
	rest := strings.TrimPrefix(key, keyRegionPrefix)
	numberText, suffix, found := strings.Cut(rest, ".")
	number, err := strconv.Atoi(numberText)
	if !found || err != nil || number < 1 {
		return 0, "", fmt.Errorf("key %q must be region.<number>.tag or region.<number>.public_key with a number of 1 or more", key)
	}
	return number, "." + suffix, nil
}

func readRegion(values map[string]string, number int) (Region, error) {
	tagKey := fmt.Sprintf("%s%d%s", keyRegionPrefix, number, keyRegionTagSuffix)
	tag, err := requiredText(values, tagKey)
	if err != nil {
		return Region{}, err
	}
	publicKeyKey := fmt.Sprintf("%s%d%s", keyRegionPrefix, number, keyRegionPublicKeySuffix)
	publicKeyPath, err := requiredText(values, publicKeyKey)
	if err != nil {
		return Region{}, err
	}
	return Region{Number: number, Tag: tag, PublicKeyPath: publicKeyPath}, nil
}

// refuseDuplicateTags stops at two regions with one tag, because a server
// lobby carries only its tag and would match both.
func refuseDuplicateTags(regions []Region) error {
	tagNumbers := make(map[string]int)
	for _, region := range regions {
		if first, exists := tagNumbers[region.Tag]; exists {
			return fmt.Errorf("region tag %q is used by region %d and region %d", region.Tag, first, region.Number)
		}
		tagNumbers[region.Tag] = region.Number
	}
	return nil
}

// refuseUnknownKeys stops at any key that is not in SERVICE.md section 11, so
// that a misspelt key is reported and not silently ignored.
func refuseUnknownKeys(values map[string]string) error {
	known := map[string]bool{
		keyListen:                    true,
		keyDatabase:                  true,
		keyPublicWSURL:               true,
		keyMOTD:                      true,
		keyRooms:                     true,
		keyTurnSecret:                true,
		keyAccountsPerAddressPerHour: true,
		keyTrustForwardedFor:         true,
	}
	for key := range values {
		if !known[key] && !strings.HasPrefix(key, keyRegionPrefix) {
			return fmt.Errorf("unknown key %q", key)
		}
	}
	return nil
}
