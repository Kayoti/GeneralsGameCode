// Little-endian integer helpers for the relay's binary messages (PROTOCOL.md section 3).
#pragma once

#include <cstdint>

namespace relay {

inline void PutU16(unsigned char *out, std::uint16_t value) {
  out[0] = static_cast<unsigned char>(value & 0xFF);
  out[1] = static_cast<unsigned char>((value >> 8) & 0xFF);
}

inline void PutU32(unsigned char *out, std::uint32_t value) {
  for (int i = 0; i < 4; ++i) {
    out[i] = static_cast<unsigned char>((value >> (8 * i)) & 0xFF);
  }
}

inline void PutU64(unsigned char *out, std::uint64_t value) {
  for (int i = 0; i < 8; ++i) {
    out[i] = static_cast<unsigned char>((value >> (8 * i)) & 0xFF);
  }
}

inline std::uint16_t GetU16(const unsigned char *in) {
  return static_cast<std::uint16_t>(in[0] | (in[1] << 8));
}

inline std::uint32_t GetU32(const unsigned char *in) {
  std::uint32_t value = 0;
  for (int i = 3; i >= 0; --i) {
    value = (value << 8) | in[i];
  }
  return value;
}

inline std::uint64_t GetU64(const unsigned char *in) {
  std::uint64_t value = 0;
  for (int i = 7; i >= 0; --i) {
    value = (value << 8) | in[i];
  }
  return value;
}

} // namespace relay
