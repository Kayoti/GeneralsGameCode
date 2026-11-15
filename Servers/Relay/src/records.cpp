#include "records.h"

#include "wire.h"

namespace relay {

namespace {

// Tag bytes of a record header (PROTOCOL.md 4.6).
const unsigned char TAG_TYPE = 'T';
const unsigned char TAG_RELAY = 'R';
const unsigned char TAG_FRAME = 'F';
const unsigned char TAG_PLAYER = 'P';
const unsigned char TAG_COMMAND_ID = 'C';
const unsigned char TAG_DATA = 'D';

// Bytes that follow a tag.
std::size_t TagSize(unsigned char tag) {
  switch (tag) {
  case TAG_TYPE:
  case TAG_RELAY:
  case TAG_PLAYER:
    return 1;
  case TAG_COMMAND_ID:
    return 2;
  case TAG_FRAME:
    return 4;
  default:
    return 0; // TAG_DATA.
  }
}

// The tags that follow T, in order, for each command type. Null when the type is not carried (PROTOCOL.md 4.6).
const char *TagOrder(std::uint8_t type) {
  switch (type) {
  case COMMAND_GAMECOMMAND:
  case COMMAND_FRAMEINFO:
  case COMMAND_CHAT:
    return "FRPCD";
  case COMMAND_PLAYERLEAVE:
  case COMMAND_RUNAHEAD:
    return "RFPCD";
  case COMMAND_RUNAHEADMETRICS:
  case COMMAND_LOADCOMPLETE:
  case COMMAND_TIMEOUTSTART:
    return "RPCD";
  case COMMAND_PROGRESS:
    return "RPD";
  default:
    return nullptr;
  }
}

// Stores the value that follows a tag. The caller has checked that TagSize(tag) bytes are there.
void ReadField(unsigned char tag, const unsigned char *value, RecordHeader &header) {
  switch (tag) {
  case TAG_RELAY:
    header.m_has_relay = true;
    header.m_relay = value[0];
    break;
  case TAG_FRAME:
    header.m_has_frame = true;
    header.m_frame = GetU32(value);
    break;
  case TAG_PLAYER:
    header.m_player = value[0];
    break;
  default:
    break; // The command id is not used by the relay.
  }
}

// Reads the data the relay needs for the type. `data` holds `length` bytes after the D tag.
RecordFault ReadData(const unsigned char *data, std::size_t length, RecordHeader &header) {
  switch (header.m_type) {
  case COMMAND_FRAMEINFO:
    if (length < 2) {
      return FAULT_BAD_DATA;
    }
    header.m_command_count = GetU16(data);
    break;
  case COMMAND_PLAYERLEAVE:
    if (length < 1) {
      return FAULT_BAD_DATA;
    }
    header.m_leaving_slot = data[0];
    break;
  case COMMAND_RUNAHEAD:
    if (length < 3) {
      return FAULT_BAD_DATA;
    }
    header.m_run_ahead = GetU16(data);
    header.m_frame_rate = data[2];
    break;
  default:
    break;
  }
  return FAULT_NONE;
}

std::uint8_t SlotBit(std::uint8_t slot) {
  return static_cast<std::uint8_t>(1u << slot);
}

// Every connected slot except the sender.
std::uint8_t OthersMask(const MatchContext &context) {
  return static_cast<std::uint8_t>(context.m_connected_mask & ~SlotBit(context.m_sender));
}

// next_frame <= frame <= next_frame + 128, written so that it cannot overflow.
bool InFrameWindow(std::uint32_t next_frame, std::uint32_t frame) {
  return frame >= next_frame && frame - next_frame <= FRAME_WINDOW;
}

} // namespace

const char *FaultName(RecordFault fault) {
  switch (fault) {
  case FAULT_BAD_FRAMING:
    return "bad framing";
  case FAULT_BAD_LENGTH:
    return "bad length";
  case FAULT_BAD_HEADER:
    return "bad header";
  case FAULT_UNKNOWN_TYPE:
    return "unknown type";
  case FAULT_BAD_DATA:
    return "short data";
  case FAULT_WRONG_PLAYER:
    return "wrong player";
  case FAULT_FRAME_WINDOW:
    return "frame outside window";
  case FAULT_WRONG_FRAME:
    return "frame is not next_frame";
  case FAULT_WRONG_LEAVER:
    return "wrong leaving slot";
  case FAULT_NOT_LEADER:
    return "not the leader";
  case FAULT_BAD_RUN_AHEAD:
    return "run-ahead out of range";
  case FAULT_NOT_HOST:
    return "not the host";
  case FAULT_COMMAND_COUNT:
    return "command count";
  case FAULT_BEFORE_START:
    return "before start";
  case FAULT_RATE:
    return "rate limit";
  default:
    return "none";
  }
}

RecordFault ScanRecord(const unsigned char *record, std::size_t length, RecordHeader &header) {
  if (length < MIN_RECORD_SIZE || length > MAX_RECORD_SIZE) {
    return FAULT_BAD_LENGTH;
  }
  if (record[0] != TAG_TYPE) {
    return FAULT_BAD_HEADER;
  }
  header = RecordHeader();
  header.m_type = record[1];
  const char *order = TagOrder(header.m_type);
  if (order == nullptr) {
    return FAULT_UNKNOWN_TYPE;
  }

  std::size_t position = 2;
  for (const char *expected = order; *expected != '\0'; ++expected) {
    const unsigned char TAG = static_cast<unsigned char>(*expected);
    if (position >= length || record[position] != TAG) {
      return FAULT_BAD_HEADER;
    }
    ++position;
    const std::size_t SIZE = TagSize(TAG);
    if (SIZE > length - position) {
      return FAULT_BAD_HEADER;
    }
    ReadField(TAG, record + position, header);
    position += SIZE;
  }
  return ReadData(record + position, length - position, header);
}

RecordFault MatchRecords::Accept(const RecordHeader &header, const MatchContext &context, RecordOutcome &outcome) {
  if (header.m_player != context.m_sender || context.m_sender >= SLOT_COUNT) {
    return FAULT_WRONG_PLAYER;
  }
  outcome = RecordOutcome();
  switch (header.m_type) {
  case COMMAND_GAMECOMMAND:
    return AcceptGameCommand(header, context, outcome);
  case COMMAND_FRAMEINFO:
    return AcceptFrameInfo(header, context, outcome);
  case COMMAND_PLAYERLEAVE:
    return AcceptPlayerLeave(header, context, outcome);
  case COMMAND_RUNAHEAD:
    return AcceptRunAhead(header, context, outcome);
  default:
    return AcceptOthers(header, context, outcome);
  }
}

// Adds one command to the count for its frame. The entry restarts when it still holds an older frame. Frames inside
// the window are 129 apart at most, so two live frames never share an entry.
void MatchRecords::CountCommand(SlotState &slot, std::uint32_t frame) {
  FrameCount &entry = slot.m_counts[frame & 0xFF];
  if (entry.m_frame != frame) {
    entry.m_frame = frame;
    entry.m_count = 0;
  }
  if (entry.m_count < 0xFFFF) {
    ++entry.m_count;
  }
}

RecordFault MatchRecords::AcceptGameCommand(const RecordHeader &header, const MatchContext &context,
                                            RecordOutcome &outcome) {
  SlotState &slot = slots_[context.m_sender];
  if (!InFrameWindow(slot.m_next_frame, header.m_frame)) {
    return FAULT_FRAME_WINDOW;
  }
  CountCommand(slot, header.m_frame);
  outcome.m_forward_mask = OthersMask(context);
  return FAULT_NONE;
}

RecordFault MatchRecords::AcceptFrameInfo(const RecordHeader &header, const MatchContext &context,
                                          RecordOutcome &outcome) {
  SlotState &slot = slots_[context.m_sender];
  if (header.m_frame != slot.m_next_frame) {
    return FAULT_WRONG_FRAME;
  }
  FrameCount &entry = slot.m_counts[header.m_frame & 0xFF];
  outcome.m_counted = entry.m_frame == header.m_frame ? entry.m_count : 0;
  if (outcome.m_counted != header.m_command_count) {
    outcome.m_count_mismatch = true;
    ++counters_.m_count_mismatches;
    if (context.m_strict_command_count) {
      return FAULT_COMMAND_COUNT;
    }
  }
  entry = FrameCount();
  ++slot.m_next_frame;
  outcome.m_forward_mask = OthersMask(context);
  return FAULT_NONE;
}

RecordFault MatchRecords::AcceptPlayerLeave(const RecordHeader &header, const MatchContext &context,
                                            RecordOutcome &outcome) {
  SlotState &slot = slots_[context.m_sender];
  if (!InFrameWindow(slot.m_next_frame, header.m_frame)) {
    return FAULT_FRAME_WINDOW;
  }
  if (header.m_leaving_slot != context.m_sender) {
    return FAULT_WRONG_LEAVER;
  }
  CountCommand(slot, header.m_frame);
  outcome.m_forward_mask = OthersMask(context);
  return FAULT_NONE;
}

RecordFault MatchRecords::AcceptRunAhead(const RecordHeader &header, const MatchContext &context,
                                         RecordOutcome &outcome) {
  SlotState &slot = slots_[context.m_sender];
  if (context.m_sender != context.m_leader_slot) {
    return FAULT_NOT_LEADER;
  }
  if (!InFrameWindow(slot.m_next_frame, header.m_frame)) {
    return FAULT_FRAME_WINDOW;
  }
  if (header.m_run_ahead < MIN_RUN_AHEAD || header.m_run_ahead > MAX_RUN_AHEAD ||
      header.m_frame_rate < MIN_FRAME_RATE || header.m_frame_rate > MAX_FRAME_RATE) {
    return FAULT_BAD_RUN_AHEAD;
  }
  CountCommand(slot, header.m_frame);
  outcome.m_forward_mask = static_cast<std::uint8_t>(OthersMask(context) & header.m_relay);
  return FAULT_NONE;
}

// RUNAHEADMETRICS, CHAT, PROGRESS, LOADCOMPLETE and TIMEOUTSTART. Their frames, when they have one, are not checked.
RecordFault MatchRecords::AcceptOthers(const RecordHeader &header, const MatchContext &context,
                                       RecordOutcome &outcome) {
  switch (header.m_type) {
  case COMMAND_RUNAHEADMETRICS:
    outcome.m_forward_mask = static_cast<std::uint8_t>(OthersMask(context) & SlotBit(context.m_leader_slot));
    break;
  case COMMAND_CHAT:
    outcome.m_forward_mask = static_cast<std::uint8_t>(OthersMask(context) & header.m_relay);
    break;
  case COMMAND_TIMEOUTSTART:
    if (context.m_sender != context.m_host_slot) {
      return FAULT_NOT_HOST;
    }
    outcome.m_forward_mask = OthersMask(context);
    break;
  default: // PROGRESS and LOADCOMPLETE.
    outcome.m_forward_mask = OthersMask(context);
    break;
  }
  return FAULT_NONE;
}

void MatchRecords::OpenWindow(SlotState &slot, std::uint32_t now_ms) {
  if (!slot.m_window_open || now_ms - slot.m_window_start_ms >= DATA_RATE_WINDOW_MS) {
    slot.m_window_open = true;
    slot.m_window_start_ms = now_ms;
    slot.m_window_bytes = 0;
    slot.m_window_records = 0;
  }
}

bool MatchRecords::TakeBytes(std::uint8_t slot_number, std::uint32_t now_ms, std::size_t bytes) {
  if (slot_number >= SLOT_COUNT) {
    return false;
  }
  SlotState &slot = slots_[slot_number];
  OpenWindow(slot, now_ms);
  slot.m_window_bytes += bytes;
  return slot.m_window_bytes <= MAX_DATA_BYTES_PER_SECOND;
}

bool MatchRecords::TakeRecord(std::uint8_t slot_number, std::uint32_t now_ms) {
  if (slot_number >= SLOT_COUNT) {
    return false;
  }
  SlotState &slot = slots_[slot_number];
  OpenWindow(slot, now_ms);
  ++slot.m_window_records;
  return slot.m_window_records <= MAX_RECORDS_PER_SECOND;
}

bool MatchRecords::Reject(std::uint8_t slot_number, RecordFault fault) {
  ++counters_.m_dropped[fault];
  if (slot_number >= SLOT_COUNT) {
    return true;
  }
  ++slots_[slot_number].m_failed_records;
  return slots_[slot_number].m_failed_records >= FAILED_RECORDS_BEFORE_DROP;
}

void MatchRecords::CountDrop(RecordFault fault) {
  ++counters_.m_dropped[fault];
}

void MatchRecords::CountIn() {
  ++counters_.m_records_in;
}

void MatchRecords::CountForwarded() {
  ++counters_.m_forwarded;
}

std::uint32_t MatchRecords::NextFrame(std::uint8_t slot) const {
  return slot < SLOT_COUNT ? slots_[slot].m_next_frame : INITIAL_NEXT_FRAME;
}

const SessionCounters &MatchRecords::Counters() const {
  return counters_;
}

} // namespace relay
