// Message types, codes, sizes and limits of the relay link.
// Source of truth: docs/servers/PROTOCOL.md, sections 4.2, 4.3, 4.9 and 6.
#pragma once

#include <cstddef>
#include <cstdint>

namespace relay {

// PROTOCOL.md 2. The relay refuses any other value in CHECK_HELLO, CREATE and JOIN.
const std::uint16_t PROTOCOL_VERSION = 1;

// PROTOCOL.md 4.2. Every message starts with one of these values.
enum MessageType : std::uint8_t {
  MSG_CHECK_HELLO = 1,
  MSG_PING = 2,
  MSG_PONG = 3,
  MSG_PASS = 4,
  MSG_REFUSE = 5,
  MSG_CREATE = 6,
  MSG_CREATED = 7,
  MSG_JOIN = 8,
  MSG_WELCOME = 9,
  MSG_ROSTER = 10,
  MSG_START = 11,
  MSG_DROP = 12,
  MSG_LEAVE = 13,
  MSG_DATA = 16,
  MSG_TIMING = 20,      // Reserved for Phase 3.
  MSG_FRAME_CLOSE = 21, // Reserved for Phase 3.
};

// PROTOCOL.md 4.9. Codes sent in REFUSE.
enum RefuseCode : std::uint8_t {
  REFUSE_PROTO_MISMATCH = 1,
  REFUSE_VPN_DETECTED = 2,
  REFUSE_PING_TOO_HIGH = 3,
  REFUSE_TIMEOUT = 4,
  REFUSE_BAD_PASS = 5,
  REFUSE_EXPIRED = 6,
  REFUSE_WRONG_REGION = 7,
  REFUSE_ADDRESS_MISMATCH = 8,
  REFUSE_PASS_IN_USE = 9,
  REFUSE_BAD_REQUEST = 10,
  REFUSE_NO_SESSION = 11,
  REFUSE_ALREADY_STARTED = 12,
  REFUSE_SLOT_TAKEN = 13,
  REFUSE_NOT_EXPECTED = 14,
  REFUSE_BAD_TOKEN = 15,
  REFUSE_BUILD_MISMATCH = 16,
  REFUSE_NOT_ENOUGH_PLAYERS = 17,
  REFUSE_SERVER_BUSY = 18,
  REFUSE_RATE_LIMITED = 19,
};

// PROTOCOL.md 4.9. Reasons sent in DROP.
enum DropReason : std::uint8_t {
  DROP_LEFT = 1,
  DROP_TIMEOUT = 2,
  DROP_PROTOCOL = 3,
  DROP_NEVER_JOINED = 4,
  DROP_TOO_SLOW = 5, // Phase 3.
};

// Body sizes in bytes, not counting the leading type byte (PROTOCOL.md 4.2 and 4.3).
const std::size_t PASS_SIZE = 100;
const std::size_t CHECK_HELLO_BODY_SIZE = 2 + 4 + 4;          // proto, exe_crc, ini_crc
const std::size_t PING_BODY_SIZE = 8;                         // nonce
const std::size_t PONG_BODY_SIZE = 8;                         // nonce
const std::size_t CREATE_BODY_SIZE = 2 + PASS_SIZE + 1 + 1;   // proto, pass, host_slot, slot_mask
const std::size_t JOIN_BODY_SIZE = 2 + PASS_SIZE + 8 + 1 + 8; // proto, pass, session_id, slot, join_token

// PROTOCOL.md 4.1 and 6.
const std::uint32_t CHANNEL_COUNT = 2;
const std::uint32_t FIRST_MESSAGE_DEADLINE_MS = 5000;
const std::size_t MAX_MESSAGE_SIZE = 16384; // DATA message limit (PROTOCOL.md 6).
// Peer slots in the ENet host. PROTOCOL.md 6 allows 256 connections in total that have not sent a first message.
const std::size_t MAX_PEERS = 256;

} // namespace relay
