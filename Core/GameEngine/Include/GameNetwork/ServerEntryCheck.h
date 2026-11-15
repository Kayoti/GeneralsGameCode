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

// ServerEntryCheck.h /////////////////////////////////////////////////////////
// The entry check of Servers mode: the relay checks the player for a VPN and
// measures the ping, and answers with a pass or a refusal.
// Protocol: docs/servers/PROTOCOL.md, sections 4.3, 4.4 and 5.1.

// TheSuperHackers @feature Kayoti 13/11/2026 Adds the entry check client of Servers mode.

#pragma once

#ifdef RTS_SERVERS

#include "GameNetwork/ServerRegions.h"

// This header must stay free of engine and ENet types.

/// Milliseconds on the local clock. It wraps after about 49 days, so only compare two values by subtracting them.
unsigned int ServerClockMs();

/**
 * A pass from the relay, with the local time it was received at.
 * How long it has left is judged by the local clock only: the relay's times are never compared with the local ones.
 */
struct ServerPass
{
	enum
	{
		SIZE = 100,										///< Size on the wire
		RENEW_BEFORE_MS = 60 * 1000,	///< The client renews a pass that has less time left than this before CREATE or JOIN.
	};

	ServerPass();

	bool isValid() const;							///< False until a pass has been received
	unsigned int remainingMs() const;	///< Time left by the local clock. 0 when invalid or expired.
	bool needsRenewal() const;				///< True when remainingMs() is below RENEW_BEFORE_MS

	unsigned char bytes[SIZE];				///< The pass exactly as received. It is sent back unchanged in CREATE and JOIN.
	unsigned short region;						///< Region number the relay put in the pass
	unsigned short rttMs;							///< Round trip time the relay measured
	unsigned int lifetimeMs;					///< expires_at - issued_at, in milliseconds
	unsigned int receivedAtMs;				///< ServerClockMs() when the pass arrived
	bool valid;
};

/// Refusal code of the entry check. The values 1 to 19 are the REFUSE codes of PROTOCOL.md 4.9.
enum ServerRefuseCode
{
	SERVER_REFUSE_NO_ANSWER = 0,	///< Not a relay code. The region did not answer or gave an unusable answer.
	SERVER_REFUSE_PROTO_MISMATCH = 1,
	SERVER_REFUSE_VPN_DETECTED = 2,
	SERVER_REFUSE_PING_TOO_HIGH = 3,
	SERVER_REFUSE_TIMEOUT = 4,
	SERVER_REFUSE_BAD_PASS = 5,
	SERVER_REFUSE_EXPIRED = 6,
	SERVER_REFUSE_WRONG_REGION = 7,
	SERVER_REFUSE_ADDRESS_MISMATCH = 8,
	SERVER_REFUSE_PASS_IN_USE = 9,
	SERVER_REFUSE_BAD_REQUEST = 10,
	SERVER_REFUSE_NO_SESSION = 11,
	SERVER_REFUSE_ALREADY_STARTED = 12,
	SERVER_REFUSE_SLOT_TAKEN = 13,
	SERVER_REFUSE_NOT_EXPECTED = 14,
	SERVER_REFUSE_BAD_TOKEN = 15,
	SERVER_REFUSE_BUILD_MISMATCH = 16,
	SERVER_REFUSE_NOT_ENOUGH_PLAYERS = 17,
	SERVER_REFUSE_SERVER_BUSY = 18,
	SERVER_REFUSE_RATE_LIMITED = 19,
};

/**
 * Runs the entry check against one or more regions at the same time.
 *
 * Call start(), then update() once per tick until getState() is no longer RUNNING. update() never blocks.
 * The relay measures the ping from when it sends a PING to when our PONG arrives, so update() should
 * be called often while the check runs: a long gap between calls shows up as a longer ping.
 *
 * Each region is given 6 seconds to answer. A region that refuses or does not answer is skipped.
 * The result is the pass with the lowest ping, and a tie goes to the region listed first. If no region
 * gives a pass, the result is the refusal of the region listed first.
 */
class ServerEntryCheck
{
public:
	enum State
	{
		STATE_IDLE,			///< Not started
		STATE_RUNNING,	///< update() must be called
		STATE_PASSED,		///< getPass() and getRegion() are valid
		STATE_REFUSED,	///< getRefuseCode(), getRttMs() and getLimitMs() are valid
	};

	ServerEntryCheck();
	~ServerEntryCheck();

	/// Starts the check against every region of the list. Drops a check in progress. Returns false if the list is empty.
	bool start(const ServerRegions &regions, unsigned int exeCrc, unsigned int iniCrc);

	/// Starts the check against one region, to renew a pass. Drops a check in progress.
	bool startRegion(const ServerRegion &region, unsigned int exeCrc, unsigned int iniCrc);

	void update();					///< Once per tick. Does not block.
	void cancel();					///< Drops any check in progress and returns to IDLE.

	State getState() const;

	// Valid when PASSED
	const ServerPass &getPass() const;
	const ServerRegion &getRegion() const;			///< The region the pass is from

	// Valid when REFUSED. The two values are 0 unless the code is SERVER_REFUSE_PING_TOO_HIGH.
	unsigned char getRefuseCode() const;				///< A ServerRefuseCode
	unsigned short getRttMs() const;
	unsigned short getLimitMs() const;

private:
	ServerEntryCheck(const ServerEntryCheck &);
	ServerEntryCheck &operator=(const ServerEntryCheck &);

	struct Impl;
	Impl *m_impl;
};

#endif // RTS_SERVERS
