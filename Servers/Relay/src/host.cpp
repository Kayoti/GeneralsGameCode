#include "host.h"

#include "log.h"
#include "protocol.h"

#include <cstddef>
#include <enet/enet.h>
#include <string>

namespace relay {

namespace {

// How long one service call waits for network activity. Deadlines are checked after each call.
const std::uint32_t SERVICE_WAIT_MS = 100;

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

} // namespace

// Per-connection state, stored in ENetPeer::data while the peer is connected.
struct RelayHost::Connection {
  std::uint32_t m_id;
  std::uint32_t m_connected_at_ms;
  bool m_first_message_seen;
};

RelayHost::RelayHost(std::uint16_t listen_port) : listen_port_(listen_port), next_connection_id_(1), host_(nullptr) {}

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

  host_ = enet_host_create(&address, MAX_PEERS, CHANNEL_COUNT, 0, 0);
  if (host_ == nullptr) {
    error = "cannot open UDP port " + std::to_string(listen_port_);
    return false;
  }
  return true;
}

bool RelayHost::Run() {
  ENetEvent event;
  for (;;) {
    int result = enet_host_service(host_, &event, SERVICE_WAIT_MS);
    while (result > 0) {
      Dispatch(event);
      result = enet_host_check_events(host_, &event);
    }
    if (result < 0) {
      LogError("relay: network error while servicing the UDP port");
      return false;
    }
    CheckFirstMessageDeadlines();
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
    OnReceive(event.peer, event.packet->data, event.packet->dataLength);
    enet_packet_destroy(event.packet);
    break;
  default:
    break;
  }
}

void RelayHost::OnConnect(ENetPeer *peer) {
  Connection *connection = new Connection();
  connection->m_id = next_connection_id_++;
  connection->m_connected_at_ms = enet_time_get();
  connection->m_first_message_seen = false;
  peer->data = connection;
  LogLine("connection %u opened", connection->m_id);
}

void RelayHost::OnDisconnect(ENetPeer *peer) {
  Connection *connection = static_cast<Connection *>(peer->data);
  if (connection == nullptr) {
    return;
  }
  LogLine("connection %u closed", connection->m_id);
  delete connection;
  peer->data = nullptr;
}

void RelayHost::OnReceive(ENetPeer *peer, const unsigned char *data, std::size_t length) {
  Connection *connection = static_cast<Connection *>(peer->data);
  if (connection == nullptr) {
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
    connection->m_first_message_seen = true;
    LogLine("connection %u: first message type %u accepted", connection->m_id, static_cast<unsigned int>(data[0]));
    return;
  }
  // Handling of messages after the first one is added by later packages (entry check, sessions).
  LogLine("connection %u: message type %u dropped, no handler in this build", connection->m_id,
          static_cast<unsigned int>(data[0]));
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

} // namespace relay
