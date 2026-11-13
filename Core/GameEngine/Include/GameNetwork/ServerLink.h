/*
**	Command & Conquer Generals Zero Hour(tm)
**	Copyright 2025 Electronic Arts Inc.
**
**	This program is free software: you can redistribute it and/or modify
**	it under the terms of the GNU General Public License as published by
**	the Free Software Foundation, either version 3 of the License, or
**	(at your option) any later version.
**
**	This program is distributed in the hope that it will be useful,
**	but WITHOUT ANY WARRANTY; without even the implied warranty of
**	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
**	GNU General Public License for more details.
**
**	You should have received a copy of the GNU General Public License
**	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

// ServerLink.h ///////////////////////////////////////////////////////////////
// The match connection of Servers mode: one connection to the relay that
// creates or joins a match session and carries the match records.
// Protocol: docs/servers/PROTOCOL.md, sections 3, 4.1 to 4.7 and 4.9.

// TheSuperHackers @feature Kayoti 13/11/2026 Adds the relay match connection of Servers mode.

#pragma once

#ifdef RTS_SERVERS

#include "GameNetwork/ServerEntryCheck.h"

// This header must stay free of engine and ENet types.

/// Reason in a DROP event. The values are the DROP reasons of PROTOCOL.md 4.9.
enum ServerDropReason
{
	SERVER_DROP_LEFT = 1,
	SERVER_DROP_TIMEOUT = 2,
	SERVER_DROP_PROTOCOL = 3,
	SERVER_DROP_NEVER_JOINED = 4,
	SERVER_DROP_TOO_SLOW = 5,
};

/// Something that happened on the link. Events are delivered in the order the relay sent them.
struct ServerLinkEvent
{
	enum Type
	{
		TYPE_NONE,
		TYPE_CREATED,		///< Host only: sessionId and joinTokens
		TYPE_WELCOME,		///< The player holds its slot: sessionId, slot, connectedMask, leaderSlot
		TYPE_ROSTER,		///< connectedMask, leaderSlot. Sent on every change.
		TYPE_START,			///< connectedMask, leaderSlot. Sent once. Match records may be sent from now on.
		TYPE_DROP,			///< slot, dropReason, lastFrame
		TYPE_REFUSE,		///< refuseCode, rttMs, limitMs. The relay closes the connection.
		TYPE_DATA,			///< data and dataLength: match records
		TYPE_LOST,			///< The connection ended, or could not be set up in time. Not given after REFUSE or after close().
	};

	enum
	{
		MAX_SLOTS = 8,
	};

	ServerLinkEvent();

	Type type;

	unsigned long long sessionId;										///< CREATED, WELCOME
	unsigned long long joinTokens[MAX_SLOTS];				///< CREATED. Index is the slot. 0 for the host slot and for slots that are not expected.
	unsigned char slot;															///< WELCOME: this player's slot. DROP: the slot that was dropped.
	unsigned char connectedMask;										///< WELCOME, ROSTER, START
	unsigned char leaderSlot;												///< WELCOME, ROSTER, START
	unsigned char dropReason;												///< DROP: a ServerDropReason
	unsigned int lastFrame;													///< DROP: the last frame the dropped slot took part in
	unsigned char refuseCode;												///< REFUSE: a ServerRefuseCode
	unsigned short rttMs;														///< REFUSE: 0 unless the code is SERVER_REFUSE_PING_TOO_HIGH
	unsigned short limitMs;													///< REFUSE: 0 unless the code is SERVER_REFUSE_PING_TOO_HIGH

	/// DATA: the records, each a 16-bit little-endian length followed by that many bytes of one engine command.
	/// The pointer stays valid until the next call to nextEvent(), update() or close().
	const unsigned char *data;
	unsigned int dataLength;
};

/**
 * A connection to the relay for one match.
 *
 * Open it with create() (the host) or join() (every other player), then call update() once per tick.
 * update() never blocks. Read what happened with nextEvent().
 *
 * Every other player in a match must wait for the host: the host's CREATED event carries the join tokens
 * that the host passes on to the other players privately.
 */
class ServerLink
{
public:
	enum State
	{
		STATE_IDLE,					///< Not opened
		STATE_CONNECTING,		///< Waiting for the relay, up to WELCOME
		STATE_CONNECTED,		///< WELCOME received
		STATE_LEAVING,			///< leave() was called. Keep calling update() until CLOSED.
		STATE_CLOSED,				///< Ended: refused, lost, or left
	};

	ServerLink();
	~ServerLink();

	/// Opens a match as the host. slotMask has one bit per human player and must contain hostSlot and at least one other slot.
	/// The relay must answer with WELCOME within timeoutMs milliseconds, or the link ends with a LOST event.
	/// Returns false, without opening anything, if an argument is invalid or the link is already open.
	bool create(const ServerRegion &region, const ServerPass &pass, unsigned char hostSlot, unsigned char slotMask, unsigned int timeoutMs);

	/// Enters the match of sessionId as slot, with the join token the host gave this player. Same timeout and return value as create().
	bool join(const ServerRegion &region, const ServerPass &pass, unsigned long long sessionId, unsigned char slot, unsigned long long joinToken, unsigned int timeoutMs);

	void update();																	///< Once per tick. Does not block.
	bool nextEvent(ServerLinkEvent &event);					///< Takes the oldest event. False if there is none.

	/// Sends match records as one DATA message. The buffer holds whole records, each a 16-bit little-endian length
	/// followed by that many bytes. Each record is 7 to 4096 bytes and the buffer is at most 16383 bytes.
	/// Returns false, sending nothing, if the buffer is malformed or the link is not CONNECTED.
	bool sendData(const unsigned char *records, unsigned int length);

	/// Sends LEAVE and closes once it is delivered. Records for the leave frame must have been sent before.
	void leave();

	/// Drops the connection at once, with no event.
	void close();

	State getState() const;

private:
	ServerLink(const ServerLink &);
	ServerLink &operator=(const ServerLink &);

	struct Impl;
	Impl *m_impl;
};

#endif // RTS_SERVERS
