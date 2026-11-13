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
const std::size_t REFUSE_BODY_SIZE = 1 + 2 + 2;               // code, rtt_ms, limit_ms
const std::size_t CREATE_BODY_SIZE = 2 + PASS_SIZE + 1 + 1;   // proto, pass, host_slot, slot_mask
const std::size_t JOIN_BODY_SIZE = 2 + PASS_SIZE + 8 + 1 + 8; // proto, pass, session_id, slot, join_token

// Body sizes of the messages the relay sends for a session (PROTOCOL.md 4.2), not counting the type byte.
const std::size_t SLOT_COUNT = 8;                         // Players per session (PROTOCOL.md 6).
const std::size_t CREATED_BODY_SIZE = 8 + SLOT_COUNT * 8; // session_id, join_token per slot
const std::size_t WELCOME_BODY_SIZE = 8 + 1 + 1 + 1;      // session_id, slot, connected_mask, leader_slot
const std::size_t ROSTER_BODY_SIZE = 1 + 1;               // connected_mask, leader_slot
const std::size_t START_BODY_SIZE = 1 + 1;                // connected_mask, leader_slot
const std::size_t DROP_BODY_SIZE = 1 + 1 + 4;             // slot, reason, last_frame

// Session limits (PROTOCOL.md 4.5, 4.7 and 6).
const std::uint32_t SESSION_START_WAIT_MS = 30000; // Wait for all players after CREATED.
const std::uint32_t NEVER_JOINED_LAST_FRAME = 29;  // last_frame of a slot that never sent a FRAMEINFO.
const std::size_t CREATE_PER_ADDRESS_PER_MINUTE = 6;
const std::size_t JOIN_PER_ADDRESS_PER_MINUTE = 12;
const std::uint32_t RATE_WINDOW_MS = 60000;
const std::size_t FAILED_JOINS_PER_SESSION = 32;
const std::size_t UNCHECKED_CONNECTIONS_PER_ADDRESS = 4; // Connections that have not sent a first message.

// PROTOCOL.md 4.1 and 6.
const std::uint32_t CHANNEL_COUNT = 2;
const std::uint32_t FIRST_MESSAGE_DEADLINE_MS = 5000;
// The ENet host's maximum packet size and waiting-data limits in host.cpp are derived from MAX_MESSAGE_SIZE.
const std::size_t MAX_MESSAGE_SIZE = 16384; // DATA message limit (PROTOCOL.md 6).
// PROTOCOL.md 6 allows 256 connections in total that have not sent a first message. The ENet host has this many
// peer slots in addition to SLOT_COUNT for every allowed session.
const std::size_t MAX_UNCHECKED_CONNECTIONS = 256;

// Match records (PROTOCOL.md 4.6 and 6).
const std::size_t MIN_RECORD_SIZE = 7;
const std::size_t MAX_RECORD_SIZE = 4096;
const std::uint32_t INITIAL_NEXT_FRAME =
    30;                                 // The engine starts with a run-ahead of 30 and treats frames 1 to 29 as empty.
const std::uint32_t FRAME_WINDOW = 128; // F may be next_frame to next_frame + FRAME_WINDOW.
const std::size_t FAILED_RECORDS_BEFORE_DROP = 32;
const std::uint32_t DATA_RATE_WINDOW_MS = 1000;
const std::size_t MAX_DATA_BYTES_PER_SECOND = 64 * 1024;
const std::size_t MAX_RECORDS_PER_SECOND = 600;
const std::uint16_t MIN_RUN_AHEAD = 4;
const std::uint16_t MAX_RUN_AHEAD = 64;
const std::uint8_t MIN_FRAME_RATE = 5;
const std::uint8_t MAX_FRAME_RATE = 60;

// Command types the relay carries. The values are the engine's NetCommandType values, which are the byte after the
// 'T' tag of a record (NetworkDefs.h).
enum CommandType : std::uint8_t {
  COMMAND_FRAMEINFO = 3,
  COMMAND_GAMECOMMAND = 4,
  COMMAND_PLAYERLEAVE = 5,
  COMMAND_RUNAHEADMETRICS = 6,
  COMMAND_RUNAHEAD = 7,
  COMMAND_CHAT = 11,
  COMMAND_PROGRESS = 14,
  COMMAND_LOADCOMPLETE = 15,
  COMMAND_TIMEOUTSTART = 16,
};

} // namespace relay
