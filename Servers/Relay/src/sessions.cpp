#include "sessions.h"

#include "log.h"
#include "pass.h"
#include "wire.h"

#include <sodium.h>
#include <vector>

namespace relay {

namespace {

// CREATE body offsets (PROTOCOL.md 4.2).
const std::size_t CREATE_PROTO_OFFSET = 0;
const std::size_t CREATE_PASS_OFFSET = 2;
const std::size_t CREATE_HOST_SLOT_OFFSET = CREATE_PASS_OFFSET + PASS_SIZE;
const std::size_t CREATE_SLOT_MASK_OFFSET = CREATE_HOST_SLOT_OFFSET + 1;

// JOIN body offsets (PROTOCOL.md 4.2).
const std::size_t JOIN_PROTO_OFFSET = 0;
const std::size_t JOIN_PASS_OFFSET = 2;
const std::size_t JOIN_SESSION_ID_OFFSET = JOIN_PASS_OFFSET + PASS_SIZE;
const std::size_t JOIN_SLOT_OFFSET = JOIN_SESSION_ID_OFFSET + 8;
const std::size_t JOIN_TOKEN_OFFSET = JOIN_SLOT_OFFSET + 1;

static_assert(CREATE_SLOT_MASK_OFFSET + 1 == CREATE_BODY_SIZE, "CREATE offsets must add up to the body size");
static_assert(JOIN_TOKEN_OFFSET + 8 == JOIN_BODY_SIZE, "JOIN offsets must add up to the body size");

// Random value from the operating system's secure source (libsodium's randombytes). sodium_init() must have run.
std::uint64_t RandomU64() {
  unsigned char bytes[8];
  randombytes_buf(bytes, sizeof(bytes));
  return GetU64(bytes);
}

// A session id or join token of 0 means "none", so 0 is never issued.
std::uint64_t RandomNonZeroU64() {
  std::uint64_t value = 0;
  while (value == 0) {
    value = RandomU64();
  }
  return value;
}

bool SlotInMask(std::uint8_t mask, std::size_t bit) {
  return bit < SLOT_COUNT && ((mask >> bit) & 1) != 0;
}

int CountBits(std::uint8_t mask) {
  int count = 0;
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    count += SlotInMask(mask, slot) ? 1 : 0;
  }
  return count;
}

bool TokensEqual(std::uint64_t a, std::uint64_t b) {
  return sodium_memcmp(&a, &b, sizeof(a)) == 0;
}

} // namespace

std::uint8_t LeaderSlot(std::uint8_t connected_mask) {
  std::uint8_t slot = 0;
  while (slot < SLOT_COUNT - 1 && !SlotInMask(connected_mask, slot)) {
    ++slot;
  }
  return slot;
}

struct SessionManager::CheckedPass {
  PassFields m_fields;
};

SessionManager::SessionManager(const SessionSettings &settings, SessionOutput &output)
    : settings_(settings), output_(output), create_limiter_(CREATE_PER_ADDRESS_PER_MINUTE, RATE_WINDOW_MS),
      join_limiter_(JOIN_PER_ADDRESS_PER_MINUTE, RATE_WINDOW_MS), out_buffers_(SLOT_COUNT * MAX_MESSAGE_SIZE) {}

void SessionManager::Refuse(std::uint32_t connection, RefuseCode code) {
  unsigned char message[1 + REFUSE_BODY_SIZE] = {};
  message[0] = MSG_REFUSE;
  message[1] = code;
  // rtt_ms and limit_ms stay 0 because they are used only with PING_TOO_HIGH.
  output_.Send(connection, message, sizeof(message));
  output_.Close(connection);
  LogLine("connection %u refused with code %u", connection, static_cast<unsigned int>(code));
}

bool SessionManager::CheckRequestPass(const SessionRequest &request, const unsigned char *pass, CheckedPass &checked) {
  PassCheckContext context;
  context.public_key = settings_.public_key;
  context.region = settings_.region;
  context.source_ip = request.source_ip;
  context.check_address = settings_.check_address;
  context.now = request.now_unix;

  RefuseCode code = REFUSE_BAD_PASS;
  if (!CheckPass(pass, PASS_SIZE, context, checked.m_fields, code)) {
    Refuse(request.connection, code);
    return false;
  }
  if (live_pass_ids_.count(checked.m_fields.pass_id) != 0) {
    Refuse(request.connection, REFUSE_PASS_IN_USE);
    return false;
  }
  return true;
}

std::uint64_t SessionManager::NewSessionId() const {
  std::uint64_t id = RandomNonZeroU64();
  while (sessions_.count(id) != 0) {
    id = RandomNonZeroU64();
  }
  return id;
}

void SessionManager::OnCreate(const SessionRequest &request, const unsigned char *body, std::size_t length) {
  if (!create_limiter_.Allow(request.source_ip, request.now_ms)) {
    Refuse(request.connection, REFUSE_RATE_LIMITED);
    return;
  }
  if (length != CREATE_BODY_SIZE || IsBound(request.connection)) {
    Refuse(request.connection, REFUSE_BAD_REQUEST);
    return;
  }
  if (GetU16(body + CREATE_PROTO_OFFSET) != PROTOCOL_VERSION) {
    Refuse(request.connection, REFUSE_PROTO_MISMATCH);
    return;
  }
  CheckedPass pass;
  if (!CheckRequestPass(request, body + CREATE_PASS_OFFSET, pass)) {
    return;
  }

  const std::uint8_t HOST_SLOT = body[CREATE_HOST_SLOT_OFFSET];
  const std::uint8_t SLOT_MASK = body[CREATE_SLOT_MASK_OFFSET];
  if (!SlotInMask(SLOT_MASK, HOST_SLOT) || CountBits(SLOT_MASK) < 2) {
    Refuse(request.connection, REFUSE_BAD_REQUEST);
    return;
  }
  if (sessions_.size() >= settings_.max_sessions) {
    Refuse(request.connection, REFUSE_SERVER_BUSY);
    return;
  }

  const std::uint64_t SESSION_ID = NewSessionId();
  Session &session = sessions_[SESSION_ID];
  session.m_host_slot = HOST_SLOT;
  session.m_expected_mask = SLOT_MASK;
  session.m_exe_crc = pass.m_fields.exe_crc;
  session.m_ini_crc = pass.m_fields.ini_crc;
  session.m_created_ms = request.now_ms;
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (SlotInMask(SLOT_MASK, slot) && slot != HOST_SLOT) {
      session.m_tokens[slot] = RandomNonZeroU64();
    }
  }

  unsigned char created[1 + CREATED_BODY_SIZE];
  created[0] = MSG_CREATED;
  PutU64(created + 1, SESSION_ID);
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    PutU64(created + 9 + (slot * 8), session.m_tokens[slot]);
  }
  output_.Send(request.connection, created, sizeof(created));

  Bind(SESSION_ID, session, HOST_SLOT, request.connection, pass.m_fields.pass_id);
  SendWelcome(SESSION_ID, session, HOST_SLOT);
  LogLine("session created by connection %u for %d players, %zu sessions active", request.connection,
          CountBits(SLOT_MASK), sessions_.size());
}

bool SessionManager::CheckJoin(const Session &session, std::uint8_t slot, std::uint64_t token, const CheckedPass &pass,
                               RefuseCode &refuse) const {
  if (slot >= SLOT_COUNT) {
    refuse = REFUSE_BAD_REQUEST;
  } else if (!SlotInMask(session.m_expected_mask, slot)) {
    refuse = REFUSE_NOT_EXPECTED;
  } else if (SlotInMask(session.m_connected_mask, slot)) {
    refuse = REFUSE_SLOT_TAKEN;
  } else if (session.m_tokens[slot] == 0 || !TokensEqual(session.m_tokens[slot], token)) {
    refuse = REFUSE_BAD_TOKEN;
  } else if (pass.m_fields.exe_crc != session.m_exe_crc || pass.m_fields.ini_crc != session.m_ini_crc) {
    refuse = REFUSE_BUILD_MISMATCH;
  } else {
    return true;
  }
  return false;
}

void SessionManager::OnJoin(const SessionRequest &request, const unsigned char *body, std::size_t length) {
  if (!join_limiter_.Allow(request.source_ip, request.now_ms)) {
    Refuse(request.connection, REFUSE_RATE_LIMITED);
    return;
  }
  if (length != JOIN_BODY_SIZE || IsBound(request.connection)) {
    Refuse(request.connection, REFUSE_BAD_REQUEST);
    return;
  }
  if (GetU16(body + JOIN_PROTO_OFFSET) != PROTOCOL_VERSION) {
    Refuse(request.connection, REFUSE_PROTO_MISMATCH);
    return;
  }
  CheckedPass pass;
  if (!CheckRequestPass(request, body + JOIN_PASS_OFFSET, pass)) {
    return;
  }

  const std::uint64_t SESSION_ID = GetU64(body + JOIN_SESSION_ID_OFFSET);
  const std::uint8_t SLOT = body[JOIN_SLOT_OFFSET];
  const std::uint64_t TOKEN = GetU64(body + JOIN_TOKEN_OFFSET);

  auto found = sessions_.find(SESSION_ID);
  if (found == sessions_.end()) {
    Refuse(request.connection, REFUSE_NO_SESSION);
    return;
  }
  Session &session = found->second;
  if (session.m_started) {
    Refuse(request.connection, REFUSE_ALREADY_STARTED);
    return;
  }
  if (session.m_failed_joins >= FAILED_JOINS_PER_SESSION) {
    Refuse(request.connection, REFUSE_RATE_LIMITED);
    return;
  }

  RefuseCode refuse = REFUSE_BAD_REQUEST;
  if (!CheckJoin(session, SLOT, TOKEN, pass, refuse)) {
    ++session.m_failed_joins;
    Refuse(request.connection, refuse);
    return;
  }
  AcceptJoin(request, SESSION_ID, session, SLOT, pass);
}

void SessionManager::AcceptJoin(const SessionRequest &request, std::uint64_t session_id, Session &session,
                                std::uint8_t slot, const CheckedPass &pass) {
  session.m_tokens[slot] = 0; // A token works once.
  Bind(session_id, session, slot, request.connection, pass.m_fields.pass_id);
  SendWelcome(session_id, session, slot);
  SendRoster(session);
  LogLine("connection %u joined slot %u", request.connection, static_cast<unsigned int>(slot));
  if (session.m_connected_mask == session.m_expected_mask) {
    SendStart(session);
  }
}

void SessionManager::OnLeave(std::uint32_t connection) {
  ReleaseConnection(connection);
  output_.Close(connection);
}

void SessionManager::OnClosed(std::uint32_t connection) {
  ReleaseConnection(connection);
}

void SessionManager::ReleaseConnection(std::uint32_t connection) {
  auto binding = bindings_.find(connection);
  if (binding == bindings_.end()) {
    return;
  }
  const std::uint64_t SESSION_ID = binding->second.m_session_id;
  const std::uint8_t SLOT = binding->second.m_slot;
  bindings_.erase(binding);

  auto found = sessions_.find(SESSION_ID);
  if (found == sessions_.end()) {
    return;
  }
  Session &session = found->second;
  live_pass_ids_.erase(session.m_pass_ids[SLOT]);
  session.m_pass_ids[SLOT] = 0;
  session.m_connections[SLOT] = 0;
  session.m_connected_mask = static_cast<std::uint8_t>(session.m_connected_mask & ~(1u << SLOT));
  LogLine("connection %u left slot %u", connection, static_cast<unsigned int>(SLOT));

  if (session.m_connected_mask == 0) {
    RemoveSession(SESSION_ID);
  } else {
    SendRoster(session);
  }
}

void SessionManager::Bind(std::uint64_t session_id, Session &session, std::uint8_t slot, std::uint32_t connection,
                          std::uint64_t pass_id) {
  session.m_connected_mask = static_cast<std::uint8_t>(session.m_connected_mask | (1u << slot));
  session.m_connections[slot] = connection;
  session.m_pass_ids[slot] = pass_id;
  live_pass_ids_.insert(pass_id);
  Binding binding;
  binding.m_session_id = session_id;
  binding.m_slot = slot;
  bindings_[connection] = binding;
}

void SessionManager::SendWelcome(std::uint64_t session_id, const Session &session, std::uint8_t slot) {
  unsigned char message[1 + WELCOME_BODY_SIZE];
  message[0] = MSG_WELCOME;
  PutU64(message + 1, session_id);
  message[9] = slot;
  message[10] = session.m_connected_mask;
  message[11] = LeaderSlot(session.m_connected_mask);
  output_.Send(session.m_connections[slot], message, sizeof(message));
}

void SessionManager::SendToConnected(const Session &session, const unsigned char *message, std::size_t length) {
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (SlotInMask(session.m_connected_mask, slot)) {
      output_.Send(session.m_connections[slot], message, length);
    }
  }
}

void SessionManager::SendRoster(const Session &session) {
  unsigned char message[1 + ROSTER_BODY_SIZE];
  message[0] = MSG_ROSTER;
  message[1] = session.m_connected_mask;
  message[2] = LeaderSlot(session.m_connected_mask);
  SendToConnected(session, message, sizeof(message));
}

void SessionManager::SendStart(Session &session) {
  unsigned char message[1 + START_BODY_SIZE];
  message[0] = MSG_START;
  message[1] = session.m_connected_mask;
  message[2] = LeaderSlot(session.m_connected_mask);
  SendToConnected(session, message, sizeof(message));
  session.m_started = true;
  LogLine("session started with %d players", CountBits(session.m_connected_mask));
}

void SessionManager::ExpireSession(std::uint64_t session_id, Session &session) {
  if (CountBits(session.m_connected_mask) < 2) {
    LogLine("session ended before start: not enough players connected");
    for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
      if (SlotInMask(session.m_connected_mask, slot)) {
        Refuse(session.m_connections[slot], REFUSE_NOT_ENOUGH_PLAYERS);
      }
    }
    RemoveSession(session_id);
    return;
  }

  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (SlotInMask(session.m_expected_mask, slot) && !SlotInMask(session.m_connected_mask, slot)) {
      unsigned char drop[1 + DROP_BODY_SIZE];
      drop[0] = MSG_DROP;
      drop[1] = static_cast<unsigned char>(slot);
      drop[2] = DROP_NEVER_JOINED;
      PutU32(drop + 3, NEVER_JOINED_LAST_FRAME);
      SendToConnected(session, drop, sizeof(drop));
    }
  }
  SendStart(session);
}

void SessionManager::RemoveSession(std::uint64_t session_id) {
  auto found = sessions_.find(session_id);
  if (found == sessions_.end()) {
    return;
  }
  const Session &session = found->second;
  LogCounters(session);
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (SlotInMask(session.m_connected_mask, slot)) {
      bindings_.erase(session.m_connections[slot]);
      live_pass_ids_.erase(session.m_pass_ids[slot]);
    }
  }
  sessions_.erase(found);
}

void SessionManager::Tick(std::uint32_t now_ms) {
  std::vector<std::uint64_t> expired;
  for (const auto &entry : sessions_) {
    if (!entry.second.m_started && now_ms - entry.second.m_created_ms >= SESSION_START_WAIT_MS) {
      expired.push_back(entry.first);
    }
  }
  for (std::uint64_t session_id : expired) {
    auto found = sessions_.find(session_id);
    if (found != sessions_.end()) {
      ExpireSession(session_id, found->second);
    }
  }
}

bool SessionManager::IsBound(std::uint32_t connection) const {
  return bindings_.count(connection) != 0;
}

std::size_t SessionManager::SessionCount() const {
  return sessions_.size();
}

} // namespace relay
