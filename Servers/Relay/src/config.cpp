#include "config.h"

#include "text.h"

#include <algorithm>
#include <cstddef>
#include <cstdlib>
#include <fstream>
#include <iterator>
#include <set>
#include <sstream>
#include <vector>

namespace relay {

namespace {

const std::size_t MAX_CONFIG_BYTES = 64 * 1024;
const std::size_t MAX_LINE_BYTES = 512;
const std::size_t MAX_PATH_BYTES = 260;
const std::size_t MAX_NUMBER_DIGITS = 5;
const unsigned long MAX_NUMBER_VALUE = 65535;

std::string LineError(std::size_t line_number, const std::string &message) {
  std::ostringstream out;
  out << "line " << line_number << ": " << message;
  return out.str();
}

// Accepts only plain decimal digits in the range 1 to 65535.
bool IsDigit(char c) {
  return c >= '0' && c <= '9';
}

bool ParseNumber(const std::string &key, const std::string &value, std::uint16_t &target, std::string &error) {
  bool valid = !value.empty() && value.size() <= MAX_NUMBER_DIGITS && std::all_of(value.begin(), value.end(), IsDigit);
  unsigned long number = 0;
  if (valid) {
    number = std::strtoul(value.c_str(), nullptr, 10);
    valid = number >= 1 && number <= MAX_NUMBER_VALUE;
  }
  if (!valid) {
    error = "key '" + key + "' must be a whole number from 1 to 65535";
    return false;
  }
  target = static_cast<std::uint16_t>(number);
  return true;
}

bool ParsePath(const std::string &key, const std::string &value, std::string &target, std::string &error) {
  if (value.size() > MAX_PATH_BYTES) {
    error = "key '" + key + "' is longer than 260 characters";
    return false;
  }
  target = value;
  return true;
}

bool ParseSwitch(const std::string &key, const std::string &value, bool &target, std::string &error) {
  if (value == "on") {
    target = true;
    return true;
  }
  if (value == "off") {
    target = false;
    return true;
  }
  error = "key '" + key + "' must be 'on' or 'off'";
  return false;
}

// Stores one key/value pair. Returns false, with error set, when the key is unknown or the value is invalid.
bool ApplyKey(const std::string &key, const std::string &value, RelayConfig &config, std::string &error) {
  if (key == "region") {
    return ParseNumber(key, value, config.region, error);
  }
  if (key == "listen_port") {
    return ParseNumber(key, value, config.listen_port, error);
  }
  if (key == "ping_limit_ms") {
    return ParseNumber(key, value, config.ping_limit_ms, error);
  }
  if (key == "max_sessions") {
    return ParseNumber(key, value, config.max_sessions, error);
  }
  if (key == "key_file") {
    return ParsePath(key, value, config.key_file, error);
  }
  if (key == "vpn_check") {
    return ParseSwitch(key, value, config.vpn_check, error);
  }
  if (key == "vpn_ranges") {
    return ParsePath(key, value, config.vpn_ranges, error);
  }
  if (key == "vpn_operators") {
    return ParsePath(key, value, config.vpn_operators, error);
  }
  if (key == "vpn_operator_db") {
    return ParsePath(key, value, config.vpn_operator_db, error);
  }
  if (key == "vpn_allow") {
    return ParsePath(key, value, config.vpn_allow, error);
  }
  error = "unknown key '" + key + "'";
  return false;
}

} // namespace

bool ParseRelayConfig(const std::string &text, RelayConfig &config, std::string &error) {
  std::vector<std::string> required = {
      "region",    "listen_port", "ping_limit_ms", "key_file",        "max_sessions",
      "vpn_check", "vpn_ranges",  "vpn_operators", "vpn_operator_db", "vpn_allow",
  };

  RelayConfig parsed;
  std::set<std::string> seen;
  std::size_t line_number = 0;
  std::size_t start = 0;

  while (start <= text.size()) {
    std::size_t end = text.find('\n', start);
    if (end == std::string::npos) {
      end = text.size();
    }
    std::string line = text.substr(start, end - start);
    start = end + 1;
    ++line_number;

    if (line.size() > MAX_LINE_BYTES) {
      error = LineError(line_number, "line is too long");
      return false;
    }

    std::string trimmed = TrimText(line);
    if (trimmed.empty() || trimmed.front() == '#') {
      continue;
    }

    std::size_t equals = trimmed.find('=');
    if (equals == std::string::npos) {
      error = LineError(line_number, "expected 'key = value'");
      return false;
    }

    std::string key = TrimText(trimmed.substr(0, equals));
    std::string value = TrimText(trimmed.substr(equals + 1));
    if (key.empty() || value.empty()) {
      error = LineError(line_number, "key and value must not be empty");
      return false;
    }
    if (seen.count(key) != 0) {
      error = LineError(line_number, "key '" + key + "' is set twice");
      return false;
    }

    std::string key_error;
    if (!ApplyKey(key, value, parsed, key_error)) {
      error = LineError(line_number, key_error);
      return false;
    }
    seen.insert(key);
  }

  for (const std::string &name : required) {
    if (seen.count(name) == 0) {
      error = "missing key '" + name + "'";
      return false;
    }
  }

  config = parsed;
  return true;
}

bool LoadRelayConfig(const std::string &path, RelayConfig &config, std::string &error) {
  std::ifstream file(path.c_str(), std::ios::in | std::ios::binary);
  if (!file) {
    error = "cannot open configuration file '" + path + "'";
    return false;
  }

  file.seekg(0, std::ios::end);
  std::streamoff size = file.tellg();
  if (size < 0 || static_cast<std::size_t>(size) > MAX_CONFIG_BYTES) {
    error = "configuration file '" + path + "' is larger than 64 KB";
    return false;
  }
  file.seekg(0, std::ios::beg);

  std::string text((std::istreambuf_iterator<char>(file)), std::istreambuf_iterator<char>());

  return ParseRelayConfig(text, config, error);
}

} // namespace relay
