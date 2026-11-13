// Unit tests for the relay's configuration, key and pass code. Built only with RELAY_BUILD_TESTS=ON.

#include "config.h"
#include "pass.h"
#include "protocol.h"
#include "region_keys.h"

#include <cstdint>
#include <cstdio>
#include <cstring>
#include <sodium.h>
#include <string>

namespace {

int g_failures = 0;

void ExpectTrue(bool condition, const char *text, const char *file, int line) {
  if (!condition) {
    static_cast<void>(std::fprintf(stderr, "%s:%d: expected true: %s\n", file, line, text));
    ++g_failures;
  }
}

#define EXPECT_TRUE(condition) ExpectTrue((condition), #condition, __FILE__, __LINE__)
#define EXPECT_EQ(actual, expected) ExpectTrue(((actual) == (expected)), #actual " == " #expected, __FILE__, __LINE__)

const char *const FULL_CONFIG = "# comment\r\n"
                                "\r\n"
                                "region            = 1\r\n"
                                "listen_port       = 7777\r\n"
                                "ping_limit_ms=150\r\n"
                                "   key_file   =   keys/region.key   \r\n"
                                "max_sessions      = 200\r\n"
                                "vpn_check         = on\r\n"
                                "vpn_ranges        = data/blocked-ranges.txt\r\n"
                                "vpn_operators     = data/blocked-operators.txt\r\n"
                                "vpn_operator_db   = data/address-to-operator.mmdb\r\n"
                                "vpn_allow         = data/allowed-ranges.txt\r\n";

bool ReplaceOnce(std::string &text, const std::string &from, const std::string &to) {
  std::size_t position = text.find(from);
  if (position == std::string::npos) {
    return false;
  }
  text.replace(position, from.size(), to);
  return true;
}

void TestConfigParsesFullFile() {
  relay::RelayConfig config;
  std::string error;
  bool ok = relay::ParseRelayConfig(FULL_CONFIG, config, error);
  EXPECT_TRUE(ok);
  EXPECT_EQ(config.region, 1);
  EXPECT_EQ(config.listen_port, 7777);
  EXPECT_EQ(config.ping_limit_ms, 150);
  EXPECT_TRUE(config.key_file == "keys/region.key");
  EXPECT_EQ(config.max_sessions, 200);
  EXPECT_TRUE(config.vpn_check);
  EXPECT_TRUE(config.vpn_allow == "data/allowed-ranges.txt");
}

void TestConfigAcceptsLfEndings() {
  std::string lf;
  for (char c : std::string(FULL_CONFIG)) {
    if (c != '\r') {
      lf.push_back(c);
    }
  }
  relay::RelayConfig config;
  std::string error;
  EXPECT_TRUE(relay::ParseRelayConfig(lf, config, error));
}

void TestConfigRejectsBadFiles() {
  struct Case {
    const char *m_name;
    std::string m_from;
    std::string m_to;
    const char *m_expected_error;
  };
  const Case CASES[] = {
      {"missing key", "region            = 1\r\n", "", "missing key 'region'"},
      {"unknown key", "vpn_allow         = data/allowed-ranges.txt\r\n",
       "vpn_allow         = data/allowed-ranges.txt\r\nfoo = 1\r\n", "unknown key 'foo'"},
      {"duplicate key", "max_sessions      = 200\r\n", "max_sessions      = 200\r\nmax_sessions = 5\r\n",
       "key 'max_sessions' is set twice"},
      {"port too large", "listen_port       = 7777", "listen_port       = 70000", "key 'listen_port'"},
      {"ping limit zero", "ping_limit_ms=150", "ping_limit_ms=0", "key 'ping_limit_ms'"},
      {"non-digit number", "max_sessions      = 200", "max_sessions      = 20a", "key 'max_sessions'"},
      {"bad switch", "vpn_check         = on", "vpn_check         = yes", "'on' or 'off'"},
      {"no equals sign", "region            = 1", "region 1", "expected 'key = value'"},
      {"empty value", "key_file   =   keys/region.key", "key_file   =", "must not be empty"},
  };

  for (std::size_t i = 0; i < sizeof(CASES) / sizeof(CASES[0]); ++i) {
    std::string text = FULL_CONFIG;
    EXPECT_TRUE(ReplaceOnce(text, CASES[i].m_from, CASES[i].m_to));
    relay::RelayConfig config;
    std::string error;
    bool ok = relay::ParseRelayConfig(text, config, error);
    if (ok || error.find(CASES[i].m_expected_error) == std::string::npos) {
      static_cast<void>(std::fprintf(stderr, "config case '%s' gave: ok=%d error='%s'\n", CASES[i].m_name, ok ? 1 : 0,
                                     error.c_str()));
      ++g_failures;
    }
  }
}

void TestKeyRoundTrip(const std::string &path, relay::RegionKey &key) {
  std::string error;
  EXPECT_TRUE(relay::GenerateRegionKeys(path, error));
  EXPECT_TRUE(relay::LoadRegionKey(path, key, error));

  // Generating over an existing file must fail and leave it alone.
  EXPECT_TRUE(!relay::GenerateRegionKeys(path, error));
}

void TestKeyRejectsCorruptFile(const std::string &path) {
  std::FILE *file = std::fopen(path.c_str(), "wb");
  EXPECT_TRUE(file != nullptr);
  if (file != nullptr) {
    static_cast<void>(std::fputs("00112233\n", file));
    static_cast<void>(std::fclose(file));
  }
  relay::RegionKey key;
  std::string error;
  EXPECT_TRUE(!relay::LoadRegionKey(path, key, error));
}

struct PassFixture {
  relay::RegionKey m_key;
  relay::PassFields m_fields;
  unsigned char m_pass[relay::PASS_SIZE] = {};
};

void MakeFixture(PassFixture &fixture) {
  fixture.m_fields.version = relay::PASS_VERSION;
  fixture.m_fields.region = 1;
  fixture.m_fields.client_ip = 0x0A000001;
  fixture.m_fields.exe_crc = 0x11111111;
  fixture.m_fields.ini_crc = 0x22222222;
  fixture.m_fields.rtt_ms = 42;
  fixture.m_fields.issued_at = 1000;
  fixture.m_fields.expires_at = 1000 + relay::PASS_LIFETIME_SECONDS;
  fixture.m_fields.pass_id = 0x0123456789ABCDEFULL;
}

bool SignAndCheck(const PassFixture &fixture, const relay::PassCheckContext &context, relay::RefuseCode &refuse,
                  relay::PassFields &decoded) {
  return relay::CheckPass(fixture.m_pass, relay::PASS_SIZE, context, decoded, refuse);
}

void TestPassRoundTripAndRules(const relay::RegionKey &key) {
  PassFixture fixture;
  fixture.m_key = key;
  MakeFixture(fixture);
  EXPECT_TRUE(relay::BuildPass(fixture.m_fields, key.secret, fixture.m_pass));

  // Layout from PROTOCOL.md 4.3: version, reserved, region little-endian, pass_id little-endian at offset 28.
  EXPECT_EQ(fixture.m_pass[0], 1);
  EXPECT_EQ(fixture.m_pass[1], 0);
  EXPECT_EQ(fixture.m_pass[2], 1);
  EXPECT_EQ(fixture.m_pass[3], 0);
  EXPECT_EQ(fixture.m_pass[28], 0xEF);
  EXPECT_EQ(fixture.m_pass[35], 0x01);

  relay::PassCheckContext context;
  context.public_key = key.public_key;
  context.region = 1;
  context.source_ip = 0x0A000001;
  context.check_address = true;
  context.now = 1000;

  relay::RefuseCode refuse = relay::REFUSE_BAD_PASS;
  relay::PassFields decoded;
  EXPECT_TRUE(SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(decoded.pass_id, 0x0123456789ABCDEFULL);
  EXPECT_EQ(decoded.rtt_ms, 42);
  EXPECT_EQ(decoded.client_ip, 0x0A000001u);

  context.now = 1599;
  EXPECT_TRUE(SignAndCheck(fixture, context, refuse, decoded));

  context.now = 1600;
  EXPECT_TRUE(!SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(refuse, relay::REFUSE_EXPIRED);
  context.now = 1000;

  context.region = 2;
  EXPECT_TRUE(!SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(refuse, relay::REFUSE_WRONG_REGION);
  context.region = 1;

  context.source_ip = 0x0A000002;
  EXPECT_TRUE(!SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(refuse, relay::REFUSE_ADDRESS_MISMATCH);
  context.check_address = false;
  EXPECT_TRUE(SignAndCheck(fixture, context, refuse, decoded));
  context.check_address = true;
  context.source_ip = 0x0A000001;

  // A changed byte breaks the signature.
  fixture.m_pass[20] ^= 0x01;
  EXPECT_TRUE(!SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(refuse, relay::REFUSE_BAD_PASS);
  fixture.m_pass[20] ^= 0x01;

  // Wrong length.
  refuse = relay::REFUSE_BAD_PASS;
  EXPECT_TRUE(!relay::CheckPass(fixture.m_pass, relay::PASS_SIZE - 1, context, decoded, refuse));
  EXPECT_EQ(refuse, relay::REFUSE_BAD_PASS);

  // A pass signed by another key is refused.
  relay::RegionKey other;
  unsigned char other_secret[relay::REGION_SECRET_KEY_SIZE];
  EXPECT_TRUE(crypto_sign_keypair(other.public_key, other_secret) == 0);
  context.public_key = other.public_key;
  EXPECT_TRUE(!SignAndCheck(fixture, context, refuse, decoded));
  EXPECT_EQ(refuse, relay::REFUSE_BAD_PASS);
  context.public_key = key.public_key;

  // A signed pass with an unknown version is refused.
  PassFixture versioned;
  versioned.m_key = key;
  MakeFixture(versioned);
  versioned.m_fields.version = 2;
  EXPECT_TRUE(relay::BuildPass(versioned.m_fields, key.secret, versioned.m_pass));
  EXPECT_TRUE(!relay::CheckPass(versioned.m_pass, relay::PASS_SIZE, context, decoded, refuse));
  EXPECT_EQ(refuse, relay::REFUSE_BAD_PASS);
}

void TestProtocolSizes() {
  EXPECT_EQ(relay::PASS_SIZE, 100u);
  EXPECT_EQ(relay::CREATE_BODY_SIZE, 104u);
  EXPECT_EQ(relay::JOIN_BODY_SIZE, 119u);
  EXPECT_EQ(relay::CHECK_HELLO_BODY_SIZE, 10u);
}

} // namespace

namespace {

int RunTests() {
  if (sodium_init() < 0) {
    static_cast<void>(std::fprintf(stderr, "sodium_init failed\n"));
    return 1;
  }

  TestProtocolSizes();
  TestConfigParsesFullFile();
  TestConfigAcceptsLfEndings();
  TestConfigRejectsBadFiles();

  std::string key_path = "relay_test_region.key";
  std::string corrupt_path = "relay_test_corrupt.key";
  relay::RegionKey key;
  TestKeyRoundTrip(key_path, key);
  TestPassRoundTripAndRules(key);
  TestKeyRejectsCorruptFile(corrupt_path);

  static_cast<void>(std::remove(key_path.c_str()));
  static_cast<void>(std::remove((key_path + ".pub").c_str()));
  static_cast<void>(std::remove(corrupt_path.c_str()));

  if (g_failures != 0) {
    static_cast<void>(std::fprintf(stderr, "%d check(s) failed\n", g_failures));
    return 1;
  }
  std::printf("all relay tests passed\n");
  return 0;
}

} // namespace

int main() {
  try {
    return RunTests();
  } catch (...) {
    static_cast<void>(std::fprintf(stderr, "relay tests stopped by an unexpected error\n"));
    return 1;
  }
}
