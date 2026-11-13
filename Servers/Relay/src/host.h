// The relay's ENet host: listens for connections, enforces the first-message deadline and limits
// (PROTOCOL.md 4.1 and 6), runs the entry check (PROTOCOL.md 4.4) and hands session messages to the session manager.
#pragma once

#include "entry_check.h"
#include "rate_limit.h"
#include "records.h"
#include "sessions.h"

#include <cstddef>
#include <cstdint>
#include <string>
#include <unordered_map>

struct _ENetEvent;
struct _ENetHost;
struct _ENetPeer;

namespace relay {

class RelayHost : private SessionOutput {
public:
  // check_address is false only with the relay's test start option. max_sessions is the concurrent session limit.
  RelayHost(std::uint16_t listen_port, const EntryCheckSettings &settings, bool check_address, std::size_t max_sessions,
            const RecordSettings &records, const DropSettings &drop);
  ~RelayHost();

  RelayHost(const RelayHost &) = delete;
  RelayHost &operator=(const RelayHost &) = delete;

  // Opens the UDP port. enet_initialize() must have succeeded first. On failure, error says why.
  bool Start(std::string &error);

  // Services the network until a fatal error. Returns false on a fatal error.
  bool Run();

private:
  struct Connection;

  // SessionOutput.
  void Send(std::uint32_t connection, const unsigned char *message, std::size_t length) override;
  void Close(std::uint32_t connection) override;

  bool AdmitConnection(const _ENetPeer *peer) const;
  void HandleFirstMessage(_ENetPeer *peer, Connection &connection, const unsigned char *data, std::size_t length);
  void Dispatch(const _ENetEvent &event);
  void OnConnect(_ENetPeer *peer);
  void OnDisconnect(_ENetPeer *peer);
  void OnReceive(_ENetPeer *peer, std::uint8_t channel_id, const unsigned char *data, std::size_t length);
  void CheckFirstMessageDeadlines();
  void NoteReceiveTimes();
  bool ConnectionWasLost(const Connection &connection) const;

  void OnCheckHello(_ENetPeer *peer, Connection *connection, const unsigned char *data);
  void OnPong(Connection *connection, const unsigned char *data);
  void ServiceEntryChecks();
  void ServiceEntryCheck(_ENetPeer *peer, Connection *connection, std::uint32_t now_ms);
  void FinishEntryCheck(_ENetPeer *peer, Connection *connection);
  void SendPing(_ENetPeer *peer, Connection *connection, std::uint32_t now_ms);
  void SendPass(_ENetPeer *peer, Connection *connection, std::uint16_t rtt_ms);
  void RefuseAndClose(_ENetPeer *peer, Connection *connection, std::uint8_t code, std::uint16_t rtt_ms,
                      std::uint16_t limit_ms);
  void CloseEntryCheck(_ENetPeer *peer, Connection *connection);
  std::uint32_t WaitMs() const;

  std::uint16_t listen_port_;
  std::uint32_t next_connection_id_;
  _ENetHost *host_;
  EntryCheckSettings settings_;
  AddressRateLimiter rate_limiter_;
  std::size_t session_limit_;
  std::uint32_t connection_timeout_ms_;
  SessionManager sessions_;
  std::unordered_map<std::uint32_t, _ENetPeer *> peers_by_connection_;
};

} // namespace relay
