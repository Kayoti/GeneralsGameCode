// Relay server entry point.
//
//   relay [config-file] [--no-address-check] [--strict-command-count] [--log-violations]
//   relay --generate-key <secret-key-file>
//
// The configuration file defaults to relay.conf in the working directory (PROTOCOL.md 7.2).
// --no-address-check turns off the pass address comparison. Use it only for testing on one PC.
// --strict-command-count makes a FRAMEINFO command count mismatch a refused record (PROTOCOL.md 4.6, after V7).
// --log-violations logs every refused match record with its type, frame and next_frame.
// --generate-key writes a new region key pair and exits.
// With the VPN check on, Ctrl+Break reloads its data files without a restart (PROTOCOL.md 4.4 and 7.2).

#include "config.h"
#include "entry_check.h"
#include "host.h"
#include "log.h"
#include "region_keys.h"
#include "vpn_check.h"

#include <cstdio>
#include <enet/enet.h>
#include <sodium.h>
#include <string>
#include <windows.h>

namespace {

const char *const DEFAULT_CONFIG_PATH = "relay.conf";

// Ctrl+Break asks for a VPN data reload. Other console events keep their default behaviour.
BOOL WINAPI OnConsoleControl(DWORD event) {
  if (event != CTRL_BREAK_EVENT) {
    return FALSE;
  }
  relay::RequestVpnReload();
  return TRUE;
}

relay::VpnSettings MakeVpnSettings(const relay::RelayConfig &config) {
  relay::VpnSettings settings;
  settings.enabled = config.vpn_check;
  settings.ranges_path = config.vpn_ranges;
  settings.operators_path = config.vpn_operators;
  settings.operator_db_path = config.vpn_operator_db;
  settings.allow_path = config.vpn_allow;
  return settings;
}

void PrintUsage() {
  relay::LogError("usage: relay [config-file] [--no-address-check] [--strict-command-count] [--log-violations]");
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
  relay::RecordSettings m_records;
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
    } else if (arg == "--strict-command-count") {
      options.m_records.m_strict_command_count = true;
    } else if (arg == "--log-violations") {
      options.m_records.m_log_violations = true;
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

  relay::DropSettings drop_settings;
  drop_settings.m_loading_ms = static_cast<std::uint32_t>(config.loading_time_s) * 1000;
  drop_settings.m_stall_ms = static_cast<std::uint32_t>(config.stall_time_s) * 1000;
  drop_settings.m_connection_timeout_ms = static_cast<std::uint32_t>(config.connection_timeout_s) * 1000;

  // The VPN data must be complete before the relay accepts anyone. A missing or invalid file stops the relay here.
  relay::VpnCheck vpn(MakeVpnSettings(config));
  if (!vpn.Load(error)) {
    relay::LogError("relay: VPN check: %s", error.c_str());
    return 1;
  }
  if (config.vpn_check) {
    relay::LogLine("VPN check on, its data loaded. Ctrl+Break reloads the data.");
    if (!SetConsoleCtrlHandler(OnConsoleControl, TRUE)) {
      relay::LogError("relay: Ctrl+Break reload could not be set up, the VPN data is not reloadable");
    }
  } else {
    relay::LogLine("VPN check off");
  }

  int exit_code = 0;
  {
    relay::RelayHost host(config.listen_port, entry_settings, vpn, options.m_check_address, config.max_sessions,
                          options.m_records, drop_settings);
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
