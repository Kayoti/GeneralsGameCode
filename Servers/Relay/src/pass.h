// Entry pass: build, sign, parse and check (PROTOCOL.md section 4.3).
#pragma once

#include "protocol.h"

#include <cstddef>
#include <cstdint>

namespace relay {

// Value of the pass version field (PROTOCOL.md 4.3).
const std::uint8_t PASS_VERSION = 1;

// Values of the pass fields, before encoding.
struct PassFields {
  std::uint8_t version = PASS_VERSION;
  std::uint16_t region = 0;
  std::uint32_t client_ip = 0; // IPv4 address as a host-order integer, for example 0x0A000001 for 10.0.0.1.
  std::uint32_t exe_crc = 0;
  std::uint32_t ini_crc = 0;
  std::uint16_t rtt_ms = 0;
  std::uint32_t issued_at = 0;
  std::uint32_t expires_at = 0;
  std::uint64_t pass_id = 0;
};

struct PassCheckContext {
  const unsigned char *public_key = nullptr; // Region public key, crypto_sign_PUBLICKEYBYTES bytes.
  std::uint16_t region = 0;                  // This relay's region.
  std::uint32_t source_ip = 0;               // Source address of the UDP packet, in the same encoding as client_ip.
  bool check_address = true;                 // False only with the relay's test start option.
  std::uint32_t now = 0;                     // Relay clock, Unix seconds.
};

// Lifetime of a pass, from issue to expiry (PROTOCOL.md 4.3).
const std::uint32_t PASS_LIFETIME_SECONDS = 600;

// Encodes fields into the 100-byte pass in out and signs it with the region secret key
// (crypto_sign_SECRETKEYBYTES bytes). Returns false if signing fails.
bool BuildPass(const PassFields &fields, const unsigned char *secret_key, unsigned char *out);

// Checks a received pass of length bytes and decodes it into fields. The checks follow PROTOCOL.md 4.3, in this
// order: length, signature, version, reserved bytes, region, expiry, address. Returns true when the pass is valid.
// Otherwise returns false and sets refuse to the REFUSE code to send.
//
// The check that pass_id is not used by another live match connection needs the session registry, so the caller
// makes it on fields.pass_id after this function returns true.
bool CheckPass(const unsigned char *data, std::size_t length, const PassCheckContext &context, PassFields &fields,
               RefuseCode &refuse);

} // namespace relay
