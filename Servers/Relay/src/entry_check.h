// Entry check: ping measurement, verdict, pass issue, rate limit and the VPN hook (PROTOCOL.md 4.4 and 6).
// Nothing here uses the network. host.cpp sends the messages and feeds in the times.
#pragma once

#include "pass.h"
#include "region_keys.h"

#include <cstddef>
#include <cstdint>
#include <map>
#include <vector>

namespace relay {

// PROTOCOL.md 4.4.
const std::size_t PING_COUNT = 8;
const std::uint32_t PING_INTERVAL_MS = 250;
const std::uint32_t ANSWER_WINDOW_MS = 4000;
const std::size_t MIN_ANSWERS = 5;

// PROTOCOL.md 6.
const std::uint32_t RATE_WINDOW_MS = 60000;
const std::size_t ENTRY_CHECKS_PER_WINDOW = 6;

// Values the entry check takes from the relay configuration and the region key.
struct EntryCheckSettings {
  std::uint16_t region = 0;
  std::uint16_t ping_limit_ms = 0;
  RegionKey key;
};

// Outcome of an entry check after its answer window closes. PASS means a pass may be issued.
enum class EntryVerdict { PASS, TIMEOUT, PING_TOO_HIGH };

// One ping measurement. Times are relay milliseconds from enet_time_get(), so differences wrap correctly.
class PingRound {
public:
  // Starts the measurement. The first PING is due at once.
  void Start(std::uint32_t now_ms);

  // True when the next PING should be sent now.
  bool PingDue(std::uint32_t now_ms) const;

  // Records that a PING with this nonce was sent now.
  void SentPing(std::uint64_t nonce, std::uint32_t now_ms);

  // Handles a PONG. Returns false, and changes nothing, when the nonce is not one of the sent pings, was already
  // answered, or arrives after the answer window.
  bool OnPong(std::uint64_t nonce, std::uint32_t now_ms);

  // True when the answer window has closed.
  bool Expired(std::uint32_t now_ms) const;

  // Milliseconds until the next PING is due, or until the window closes once all pings are sent.
  std::uint32_t MsUntilNextAction(std::uint32_t now_ms) const;

  // The verdict once the window has closed. median_ms is set when the verdict is PASS or PING_TOO_HIGH.
  // The median of an even number of answers is the average of the two middle values, rounded down.
  EntryVerdict Verdict(std::uint16_t limit_ms, std::uint16_t &median_ms) const;

private:
  struct Ping {
    std::uint64_t m_nonce = 0;
    std::uint32_t m_sent_ms = 0;
    std::uint32_t m_rtt_ms = 0;
    bool m_answered = false;
  };

  std::uint32_t ElapsedMs(std::uint32_t now_ms) const;

  Ping m_pings[PING_COUNT];
  std::size_t m_sent = 0;
  std::uint32_t m_start_ms = 0;
};

// Counts entry checks per source address in a sliding window (PROTOCOL.md 6).
class AddressRateLimiter {
public:
  // Records an entry check from address at now_ms. Returns false, without recording it, when the address has used
  // its checks for the window.
  bool Allow(std::uint32_t address, std::uint32_t now_ms);

private:
  void Sweep(std::uint32_t now_ms);

  std::map<std::uint32_t, std::vector<std::uint32_t>> m_recent;
  std::uint32_t m_last_sweep_ms = 0;
};

// Hook for the VPN check (package A3). Returns false when the address is flagged. Until A3 is built, every address
// passes.
bool VpnCheckPasses(std::uint32_t address);

// Random 64-bit value from the system's secure random source. Used for PING nonces and pass ids.
std::uint64_t RandomId();

// Fills out, which must hold PASS_SIZE bytes, with a pass signed by the region key. The region, the lifetime and a
// random pass_id are set here. Other fields come from fields. Returns false if signing fails.
bool IssuePass(const EntryCheckSettings &settings, PassFields fields, std::uint32_t now_unix, unsigned char *out);

} // namespace relay
