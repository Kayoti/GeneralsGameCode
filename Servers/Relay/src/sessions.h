// Match sessions: create, join, roster, start (PROTOCOL.md section 4.5).
//
// The manager knows nothing about ENet. It receives parsed requests and writes its replies through SessionOutput,
// so the same code runs in the relay and in the unit tests. A connection is identified by the id the host gave it.
#pragma once

#include "protocol.h"
#include "rate_limit.h"

#include <cstddef>
#include <cstdint>
#include <unordered_map>
#include <unordered_set>

namespace relay {

// Where the manager sends messages. Both calls take effect after the manager returns to its caller.
class SessionOutput {
public:
  virtual ~SessionOutput() {}

  // Queues one complete message (type byte included) on the reliable channel.
  virtual void Send(std::uint32_t connection, const unsigned char *message, std::size_t length) = 0;

  // Closes the connection once messages already queued for it have been sent.
  virtual void Close(std::uint32_t connection) = 0;
};

struct SessionSettings {
  const unsigned char *public_key = nullptr; // Region public key used to check passes.
  std::uint16_t region = 0;
  bool check_address = true; // False only with the relay's test start option.
  std::size_t max_sessions = 200;
};

// What the host knows about one received first message.
struct SessionRequest {
  std::uint32_t connection = 0;
  std::uint32_t source_ip = 0; // Host-order IPv4 address of the UDP source.
  std::uint32_t now_ms = 0;    // Monotonic clock in milliseconds. Differences are taken with unsigned arithmetic.
  std::uint32_t now_unix = 0;  // Wall clock in Unix seconds, for pass expiry.
};

class SessionManager {
public:
  SessionManager(const SessionSettings &settings, SessionOutput &output);

  SessionManager(const SessionManager &) = delete;
  SessionManager &operator=(const SessionManager &) = delete;

  // body is the CREATE or JOIN message without its type byte. Both always answer the connection: with
  // CREATED and WELCOME, WELCOME, or REFUSE followed by a close.
  void OnCreate(const SessionRequest &request, const unsigned char *body, std::size_t length);
  void OnJoin(const SessionRequest &request, const unsigned char *body, std::size_t length);

  // LEAVE from a bound connection. Has the effect of closing the connection.
  void OnLeave(std::uint32_t connection);

  // The connection ended for any reason. Unknown connections are ignored.
  void OnClosed(std::uint32_t connection);

  // Checks the 30 second start timers.
  void Tick(std::uint32_t now_ms);

  // True while the connection holds a slot in a session.
  bool IsBound(std::uint32_t connection) const;

  std::size_t SessionCount() const;

private:
  struct Session {
    std::uint8_t m_host_slot = 0;
    std::uint8_t m_expected_mask = 0;
    std::uint8_t m_connected_mask = 0;
    std::uint64_t m_tokens[SLOT_COUNT] = {}; // 0 once used, and for slots that are not expected joiners.
    std::uint32_t m_connections[SLOT_COUNT] = {};
    std::uint64_t m_pass_ids[SLOT_COUNT] = {};
    std::uint32_t m_exe_crc = 0;
    std::uint32_t m_ini_crc = 0;
    std::uint32_t m_created_ms = 0;
    std::size_t m_failed_joins = 0;
    bool m_started = false;
  };

  struct Binding {
    std::uint64_t m_session_id = 0;
    std::uint8_t m_slot = 0;
  };

  struct CheckedPass;

  bool CheckRequestPass(const SessionRequest &request, const unsigned char *pass, CheckedPass &checked);
  void Refuse(std::uint32_t connection, RefuseCode code);
  bool CheckJoin(const Session &session, std::uint8_t slot, std::uint64_t token, const CheckedPass &pass,
                 RefuseCode &refuse) const;
  void AcceptJoin(const SessionRequest &request, std::uint64_t session_id, Session &session, std::uint8_t slot,
                  const CheckedPass &pass);
  std::uint64_t NewSessionId() const;
  void Bind(std::uint64_t session_id, Session &session, std::uint8_t slot, std::uint32_t connection,
            std::uint64_t pass_id);
  void ReleaseConnection(std::uint32_t connection);
  void SendWelcome(std::uint64_t session_id, const Session &session, std::uint8_t slot);
  void SendToConnected(const Session &session, const unsigned char *message, std::size_t length);
  void SendRoster(const Session &session);
  void SendStart(Session &session);
  void ExpireSession(std::uint64_t session_id, Session &session);
  void RemoveSession(std::uint64_t session_id);

  SessionSettings settings_;
  SessionOutput &output_;
  AddressRateLimiter create_limiter_;
  AddressRateLimiter join_limiter_;
  std::unordered_map<std::uint64_t, Session> sessions_;
  std::unordered_map<std::uint32_t, Binding> bindings_;
  std::unordered_set<std::uint64_t> live_pass_ids_;
};

// Lowest set bit of a connected mask, which is the leader slot (PROTOCOL.md 4.6). The mask must not be 0.
std::uint8_t LeaderSlot(std::uint8_t connected_mask);

} // namespace relay
