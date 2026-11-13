// Removing players from a running match other than for a protocol fault: a leave that was not finished, the loading
// time and the stall rule (PROTOCOL.md 4.7). The drop itself is DropConnection in match_data.cpp.

#include "sessions.h"

namespace relay {

namespace {

bool SlotSet(std::uint8_t mask, std::size_t slot) {
  return slot < SLOT_COUNT && ((mask >> slot) & 1) != 0;
}

} // namespace

// Called for every record that was accepted, before it is forwarded. Keeps what the loading, stall and leave rules
// need.
void SessionManager::NoteAccepted(Session &session, std::uint8_t slot, std::uint32_t now_ms,
                                  const RecordHeader &header) {
  if (slot >= SLOT_COUNT) {
    return;
  }
  switch (header.m_type) {
  case COMMAND_LOADCOMPLETE:
    session.m_loaded_mask = static_cast<std::uint8_t>(session.m_loaded_mask | (1u << slot));
    break;
  case COMMAND_FRAMEINFO:
    if (!session.m_frame_info_forwarded) {
      // The stall rule starts here, so every slot's idle time counts from the first FRAMEINFO of the session.
      session.m_frame_info_forwarded = true;
      for (std::size_t other = 0; other < SLOT_COUNT; ++other) {
        session.m_advance_ms[other] = now_ms;
      }
    }
    session.m_advance_ms[slot] = now_ms;
    break;
  case COMMAND_PLAYERLEAVE:
    if (!SlotSet(session.m_leave_mask, slot) || header.m_frame > session.m_leave_frames[slot]) {
      session.m_leave_frames[slot] = header.m_frame;
    }
    session.m_leave_mask = static_cast<std::uint8_t>(session.m_leave_mask | (1u << slot));
    break;
  default:
    break;
  }
}

// True when the slot's PLAYERLEAVE was forwarded and so were all its FRAMEINFO through the leave frame. FRAMEINFO
// for the leave frame has been forwarded once next_frame is above it.
bool SessionManager::LeaveCompleted(const Session &session, std::uint8_t slot) const {
  return SlotSet(session.m_leave_mask, slot) && session.m_records.NextFrame(slot) > session.m_leave_frames[slot];
}

// The connection asked to leave or ended. In a running match, a player whose leave is not complete is dropped with
// the given reason, and the function returns true. Otherwise nothing is done and the caller handles the end as a
// plain leave, which sends ROSTER only.
bool SessionManager::DropIfMatchActive(std::uint32_t connection, DropReason reason) {
  auto binding = bindings_.find(connection);
  if (binding == bindings_.end()) {
    return false;
  }
  auto found = sessions_.find(binding->second.m_session_id);
  if (found == sessions_.end() || !found->second.m_started || LeaveCompleted(found->second, binding->second.m_slot)) {
    return false;
  }
  DropConnection(connection, reason);
  return true;
}

// Loading rule and stall rule for every running session. Nothing is dropped here, because a drop can remove a
// session.
void SessionManager::CollectTimeouts(std::uint32_t now_ms, std::vector<PendingDrop> &due) const {
  for (const auto &entry : sessions_) {
    if (entry.second.m_started) {
      CollectSessionTimeouts(entry.second, now_ms, due);
    }
  }
}

void SessionManager::CollectSessionTimeouts(const Session &session, std::uint32_t now_ms,
                                            std::vector<PendingDrop> &due) const {
  for (std::size_t slot = 0; slot < SLOT_COUNT; ++slot) {
    if (!SlotSet(session.m_connected_mask, slot)) {
      continue;
    }
    const bool LOADING_EXPIRED =
        !SlotSet(session.m_loaded_mask, slot) && now_ms - session.m_started_ms >= settings_.drop.m_loading_ms;
    if (LOADING_EXPIRED || IsStalled(session, static_cast<std::uint8_t>(slot), now_ms)) {
      PendingDrop drop;
      drop.m_connection = session.m_connections[slot];
      drop.m_reason = DROP_TIMEOUT;
      due.push_back(drop);
    }
  }
}

// Stall rule: a FRAMEINFO has been forwarded in the session, the slot's next_frame is lower than that of every other
// connected slot, and it has not increased for the stall time.
bool SessionManager::IsStalled(const Session &session, std::uint8_t slot, std::uint32_t now_ms) const {
  if (!session.m_frame_info_forwarded || now_ms - session.m_advance_ms[slot] < settings_.drop.m_stall_ms) {
    return false;
  }
  const std::uint32_t OWN_FRAME = session.m_records.NextFrame(slot);
  bool has_other = false;
  for (std::size_t other = 0; other < SLOT_COUNT; ++other) {
    if (other == slot || !SlotSet(session.m_connected_mask, other)) {
      continue;
    }
    if (session.m_records.NextFrame(static_cast<std::uint8_t>(other)) <= OWN_FRAME) {
      return false;
    }
    has_other = true;
  }
  return has_other;
}

} // namespace relay
