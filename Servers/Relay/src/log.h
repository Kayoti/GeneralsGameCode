// Console logging for the relay. Never pass passes, signing keys or full IP addresses here.
#pragma once

#include <cstdarg>
#include <cstdio>

namespace relay {

inline void LogLine(const char *format, ...) {
  va_list args;
  va_start(args, format);
  std::vfprintf(stdout, format, args);
  va_end(args);
  std::fputc('\n', stdout);
  std::fflush(stdout);
}

inline void LogError(const char *format, ...) {
  va_list args;
  va_start(args, format);
  std::vfprintf(stderr, format, args);
  va_end(args);
  std::fputc('\n', stderr);
  std::fflush(stderr);
}

} // namespace relay
