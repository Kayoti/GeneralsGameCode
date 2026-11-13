// Small text helpers shared by the relay's configuration and key code.
#pragma once

#include <cstddef>
#include <string>

namespace relay {

// Returns text without leading and trailing spaces, tabs and line endings.
inline std::string TrimText(const std::string &text) {
  const std::string blanks = " \t\r\n";
  const std::size_t first = text.find_first_not_of(blanks);
  if (first == std::string::npos) {
    return std::string();
  }
  const std::size_t last = text.find_last_not_of(blanks);
  return text.substr(first, last - first + 1);
}

} // namespace relay
