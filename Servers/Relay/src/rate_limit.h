// Per-address event limit over a sliding time window (PROTOCOL.md section 6).
#pragma once

#include <cstddef>
#include <cstdint>
#include <deque>
#include <unordered_map>

namespace relay {

class AddressRateLimiter {
public:
  // At most max_events events per address within any window of window_ms milliseconds.
  AddressRateLimiter(std::size_t max_events, std::uint32_t window_ms);

  // Records an event from address at now_ms. Returns false, and records nothing, when the address is at its limit.
  // Once per window, addresses with no event inside the window are forgotten, so memory follows recent senders only.
  bool Allow(std::uint32_t address, std::uint32_t now_ms);

  // Number of addresses currently remembered.
  std::size_t TrackedAddresses() const;

private:
  void DropExpired(std::deque<std::uint32_t> &times, std::uint32_t now_ms) const;
  void Sweep(std::uint32_t now_ms);

  std::size_t max_events_;
  std::uint32_t window_ms_;
  std::uint32_t last_sweep_ms_ = 0;
  std::unordered_map<std::uint32_t, std::deque<std::uint32_t>> events_;
};

} // namespace relay
