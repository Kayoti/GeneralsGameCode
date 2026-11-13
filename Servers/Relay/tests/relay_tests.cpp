// Unit tests for the relay's configuration, key and pass code. Built only with RELAY_BUILD_TESTS=ON.

#include "config.h"
#include "entry_check.h"
#include "pass.h"
#include "protocol.h"
#include "rate_limit.h"
#include "region_keys.h"
#include "sessions.h"
#include "wire.h"

#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <sodium.h>
#include <string>
#include <vector>

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
  EXPECT_EQ(relay::REFUSE_BODY_SIZE, 5u);
}

// Runs one ping measurement that starts just before the 32-bit clock wraps. Ping i is sent at 250 ms * i and
// answered after rtts[i] ms. A value of 0 means no answer.
relay::EntryVerdict RunMeasurement(const std::uint32_t *rtts, std::uint16_t limit_ms, std::uint16_t &median_ms) {
  const std::uint32_t START_MS = 0xFFFFFF00u;
  relay::PingRound round;
  round.Start(START_MS);
  for (std::size_t i = 0; i < relay::PING_COUNT; ++i) {
    std::uint32_t sent = START_MS + (static_cast<std::uint32_t>(i) * relay::PING_INTERVAL_MS);
    EXPECT_TRUE(round.PingDue(sent));
    round.SentPing(100 + i, sent);
    if (rtts[i] != 0) {
      EXPECT_TRUE(round.OnPong(100 + i, sent + rtts[i]));
    }
  }
  return round.Verdict(limit_ms, median_ms);
}

void TestPingRoundVerdicts() {
  const std::uint32_t ALL_ANSWERS[relay::PING_COUNT] = {10, 20, 30, 40, 50, 60, 70, 80};
  std::uint16_t median = 0;
  EXPECT_TRUE(RunMeasurement(ALL_ANSWERS, 150, median) == relay::EntryVerdict::PASS);
  EXPECT_EQ(median, 45); // Even count: average of the two middle values.

  const std::uint32_t MIXED_ANSWERS[relay::PING_COUNT] = {100, 0, 300, 0, 200, 400, 0, 500};
  EXPECT_TRUE(RunMeasurement(MIXED_ANSWERS, 150, median) == relay::EntryVerdict::PING_TOO_HIGH);
  EXPECT_EQ(median, 300);
  EXPECT_TRUE(RunMeasurement(MIXED_ANSWERS, 300, median) == relay::EntryVerdict::PASS); // Equal to the limit passes.
  EXPECT_EQ(median, 300);

  const std::uint32_t FOUR_ANSWERS[relay::PING_COUNT] = {10, 10, 10, 10, 0, 0, 0, 0};
  EXPECT_TRUE(RunMeasurement(FOUR_ANSWERS, 150, median) == relay::EntryVerdict::TIMEOUT);
}

void TestPingRoundRejectsBadPongs() {
  relay::PingRound round;
  round.Start(0);
  round.SentPing(7, 0);
  EXPECT_TRUE(!round.OnPong(8, 10)); // Unknown nonce.
  EXPECT_TRUE(round.OnPong(7, 10));
  EXPECT_TRUE(!round.OnPong(7, 20)); // Already answered.

  relay::PingRound late;
  late.Start(0);
  late.SentPing(9, 0);
  EXPECT_TRUE(!late.OnPong(9, relay::ANSWER_WINDOW_MS)); // Arrives when the window has closed.
  EXPECT_TRUE(late.Expired(relay::ANSWER_WINDOW_MS));
  EXPECT_TRUE(!late.Expired(relay::ANSWER_WINDOW_MS - 1));
}

void TestPingRoundSchedule() {
  relay::PingRound round;
  round.Start(1000);
  EXPECT_TRUE(round.PingDue(1000));
  round.SentPing(1, 1000);
  EXPECT_TRUE(!round.PingDue(1249));
  EXPECT_TRUE(round.PingDue(1250));
  EXPECT_EQ(round.MsUntilNextAction(1100), 150u);
  EXPECT_EQ(round.MsUntilNextAction(1300), 0u);
  for (std::uint32_t i = 1; i < relay::PING_COUNT; ++i) {
    round.SentPing(i + 1, 1000 + (i * relay::PING_INTERVAL_MS));
  }
  EXPECT_TRUE(!round.PingDue(1000 + 10000));
  EXPECT_EQ(round.MsUntilNextAction(1000 + 1750), 2250u);
}

void TestMedianWithFewAnswers() {
  const std::uint32_t FIVE_ANSWERS[relay::PING_COUNT] = {1, 2, 3, 4, 5, 0, 0, 0};
  std::uint16_t median = 0;
  EXPECT_TRUE(RunMeasurement(FIVE_ANSWERS, 150, median) == relay::EntryVerdict::PASS);
  EXPECT_EQ(median, 3);
}

void TestAddressRateLimit() {
  relay::AddressRateLimiter limiter(relay::ENTRY_CHECKS_PER_WINDOW, relay::RATE_WINDOW_MS);
  const std::uint32_t TEST_ADDRESS = 0x0A000001u;
  for (std::uint32_t i = 0; i < 6; ++i) {
    EXPECT_TRUE(limiter.Allow(TEST_ADDRESS, i));
  }
  EXPECT_TRUE(!limiter.Allow(TEST_ADDRESS, 6));
  EXPECT_TRUE(!limiter.Allow(TEST_ADDRESS, 59999));
  EXPECT_TRUE(limiter.Allow(0x0A000002u, 6));      // Another address is not affected.
  EXPECT_TRUE(limiter.Allow(TEST_ADDRESS, 60000)); // The check at time 0 has left the window.
  EXPECT_TRUE(!limiter.Allow(TEST_ADDRESS, 60000));
}

void TestVpnHookAllowsEveryoneForNow() {
  EXPECT_TRUE(relay::VpnCheckPasses(0x0A000001u));
}

void TestIssuePass(const relay::RegionKey &key) {
  relay::EntryCheckSettings settings;
  settings.region = 1;
  settings.ping_limit_ms = 150;
  settings.key = key;

  relay::PassFields fields;
  fields.client_ip = 0x0A000001u;
  fields.exe_crc = 0x11111111u;
  fields.ini_crc = 0x22222222u;
  fields.rtt_ms = 42;
  unsigned char pass[relay::PASS_SIZE] = {};
  EXPECT_TRUE(relay::IssuePass(settings, fields, 1000, pass));

  relay::PassCheckContext context;
  context.public_key = key.public_key;
  context.region = 1;
  context.source_ip = 0x0A000001u;
  context.check_address = true;
  context.now = 1000;
  relay::PassFields decoded;
  relay::RefuseCode refuse = relay::REFUSE_BAD_PASS;
  EXPECT_TRUE(relay::CheckPass(pass, relay::PASS_SIZE, context, decoded, refuse));
  EXPECT_EQ(decoded.region, 1);
  EXPECT_EQ(decoded.rtt_ms, 42);
  EXPECT_EQ(decoded.client_ip, 0x0A000001u);
  EXPECT_EQ(decoded.exe_crc, 0x11111111u);
  EXPECT_EQ(decoded.ini_crc, 0x22222222u);
  EXPECT_EQ(decoded.issued_at, 1000u);
  EXPECT_EQ(decoded.expires_at, 1000u + relay::PASS_LIFETIME_SECONDS);
  EXPECT_TRUE(decoded.pass_id != relay::RandomId());
}

// Records what the session manager sends.
class RecordingOutput : public relay::SessionOutput {
public:
  struct Sent {
    std::uint32_t m_connection = 0;
    std::vector<unsigned char> m_message;
  };

  void Send(std::uint32_t connection, const unsigned char *message, std::size_t length) override {
    Sent sent;
    sent.m_connection = connection;
    sent.m_message.assign(message, message + length);
    m_sent.push_back(sent);
  }

  void Close(std::uint32_t connection) override {
    m_closed.push_back(connection);
  }

  // Messages for one connection, in order.
  std::vector<std::vector<unsigned char>> For(std::uint32_t connection) const {
    std::vector<std::vector<unsigned char>> result;
    for (const Sent &sent : m_sent) {
      if (sent.m_connection == connection) {
        result.push_back(sent.m_message);
      }
    }
    return result;
  }

  bool WasClosed(std::uint32_t connection) const {
    return std::any_of(m_closed.begin(), m_closed.end(),
                       [connection](std::uint32_t closed) { return closed == connection; });
  }

  void Clear() {
    m_sent.clear();
    m_closed.clear();
  }

  std::vector<Sent> m_sent;
  std::vector<std::uint32_t> m_closed;
};

const std::uint32_t TEST_IP = 0x0A000001;
const std::uint32_t TEST_NOW_UNIX = 1000;

struct SessionFixture {
  relay::RegionKey m_key;
  RecordingOutput m_output;
  relay::SessionSettings m_settings;
  std::uint32_t m_now_ms = 100000;
  std::uint64_t m_next_pass_id = 1;

  explicit SessionFixture(const relay::RegionKey &key, std::size_t max_sessions = 200) : m_key(key) {
    m_settings.public_key = m_key.public_key;
    m_settings.region = 1;
    m_settings.check_address = false;
    m_settings.max_sessions = max_sessions;
  }

  relay::SessionRequest Request(std::uint32_t connection, std::uint32_t ip = TEST_IP) const {
    relay::SessionRequest request;
    request.connection = connection;
    request.source_ip = ip;
    request.now_ms = m_now_ms;
    request.now_unix = TEST_NOW_UNIX;
    return request;
  }

  std::vector<unsigned char> Pass(std::uint64_t pass_id, std::uint32_t exe_crc = 0x11111111,
                                  std::uint32_t ini_crc = 0x22222222, std::uint32_t expires_at = 1600) const {
    relay::PassFields fields;
    fields.region = 1;
    fields.client_ip = TEST_IP;
    fields.exe_crc = exe_crc;
    fields.ini_crc = ini_crc;
    fields.rtt_ms = 20;
    fields.issued_at = 1000;
    fields.expires_at = expires_at;
    fields.pass_id = pass_id;
    std::vector<unsigned char> pass(relay::PASS_SIZE);
    EXPECT_TRUE(relay::BuildPass(fields, m_key.secret, pass.data()));
    return pass;
  }

  std::vector<unsigned char> CreateBody(const std::vector<unsigned char> &pass, unsigned host_slot, unsigned mask,
                                        std::uint16_t proto = relay::PROTOCOL_VERSION) const {
    std::vector<unsigned char> body(relay::CREATE_BODY_SIZE);
    relay::PutU16(body.data(), proto);
    std::memcpy(body.data() + 2, pass.data(), relay::PASS_SIZE);
    body.at(102) = static_cast<unsigned char>(host_slot);
    body.at(103) = static_cast<unsigned char>(mask);
    return body;
  }

  std::vector<unsigned char> JoinBody(const std::vector<unsigned char> &pass, std::uint64_t session_id, unsigned slot,
                                      std::uint64_t token) const {
    std::vector<unsigned char> body(relay::JOIN_BODY_SIZE);
    relay::PutU16(body.data(), relay::PROTOCOL_VERSION);
    std::memcpy(body.data() + 2, pass.data(), relay::PASS_SIZE);
    relay::PutU64(body.data() + 102, session_id);
    body.at(110) = static_cast<unsigned char>(slot);
    relay::PutU64(body.data() + 111, token);
    return body;
  }
};

// Result of a successful CREATE as the host sees it.
struct Created {
  std::uint64_t m_session_id = 0;
  std::uint64_t m_tokens[relay::SLOT_COUNT] = {};
};

bool ReadCreated(const RecordingOutput &output, std::uint32_t connection, Created &created) {
  std::vector<std::vector<unsigned char>> messages = output.For(connection);
  if (messages.size() != 2 || messages.at(0).size() != 1 + relay::CREATED_BODY_SIZE ||
      messages.at(0).at(0) != relay::MSG_CREATED || messages.at(1).size() != 1 + relay::WELCOME_BODY_SIZE ||
      messages.at(1).at(0) != relay::MSG_WELCOME) {
    return false;
  }
  created.m_session_id = relay::GetU64(messages.at(0).data() + 1);
  for (std::size_t slot = 0; slot < relay::SLOT_COUNT; ++slot) {
    created.m_tokens[slot] = relay::GetU64(messages.at(0).data() + 9 + (slot * 8));
  }
  return relay::GetU64(messages.at(1).data() + 1) == created.m_session_id;
}

// The one REFUSE code a connection received, or 0 when it received something else.
unsigned RefuseCodeOf(const RecordingOutput &output, std::uint32_t connection) {
  std::vector<std::vector<unsigned char>> messages = output.For(connection);
  if (messages.size() != 1 || messages.at(0).size() != 1 + relay::REFUSE_BODY_SIZE ||
      messages.at(0).at(0) != relay::MSG_REFUSE) {
    return 0;
  }
  return messages.at(0).at(1);
}

bool IsMessage(const std::vector<unsigned char> &message, unsigned type, unsigned mask, unsigned leader) {
  return message.size() == 3 && message.at(0) == type && message.at(1) == mask && message.at(2) == leader;
}

// Creates a session on host_connection with a fresh pass. Returns false when CREATED and WELCOME did not arrive.
bool MakeSession(SessionFixture &fixture, relay::SessionManager &manager, unsigned mask, unsigned host_slot,
                 Created &created, std::uint32_t host_connection) {
  std::vector<unsigned char> pass = fixture.Pass(fixture.m_next_pass_id++);
  std::vector<unsigned char> body = fixture.CreateBody(pass, host_slot, mask);
  manager.OnCreate(fixture.Request(host_connection), body.data(), body.size());
  return ReadCreated(fixture.m_output, host_connection, created);
}

void Join(SessionFixture &fixture, relay::SessionManager &manager, std::uint32_t connection, const Created &created,
          unsigned slot, std::uint64_t token) {
  std::vector<unsigned char> pass = fixture.Pass(fixture.m_next_pass_id++);
  std::vector<unsigned char> body = fixture.JoinBody(pass, created.m_session_id, slot, token);
  manager.OnJoin(fixture.Request(connection), body.data(), body.size());
}

void TestCreateValidation(const relay::RegionKey &key) {
  struct Case {
    const char *m_name;
    unsigned m_host_slot;
    unsigned m_mask;
    unsigned m_expected;
  };
  const Case CASES[] = {
      {"host outside mask", 2, 0x03, relay::REFUSE_BAD_REQUEST},
      {"no other player", 0, 0x01, relay::REFUSE_BAD_REQUEST},
      {"empty mask", 0, 0x00, relay::REFUSE_BAD_REQUEST},
      {"host slot out of range", 8, 0xFF, relay::REFUSE_BAD_REQUEST},
  };
  for (const Case &test : CASES) {
    SessionFixture fixture(key);
    relay::SessionManager manager(fixture.m_settings, fixture.m_output);
    std::vector<unsigned char> body = fixture.CreateBody(fixture.Pass(7), test.m_host_slot, test.m_mask);
    manager.OnCreate(fixture.Request(1), body.data(), body.size());
    if (RefuseCodeOf(fixture.m_output, 1) != test.m_expected || !fixture.m_output.WasClosed(1) ||
        manager.SessionCount() != 0 || manager.IsBound(1)) {
      static_cast<void>(std::fprintf(stderr, "create case '%s' failed\n", test.m_name));
      ++g_failures;
    }
  }

  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);

  std::vector<unsigned char> wrong_proto = fixture.CreateBody(fixture.Pass(1), 0, 0x03, 2);
  manager.OnCreate(fixture.Request(1), wrong_proto.data(), wrong_proto.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 1), static_cast<unsigned>(relay::REFUSE_PROTO_MISMATCH));

  std::vector<unsigned char> expired = fixture.CreateBody(fixture.Pass(2, 1, 1, TEST_NOW_UNIX), 0, 0x03);
  manager.OnCreate(fixture.Request(2), expired.data(), expired.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 2), static_cast<unsigned>(relay::REFUSE_EXPIRED));

  std::vector<unsigned char> forged = fixture.CreateBody(fixture.Pass(3), 0, 0x03);
  forged.at(2 + 20) ^= 0x01;
  manager.OnCreate(fixture.Request(3), forged.data(), forged.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 3), static_cast<unsigned>(relay::REFUSE_BAD_PASS));

  std::vector<unsigned char> short_body(relay::CREATE_BODY_SIZE - 1);
  manager.OnCreate(fixture.Request(4), short_body.data(), short_body.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 4), static_cast<unsigned>(relay::REFUSE_BAD_REQUEST));
  EXPECT_EQ(manager.SessionCount(), 0u);

  // With the address check on, a pass for another address is refused.
  fixture.m_settings.check_address = true;
  relay::SessionManager strict(fixture.m_settings, fixture.m_output);
  std::vector<unsigned char> body = fixture.CreateBody(fixture.Pass(5), 0, 0x03);
  strict.OnCreate(fixture.Request(5, TEST_IP + 1), body.data(), body.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 5), static_cast<unsigned>(relay::REFUSE_ADDRESS_MISMATCH));
  strict.OnCreate(fixture.Request(6, TEST_IP), body.data(), body.size());
  Created created;
  EXPECT_TRUE(ReadCreated(fixture.m_output, 6, created));
}

void TestCreateAcceptedAndRandom(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created first;
  Created second;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x15, 2, first, 1)); // Slots 0, 2, 4. Host in slot 2.
  EXPECT_TRUE(MakeSession(fixture, manager, 0x15, 2, second, 2));
  EXPECT_EQ(manager.SessionCount(), 2u);
  EXPECT_TRUE(manager.IsBound(1));
  EXPECT_TRUE(first.m_session_id != 0 && first.m_session_id != second.m_session_id);

  for (std::size_t slot = 0; slot < relay::SLOT_COUNT; ++slot) {
    bool expected_joiner = slot == 0 || slot == 4;
    EXPECT_EQ(first.m_tokens[slot] != 0, expected_joiner);
  }
  EXPECT_TRUE(first.m_tokens[0] != first.m_tokens[4]);
  EXPECT_TRUE(first.m_tokens[0] != second.m_tokens[0]);

  // WELCOME: slot 2, connected mask 0x04, leader 2.
  std::vector<std::vector<unsigned char>> messages = fixture.m_output.For(1);
  EXPECT_EQ(messages.at(1).at(9), 2);
  EXPECT_EQ(messages.at(1).at(10), 0x04);
  EXPECT_EQ(messages.at(1).at(11), 2);
}

void TestJoinFailures(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x07, 0, created, 1));
  fixture.m_output.Clear();

  struct Case {
    const char *m_name;
    std::uint64_t m_session_id;
    unsigned m_slot;
    std::uint64_t m_token;
    std::uint32_t m_exe_crc;
    unsigned m_expected;
  };
  const Case CASES[] = {
      {"unknown session", created.m_session_id + 1, 1, created.m_tokens[1], 0x11111111, relay::REFUSE_NO_SESSION},
      {"slot not expected", created.m_session_id, 5, created.m_tokens[1], 0x11111111, relay::REFUSE_NOT_EXPECTED},
      {"slot out of range", created.m_session_id, 9, created.m_tokens[1], 0x11111111, relay::REFUSE_BAD_REQUEST},
      {"host slot taken", created.m_session_id, 0, 0, 0x11111111, relay::REFUSE_SLOT_TAKEN},
      {"wrong token", created.m_session_id, 1, created.m_tokens[1] + 1, 0x11111111, relay::REFUSE_BAD_TOKEN},
      {"token of another slot", created.m_session_id, 1, created.m_tokens[2], 0x11111111, relay::REFUSE_BAD_TOKEN},
      {"zero token", created.m_session_id, 1, 0, 0x11111111, relay::REFUSE_BAD_TOKEN},
      {"other build", created.m_session_id, 1, created.m_tokens[1], 0x33333333, relay::REFUSE_BUILD_MISMATCH},
  };
  std::uint32_t connection = 10;
  for (const Case &test : CASES) {
    std::vector<unsigned char> pass = fixture.Pass(fixture.m_next_pass_id++, test.m_exe_crc);
    std::vector<unsigned char> body = fixture.JoinBody(pass, test.m_session_id, test.m_slot, test.m_token);
    manager.OnJoin(fixture.Request(connection), body.data(), body.size());
    if (RefuseCodeOf(fixture.m_output, connection) != test.m_expected || !fixture.m_output.WasClosed(connection) ||
        manager.IsBound(connection)) {
      static_cast<void>(std::fprintf(stderr, "join case '%s' failed\n", test.m_name));
      ++g_failures;
    }
    ++connection;
  }
  // None of the failures changed the session, and the host heard nothing.
  EXPECT_EQ(manager.SessionCount(), 1u);
  EXPECT_EQ(fixture.m_output.For(1).size(), 0u);

  // A wrong build did not use up the token.
  Join(fixture, manager, 50, created, 1, created.m_tokens[1]);
  EXPECT_TRUE(manager.IsBound(50));
}

void TestJoinFlowRosterAndStart(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x0D, 0, created, 1)); // Slots 0, 2, 3.
  fixture.m_output.Clear();

  Join(fixture, manager, 2, created, 3, created.m_tokens[3]);
  std::vector<std::vector<unsigned char>> joiner = fixture.m_output.For(2);
  EXPECT_EQ(joiner.size(), 2u);
  EXPECT_EQ(joiner.at(0).at(0), relay::MSG_WELCOME);
  EXPECT_EQ(joiner.at(0).at(9), 3);
  EXPECT_EQ(joiner.at(0).at(10), 0x09);
  EXPECT_EQ(joiner.at(0).at(11), 0);
  EXPECT_TRUE(IsMessage(joiner.at(1), relay::MSG_ROSTER, 0x09, 0));
  std::vector<std::vector<unsigned char>> host = fixture.m_output.For(1);
  EXPECT_EQ(host.size(), 1u);
  EXPECT_TRUE(IsMessage(host.at(0), relay::MSG_ROSTER, 0x09, 0));

  // The slot is taken, so the same token is refused.
  Join(fixture, manager, 3, created, 3, created.m_tokens[3]);
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 3), static_cast<unsigned>(relay::REFUSE_SLOT_TAKEN));
  fixture.m_output.Clear();

  Join(fixture, manager, 4, created, 2, created.m_tokens[2]);
  // The last expected slot arrived: each of the three players gets ROSTER and then one START.
  const std::uint32_t PLAYERS[] = {1, 2, 4};
  for (std::uint32_t connection : PLAYERS) {
    std::vector<std::vector<unsigned char>> messages = fixture.m_output.For(connection);
    std::size_t starts = 0;
    for (const std::vector<unsigned char> &message : messages) {
      if (message.at(0) == relay::MSG_START) {
        ++starts;
        EXPECT_TRUE(IsMessage(message, relay::MSG_START, 0x0D, 0));
      }
    }
    EXPECT_EQ(starts, 1u);
    EXPECT_TRUE(messages.back().at(0) == relay::MSG_START);
  }

  // A late joiner is refused because the match started.
  fixture.m_output.Clear();
  Join(fixture, manager, 5, created, 2, created.m_tokens[2]);
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 5), static_cast<unsigned>(relay::REFUSE_ALREADY_STARTED));

  // The timer does not start an already started session again.
  fixture.m_output.Clear();
  manager.Tick(fixture.m_now_ms + relay::SESSION_START_WAIT_MS + 1);
  EXPECT_EQ(fixture.m_output.m_sent.size(), 0u);
}

void TestTokenUsedOnceAfterLeave(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x07, 0, created, 1));
  Join(fixture, manager, 2, created, 1, created.m_tokens[1]);
  manager.OnLeave(2);
  fixture.m_output.Clear();
  Join(fixture, manager, 3, created, 1, created.m_tokens[1]);
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 3), static_cast<unsigned>(relay::REFUSE_BAD_TOKEN));
}

void TestPassInUse(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x03, 0, created, 1)); // The host used pass id 1.

  // Another connection presents a pass with the same id, for a CREATE and for a JOIN.
  std::vector<unsigned char> same_pass = fixture.Pass(1);
  std::vector<unsigned char> create = fixture.CreateBody(same_pass, 0, 0x03);
  manager.OnCreate(fixture.Request(2), create.data(), create.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 2), static_cast<unsigned>(relay::REFUSE_PASS_IN_USE));
  std::vector<unsigned char> join = fixture.JoinBody(same_pass, created.m_session_id, 1, created.m_tokens[1]);
  manager.OnJoin(fixture.Request(3), join.data(), join.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 3), static_cast<unsigned>(relay::REFUSE_PASS_IN_USE));

  // When the holder closes, the pass id is free again and the empty session is gone.
  manager.OnClosed(1);
  EXPECT_EQ(manager.SessionCount(), 0u);
  fixture.m_output.Clear();
  manager.OnCreate(fixture.Request(4), create.data(), create.size());
  Created again;
  EXPECT_TRUE(ReadCreated(fixture.m_output, 4, again));
}

void TestStartTimeout(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x0F, 0, created, 1)); // Slots 0..3 expected.
  Join(fixture, manager, 2, created, 2, created.m_tokens[2]);
  fixture.m_output.Clear();

  manager.Tick(fixture.m_now_ms + relay::SESSION_START_WAIT_MS - 1);
  EXPECT_EQ(fixture.m_output.m_sent.size(), 0u);

  manager.Tick(fixture.m_now_ms + relay::SESSION_START_WAIT_MS);
  std::vector<std::vector<unsigned char>> host = fixture.m_output.For(1);
  // DROP slot 1, DROP slot 3, then START with mask 0b0101 and leader 0.
  EXPECT_EQ(host.size(), 3u);
  if (host.size() == 3) {
    for (std::size_t i = 0; i < 2; ++i) {
      EXPECT_EQ(host.at(i).size(), 1 + relay::DROP_BODY_SIZE);
      EXPECT_EQ(host.at(i).at(0), relay::MSG_DROP);
      EXPECT_EQ(host.at(i).at(1), i == 0 ? 1 : 3);
      EXPECT_EQ(host.at(i).at(2), relay::DROP_NEVER_JOINED);
      EXPECT_EQ(relay::GetU32(host.at(i).data() + 3), 29u);
    }
    EXPECT_TRUE(IsMessage(host.at(2), relay::MSG_START, 0x05, 0));
  }
  EXPECT_TRUE(fixture.m_output.For(2) == host);

  // Sent once only, and joining is closed.
  fixture.m_output.Clear();
  manager.Tick(fixture.m_now_ms + (2 * relay::SESSION_START_WAIT_MS));
  EXPECT_EQ(fixture.m_output.m_sent.size(), 0u);
  Join(fixture, manager, 3, created, 1, created.m_tokens[1]);
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 3), static_cast<unsigned>(relay::REFUSE_ALREADY_STARTED));
}

void TestStartTimeoutNotEnoughPlayers(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x07, 0, created, 1));
  fixture.m_output.Clear();
  manager.Tick(fixture.m_now_ms + relay::SESSION_START_WAIT_MS);
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 1), static_cast<unsigned>(relay::REFUSE_NOT_ENOUGH_PLAYERS));
  EXPECT_TRUE(fixture.m_output.WasClosed(1));
  EXPECT_EQ(manager.SessionCount(), 0u);
  EXPECT_TRUE(!manager.IsBound(1));
}

void TestLeaderAndLeaving(const relay::RegionKey &key) {
  EXPECT_EQ(relay::LeaderSlot(0x01), 0);
  EXPECT_EQ(relay::LeaderSlot(0x28), 3);
  EXPECT_EQ(relay::LeaderSlot(0x80), 7);

  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x2C, 5, created, 1)); // Slots 2, 3, 5. Host in slot 5.
  Join(fixture, manager, 2, created, 3, created.m_tokens[3]);
  std::vector<std::vector<unsigned char>> host = fixture.m_output.For(1);
  // The lowest connected slot leads, not the host.
  EXPECT_TRUE(IsMessage(host.back(), relay::MSG_ROSTER, 0x28, 3));

  fixture.m_output.Clear();
  Join(fixture, manager, 3, created, 2, created.m_tokens[2]);
  host = fixture.m_output.For(1);
  EXPECT_TRUE(IsMessage(host.at(0), relay::MSG_ROSTER, 0x2C, 2));

  // The leader leaves: the next lowest slot leads.
  fixture.m_output.Clear();
  manager.OnLeave(3);
  EXPECT_TRUE(fixture.m_output.WasClosed(3));
  host = fixture.m_output.For(1);
  EXPECT_EQ(host.size(), 1u);
  EXPECT_TRUE(IsMessage(host.at(0), relay::MSG_ROSTER, 0x28, 3));

  // Everyone gone: the session and its bindings are released.
  manager.OnClosed(1);
  manager.OnClosed(2);
  EXPECT_EQ(manager.SessionCount(), 0u);
  EXPECT_TRUE(!manager.IsBound(1) && !manager.IsBound(2) && !manager.IsBound(3));
}

void TestSessionAndAddressLimits(const relay::RegionKey &key) {
  SessionFixture limited(key, 2);
  relay::SessionManager manager(limited.m_settings, limited.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(limited, manager, 0x03, 0, created, 1));
  EXPECT_TRUE(MakeSession(limited, manager, 0x03, 0, created, 2));
  limited.m_output.Clear();
  std::vector<unsigned char> body = limited.CreateBody(limited.Pass(900), 0, 0x03);
  manager.OnCreate(limited.Request(3, TEST_IP + 1), body.data(), body.size());
  EXPECT_EQ(RefuseCodeOf(limited.m_output, 3), static_cast<unsigned>(relay::REFUSE_SERVER_BUSY));

  // CREATE per address: the seventh within a minute is refused, and a minute later it works again.
  SessionFixture fixture(key);
  relay::SessionManager creates(fixture.m_settings, fixture.m_output);
  for (std::uint32_t connection = 1; connection <= relay::CREATE_PER_ADDRESS_PER_MINUTE; ++connection) {
    EXPECT_TRUE(MakeSession(fixture, creates, 0x03, 0, created, connection));
  }
  fixture.m_output.Clear();
  std::vector<unsigned char> seventh = fixture.CreateBody(fixture.Pass(901), 0, 0x03);
  creates.OnCreate(fixture.Request(100), seventh.data(), seventh.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 100), static_cast<unsigned>(relay::REFUSE_RATE_LIMITED));
  fixture.m_now_ms += relay::RATE_WINDOW_MS;
  fixture.m_output.Clear();
  creates.OnCreate(fixture.Request(101), seventh.data(), seventh.size());
  EXPECT_TRUE(ReadCreated(fixture.m_output, 101, created));

  // JOIN per address: the thirteenth within a minute is refused, whatever the reason would have been.
  SessionFixture joins(key);
  relay::SessionManager join_manager(joins.m_settings, joins.m_output);
  EXPECT_TRUE(MakeSession(joins, join_manager, 0x03, 0, created, 1));
  joins.m_output.Clear();
  for (std::uint32_t i = 0; i < relay::JOIN_PER_ADDRESS_PER_MINUTE; ++i) {
    Join(joins, join_manager, 200 + i, created, 1, 12345);
    EXPECT_EQ(RefuseCodeOf(joins.m_output, 200 + i), static_cast<unsigned>(relay::REFUSE_BAD_TOKEN));
  }
  Join(joins, join_manager, 300, created, 1, created.m_tokens[1]);
  EXPECT_EQ(RefuseCodeOf(joins.m_output, 300), static_cast<unsigned>(relay::REFUSE_RATE_LIMITED));
  EXPECT_TRUE(!join_manager.IsBound(300));
}

void TestFailedJoinsPerSession(const relay::RegionKey &key) {
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  Created created;
  EXPECT_TRUE(MakeSession(fixture, manager, 0x03, 0, created, 1));
  fixture.m_output.Clear();

  // Failed joins come from different addresses so the per-address limit does not hide the per-session one.
  for (std::uint32_t i = 0; i < relay::FAILED_JOINS_PER_SESSION; ++i) {
    std::vector<unsigned char> pass = fixture.Pass(fixture.m_next_pass_id++);
    std::vector<unsigned char> body = fixture.JoinBody(pass, created.m_session_id, 1, 777);
    manager.OnJoin(fixture.Request(1000 + i, 0x0B000000 + i), body.data(), body.size());
    EXPECT_EQ(RefuseCodeOf(fixture.m_output, 1000 + i), static_cast<unsigned>(relay::REFUSE_BAD_TOKEN));
  }
  // Now even the right token is refused.
  std::vector<unsigned char> pass = fixture.Pass(fixture.m_next_pass_id++);
  std::vector<unsigned char> body = fixture.JoinBody(pass, created.m_session_id, 1, created.m_tokens[1]);
  manager.OnJoin(fixture.Request(2000, 0x0C000000), body.data(), body.size());
  EXPECT_EQ(RefuseCodeOf(fixture.m_output, 2000), static_cast<unsigned>(relay::REFUSE_RATE_LIMITED));
}

void TestRateLimiter() {
  relay::AddressRateLimiter limiter(2, 1000);
  EXPECT_TRUE(limiter.Allow(1, 0));
  EXPECT_TRUE(limiter.Allow(1, 10));
  EXPECT_TRUE(!limiter.Allow(1, 20));
  EXPECT_TRUE(limiter.Allow(2, 20));
  EXPECT_TRUE(limiter.Allow(1, 1000)); // The first event left the window.
  EXPECT_TRUE(!limiter.Allow(1, 1005));
  EXPECT_EQ(limiter.TrackedAddresses(), 2u);
  EXPECT_TRUE(limiter.Allow(3, 5000)); // Also sweeps the addresses that have no event inside the window.
  EXPECT_EQ(limiter.TrackedAddresses(), 1u);
}

void TestRandomValuesDiffer(const relay::RegionKey &key) {
  // Six sessions give six session ids and forty-two join tokens, none zero and none repeated.
  SessionFixture fixture(key);
  relay::SessionManager manager(fixture.m_settings, fixture.m_output);
  std::vector<std::uint64_t> values;
  for (std::uint32_t i = 0; i < 6; ++i) {
    Created created;
    EXPECT_TRUE(MakeSession(fixture, manager, 0xFF, 0, created, 1 + i));
    values.push_back(created.m_session_id);
    for (std::size_t slot = 1; slot < relay::SLOT_COUNT; ++slot) {
      values.push_back(created.m_tokens[slot]);
    }
  }
  for (std::size_t a = 0; a < values.size(); ++a) {
    EXPECT_TRUE(values.at(a) != 0);
    for (std::size_t b = a + 1; b < values.size(); ++b) {
      EXPECT_TRUE(values.at(a) != values.at(b));
    }
  }
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
  TestPingRoundVerdicts();
  TestPingRoundRejectsBadPongs();
  TestPingRoundSchedule();
  TestMedianWithFewAnswers();
  TestAddressRateLimit();
  TestVpnHookAllowsEveryoneForNow();
  TestIssuePass(key);
  TestRateLimiter();
  TestCreateValidation(key);
  TestCreateAcceptedAndRandom(key);
  TestJoinFailures(key);
  TestJoinFlowRosterAndStart(key);
  TestTokenUsedOnceAfterLeave(key);
  TestPassInUse(key);
  TestStartTimeout(key);
  TestStartTimeoutNotEnoughPlayers(key);
  TestLeaderAndLeaving(key);
  TestSessionAndAddressLimits(key);
  TestFailedJoinsPerSession(key);
  TestRandomValuesDiffer(key);

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
