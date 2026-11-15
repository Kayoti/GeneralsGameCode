#include "vpn_check.h"

#include "text.h"

#include <algorithm>
#include <atomic>
#include <cstddef>
#include <cstdint>
#include <cstdio>
#include <fstream>
#include <maxminddb.h>
#include <utility>
#include <vector>
#include <winsock2.h>

namespace relay {

namespace {

// The longest line in a list file. The longest valid entry is "255.255.255.255/32".
const std::size_t MAX_LINE_BYTES = 512;
const std::size_t MAX_PREFIX_DIGITS = 2;
const std::size_t MAX_OPERATOR_DIGITS = 10;
const std::uint32_t MAX_PREFIX = 32;
const std::size_t TOUCH_STEP_BYTES = 4096;

std::atomic<bool> g_reload_requested(false);

// An inclusive range of IPv4 addresses, held as host-order integers.
struct AddressRange {
  std::uint32_t m_first = 0;
  std::uint32_t m_last = 0;
};

// One non-blank, non-comment line of a list file, with its line number for error messages.
struct ListEntry {
  std::size_t m_line = 0;
  std::string m_text;
};

bool IsDigit(char c) {
  return c >= '0' && c <= '9';
}

bool AllDigits(const std::string &text) {
  return !text.empty() && std::all_of(text.begin(), text.end(), IsDigit);
}

// Dotted-quad IPv4 address. Each part is one to three decimal digits with no leading zero, so no part can be read
// as octal.
bool ParseIPv4(const std::string &text, std::uint32_t &address) {
  std::uint32_t value = 0;
  std::size_t position = 0;
  for (std::size_t part = 0; part < 4; ++part) {
    if (part > 0) {
      if (position >= text.size() || text.at(position) != '.') {
        return false;
      }
      ++position;
    }
    const std::size_t START = position;
    std::uint32_t octet = 0;
    while (position < text.size() && IsDigit(text.at(position)) && position - START < 3) {
      octet = (octet * 10) + static_cast<std::uint32_t>(text.at(position) - '0');
      ++position;
    }
    const std::size_t DIGITS = position - START;
    if (DIGITS == 0 || (DIGITS > 1 && text.at(START) == '0') || octet > 255) {
      return false;
    }
    value = (value << 8) | octet;
  }
  if (position != text.size()) {
    return false;
  }
  address = value;
  return true;
}

// One network, written as "a.b.c.d/n". A bare address is one host, the same as /32. Host bits are cleared.
bool ParseNetwork(const std::string &text, AddressRange &range) {
  const std::size_t SLASH = text.find('/');
  std::uint32_t address = 0;
  if (!ParseIPv4(text.substr(0, SLASH), address)) {
    return false;
  }
  std::uint32_t prefix = MAX_PREFIX;
  if (SLASH != std::string::npos) {
    const std::string BITS = text.substr(SLASH + 1);
    if (BITS.size() > MAX_PREFIX_DIGITS || !AllDigits(BITS)) {
      return false;
    }
    prefix = 0;
    for (char c : BITS) {
      prefix = (prefix * 10) + static_cast<std::uint32_t>(c - '0');
    }
    if (prefix > MAX_PREFIX) {
      return false;
    }
  }
  const std::uint32_t MASK = prefix == 0 ? 0 : 0xFFFFFFFFu << (MAX_PREFIX - prefix);
  range.m_first = address & MASK;
  range.m_last = range.m_first | ~MASK;
  return true;
}

// Network operator number, in decimal, as held by the address-to-operator database.
bool ParseOperatorNumber(const std::string &text, std::uint32_t &number) {
  if (text.size() > MAX_OPERATOR_DIGITS || !AllDigits(text)) {
    return false;
  }
  unsigned long long value = 0;
  for (char c : text) {
    value = (value * 10) + static_cast<unsigned long long>(c - '0');
  }
  if (value > 0xFFFFFFFFull) {
    return false;
  }
  number = static_cast<std::uint32_t>(value);
  return true;
}

// Reads a list file. Blank lines and lines starting with '#' are skipped.
bool ReadListFile(const std::string &path, std::vector<ListEntry> &entries, std::string &error) {
  std::ifstream file(path.c_str(), std::ios::in | std::ios::binary);
  if (!file) {
    error = "cannot open '" + path + "'";
    return false;
  }
  std::string raw;
  std::size_t line_number = 0;
  while (std::getline(file, raw)) {
    ++line_number;
    if (raw.size() > MAX_LINE_BYTES) {
      error = "'" + path + "' line " + std::to_string(line_number) + " is too long";
      return false;
    }
    const std::string TRIMMED = TrimText(raw);
    if (TRIMMED.empty() || TRIMMED.front() == '#') {
      continue;
    }
    ListEntry entry;
    entry.m_line = line_number;
    entry.m_text = TRIMMED;
    entries.push_back(entry);
  }
  if (!file.eof()) {
    error = "cannot read '" + path + "'";
    return false;
  }
  return true;
}

// Sorts the ranges and joins those that overlap or touch, so that each address is in at most one range.
void MergeRanges(std::vector<AddressRange> &ranges) {
  std::sort(ranges.begin(), ranges.end(),
            [](const AddressRange &a, const AddressRange &b) { return a.m_first < b.m_first; });
  std::vector<AddressRange> merged;
  for (const AddressRange &range : ranges) {
    if (!merged.empty() && (range.m_first <= merged.back().m_last || range.m_first - 1 == merged.back().m_last)) {
      merged.back().m_last = (std::max)(merged.back().m_last, range.m_last);
    } else {
      merged.push_back(range);
    }
  }
  ranges.swap(merged);
}

bool LoadNetworks(const std::string &path, std::vector<AddressRange> &ranges, std::string &error) {
  std::vector<ListEntry> entries;
  if (!ReadListFile(path, entries, error)) {
    return false;
  }
  for (const ListEntry &entry : entries) {
    AddressRange range;
    if (!ParseNetwork(entry.m_text, range)) {
      error = "'" + path + "' line " + std::to_string(entry.m_line) +
              " is not an IPv4 address or network such as 198.51.100.0/24";
      return false;
    }
    ranges.push_back(range);
  }
  MergeRanges(ranges);
  return true;
}

bool LoadOperators(const std::string &path, std::vector<std::uint32_t> &operators, std::string &error) {
  std::vector<ListEntry> entries;
  if (!ReadListFile(path, entries, error)) {
    return false;
  }
  for (const ListEntry &entry : entries) {
    std::uint32_t number = 0;
    if (!ParseOperatorNumber(entry.m_text, number)) {
      error = "'" + path + "' line " + std::to_string(entry.m_line) + " is not a network operator number";
      return false;
    }
    operators.push_back(number);
  }
  std::sort(operators.begin(), operators.end());
  operators.erase(std::unique(operators.begin(), operators.end()), operators.end());
  return true;
}

// True when the address lies in one of the merged, sorted ranges.
bool InRanges(const std::vector<AddressRange> &ranges, std::uint32_t address) {
  auto after = std::upper_bound(ranges.begin(), ranges.end(), address,
                                [](std::uint32_t value, const AddressRange &range) { return value < range.m_first; });
  if (after == ranges.begin()) {
    return false;
  }
  --after;
  return address <= after->m_last;
}

// The address-to-operator database, read with libmaxminddb. The file is mapped read-only, and every page is read
// once at open, so lookups do not wait for the disk.
class OperatorDatabase {
public:
  OperatorDatabase() = default;
  ~OperatorDatabase();

  OperatorDatabase(const OperatorDatabase &) = delete;
  OperatorDatabase &operator=(const OperatorDatabase &) = delete;

  bool Open(const std::string &path, std::string &error);

  // Sets number to the network operator number of the address. Returns false when the database has no number for it.
  bool Lookup(std::uint32_t address, std::uint32_t &number) const;

private:
  MMDB_s m_db = {};
  bool m_open = false;
};

OperatorDatabase::~OperatorDatabase() {
  if (m_open) {
    MMDB_close(&m_db);
  }
}

bool OperatorDatabase::Open(const std::string &path, std::string &error) {
  const int STATUS = MMDB_open(path.c_str(), MMDB_MODE_MMAP, &m_db);
  if (STATUS != MMDB_SUCCESS) {
    error = "cannot open operator database '" + path + "': " + MMDB_strerror(STATUS);
    return false;
  }
  m_open = true;

  // libmaxminddb does not read the whole file itself. Reading one byte per page here brings every page in now.
  const volatile std::uint8_t *content = m_db.file_content;
  const std::size_t SIZE = static_cast<std::size_t>(m_db.file_size);
  for (std::size_t offset = 0; offset < SIZE; offset += TOUCH_STEP_BYTES) {
    static_cast<void>(content[offset]);
  }
  return true;
}

bool OperatorDatabase::Lookup(std::uint32_t address, std::uint32_t &number) const {
  if (!m_open) {
    return false;
  }
  sockaddr_in socket_address = {};
  socket_address.sin_family = AF_INET;
  socket_address.sin_addr.s_addr = htonl(address);

  int mmdb_error = MMDB_SUCCESS;
  const MMDB_lookup_result_s RESULT =
      MMDB_lookup_sockaddr(&m_db, reinterpret_cast<const sockaddr *>(&socket_address), &mmdb_error);
  if (mmdb_error != MMDB_SUCCESS || !RESULT.found_entry) {
    return false;
  }

  MMDB_entry_s entry = RESULT.entry;
  const char *const PATH[] = {"autonomous_system_number", nullptr};
  MMDB_entry_data_s data = {};
  if (MMDB_aget_value(&entry, &data, PATH) != MMDB_SUCCESS || !data.has_data || data.type != MMDB_DATA_TYPE_UINT32) {
    return false;
  }
  number = data.uint32;
  return true;
}

} // namespace

// The lists in memory and the operator database. Built as a whole, then swapped in only when all of it loaded.
struct VpnData {
  std::vector<AddressRange> m_blocked_ranges;
  std::vector<AddressRange> m_allowed_ranges;
  std::vector<std::uint32_t> m_blocked_operators;
  OperatorDatabase m_operator_db;
};

namespace {

bool BuildVpnData(const VpnSettings &settings, VpnData &data, std::string &error) {
  return LoadNetworks(settings.ranges_path, data.m_blocked_ranges, error) &&
         LoadNetworks(settings.allow_path, data.m_allowed_ranges, error) &&
         LoadOperators(settings.operators_path, data.m_blocked_operators, error) &&
         data.m_operator_db.Open(settings.operator_db_path, error);
}

} // namespace

VpnCheck::VpnCheck(VpnSettings settings) : m_settings(std::move(settings)) {}

VpnCheck::~VpnCheck() = default;

bool VpnCheck::Load(std::string &error) {
  if (!m_settings.enabled) {
    m_data.reset();
    return true;
  }
  std::unique_ptr<VpnData> data = std::make_unique<VpnData>();
  if (!BuildVpnData(m_settings, *data, error)) {
    return false;
  }
  m_data = std::move(data);
  return true;
}

VpnVerdict VpnCheck::Check(std::uint32_t address) const {
  VpnVerdict verdict;
  if (!m_data || InRanges(m_data->m_allowed_ranges, address)) {
    return verdict;
  }
  if (InRanges(m_data->m_blocked_ranges, address)) {
    verdict.reason = VpnReason::LISTED_RANGE;
    return verdict;
  }
  std::uint32_t number = 0;
  if (!m_data->m_blocked_operators.empty() && m_data->m_operator_db.Lookup(address, number) &&
      std::binary_search(m_data->m_blocked_operators.begin(), m_data->m_blocked_operators.end(), number)) {
    verdict.reason = VpnReason::LISTED_OPERATOR;
    verdict.operator_number = number;
  }
  return verdict;
}

void RequestVpnReload() {
  g_reload_requested.store(true);
}

bool TakeVpnReloadRequest() {
  return g_reload_requested.exchange(false);
}

std::string ShortAddress(std::uint32_t address) {
  char text[16] = {};
  static_cast<void>(std::snprintf(text, sizeof(text), "%u.%u.%u.x", (address >> 24) & 0xFFu, (address >> 16) & 0xFFu,
                                  (address >> 8) & 0xFFu));
  return std::string(text);
}

} // namespace relay
