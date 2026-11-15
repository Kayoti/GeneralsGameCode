// Relay configuration file (PROTOCOL.md section 7.2).
#pragma once

#include <cstdint>
#include <string>

namespace relay {

struct RelayConfig {
  std::uint16_t region = 0;
  std::uint16_t listen_port = 0;
  std::uint16_t ping_limit_ms = 0;
  std::string key_file;
  std::uint16_t max_sessions = 0;
  bool vpn_check = false;
  std::string vpn_ranges;
  std::string vpn_operators;
  std::string vpn_operator_db;
  std::string vpn_allow;
  // Optional keys, in seconds. They hold the PROTOCOL.md section 6 defaults when the file does not set them.
  std::uint16_t loading_time_s = 120;
  std::uint16_t stall_time_s = 20;
  std::uint16_t connection_timeout_s = 30;
};

// Parses the text of a configuration file. Blank lines and lines starting with '#' are skipped, whitespace
// around keys and values is ignored, and both LF and CRLF line endings are accepted. Every key in section 7.2
// must be present exactly once, and no other key is allowed. On failure, error names the line or the key.
bool ParseRelayConfig(const std::string &text, RelayConfig &config, std::string &error);

// Reads the file at path and parses it with ParseRelayConfig.
bool LoadRelayConfig(const std::string &path, RelayConfig &config, std::string &error);

} // namespace relay
