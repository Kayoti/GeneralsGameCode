// Match records: header scan, per-type checks, frame tracking and counters (PROTOCOL.md section 4.6).
//
// Nothing here knows about ENet or sessions. The session manager gives each record to ScanRecord and then to
// MatchRecords::Accept, and forwards the record when both agree.
#pragma once

#include "protocol.h"

#include <cstddef>
#include <cstdint>

namespace relay {

// Why a record or a DATA message was refused. FAULT_COUNT sizes the per-reason counters.
enum RecordFault : std::uint8_t {
  FAULT_NONE = 0,
  FAULT_BAD_FRAMING,   // The length prefixes do not fit the DATA message, or it holds no record.
  FAULT_BAD_LENGTH,    // Record length outside 7 to 4096.
  FAULT_BAD_HEADER,    // A tag is missing, repeated, unexpected, or the header runs past the record.
  FAULT_UNKNOWN_TYPE,  // T is not a carried command type.
  FAULT_BAD_DATA,      // The data the relay reads is cut short.
  FAULT_WRONG_PLAYER,  // P differs from the connection's slot.
  FAULT_FRAME_WINDOW,  // F is outside next_frame to next_frame + 128.
  FAULT_WRONG_FRAME,   // FRAMEINFO whose F is not next_frame.
  FAULT_WRONG_LEAVER,  // PLAYERLEAVE for a slot other than the sender's.
  FAULT_NOT_LEADER,    // RUNAHEAD from a slot that is not leader_slot.
  FAULT_BAD_RUN_AHEAD, // run_ahead or frame_rate outside the stated range.
  FAULT_NOT_HOST,      // TIMEOUTSTART from a slot that is not the host slot.
  FAULT_COMMAND_COUNT, // command_count differs from the commands counted, with the strict setting on.
  FAULT_BEFORE_START,  // DATA before START.
  FAULT_RATE,          // More than 64 KB or 600 records in one second.
  FAULT_COUNT
};

// Short name for logs and counters.
const char *FaultName(RecordFault fault);

// Relay settings for records. Both are off by default.
struct RecordSettings {
  bool m_strict_command_count = false; // A command_count mismatch is a violation (PROTOCOL.md 4.6, after V7).
  bool m_log_violations = false;       // Log every refused record with its type, frame and next_frame.
};

// The fields of a record header that the relay uses. The has_ flags say which tags the type carries.
struct RecordHeader {
  std::uint8_t m_type = 0;
  bool m_has_relay = false;
  std::uint8_t m_relay = 0;
  bool m_has_frame = false;
  std::uint32_t m_frame = 0;
  std::uint8_t m_player = 0;
  std::uint16_t m_command_count = 0; // FRAMEINFO.
  std::uint8_t m_leaving_slot = 0;   // PLAYERLEAVE.
  std::uint16_t m_run_ahead = 0;     // RUNAHEAD.
  std::uint8_t m_frame_rate = 0;     // RUNAHEAD.
};

// Checks 1 to 3 of PROTOCOL.md 4.6 on one record of `length` bytes: the length, the tag list for the record's type
// and the type itself. Reads the data fields the relay needs. Never reads outside record[0, length).
RecordFault ScanRecord(const unsigned char *record, std::size_t length, RecordHeader &header);

// What the session knows about the sender of a record.
struct MatchContext {
  std::uint8_t m_sender = 0; // Slot bound to the connection.
  std::uint8_t m_connected_mask = 0;
  std::uint8_t m_leader_slot = 0;
  std::uint8_t m_host_slot = 0;
  bool m_strict_command_count = false;
};

struct RecordOutcome {
  std::uint8_t m_forward_mask = 0; // Slots that receive the record. Never includes the sender.
  bool m_count_mismatch = false;   // FRAMEINFO whose command_count differs from the commands counted.
  std::uint16_t m_counted = 0;     // The commands counted for that frame.
};

// Counters kept for each session. m_forwarded counts valid records, whatever the number of recipients.
struct SessionCounters {
  std::uint64_t m_records_in = 0;
  std::uint64_t m_forwarded = 0;
  std::uint64_t m_count_mismatches = 0;
  std::uint64_t m_dropped[FAULT_COUNT] = {};
};

// Frame tracking, command counting, data rate limits and failure counts of one session.
class MatchRecords {
public:
  MatchRecords() = default;

  // Checks 4 and 5 of PROTOCOL.md 4.6 and the forwarding rule for the record's type. On success the frame tracking
  // state changes and `outcome` is filled in. On failure nothing changes.
  RecordFault Accept(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);

  // Data rate limit (PROTOCOL.md 6): adds to the slot's current one-second window. False when a limit is passed.
  bool TakeBytes(std::uint8_t slot, std::uint32_t now_ms, std::size_t bytes);
  bool TakeRecord(std::uint8_t slot, std::uint32_t now_ms);

  // Counts a refused record for the slot. Returns true once the slot has reached the failed record limit.
  bool Reject(std::uint8_t slot, RecordFault fault);

  // Counts a refusal that is not a failed record, such as a rate limit.
  void CountDrop(RecordFault fault);
  void CountIn();
  void CountForwarded();

  std::uint32_t NextFrame(std::uint8_t slot) const;
  const SessionCounters &Counters() const;

private:
  struct FrameCount {
    std::uint32_t m_frame = 0;
    std::uint16_t m_count = 0;
  };

  struct SlotState {
    std::uint32_t m_next_frame = INITIAL_NEXT_FRAME;
    std::size_t m_failed_records = 0;
    bool m_window_open = false;
    std::uint32_t m_window_start_ms = 0;
    std::size_t m_window_bytes = 0;
    std::size_t m_window_records = 0;
    FrameCount m_counts[256]; // Indexed by frame & 255. The frame window is 129 frames, so live frames never share.
  };

  RecordFault AcceptFrameInfo(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);
  RecordFault AcceptRunAhead(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);
  RecordFault AcceptPlayerLeave(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);
  RecordFault AcceptGameCommand(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);
  RecordFault AcceptOthers(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome);
  void OpenWindow(SlotState &slot, std::uint32_t now_ms);
  void CountCommand(SlotState &slot, std::uint32_t frame);

  SlotState slots_[SLOT_COUNT];
  SessionCounters counters_;
};

} // namespace relay
