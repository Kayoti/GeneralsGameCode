#include "host.h"

#include "entry_check.h"
#include "log.h"
#include "pass.h"
#include "protocol.h"
#include "vpn_check.h"
#include "wire.h"

#include <algorithm>
#include <cstddef>
#include <cstdint>
#include <ctime>
#include <enet/enet.h>
#include <string>

namespace relay {

namespace {

// Where a connection is in the entry check (PROTOCOL.md 4.4).
enum class EntryState : std::uint8_t {
  NONE,
  MEASURING,
  DONE,
};

// How long one service call waits for network activity when no entry check needs an earlier wake-up. Deadlines are
// checked after each call.
const std::uint32_t SERVICE_WAIT_MS = 100;

// Every message uses channel 0 (PROTOCOL.md 4.1). Channel 1 is reserved and unused.
const std::uint8_t MESSAGE_CHANNEL = 0;

// Unreassembled data one peer may hold at a time, in messages. Four is enough for a few messages in flight
// and keeps a single peer from holding the 32 MiB ENet default.
const std::size_t WAITING_MESSAGES_PER_PEER = 4;

// True when the message is one of the three first messages, with the exact length from PROTOCOL.md 4.2.
bool IsFirstMessage(const unsigned char *data, std::size_t length) {
  if (length == 0) {
    return false;
  }
  switch (data[0]) {
  case MSG_CHECK_HELLO:
    return length == 1 + CHECK_HELLO_BODY_SIZE;
  case MSG_CREATE:
    return length == 1 + CREATE_BODY_SIZE;
  case MSG_JOIN:
    return length == 1 + JOIN_BODY_SIZE;
  default:
    return false;
  }
}

// Source address of a peer in the encoding of PROTOCOL.md 4.3: the IPv4 address as a host-order integer.
std::uint32_t ClientAddress(const ENetPeer *peer) {
  return ENET_NET_TO_HOST_32(peer->address.host);
}

// Queues one message on the message channel, reliable and ordered. A packet that ENet refuses is freed here.
void SendMessage(ENetPeer *peer, const unsigned char *data, std::size_t length) {
  ENetPacket *packet = enet_packet_create(data, length, ENET_PACKET_FLAG_RELIABLE);
  if (packet == nullptr) {
    LogLine("relay: a message could not be created");
    return;
  }
  if (enet_peer_send(peer, MESSAGE_CHANNEL, packet) != 0) {
    enet_packet_destroy(packet);
    LogLine("relay: a message could not be queued");
  }
}

// Logs a VPN refusal with its reason. The address is shortened, so the log never holds a full address.
void LogVpnRefusal(std::uint32_t connection_id, std::uint32_t address, const VpnVerdict &verdict) {
  if (verdict.reason == VpnReason::LISTED_OPERATOR) {
    LogLine("connection %u: VPN check refused %s, listed network operator %u", connection_id,
            ShortAddress(address).c_str(), static_cast<unsigned int>(verdict.operator_number));
  } else {
    LogLine("connection %u: VPN check refused %s, listed network range", connection_id, ShortAddress(address).c_str());
  }
}

// Settings of the session manager. It checks passes with the public half of the region key held in settings.
SessionSettings MakeSessionSettings(const EntryCheckSettings &settings, bool check_address, std::size_t max_sessions,
                                    const RecordSettings &records, const DropSettings &drop) {
  SessionSettings session_settings;
  session_settings.public_key = settings.key.public_key;
  session_settings.region = settings.region;
  session_settings.check_address = check_address;
  session_settings.max_sessions = max_sessions;
  session_settings.records = records;
  session_settings.drop = drop;
  return session_settings;
}

} // namespace

// Per-connection state, stored in ENetPeer::data while the peer is connected.
struct RelayHost::Connection {
  std::uint32_t m_id = 0;
  std::uint32_t m_connected_at_ms = 0;
  std::uint32_t m_source_ip = 0; // Host-order IPv4 address.
  bool m_first_message_seen = false;
  bool m_closing = false;
  EntryState m_entry_state = EntryState::NONE;
  std::uint32_t m_exe_crc = 0;
  std::uint32_t m_ini_crc = 0;
  PingRound m_round;
  std::uint32_t m_last_heard_ms = 0; // When ENet last received anything from the peer, seen at the last service pass.
};

RelayHost::RelayHost(std::uint16_t listen_port, const EntryCheckSettings &settings, VpnCheck &vpn, bool check_address,
                     std::size_t max_sessions, const RecordSettings &records, const DropSettings &drop)
    : listen_port_(listen_port), next_connection_id_(1), host_(nullptr), settings_(settings), vpn_(vpn),
      rate_limiter_(ENTRY_CHECKS_PER_WINDOW, RATE_WINDOW_MS), session_limit_(max_sessions),
      connection_timeout_ms_(drop.m_connection_timeout_ms),
      sessions_(MakeSessionSettings(settings_, check_address, max_sessions, records, drop), *this) {}

RelayHost::~RelayHost() {
  if (host_ == nullptr) {
    return;
  }
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    delete static_cast<Connection *>(host_->peers[i].data);
    host_->peers[i].data = nullptr;
  }
  enet_host_destroy(host_);
}

bool RelayHost::Start(std::string &error) {
  ENetAddress address;
  address.host = ENET_HOST_ANY;
  address.port = listen_port_;

  // PROTOCOL.md 6 allows MAX_UNCHECKED_CONNECTIONS connections without a first message. Every session may also
  // hold SLOT_COUNT connections, so connections inside matches never take those places.
  const std::size_t PEER_COUNT = MAX_UNCHECKED_CONNECTIONS + (SLOT_COUNT * session_limit_);
  if (PEER_COUNT > ENET_PROTOCOL_MAXIMUM_PEER_ID) {
    error = "max_sessions is too large, at most " +
            std::to_string((ENET_PROTOCOL_MAXIMUM_PEER_ID - MAX_UNCHECKED_CONNECTIONS) / SLOT_COUNT) + " are supported";
    return false;
  }
  host_ = enet_host_create(&address, PEER_COUNT, CHANNEL_COUNT, 0, 0);
  if (host_ == nullptr) {
    error = "cannot open UDP port " + std::to_string(listen_port_);
    return false;
  }
  // ENet checks these limits while it reassembles a packet, before it allocates the buffer, so an oversize
  // announcement is refused before any memory is used for it.
  host_->maximumPacketSize = MAX_MESSAGE_SIZE;
  host_->maximumWaitingData = WAITING_MESSAGES_PER_PEER * MAX_MESSAGE_SIZE;
  return true;
}

bool RelayHost::Run() {
  ENetEvent event;
  for (;;) {
    int result = enet_host_service(host_, &event, WaitMs());
    while (result > 0) {
      Dispatch(event);
      result = enet_host_check_events(host_, &event);
    }
    if (result < 0) {
      LogError("relay: network error while servicing the UDP port");
      return false;
    }
    NoteReceiveTimes();
    CheckFirstMessageDeadlines();
    ReloadVpnDataIfRequested();
    ServiceEntryChecks();
    sessions_.Tick(enet_time_get());
  }
}

void RelayHost::Dispatch(const ENetEvent &event) {
  switch (event.type) {
  case ENET_EVENT_TYPE_CONNECT:
    OnConnect(event.peer);
    break;
  case ENET_EVENT_TYPE_DISCONNECT:
    OnDisconnect(event.peer);
    break;
  case ENET_EVENT_TYPE_RECEIVE:
    OnReceive(event.peer, event.channelID, event.packet->data, event.packet->dataLength);
    enet_packet_destroy(event.packet);
    break;
  default:
    break;
  }
}

bool RelayHost::AdmitConnection(const ENetPeer *peer) const {
  std::size_t unchecked_total = 0;
  std::size_t unchecked_from_address = 0;
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    const ENetPeer *other = &host_->peers[i];
    const Connection *connection = static_cast<const Connection *>(other->data);
    if (other == peer || connection == nullptr || connection->m_first_message_seen || connection->m_closing) {
      continue;
    }
    ++unchecked_total;
    if (other->address.host == peer->address.host) {
      ++unchecked_from_address;
    }
  }
  return unchecked_total < MAX_UNCHECKED_CONNECTIONS && unchecked_from_address < UNCHECKED_CONNECTIONS_PER_ADDRESS;
}

void RelayHost::OnConnect(ENetPeer *peer) {
  if (!AdmitConnection(peer)) {
    LogLine("connection refused: too many connections without a first message");
    enet_peer_disconnect_now(peer, 0);
    return;
  }
  Connection *connection = new Connection();
  connection->m_id = next_connection_id_++;
  connection->m_connected_at_ms = enet_time_get();
  connection->m_source_ip = ClientAddress(peer);
  connection->m_first_message_seen = false;
  connection->m_closing = false;
  connection->m_entry_state = EntryState::NONE;
  connection->m_last_heard_ms = enet_time_get();
  // The configured time is how long ENet waits for an answer before it reports the connection lost. The minimum
  // is set to the same value so that this is the only time that applies. The limit keeps ENet's default.
  enet_peer_timeout(peer, 0, connection_timeout_ms_, connection_timeout_ms_);
  peer->data = connection;
  peers_by_connection_[connection->m_id] = peer;
  LogLine("connection %u opened", connection->m_id);
}

void RelayHost::OnDisconnect(ENetPeer *peer) {
  Connection *connection = static_cast<Connection *>(peer->data);
  if (connection == nullptr) {
    return;
  }
  const bool LOST = ConnectionWasLost(*connection);
  LogLine("connection %u closed%s", connection->m_id, LOST ? " (connection lost)" : "");
  sessions_.OnClosed(connection->m_id, LOST);
  peers_by_connection_.erase(connection->m_id);
  delete connection;
  peer->data = nullptr;
}

void RelayHost::Send(std::uint32_t connection, const unsigned char *message, std::size_t length) {
  auto found = peers_by_connection_.find(connection);
  if (found != peers_by_connection_.end()) {
    SendMessage(found->second, message, length);
  }
}

void RelayHost::Close(std::uint32_t connection) {
  auto found = peers_by_connection_.find(connection);
  if (found == peers_by_connection_.end()) {
    return;
  }
  Connection *state = static_cast<Connection *>(found->second->data);
  if (state != nullptr) {
    state->m_closing = true;
  }
  enet_peer_disconnect_later(found->second, 0);
}

void RelayHost::HandleFirstMessage(ENetPeer *peer, Connection &connection, const unsigned char *data,
                                   std::size_t length) {
  connection.m_first_message_seen = true;
  LogLine("connection %u: first message type %u accepted", connection.m_id, static_cast<unsigned int>(data[0]));

  SessionRequest request;
  request.connection = connection.m_id;
  request.source_ip = connection.m_source_ip;
  request.now_ms = enet_time_get();
  request.now_unix = static_cast<std::uint32_t>(std::time(nullptr));
  if (data[0] == MSG_CHECK_HELLO) {
    OnCheckHello(peer, &connection, data);
  } else if (data[0] == MSG_CREATE) {
    sessions_.OnCreate(request, data + 1, length - 1);
  } else if (data[0] == MSG_JOIN) {
    sessions_.OnJoin(request, data + 1, length - 1);
  }
}

void RelayHost::OnReceive(ENetPeer *peer, std::uint8_t channel_id, const unsigned char *data, std::size_t length) {
  Connection *connection = static_cast<Connection *>(peer->data);
  if (connection == nullptr || connection->m_closing) {
    return;
  }
  if (channel_id != MESSAGE_CHANNEL) {
    LogLine("connection %u: message on channel %u dropped, only channel 0 is used", connection->m_id,
            static_cast<unsigned int>(channel_id));
    return;
  }
  if (length == 0 || length > MAX_MESSAGE_SIZE) {
    LogLine("connection %u: message of %zu bytes dropped, bad length", connection->m_id, length);
    return;
  }
  if (!connection->m_first_message_seen) {
    if (!IsFirstMessage(data, length)) {
      LogLine("connection %u: message type %u dropped, not a valid first message", connection->m_id,
              static_cast<unsigned int>(data[0]));
      return;
    }
    HandleFirstMessage(peer, *connection, data, length);
    return;
  }
  if (data[0] == MSG_PONG && length == 1 + PONG_BODY_SIZE && connection->m_entry_state == EntryState::MEASURING) {
    OnPong(connection, data + 1);
    return;
  }
  if (data[0] == MSG_LEAVE && length == 1 && sessions_.IsBound(connection->m_id)) {
    sessions_.OnLeave(connection->m_id);
    return;
  }
  if (data[0] == MSG_DATA && sessions_.IsBound(connection->m_id)) {
    sessions_.OnData(connection->m_id, enet_time_get(), data + 1, length - 1);
    return;
  }
  LogLine("connection %u: message type %u dropped, no handler in this build", connection->m_id,
          static_cast<unsigned int>(data[0]));
}

// ENet reports a lost connection and a normal disconnect with the same event, so the two are told apart by how long
// the peer was silent. A peer that closes normally was heard from a moment ago. A peer that is lost has been silent
// for about the configured timeout, so half of it is the dividing line.
void RelayHost::NoteReceiveTimes() {
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    const ENetPeer *peer = &host_->peers[i];
    Connection *connection = static_cast<Connection *>(peer->data);
    if (connection != nullptr && peer->state == ENET_PEER_STATE_CONNECTED) {
      connection->m_last_heard_ms = peer->lastReceiveTime;
    }
  }
}

bool RelayHost::ConnectionWasLost(const Connection &connection) const {
  return enet_time_get() - connection.m_last_heard_ms >= connection_timeout_ms_ / 2;
}

void RelayHost::CheckFirstMessageDeadlines() {
  std::uint32_t now = enet_time_get();
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    ENetPeer *peer = &host_->peers[i];
    Connection *connection = static_cast<Connection *>(peer->data);
    if (peer->state != ENET_PEER_STATE_CONNECTED || connection == nullptr || connection->m_first_message_seen) {
      continue;
    }
    if (now - connection->m_connected_at_ms < FIRST_MESSAGE_DEADLINE_MS) {
      continue;
    }
    LogLine("connection %u sent no valid first message within 5 seconds, disconnecting", connection->m_id);
    enet_peer_disconnect(peer, 0);
  }
}

// Reloads the VPN data after a Ctrl+Break (see main.cpp). A failed reload keeps the data already in use.
void RelayHost::ReloadVpnDataIfRequested() {
  if (!TakeVpnReloadRequest()) {
    return;
  }
  std::string error;
  if (vpn_.Load(error)) {
    LogLine("VPN data reloaded");
  } else {
    LogError("relay: VPN data not reloaded, the data in use is kept: %s", error.c_str());
  }
}

// CHECK_HELLO body: u16 proto, u32 exe_crc, u32 ini_crc. The checks run in this order: protocol version, rate limit,
// VPN check, then the ping measurement.
void RelayHost::OnCheckHello(ENetPeer *peer, Connection *connection, const unsigned char *data) {
  std::uint16_t proto = GetU16(data + 1);
  if (proto != PROTOCOL_VERSION) {
    LogLine("connection %u: protocol version %u is not supported, refused", connection->m_id,
            static_cast<unsigned int>(proto));
    RefuseAndClose(peer, connection, REFUSE_PROTO_MISMATCH, 0, 0);
    return;
  }

  std::uint32_t now_ms = enet_time_get();
  std::uint32_t address = ClientAddress(peer);
  if (!rate_limiter_.Allow(address, now_ms)) {
    LogLine("connection %u: too many entry checks from this address, refused", connection->m_id);
    RefuseAndClose(peer, connection, REFUSE_RATE_LIMITED, 0, 0);
    return;
  }
  const VpnVerdict VERDICT = vpn_.Check(address);
  if (VERDICT.reason != VpnReason::NONE) {
    LogVpnRefusal(connection->m_id, address, VERDICT);
    RefuseAndClose(peer, connection, REFUSE_VPN_DETECTED, 0, 0);
    return;
  }

  connection->m_exe_crc = GetU32(data + 3);
  connection->m_ini_crc = GetU32(data + 7);
  connection->m_round.Start(now_ms);
  connection->m_entry_state = EntryState::MEASURING;
  ServiceEntryCheck(peer, connection, now_ms);
}

// PONG body: u64 nonce. Unknown, repeated or late nonces are ignored by PingRound.
void RelayHost::OnPong(Connection *connection, const unsigned char *data) {
  connection->m_round.OnPong(GetU64(data), enet_time_get());
}

void RelayHost::ServiceEntryChecks() {
  std::uint32_t now_ms = enet_time_get();
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    ENetPeer *peer = &host_->peers[i];
    Connection *connection = static_cast<Connection *>(peer->data);
    if (connection != nullptr && connection->m_entry_state == EntryState::MEASURING) {
      ServiceEntryCheck(peer, connection, now_ms);
    }
  }
}

void RelayHost::ServiceEntryCheck(ENetPeer *peer, Connection *connection, std::uint32_t now_ms) {
  if (connection->m_round.Expired(now_ms)) {
    FinishEntryCheck(peer, connection);
    return;
  }
  if (connection->m_round.PingDue(now_ms)) {
    SendPing(peer, connection, now_ms);
  }
}

void RelayHost::FinishEntryCheck(ENetPeer *peer, Connection *connection) {
  std::uint16_t median_ms = 0;
  switch (connection->m_round.Verdict(settings_.ping_limit_ms, median_ms)) {
  case EntryVerdict::TIMEOUT:
    LogLine("connection %u: fewer than %u PONG answers in time, refused", connection->m_id,
            static_cast<unsigned int>(MIN_ANSWERS));
    RefuseAndClose(peer, connection, REFUSE_TIMEOUT, 0, 0);
    break;
  case EntryVerdict::PING_TOO_HIGH:
    LogLine("connection %u: ping %u ms is above the limit of %u ms, refused", connection->m_id,
            static_cast<unsigned int>(median_ms), static_cast<unsigned int>(settings_.ping_limit_ms));
    RefuseAndClose(peer, connection, REFUSE_PING_TOO_HIGH, median_ms, settings_.ping_limit_ms);
    break;
  case EntryVerdict::PASS:
    SendPass(peer, connection, median_ms);
    break;
  }
}

// PING body: u64 nonce, from a fresh random value.
void RelayHost::SendPing(ENetPeer *peer, Connection *connection, std::uint32_t now_ms) {
  std::uint64_t nonce = RandomId();
  unsigned char message[1 + PING_BODY_SIZE] = {};
  message[0] = MSG_PING;
  PutU64(message + 1, nonce);
  SendMessage(peer, message, sizeof(message));
  connection->m_round.SentPing(nonce, now_ms);
}

// PASS body: the 100-byte pass (PROTOCOL.md 4.3). If signing fails no code fits, so the connection is closed without
// a reply and the client reports that the server cannot be reached.
void RelayHost::SendPass(ENetPeer *peer, Connection *connection, std::uint16_t rtt_ms) {
  PassFields fields;
  fields.client_ip = ClientAddress(peer);
  fields.exe_crc = connection->m_exe_crc;
  fields.ini_crc = connection->m_ini_crc;
  fields.rtt_ms = rtt_ms;

  unsigned char message[1 + PASS_SIZE] = {};
  message[0] = MSG_PASS;
  if (!IssuePass(settings_, fields, static_cast<std::uint32_t>(std::time(nullptr)), message + 1)) {
    LogLine("connection %u: pass could not be signed, closed without a reply", connection->m_id);
    CloseEntryCheck(peer, connection);
    return;
  }
  SendMessage(peer, message, sizeof(message));
  LogLine("connection %u: pass issued", connection->m_id);
  CloseEntryCheck(peer, connection);
}

// REFUSE body: u8 code, u16 rtt_ms, u16 limit_ms (PROTOCOL.md 4.2). The two values are 0 unless the code is
// PING_TOO_HIGH.
void RelayHost::RefuseAndClose(ENetPeer *peer, Connection *connection, std::uint8_t code, std::uint16_t rtt_ms,
                               std::uint16_t limit_ms) {
  unsigned char message[1 + REFUSE_BODY_SIZE] = {};
  message[0] = MSG_REFUSE;
  message[1] = code;
  PutU16(message + 2, rtt_ms);
  PutU16(message + 4, limit_ms);
  SendMessage(peer, message, sizeof(message));
  CloseEntryCheck(peer, connection);
}

// Ends the entry check. The disconnect waits until the queued reply has been sent (PROTOCOL.md 4.2, the relay
// disconnects after sending).
void RelayHost::CloseEntryCheck(ENetPeer *peer, Connection *connection) {
  connection->m_entry_state = EntryState::DONE;
  enet_peer_disconnect_later(peer, 0);
}

// Time until the next PING or the end of a measurement, so the loop wakes up in time for the 250 ms spacing.
std::uint32_t RelayHost::WaitMs() const {
  std::uint32_t wait_ms = SERVICE_WAIT_MS;
  std::uint32_t now_ms = enet_time_get();
  for (std::size_t i = 0; i < host_->peerCount; ++i) {
    const Connection *connection = static_cast<const Connection *>(host_->peers[i].data);
    if (connection == nullptr || connection->m_entry_state != EntryState::MEASURING) {
      continue;
    }
    wait_ms = (std::min)(wait_ms, connection->m_round.MsUntilNextAction(now_ms));
  }
  return wait_ms;
}

} // namespace relay
