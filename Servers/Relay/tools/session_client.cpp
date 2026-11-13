// Command-line test client for relay sessions. It creates or joins a session and prints every message it receives.
// It signs its own pass with the region key file, so it is for testing against a relay started with that key.
//
//   session_client create --relay 127.0.0.1:7777 --key keys/region.key --host-slot 0 --slots 0x03
//   session_client join   --relay 127.0.0.1:7777 --key keys/region.key --session <16 hex> --slot 1 --token <16 hex>
//   session_client idle   --relay 127.0.0.1:7777 --count 5
//
// The idle mode opens --count connections and sends nothing, to test the limit on connections without a first
// message. It prints how many were still connected after --wait seconds (default 3 in this mode).
//
// Optional: --exe-crc <n> --ini-crc <n> (pass build fingerprints, default 1), --region <n> (default 1),
// --ip a.b.c.d (address written in the pass, default 127.0.0.1), --pass-id <16 hex> (default random),
// --wait <seconds> (how long to listen, default 40), --leave (send LEAVE after START or after a WELCOME that
// is the last event), --expire <seconds> (make the pass this old, to test expiry).
//
// Exit code 0 when the match started, 1 for a refusal, a timeout or a connection error, 2 for bad arguments.

#include "pass.h"
#include "protocol.h"
#include "region_keys.h"
#include "wire.h"

#include <cstdio>
#include <cstdlib>
#include <ctime>
#include <enet/enet.h>
#include <map>
#include <sodium.h>
#include <string>
#include <vector>

namespace {

// Prints text to standard error. The result of the write is not interesting for a test tool.
void PrintError(const std::string &text) {
  static_cast<void>(std::fputs(text.c_str(), stderr));
}

struct Options {
  std::string m_mode;
  std::string m_relay = "127.0.0.1:7777";
  std::string m_key_file;
  std::string m_ip = "127.0.0.1";
  unsigned int m_host_slot = 0;
  unsigned int m_slot_mask = 0x03;
  unsigned int m_slot = 1;
  unsigned int m_region = 1;
  unsigned int m_count = 5;
  unsigned int m_wait_seconds = 40;
  std::uint32_t m_exe_crc = 1;
  std::uint32_t m_ini_crc = 1;
  std::uint64_t m_session_id = 0;
  std::uint64_t m_token = 0;
  std::uint64_t m_pass_id = 0;
  bool m_have_pass_id = false;
  bool m_leave = false;
  long m_pass_age_seconds = 0;
};

bool ParseNumber(const char *text, int base, unsigned long long &value) {
  char *end = nullptr;
  value = std::strtoull(text, &end, base);
  return end != text && *end == '\0';
}

bool ParseIp(const std::string &text, std::uint32_t &ip) {
  ip = 0;
  const char *position = text.c_str();
  for (int part = 0; part < 4; ++part) {
    char *end = nullptr;
    unsigned long value = std::strtoul(position, &end, 10);
    if (end == position || value > 255 || (part < 3 && *end != '.') || (part == 3 && *end != '\0')) {
      return false;
    }
    ip = (ip << 8) | static_cast<std::uint32_t>(value);
    position = end + 1;
  }
  return true;
}

bool ParseArguments(int argc, char **argv, Options &options) {
  if (argc < 2) {
    return false;
  }
  options.m_mode = argv[1];
  if (options.m_mode != "create" && options.m_mode != "join" && options.m_mode != "idle") {
    return false;
  }
  for (int i = 2; i < argc; ++i) {
    std::string name = argv[i];
    unsigned long long number = 0;
    if (name == "--leave") {
      options.m_leave = true;
      continue;
    }
    if (i + 1 >= argc) {
      return false;
    }
    const char *value = argv[++i];
    if (name == "--relay") {
      options.m_relay = value;
    } else if (name == "--key") {
      options.m_key_file = value;
    } else if (name == "--ip") {
      options.m_ip = value;
    } else if (name == "--host-slot" && ParseNumber(value, 0, number)) {
      options.m_host_slot = static_cast<unsigned int>(number);
    } else if (name == "--slots" && ParseNumber(value, 0, number)) {
      options.m_slot_mask = static_cast<unsigned int>(number);
    } else if (name == "--slot" && ParseNumber(value, 0, number)) {
      options.m_slot = static_cast<unsigned int>(number);
    } else if (name == "--count" && ParseNumber(value, 0, number)) {
      options.m_count = static_cast<unsigned int>(number);
    } else if (name == "--region" && ParseNumber(value, 0, number)) {
      options.m_region = static_cast<unsigned int>(number);
    } else if (name == "--wait" && ParseNumber(value, 0, number)) {
      options.m_wait_seconds = static_cast<unsigned int>(number);
    } else if (name == "--exe-crc" && ParseNumber(value, 0, number)) {
      options.m_exe_crc = static_cast<std::uint32_t>(number);
    } else if (name == "--ini-crc" && ParseNumber(value, 0, number)) {
      options.m_ini_crc = static_cast<std::uint32_t>(number);
    } else if (name == "--session" && ParseNumber(value, 16, number)) {
      options.m_session_id = number;
    } else if (name == "--token" && ParseNumber(value, 16, number)) {
      options.m_token = number;
    } else if (name == "--pass-id" && ParseNumber(value, 16, number)) {
      options.m_pass_id = number;
      options.m_have_pass_id = true;
    } else if (name == "--expire" && ParseNumber(value, 0, number)) {
      options.m_pass_age_seconds = static_cast<long>(number);
    } else {
      return false;
    }
  }
  return options.m_mode == "idle" || !options.m_key_file.empty();
}

bool BuildSignedPass(const Options &options, unsigned char *pass) {
  relay::RegionKey key;
  std::string error;
  if (!relay::LoadRegionKey(options.m_key_file, key, error)) {
    PrintError("key file: " + error + "\n");
    return false;
  }
  relay::PassFields fields;
  if (!ParseIp(options.m_ip, fields.client_ip)) {
    PrintError("bad --ip value\n");
    return false;
  }
  std::uint32_t now = static_cast<std::uint32_t>(std::time(nullptr));
  fields.region = static_cast<std::uint16_t>(options.m_region);
  fields.exe_crc = options.m_exe_crc;
  fields.ini_crc = options.m_ini_crc;
  fields.rtt_ms = 10;
  fields.issued_at = now - static_cast<std::uint32_t>(options.m_pass_age_seconds);
  fields.expires_at = fields.issued_at + relay::PASS_LIFETIME_SECONDS;
  if (options.m_have_pass_id) {
    fields.pass_id = options.m_pass_id;
  } else {
    randombytes_buf(&fields.pass_id, sizeof(fields.pass_id));
  }
  return relay::BuildPass(fields, key.secret, pass);
}

// Builds the CREATE or JOIN message. message must hold 1 + JOIN_BODY_SIZE bytes. Returns its length.
std::size_t BuildRequest(const Options &options, const unsigned char *pass, unsigned char *message) {
  std::size_t used = 0;
  bool create = options.m_mode == "create";
  message[used++] = create ? relay::MSG_CREATE : relay::MSG_JOIN;
  relay::PutU16(message + used, relay::PROTOCOL_VERSION);
  used += 2;
  for (std::size_t i = 0; i < relay::PASS_SIZE; ++i) {
    message[used++] = pass[i];
  }
  if (create) {
    message[used++] = static_cast<unsigned char>(options.m_host_slot);
    message[used++] = static_cast<unsigned char>(options.m_slot_mask);
  } else {
    relay::PutU64(message + used, options.m_session_id);
    used += 8;
    message[used++] = static_cast<unsigned char>(options.m_slot);
    relay::PutU64(message + used, options.m_token);
    used += 8;
  }
  return used;
}

// Prints one received message. Returns true when it ends the wait: START or REFUSE.
bool PrintMessage(const unsigned char *data, std::size_t length, bool &started) {
  using namespace relay;
  if (length == 0) {
    return false;
  }
  switch (data[0]) {
  case MSG_CREATED:
    if (length == 1 + CREATED_BODY_SIZE) {
      std::printf("CREATED session=%016llx\n", static_cast<unsigned long long>(GetU64(data + 1)));
      for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
        std::uint64_t token = GetU64(data + 9 + (slot * 8));
        if (token != 0) {
          std::printf("  token slot=%zu %016llx\n", slot, static_cast<unsigned long long>(token));
        }
      }
    }
    return false;
  case MSG_WELCOME:
    if (length == 1 + WELCOME_BODY_SIZE) {
      std::printf("WELCOME session=%016llx slot=%u connected=0x%02x leader=%u\n",
                  static_cast<unsigned long long>(GetU64(data + 1)), data[9], data[10], data[11]);
    }
    return false;
  case MSG_ROSTER:
    std::printf("ROSTER connected=0x%02x leader=%u\n", length > 1 ? data[1] : 0, length > 2 ? data[2] : 0);
    return false;
  case MSG_START:
    std::printf("START connected=0x%02x leader=%u\n", length > 1 ? data[1] : 0, length > 2 ? data[2] : 0);
    started = true;
    return true;
  case MSG_DROP:
    if (length == 1 + DROP_BODY_SIZE) {
      std::printf("DROP slot=%u reason=%u last_frame=%u\n", data[1], data[2], GetU32(data + 3));
    }
    return false;
  case MSG_REFUSE:
    std::printf("REFUSE code=%u\n", length > 1 ? data[1] : 0);
    return true;
  default:
    std::printf("message type %u, %zu bytes\n", data[0], length);
    return false;
  }
}

// Runs the session until START, REFUSE, a disconnect or the wait time. Returns the exit code.
int RunSession(ENetHost *host, ENetPeer *peer, const Options &options, const unsigned char *request,
               std::size_t request_length) {
  std::uint32_t deadline = enet_time_get() + (options.m_wait_seconds * 1000);
  bool request_sent = false;
  bool started = false;
  bool finished = false;
  ENetEvent event;
  while (!finished && static_cast<std::int32_t>(deadline - enet_time_get()) > 0) {
    if (enet_host_service(host, &event, 100) < 0) {
      PrintError("network error\n");
      return 1;
    }
    if (event.type == ENET_EVENT_TYPE_CONNECT && !request_sent) {
      ENetPacket *packet = enet_packet_create(request, request_length, ENET_PACKET_FLAG_RELIABLE);
      if (packet == nullptr || enet_peer_send(peer, 0, packet) != 0) {
        PrintError("cannot send the request\n");
        return 1;
      }
      request_sent = true;
    } else if (event.type == ENET_EVENT_TYPE_RECEIVE) {
      finished = PrintMessage(event.packet->data, event.packet->dataLength, started);
      enet_packet_destroy(event.packet);
    } else if (event.type == ENET_EVENT_TYPE_DISCONNECT) {
      std::printf("DISCONNECTED\n");
      return started ? 0 : 1;
    }
  }
  if (!finished) {
    std::printf("TIMEOUT waiting for the match to start\n");
  }
  if (options.m_leave && started) {
    unsigned char leave = relay::MSG_LEAVE;
    ENetPacket *packet = enet_packet_create(&leave, 1, ENET_PACKET_FLAG_RELIABLE);
    if (packet != nullptr && enet_peer_send(peer, 0, packet) == 0) {
      enet_host_flush(host);
    }
  }
  enet_peer_disconnect(peer, 0);
  enet_host_service(host, &event, 200);
  return started ? 0 : 1;
}

bool ResolveRelay(const Options &options, ENetAddress &address) {
  std::size_t colon = options.m_relay.rfind(':');
  unsigned long long port = 0;
  if (colon == std::string::npos || !ParseNumber(options.m_relay.c_str() + colon + 1, 10, port) || port > 65535) {
    PrintError("--relay must look like host:port\n");
    return false;
  }
  if (enet_address_set_host(&address, options.m_relay.substr(0, colon).c_str()) != 0) {
    PrintError("cannot resolve the relay address\n");
    return false;
  }
  address.port = static_cast<enet_uint16>(port);
  return true;
}

int RunClient(const Options &options) {
  unsigned char pass[relay::PASS_SIZE];
  if (!BuildSignedPass(options, pass)) {
    return 1;
  }
  unsigned char request[1 + relay::JOIN_BODY_SIZE];
  std::size_t request_length = BuildRequest(options, pass, request);

  ENetAddress address;
  if (!ResolveRelay(options, address)) {
    return 2;
  }
  ENetHost *host = enet_host_create(nullptr, 1, relay::CHANNEL_COUNT, 0, 0);
  if (host == nullptr) {
    PrintError("cannot create the ENet host\n");
    return 1;
  }
  ENetPeer *peer = enet_host_connect(host, &address, relay::CHANNEL_COUNT, 0);
  int result = 1;
  if (peer == nullptr) {
    PrintError("cannot start the connection\n");
  } else {
    result = RunSession(host, peer, options, request, request_length);
  }
  enet_host_destroy(host);
  return result;
}

// Opens options.m_count connections that never send a message and reports how many the relay kept.
int RunIdle(const Options &options) {
  ENetAddress address;
  if (!ResolveRelay(options, address)) {
    return 2;
  }
  ENetHost *host = enet_host_create(nullptr, options.m_count, relay::CHANNEL_COUNT, 0, 0);
  if (host == nullptr) {
    PrintError("cannot create the ENet host\n");
    return 1;
  }
  std::vector<ENetPeer *> peers;
  for (unsigned int i = 0; i < options.m_count; ++i) {
    ENetPeer *peer = enet_host_connect(host, &address, relay::CHANNEL_COUNT, 0);
    if (peer != nullptr) {
      peers.push_back(peer);
    }
  }
  unsigned int wait_seconds = options.m_wait_seconds == 40 ? 3 : options.m_wait_seconds;
  std::uint32_t deadline = enet_time_get() + (wait_seconds * 1000);
  ENetEvent event;
  while (static_cast<std::int32_t>(deadline - enet_time_get()) > 0) {
    static_cast<void>(enet_host_service(host, &event, 100));
  }
  unsigned int kept = 0;
  for (ENetPeer *peer : peers) {
    kept += peer->state == ENET_PEER_STATE_CONNECTED ? 1 : 0;
  }
  std::printf("opened=%zu kept=%u\n", peers.size(), kept);
  enet_host_destroy(host);
  return 0;
}

int RunTool(int argc, char **argv) {
  static_cast<void>(std::setvbuf(stdout, nullptr, _IONBF, 0)); // Show each message as it arrives, also when piped.
  Options options;
  if (!ParseArguments(argc, argv, options)) {
    PrintError("usage: session_client create|join|idle --key <key file> [--relay host:port] [options]\n"
               "see the comment at the top of tools/session_client.cpp\n");
    return 2;
  }
  if (sodium_init() < 0 || enet_initialize() != 0) {
    PrintError("cannot start libsodium or ENet\n");
    return 1;
  }
  int result = options.m_mode == "idle" ? RunIdle(options) : RunClient(options);
  enet_deinitialize();
  return result;
}

} // namespace

int main(int argc, char **argv) {
  try {
    return RunTool(argc, argv);
  } catch (...) {
    static_cast<void>(std::fputs("session_client: stopped by an unexpected error\n", stderr));
    return 1;
  }
}
