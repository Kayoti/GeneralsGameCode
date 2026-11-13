// The part of the session manager that carries a running match: DATA messages, record forwarding and the drop of a
// player that breaks the rules (PROTOCOL.md 4.6 and 4.7).

#include "log.h"
#include "sessions.h"
#include "wire.h"

#include <cstdio>
#include <cstring>

namespace relay {

namespace {

bool SlotSet(std::uint8_t mask, std::size_t slot) {
  return slot < SLOT_COUNT && ((mask >> slot) & 1) != 0;
}

// Only the first few command count mismatches of a session are written to the log. All are counted.
const std::uint64_t MISMATCH_LOG_LIMIT = 20;

unsigned int AsUnsigned(std::uint32_t value) {
  return static_cast<unsigned int>(value);
}

} // namespace

void SessionManager::OnData(std::uint32_t connection, std::uint32_t now_ms, const unsigned char *body,
                            std::size_t length) {
  auto binding = bindings_.find(connection);
  if (binding == bindings_.end()) {
    return;
  }
  auto found = sessions_.find(binding->second.m_session_id);
  if (found == sessions_.end()) {
    return;
  }
  Session &session = found->second;
  const std::uint8_t SLOT = binding->second.m_slot;

  bool drop = false;
  if (!session.m_started) {
    session.m_records.CountDrop(FAULT_BEFORE_START);
    LogLine("slot %u sent DATA before START", AsUnsigned(SLOT));
    drop = true;
  } else {
    drop = ProcessRecords(session, SLOT, now_ms, body, length);
  }
  FlushForwarded(session);
  if (drop) {
    DropConnection(connection, DROP_PROTOCOL);
  }
}

// Returns true when the sender must be dropped. The records before the one that decides it have been handled.
bool SessionManager::ProcessRecords(Session &session, std::uint8_t slot, std::uint32_t now_ms,
                                    const unsigned char *body, std::size_t length) {
  if (!session.m_records.TakeBytes(slot, now_ms, 1 + length)) {
    session.m_records.CountDrop(FAULT_RATE);
    LogLine("slot %u passed the data rate limit", AsUnsigned(slot));
    return true;
  }
  if (length == 0 || 1 + length > MAX_MESSAGE_SIZE) {
    return RejectRecord(session, slot, FAULT_BAD_FRAMING, nullptr);
  }

  std::size_t position = 0;
  while (position < length) {
    if (length - position < 2) {
      return RejectRecord(session, slot, FAULT_BAD_FRAMING, nullptr);
    }
    const std::size_t RECORD_LENGTH = GetU16(body + position);
    position += 2;
    if (RECORD_LENGTH > length - position) {
      return RejectRecord(session, slot, FAULT_BAD_FRAMING, nullptr);
    }
    if (ProcessRecord(session, slot, now_ms, body + position, RECORD_LENGTH)) {
      return true;
    }
    position += RECORD_LENGTH;
  }
  return false;
}

// Returns true when the sender must be dropped.
bool SessionManager::ProcessRecord(Session &session, std::uint8_t slot, std::uint32_t now_ms,
                                   const unsigned char *record, std::size_t length) {
  MatchRecords &records = session.m_records;
  records.CountIn();
  if (!records.TakeRecord(slot, now_ms)) {
    records.CountDrop(FAULT_RATE);
    LogLine("slot %u passed the data rate limit", AsUnsigned(slot));
    return true;
  }

  RecordHeader header;
  RecordFault fault = ScanRecord(record, length, header);
  MatchContext context;
  context.m_sender = slot;
  context.m_connected_mask = session.m_connected_mask;
  context.m_leader_slot = LeaderSlot(session.m_connected_mask);
  context.m_host_slot = session.m_host_slot;
  context.m_strict_command_count = settings_.records.m_strict_command_count;
  RecordOutcome outcome;
  if (fault == FAULT_NONE) {
    fault = records.Accept(header, context, outcome);
  }
  if (fault != FAULT_NONE) {
    return RejectRecord(session, slot, fault, &header);
  }

  if (outcome.m_count_mismatch && records.Counters().m_count_mismatches <= MISMATCH_LOG_LIMIT) {
    LogLine("slot %u frame %u: FRAMEINFO says %u commands, %u counted", AsUnsigned(slot), AsUnsigned(header.m_frame),
            AsUnsigned(header.m_command_count), AsUnsigned(outcome.m_counted));
  }
  records.CountForwarded();
  ForwardRecord(session, outcome.m_forward_mask, record, length);
  return false;
}

// Counts a failed record. Returns true when this was the one that reaches the limit.
bool SessionManager::RejectRecord(Session &session, std::uint8_t slot, RecordFault fault,
                                  const RecordHeader *header) const {
  if (settings_.records.m_log_violations) {
    const unsigned int TYPE = header != nullptr ? header->m_type : 0;
    const unsigned int FRAME = header != nullptr ? AsUnsigned(header->m_frame) : 0;
    LogLine("violation: slot %u, %s, type %u, frame %u, next_frame %u", AsUnsigned(slot), FaultName(fault), TYPE, FRAME,
            AsUnsigned(session.m_records.NextFrame(slot)));
  }
  const bool LIMIT_REACHED = session.m_records.Reject(slot, fault);
  if (LIMIT_REACHED) {
    LogLine("slot %u reached %zu failed records", AsUnsigned(slot), FAILED_RECORDS_BEFORE_DROP);
  }
  return LIMIT_REACHED;
}

// Appends the record, with its length prefix, to the outgoing message of every slot in the mask.
void SessionManager::ForwardRecord(const Session &session, std::uint8_t mask, const unsigned char *record,
                                   std::size_t length) {
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (!SlotSet(mask, slot) || !SlotSet(session.m_connected_mask, slot)) {
      continue;
    }
    if (out_lengths_[slot] != 0 && out_lengths_[slot] + 2 + length > MAX_MESSAGE_SIZE) {
      FlushDestination(session, slot);
    }
    unsigned char *buffer = out_buffers_.data() + (slot * MAX_MESSAGE_SIZE);
    if (out_lengths_[slot] == 0) {
      buffer[0] = MSG_DATA;
      out_lengths_[slot] = 1;
    }
    PutU16(buffer + out_lengths_[slot], static_cast<std::uint16_t>(length));
    std::memcpy(buffer + out_lengths_[slot] + 2, record, length);
    out_lengths_[slot] += 2 + length;
  }
}

void SessionManager::FlushDestination(const Session &session, std::size_t slot) {
  if (out_lengths_[slot] != 0) {
    output_.Send(session.m_connections[slot], out_buffers_.data() + (slot * MAX_MESSAGE_SIZE), out_lengths_[slot]);
    out_lengths_[slot] = 0;
  }
}

void SessionManager::FlushForwarded(const Session &session) {
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    FlushDestination(session, slot);
  }
}

// The relay's part of removing a player (PROTOCOL.md 4.7): stop forwarding, then DROP and ROSTER to the others, then
// close the connection. last_frame is next_frame - 1.
void SessionManager::DropConnection(std::uint32_t connection, DropReason reason) {
  auto binding = bindings_.find(connection);
  if (binding == bindings_.end()) {
    return;
  }
  auto found = sessions_.find(binding->second.m_session_id);
  if (found == sessions_.end()) {
    return;
  }
  const Session &session = found->second;
  const std::uint8_t SLOT = binding->second.m_slot;

  unsigned char drop[1 + DROP_BODY_SIZE];
  drop[0] = MSG_DROP;
  drop[1] = SLOT;
  drop[2] = reason;
  PutU32(drop + 3, session.m_records.NextFrame(SLOT) - 1);
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (slot != SLOT && SlotSet(session.m_connected_mask, slot)) {
      output_.Send(session.m_connections[slot], drop, sizeof(drop));
    }
  }
  LogLine("slot %u dropped, reason %u", AsUnsigned(SLOT), static_cast<unsigned int>(reason));

  ReleaseConnection(connection); // Sends ROSTER to the others, and ends the session when nobody is left.
  output_.Close(connection);
}

void SessionManager::LogCounters(const Session &session) const {
  const SessionCounters &counters = session.m_records.Counters();
  char dropped[512];
  std::size_t used = 0;
  std::uint64_t total = 0;
  dropped[0] = '\0';
  for (std::size_t fault = 1; fault < FAULT_COUNT; ++fault) {
    if (counters.m_dropped[fault] == 0) {
      continue;
    }
    total += counters.m_dropped[fault];
    const int WRITTEN = std::snprintf(dropped + used, sizeof(dropped) - used, "%s%s %llu", used == 0 ? "" : ", ",
                                      FaultName(static_cast<RecordFault>(fault)),
                                      static_cast<unsigned long long>(counters.m_dropped[fault]));
    if (WRITTEN < 0 || static_cast<std::size_t>(WRITTEN) >= sizeof(dropped) - used) {
      break;
    }
    used += static_cast<std::size_t>(WRITTEN);
  }
  if (counters.m_records_in == 0 && total == 0) {
    return;
  }
  LogLine("session records: %llu in, %llu forwarded, %llu dropped (%s), %llu command count mismatches",
          static_cast<unsigned long long>(counters.m_records_in), static_cast<unsigned long long>(counters.m_forwarded),
          static_cast<unsigned long long>(total), dropped,
          static_cast<unsigned long long>(counters.m_count_mismatches));
}

} // namespace relay
