// The relay's ENet host: listens for connections, logs them, and enforces the first-message deadline
// (PROTOCOL.md 4.1 and 6).
#pragma once

#include <cstddef>
#include <cstdint>
#include <string>

struct _ENetEvent;
struct _ENetHost;
struct _ENetPeer;

namespace relay {

class RelayHost {
public:
  explicit RelayHost(std::uint16_t listen_port);
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

  std::uint16_t listen_port_;
  std::uint32_t next_connection_id_;
  _ENetHost *host_;
};

} // namespace relay
