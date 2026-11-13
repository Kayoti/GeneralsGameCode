// Relay server entry point.
//
//   relay [config-file] [--no-address-check]
//   relay --generate-key <secret-key-file>
//
// The configuration file defaults to relay.conf in the working directory (PROTOCOL.md 7.2).
// --no-address-check turns off the pass address comparison. Use it only for testing on one PC.
// --generate-key writes a new region key pair and exits.

#include "config.h"
#include "entry_check.h"
#include "host.h"
#include "log.h"
#include "region_keys.h"

#include <cstdio>
#include <enet/enet.h>
#include <sodium.h>
#include <string>

namespace {

const char *const DEFAULT_CONFIG_PATH = "relay.conf";

void PrintUsage() {
  relay::LogError("usage: relay [config-file] [--no-address-check]");
  relay::LogError("       relay --generate-key <secret-key-file>");
}

int GenerateKeys(const std::string &secret_path) {
  if (sodium_init() < 0) {
    relay::LogError("relay: libsodium could not start");
    return 1;
  }
  std::string error;
  if (!relay::GenerateRegionKeys(secret_path, error)) {
    relay::LogError("relay: %s", error.c_str());
    return 1;
  }
  relay::LogLine("region key pair written: secret key to '%s', public key to '%s.pub'", secret_path.c_str(),
                 secret_path.c_str());
  return 0;
}

struct Options {
  std::string m_config_path = DEFAULT_CONFIG_PATH;
  bool m_check_address = true;
  bool m_generate_keys = false;
  std::string m_generate_path;
};

// Returns false for an unknown or malformed argument.
bool ParseArguments(int argc, char **argv, Options &options) {
  bool config_path_given = false;
  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if (arg == "--generate-key" && i + 1 < argc) {
      options.m_generate_keys = true;
      options.m_generate_path = argv[++i];
    } else if (arg == "--no-address-check") {
      options.m_check_address = false;
    } else if (!config_path_given && !arg.empty() && arg.front() != '-') {
      options.m_config_path = arg;
      config_path_given = true;
    } else {
      return false;
    }
  }
  return true;
}

int RunRelay(int argc, char **argv) {
  Options options;
  if (!ParseArguments(argc, argv, options)) {
    PrintUsage();
    return 2;
  }

  if (options.m_generate_keys) {
    return GenerateKeys(options.m_generate_path);
  }

  relay::RelayConfig config;
  std::string error;
  if (!relay::LoadRelayConfig(options.m_config_path, config, error)) {
    relay::LogError("relay: invalid configuration: %s", error.c_str());
    return 1;
  }

  relay::RegionKey region_key;
  if (!relay::LoadRegionKey(config.key_file, region_key, error)) {
    relay::LogError("relay: key_file: %s", error.c_str());
    return 1;
  }
  relay::LogLine("region key loaded from '%s'", config.key_file.c_str());

  if (sodium_init() < 0) {
    relay::LogError("relay: libsodium could not start");
    return 1;
  }
  if (enet_initialize() != 0) {
    relay::LogError("relay: ENet could not start");
    return 1;
  }

  relay::EntryCheckSettings entry_settings;
  entry_settings.region = config.region;
  entry_settings.ping_limit_ms = config.ping_limit_ms;
  entry_settings.key = region_key;

  int exit_code = 0;
  {
    relay::RelayHost host(config.listen_port, entry_settings, options.m_check_address, config.max_sessions);
    if (!host.Start(error)) {
      relay::LogError("relay: %s", error.c_str());
      exit_code = 1;
    } else {
      relay::LogLine("relay for region %u listening on UDP port %u", static_cast<unsigned int>(config.region),
                     static_cast<unsigned int>(config.listen_port));
      if (!options.m_check_address) {
        relay::LogLine(
            "WARNING: the pass address check is off (--no-address-check). Use this for testing on one PC only.");
      }
      if (!host.Run()) {
        exit_code = 1;
      }
    }
  }

  enet_deinitialize();
  return exit_code;
}

} // namespace

int main(int argc, char **argv) {
  try {
    return RunRelay(argc, argv);
  } catch (...) {
    relay::LogError("relay: stopped by an unexpected error");
    return 1;
  }
}
