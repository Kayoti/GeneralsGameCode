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

// ServerRegions.h ////////////////////////////////////////////////////////////
// The region list of Servers mode, read from the client region file, and a
// non-blocking host name lookup for the relay addresses in it.
// Format: docs/servers/PROTOCOL.md, section 7.1.

// TheSuperHackers @feature Kayoti 13/11/2026 Adds the client region list and a non-blocking relay address lookup.

#pragma once

#ifdef RTS_SERVERS

// This header must stay free of engine and ENet types. It is included both by
// game code and by the serverlink library, which is built without the engine's
// precompiled header.

struct ServerRegion
{
	enum
	{
		NAME_SIZE = 64,
		HOST_SIZE = 128,
		TAG_SIZE = 33,	///< 32 characters and the terminator
	};

	unsigned short number;						///< Region number, 1 to 65535, the same value as in the pass
	char name[NAME_SIZE];					///< Shown to the player
	char relayHost[HOST_SIZE];		///< Host name or IPv4 address of the relay
	unsigned short relayPort;			///< UDP port of the relay
	char tag[TAG_SIZE];						///< Lobby tag of this region
	unsigned short tagPort;				///< Lobby tag port of this region
};

/**
 * The regions of the client region file, in file order. File order decides ties between regions.
 */
class ServerRegions
{
public:
	ServerRegions();
	~ServerRegions();

	/// Parses region file text. Returns false and leaves the list empty if any line is invalid; getError() says why.
	bool parse(const char *text, unsigned int length);

	/// Reads and parses a region file. Returns false and leaves the list empty if it cannot be read or is invalid.
	bool load(const char *path);

	unsigned int getCount() const;
	const ServerRegion *getRegion(unsigned int index) const;						///< Null if index is out of range.
	const ServerRegion *findByNumber(unsigned short number) const;			///< Null if there is no such region.
	const ServerRegion *findByTag(const char *tag, unsigned short tagPort) const;	///< Exact comparison. Null if no region has this tag.

	/// Text of the last failure of parse() or load(), empty after a success.
	const char *getError() const;

private:
	ServerRegions(const ServerRegions &);
	ServerRegions &operator=(const ServerRegions &);

	struct Impl;
	Impl *m_impl;
};

/**
 * Looks up a relay host name without blocking the caller. The lookup runs on a worker thread.
 * A host given as a dotted IPv4 address is answered at once.
 */
class ServerHostResolver
{
public:
	enum State
	{
		STATE_IDLE,			///< Nothing started
		STATE_PENDING,	///< Lookup running
		STATE_RESOLVED,	///< getAddress() is valid
		STATE_FAILED,		///< The name has no IPv4 address
	};

	ServerHostResolver();
	~ServerHostResolver();

	/// Starts a lookup, dropping any lookup in progress. Returns false if host is empty or longer than ServerRegion::HOST_SIZE - 1.
	bool start(const char *host);
	State getState() const;
	unsigned int getAddress() const;	///< IPv4 address in network byte order. Only valid when RESOLVED.
	void reset();											///< Drops any lookup and returns to IDLE.

private:
	ServerHostResolver(const ServerHostResolver &);
	ServerHostResolver &operator=(const ServerHostResolver &);

	struct Impl;
	Impl *m_impl;
};

#endif // RTS_SERVERS
