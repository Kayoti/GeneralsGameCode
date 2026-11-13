#include "rate_limit.h"

namespace relay {

AddressRateLimiter::AddressRateLimiter(std::size_t max_events, std::uint32_t window_ms)
    : max_events_(max_events), window_ms_(window_ms) {}

void AddressRateLimiter::DropExpired(std::deque<std::uint32_t> &times, std::uint32_t now_ms) const {
  while (!times.empty() && now_ms - times.front() >= window_ms_) {
    times.pop_front();
  }
}

bool AddressRateLimiter::Allow(std::uint32_t address, std::uint32_t now_ms) {
  if (now_ms - last_sweep_ms_ >= window_ms_) {
    Sweep(now_ms);
  }
  std::deque<std::uint32_t> &times = events_[address];
  DropExpired(times, now_ms);
  if (times.size() >= max_events_) {
    return false;
  }
  times.push_back(now_ms);
  return true;
}

void AddressRateLimiter::Sweep(std::uint32_t now_ms) {
  for (auto it = events_.begin(); it != events_.end();) {
    DropExpired(it->second, now_ms);
    if (it->second.empty()) {
      it = events_.erase(it);
    } else {
      ++it;
    }
  }
  last_sweep_ms_ = now_ms;
}

std::size_t AddressRateLimiter::TrackedAddresses() const {
  return events_.size();
}

} // namespace relay
