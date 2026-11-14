#include "pass.h"

#include "wire.h"

#include <sodium.h>

namespace relay {

namespace {

// Byte offsets of the pass fields (PROTOCOL.md 4.3). Integers are little-endian.
const std::size_t VERSION_OFFSET = 0;
const std::size_t RESERVED_OFFSET = 1;
const std::size_t REGION_OFFSET = 2;
const std::size_t CLIENT_IP_OFFSET = 4;
const std::size_t EXE_CRC_OFFSET = 8;
const std::size_t INI_CRC_OFFSET = 12;
const std::size_t RTT_OFFSET = 16;
const std::size_t RESERVED2_OFFSET = 18;
const std::size_t ISSUED_AT_OFFSET = 20;
const std::size_t EXPIRES_AT_OFFSET = 24;
const std::size_t PASS_ID_OFFSET = 28;
const std::size_t SIGNED_SIZE = 36;
const std::size_t SIGNATURE_OFFSET = SIGNED_SIZE;

static_assert(SIGNATURE_OFFSET + crypto_sign_BYTES == PASS_SIZE, "The pass layout must total PASS_SIZE bytes");

} // namespace

bool BuildPass(const PassFields &fields, const unsigned char *secret_key, unsigned char *out) {
  out[VERSION_OFFSET] = fields.version;
  out[RESERVED_OFFSET] = 0;
  PutU16(out + REGION_OFFSET, fields.region);
  PutU32(out + CLIENT_IP_OFFSET, fields.client_ip);
  PutU32(out + EXE_CRC_OFFSET, fields.exe_crc);
  PutU32(out + INI_CRC_OFFSET, fields.ini_crc);
  PutU16(out + RTT_OFFSET, fields.rtt_ms);
  PutU16(out + RESERVED2_OFFSET, 0);
  PutU32(out + ISSUED_AT_OFFSET, fields.issued_at);
  PutU32(out + EXPIRES_AT_OFFSET, fields.expires_at);
  PutU64(out + PASS_ID_OFFSET, fields.pass_id);

  unsigned long long signature_length = 0;
  int result = crypto_sign_detached(out + SIGNATURE_OFFSET, &signature_length, out,
                                    static_cast<unsigned long long>(SIGNED_SIZE), secret_key);
  return result == 0 && signature_length == crypto_sign_BYTES;
}

bool CheckPass(const unsigned char *data, std::size_t length, const PassCheckContext &context, PassFields &fields,
               RefuseCode &refuse) {
  if (length != PASS_SIZE) {
    refuse = REFUSE_BAD_PASS;
    return false;
  }

  int signature_ok = crypto_sign_verify_detached(data + SIGNATURE_OFFSET, data,
                                                 static_cast<unsigned long long>(SIGNED_SIZE), context.public_key);
  if (signature_ok != 0) {
    refuse = REFUSE_BAD_PASS;
    return false;
  }

  fields.version = data[VERSION_OFFSET];
  fields.region = GetU16(data + REGION_OFFSET);
  fields.client_ip = GetU32(data + CLIENT_IP_OFFSET);
  fields.exe_crc = GetU32(data + EXE_CRC_OFFSET);
  fields.ini_crc = GetU32(data + INI_CRC_OFFSET);
  fields.rtt_ms = GetU16(data + RTT_OFFSET);
  fields.issued_at = GetU32(data + ISSUED_AT_OFFSET);
  fields.expires_at = GetU32(data + EXPIRES_AT_OFFSET);
  fields.pass_id = GetU64(data + PASS_ID_OFFSET);

  if (fields.version != PASS_VERSION || data[RESERVED_OFFSET] != 0 || GetU16(data + RESERVED2_OFFSET) != 0) {
    refuse = REFUSE_BAD_PASS;
    return false;
  }
  if (fields.region != context.region) {
    refuse = REFUSE_WRONG_REGION;
    return false;
  }
  if (context.now >= fields.expires_at) {
    refuse = REFUSE_EXPIRED;
    return false;
  }
  if (context.check_address && fields.client_ip != context.source_ip) {
    refuse = REFUSE_ADDRESS_MISMATCH;
    return false;
  }
  return true;
}

} // namespace relay
