#include "entry_check.h"

#include "pass.h"
#include "protocol.h"

#include <algorithm>
#include <cstddef>
#include <cstdint>
#include <sodium.h>

namespace relay {

namespace {

// Removes the timestamps that are no longer inside the rate window. Timestamps are stored in increasing order.
void DropExpired(std::vector<std::uint32_t> &times, std::uint32_t now_ms) {
  auto first_live =
      std::find_if(times.begin(), times.end(), [now_ms](std::uint32_t time) { return now_ms - time < RATE_WINDOW_MS; });
  times.erase(times.begin(), first_live);
}

} // namespace

void PingRound::Start(std::uint32_t now_ms) {
  m_start_ms = now_ms;
  m_sent = 0;
}

std::uint32_t PingRound::ElapsedMs(std::uint32_t now_ms) const {
  return now_ms - m_start_ms;
}

bool PingRound::PingDue(std::uint32_t now_ms) const {
  return m_sent < PING_COUNT && ElapsedMs(now_ms) >= static_cast<std::uint32_t>(m_sent) * PING_INTERVAL_MS;
}

void PingRound::SentPing(std::uint64_t nonce, std::uint32_t now_ms) {
  if (m_sent >= PING_COUNT) {
    return;
  }
  m_pings[m_sent].m_nonce = nonce;
  m_pings[m_sent].m_sent_ms = now_ms;
  ++m_sent;
}

bool PingRound::OnPong(std::uint64_t nonce, std::uint32_t now_ms) {
  if (ElapsedMs(now_ms) >= ANSWER_WINDOW_MS) {
    return false;
  }
  for (std::size_t i = 0; i < m_sent; ++i) {
    Ping &ping = m_pings[i];
    if (ping.m_answered || ping.m_nonce != nonce) {
      continue;
    }
    ping.m_answered = true;
    ping.m_rtt_ms = now_ms - ping.m_sent_ms;
    return true;
  }
  return false;
}

bool PingRound::Expired(std::uint32_t now_ms) const {
  return ElapsedMs(now_ms) >= ANSWER_WINDOW_MS;
}

std::uint32_t PingRound::MsUntilNextAction(std::uint32_t now_ms) const {
  std::uint32_t elapsed = ElapsedMs(now_ms);
  std::uint32_t due = m_sent < PING_COUNT ? static_cast<std::uint32_t>(m_sent) * PING_INTERVAL_MS : ANSWER_WINDOW_MS;
  return due > elapsed ? due - elapsed : 0;
}

EntryVerdict PingRound::Verdict(std::uint16_t limit_ms, std::uint16_t &median_ms) const {
  std::uint32_t answers[PING_COUNT] = {};
  std::size_t count = 0;
  for (std::size_t i = 0; i < m_sent; ++i) {
    if (m_pings[i].m_answered) {
      answers[count++] = m_pings[i].m_rtt_ms;
    }
  }
  if (count < MIN_ANSWERS) {
    return EntryVerdict::TIMEOUT;
  }

  std::sort(answers, answers + count);
  std::size_t upper = count / 2;
  std::uint32_t middle = count % 2 == 1 ? answers[upper] : (answers[upper - 1] + answers[upper]) / 2;
  median_ms = static_cast<std::uint16_t>(middle);
  return median_ms > limit_ms ? EntryVerdict::PING_TOO_HIGH : EntryVerdict::PASS;
}

bool AddressRateLimiter::Allow(std::uint32_t address, std::uint32_t now_ms) {
  if (now_ms - m_last_sweep_ms >= RATE_WINDOW_MS) {
    Sweep(now_ms);
  }

  std::vector<std::uint32_t> &times = m_recent[address];
  DropExpired(times, now_ms);
  if (times.size() >= ENTRY_CHECKS_PER_WINDOW) {
    return false;
  }
  times.push_back(now_ms);
  return true;
}

void AddressRateLimiter::Sweep(std::uint32_t now_ms) {
  for (auto it = m_recent.begin(); it != m_recent.end();) {
    DropExpired(it->second, now_ms);
    if (it->second.empty()) {
      it = m_recent.erase(it);
    } else {
      ++it;
    }
  }
  m_last_sweep_ms = now_ms;
}

bool VpnCheckPasses(std::uint32_t address) {
  static_cast<void>(address);
  return true;
}

std::uint64_t RandomId() {
  std::uint64_t value = 0;
  randombytes_buf(&value, sizeof(value));
  return value;
}

bool IssuePass(const EntryCheckSettings &settings, PassFields fields, std::uint32_t now_unix, unsigned char *out) {
  fields.version = PASS_VERSION;
  fields.region = settings.region;
  fields.issued_at = now_unix;
  fields.expires_at = now_unix + PASS_LIFETIME_SECONDS;
  fields.pass_id = RandomId();
  return BuildPass(fields, settings.key.secret, out);
}

} // namespace relay
