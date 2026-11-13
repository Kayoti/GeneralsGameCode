// The relay's ENet host: listens for connections, logs them, enforces the first-message deadline
// (PROTOCOL.md 4.1 and 6) and runs the entry check (PROTOCOL.md 4.4).
#pragma once

#include "entry_check.h"

#include <cstddef>
#include <cstdint>
#include <string>

struct _ENetEvent;
struct _ENetHost;
struct _ENetPeer;

namespace relay {

class RelayHost {
public:
  RelayHost(std::uint16_t listen_port, const EntryCheckSettings &settings);
  ~RelayHost();

  RelayHost(const RelayHost &) = delete;
  RelayHost &operator=(const RelayHost &) = delete;

  // Opens the UDP port. enet_initialize() must have succeeded first. On failure, error says why.
  bool Start(std::string &error);

  // Services the network until a fatal error. Returns false on a fatal error.
  bool Run();

private:
  struct Connection;

  void Dispatch(const _ENetEvent &event);
  void OnConnect(_ENetPeer *peer);
  void OnDisconnect(_ENetPeer *peer);
  void OnReceive(_ENetPeer *peer, std::uint8_t channel_id, const unsigned char *data, std::size_t length);
  void CheckFirstMessageDeadlines();

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
};

} // namespace relay
