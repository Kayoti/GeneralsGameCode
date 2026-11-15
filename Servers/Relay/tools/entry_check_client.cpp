// Test client for the relay's entry check (PROTOCOL.md 4.4). It connects to a relay, sends CHECK_HELLO, answers the
// PINGs, and prints the result. It is used to test a relay by hand, not by the game.
//
//   entry_check_client <host> <port> [--proto N] [--delay-ms N] [--no-pong] [--trace]
//
// --proto N     Protocol version sent in CHECK_HELLO. Default 1.
// --delay-ms N  Holds each PONG back by N milliseconds, to test the ping limit.
// --no-pong     Never answers a PING, to test the timeout.
// --trace       Prints the arrival time of each PING on the client clock, to check the spacing.
//
// Exit codes: 0 PASS received, 1 REFUSE received, 2 no answer or connection failure, 64 bad arguments.
// The pass is opaque to the game. This tool only prints its region, round-trip time and lifetime.

#include "protocol.h"
#include "wire.h"

#include <cstddef>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <enet/enet.h>
#include <string>

namespace {

const std::uint32_t CONNECT_WAIT_MS = 5000;
const std::uint32_t RUN_LIMIT_MS = 10000;
const std::uint32_t SERVICE_WAIT_MS = 20;
const std::size_t MAX_PENDING_PONGS = 8;
const std::uint32_t TEST_EXE_CRC = 1;
const std::uint32_t TEST_INI_CRC = 2;
const int EXIT_PASS = 0;
const int EXIT_REFUSE = 1;
const int EXIT_NO_ANSWER = 2;
const int EXIT_USAGE = 64;

// Names of the REFUSE codes in PROTOCOL.md 4.9, indexed by code. Index 0 is unused.
const char *const REFUSE_NAMES[] = {
    "NONE",        "PROTO_MISMATCH", "VPN_DETECTED",       "PING_TOO_HIGH",    "TIMEOUT",
    "BAD_PASS",    "EXPIRED",        "WRONG_REGION",       "ADDRESS_MISMATCH", "PASS_IN_USE",
    "BAD_REQUEST", "NO_SESSION",     "ALREADY_STARTED",    "SLOT_TAKEN",       "NOT_EXPECTED",
    "BAD_TOKEN",   "BUILD_MISMATCH", "NOT_ENOUGH_PLAYERS", "SERVER_BUSY",      "RATE_LIMITED",
};

struct Options {
  std::string m_host;
  std::uint16_t m_port = 0;
  std::uint16_t m_proto = relay::PROTOCOL_VERSION;
  std::uint32_t m_delay_ms = 0;
  bool m_answer_pings = true;
  bool m_trace = false;
};

struct PendingPong {
  std::uint64_t m_nonce = 0;
  std::uint32_t m_due_ms = 0;
};

// What a received message means for the run.
enum class Outcome : std::uint8_t { CONTINUE, PASSED, REFUSED };

void PrintUsage() {
  static_cast<void>(std::fprintf(stderr, "usage: entry_check_client <host> <port> [--proto N] [--delay-ms N] "
                                         "[--no-pong]\n"));
}

// Parses a whole decimal number in the range 0 to max. Returns false for anything else.
bool ParseNumber(const std::string &text, unsigned long max, unsigned long &value) {
  if (text.empty() || text.size() > 10 || text.find_first_not_of("0123456789") != std::string::npos) {
    return false;
  }
  value = std::strtoul(text.c_str(), nullptr, 10);
  return value <= max;
}

bool ParseArguments(int argc, char **argv, Options &options) {
  if (argc < 3) {
    return false;
  }
  options.m_host = argv[1];
  unsigned long port = 0;
  if (!ParseNumber(argv[2], 65535, port) || port == 0) {
    return false;
  }
  options.m_port = static_cast<std::uint16_t>(port);

  for (int i = 3; i < argc; ++i) {
    std::string arg = argv[i];
    unsigned long value = 0;
    if (arg == "--no-pong") {
      options.m_answer_pings = false;
    } else if (arg == "--trace") {
      options.m_trace = true;
    } else if (arg == "--proto" && i + 1 < argc && ParseNumber(argv[i + 1], 65535, value)) {
      options.m_proto = static_cast<std::uint16_t>(value);
      ++i;
    } else if (arg == "--delay-ms" && i + 1 < argc && ParseNumber(argv[i + 1], 60000, value)) {
      options.m_delay_ms = static_cast<std::uint32_t>(value);
      ++i;
    } else {
      return false;
    }
  }
  return true;
}

// CHECK_HELLO body: u16 proto, u32 exe_crc, u32 ini_crc.
void SendHello(ENetPeer *peer, const Options &options) {
  unsigned char message[1 + relay::CHECK_HELLO_BODY_SIZE] = {};
  message[0] = relay::MSG_CHECK_HELLO;
  relay::PutU16(message + 1, options.m_proto);
  relay::PutU32(message + 3, TEST_EXE_CRC);
  relay::PutU32(message + 7, TEST_INI_CRC);
  ENetPacket *packet = enet_packet_create(message, sizeof(message), ENET_PACKET_FLAG_RELIABLE);
  if (packet != nullptr && enet_peer_send(peer, 0, packet) != 0) {
    enet_packet_destroy(packet);
  }
}

void SendPong(ENetPeer *peer, std::uint64_t nonce) {
  unsigned char message[1 + relay::PONG_BODY_SIZE] = {};
  message[0] = relay::MSG_PONG;
  relay::PutU64(message + 1, nonce);
  ENetPacket *packet = enet_packet_create(message, sizeof(message), ENET_PACKET_FLAG_RELIABLE);
  if (packet != nullptr && enet_peer_send(peer, 0, packet) != 0) {
    enet_packet_destroy(packet);
  }
}

void PrintPass(const unsigned char *pass) {
  std::uint16_t region = relay::GetU16(pass + 2);
  std::uint16_t rtt_ms = relay::GetU16(pass + 16);
  std::uint32_t issued_at = relay::GetU32(pass + 20);
  std::uint32_t expires_at = relay::GetU32(pass + 24);
  static_cast<void>(std::printf("PASS region %u rtt %u ms lifetime %u s\n", static_cast<unsigned int>(region),
                                static_cast<unsigned int>(rtt_ms), static_cast<unsigned int>(expires_at - issued_at)));
}

void PrintRefuse(const unsigned char *body) {
  std::uint8_t code = body[0];
  const char *name = code < sizeof(REFUSE_NAMES) / sizeof(REFUSE_NAMES[0]) ? REFUSE_NAMES[code] : "UNKNOWN";
  static_cast<void>(std::printf("REFUSE %s (code %u) rtt %u ms limit %u ms\n", name, static_cast<unsigned int>(code),
                                static_cast<unsigned int>(relay::GetU16(body + 1)),
                                static_cast<unsigned int>(relay::GetU16(body + 3))));
}

Outcome HandleMessage(const Options &options, const unsigned char *data, std::size_t length, std::uint32_t now_ms,
                      PendingPong *pending, std::size_t &pending_count) {
  if (length == 0) {
    return Outcome::CONTINUE;
  }
  switch (data[0]) {
  case relay::MSG_PING:
    if (length == 1 + relay::PING_BODY_SIZE && options.m_trace) {
      static_cast<void>(std::printf("PING arrived at %u ms (client clock)\n", static_cast<unsigned int>(now_ms)));
    }
    if (length == 1 + relay::PING_BODY_SIZE && options.m_answer_pings && pending_count < MAX_PENDING_PONGS) {
      pending[pending_count].m_nonce = relay::GetU64(data + 1);
      pending[pending_count].m_due_ms = now_ms + options.m_delay_ms;
      ++pending_count;
    }
    return Outcome::CONTINUE;
  case relay::MSG_PASS:
    if (length == 1 + relay::PASS_SIZE) {
      PrintPass(data + 1);
      return Outcome::PASSED;
    }
    break;
  case relay::MSG_REFUSE:
    if (length == 1 + relay::REFUSE_BODY_SIZE) {
      PrintRefuse(data + 1);
      return Outcome::REFUSED;
    }
    break;
  default:
    break;
  }
  static_cast<void>(
      std::fprintf(stderr, "unexpected message type %u of %zu bytes\n", static_cast<unsigned int>(data[0]), length));
  return Outcome::CONTINUE;
}

void SendDuePongs(ENetPeer *peer, std::uint32_t now_ms, PendingPong *pending, std::size_t &pending_count) {
  std::size_t kept = 0;
  for (std::size_t i = 0; i < pending_count; ++i) {
    if (pending[i].m_due_ms <= now_ms) {
      SendPong(peer, pending[i].m_nonce);
    } else {
      pending[kept++] = pending[i];
    }
  }
  pending_count = kept;
}

int RunCheck(const Options &options) {
  ENetHost *client = enet_host_create(nullptr, 1, relay::CHANNEL_COUNT, 0, 0);
  if (client == nullptr) {
    static_cast<void>(std::fprintf(stderr, "cannot create the client host\n"));
    return EXIT_NO_ANSWER;
  }

  ENetAddress address;
  if (enet_address_set_host(&address, options.m_host.c_str()) != 0) {
    static_cast<void>(std::fprintf(stderr, "cannot resolve '%s'\n", options.m_host.c_str()));
    enet_host_destroy(client);
    return EXIT_NO_ANSWER;
  }
  address.port = options.m_port;

  ENetPeer *peer = enet_host_connect(client, &address, relay::CHANNEL_COUNT, 0);
  if (peer == nullptr) {
    static_cast<void>(std::fprintf(stderr, "cannot start the connection\n"));
    enet_host_destroy(client);
    return EXIT_NO_ANSWER;
  }

  int exit_code = EXIT_NO_ANSWER;
  bool connected = false;
  bool finished = false;
  std::uint32_t start_ms = enet_time_get();
  PendingPong pending[MAX_PENDING_PONGS];
  std::size_t pending_count = 0;

  while (!finished) {
    std::uint32_t now_ms = enet_time_get();
    if (!connected && now_ms - start_ms >= CONNECT_WAIT_MS) {
      static_cast<void>(std::fprintf(stderr, "no connection to the relay\n"));
      break;
    }
    if (now_ms - start_ms >= RUN_LIMIT_MS) {
      static_cast<void>(std::fprintf(stderr, "no answer from the relay\n"));
      break;
    }

    ENetEvent event;
    if (enet_host_service(client, &event, SERVICE_WAIT_MS) > 0) {
      if (event.type == ENET_EVENT_TYPE_CONNECT) {
        connected = true;
        SendHello(peer, options);
      } else if (event.type == ENET_EVENT_TYPE_RECEIVE) {
        Outcome outcome = HandleMessage(options, event.packet->data, event.packet->dataLength, enet_time_get(), pending,
                                        pending_count);
        enet_packet_destroy(event.packet);
        if (outcome != Outcome::CONTINUE) {
          finished = true;
          exit_code = outcome == Outcome::PASSED ? EXIT_PASS : EXIT_REFUSE;
        }
      } else if (event.type == ENET_EVENT_TYPE_DISCONNECT) {
        static_cast<void>(std::fprintf(stderr, "the relay closed the connection without an answer\n"));
        break;
      }
    }
    SendDuePongs(peer, enet_time_get(), pending, pending_count);
  }

  enet_peer_reset(peer);
  enet_host_destroy(client);
  return exit_code;
}

int RunClient(int argc, char **argv) {
  Options options;
  if (!ParseArguments(argc, argv, options)) {
    PrintUsage();
    return EXIT_USAGE;
  }
  if (enet_initialize() != 0) {
    static_cast<void>(std::fprintf(stderr, "ENet could not start\n"));
    return EXIT_NO_ANSWER;
  }
  int exit_code = RunCheck(options);
  enet_deinitialize();
  return exit_code;
}

} // namespace

int main(int argc, char **argv) {
  try {
    return RunClient(argc, argv);
  } catch (...) {
    static_cast<void>(std::fprintf(stderr, "entry check client stopped by an unexpected error\n"));
    return EXIT_NO_ANSWER;
  }
}
