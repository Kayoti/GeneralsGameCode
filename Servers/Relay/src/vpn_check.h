// VPN check for the entry check (PROTOCOL.md 4.4, DECISIONS D20 to D24).
// The data files are read into memory at start and on reload. A lookup never touches a file or the network.
#pragma once

#include <cstdint>
#include <memory>
#include <string>

namespace relay {

// The VPN check files named in the relay configuration (PROTOCOL.md 7.2). The paths are used only when enabled.
struct VpnSettings {
  bool enabled = false;
  std::string ranges_path;
  std::string operators_path;
  std::string operator_db_path;
  std::string allow_path;
};

// Why an address is flagged. NONE means the address passes the check.
enum class VpnReason : std::uint8_t {
  NONE,
  LISTED_RANGE,
  LISTED_OPERATOR,
};

struct VpnVerdict {
  VpnReason reason = VpnReason::NONE;
  std::uint32_t operator_number = 0; // Set when reason is LISTED_OPERATOR.
};

struct VpnData;

class VpnCheck {
public:
  explicit VpnCheck(VpnSettings settings);
  ~VpnCheck();

  VpnCheck(const VpnCheck &) = delete;
  VpnCheck &operator=(const VpnCheck &) = delete;

  // Reads every file named in the settings and replaces the data in use. With the check off, it only clears the data.
  // If any file cannot be read or is invalid, error says why and the data in use is kept.
  bool Load(std::string &error);

  // Classifies an IPv4 address held as a host-order integer. The allow list is checked first and overrides the
  // range list and the operator list.
  VpnVerdict Check(std::uint32_t address) const;

private:
  VpnSettings m_settings;
  std::unique_ptr<VpnData> m_data;
};

// Asks the relay to reload the VPN data at its next loop pass. Safe to call from another thread, such as a console
// control handler. TakeVpnReloadRequest returns true once for each request.
void RequestVpnReload();
bool TakeVpnReloadRequest();

// The address with its last octet hidden, such as "203.0.113.x", for log lines.
std::string ShortAddress(std::uint32_t address);

} // namespace relay
