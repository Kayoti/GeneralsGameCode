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

// ServerRegions.cpp //////////////////////////////////////////////////////////
// TheSuperHackers @feature Kayoti 13/11/2026 Adds the client region list and a non-blocking relay address lookup.

#ifdef RTS_SERVERS

// This file is built without the engine's precompiled header.
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>

#include <stdio.h>
#include <string.h>
#include <vector>

#include "GameNetwork/ServerRegions.h"

namespace
{

const unsigned int MAX_FILE_SIZE = 64 * 1024;
const unsigned int MAX_TAG_LENGTH = 32;

bool isSpace(char c)
{
	return c == ' ' || c == '\t' || c == '\r' || c == '\n';
}

bool isDigit(char c)
{
	return c >= '0' && c <= '9';
}

/// Trims whitespace on both sides of [begin, end) in place.
void trim(const char *&begin, const char *&end)
{
	while (begin < end && isSpace(*begin))
		++begin;
	while (end > begin && isSpace(*(end - 1)))
		--end;
}

/// Reads a decimal number of 1 to 5 digits with the value 1 to 65535.
bool parsePort(const char *begin, const char *end, unsigned short &out)
{
	if (begin == end || end - begin > 5)
		return false;

	unsigned int value = 0;
	for (const char *p = begin; p != end; ++p)
	{
		if (!isDigit(*p))
			return false;
		value = value * 10 + (unsigned int)(*p - '0');
	}
	if (value < 1 || value > 65535)
		return false;

	out = (unsigned short)value;
	return true;
}

bool isTagChar(char c)
{
	return (c >= 'a' && c <= 'z') || isDigit(c) || c == '.' || c == '-';
}

bool isHostChar(char c)
{
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigit(c) || c == '.' || c == '-';
}

/// Copies [begin, end) into a buffer of the given size. Fails if it does not fit with its terminator.
bool copyText(char *dest, unsigned int destSize, const char *begin, const char *end)
{
	const unsigned int length = (unsigned int)(end - begin);
	if (length == 0 || length >= destSize)
		return false;
	memcpy(dest, begin, length);
	dest[length] = '\0';
	return true;
}

bool equalsKey(const char *begin, const char *end, const char *key)
{
	const size_t length = strlen(key);
	return (size_t)(end - begin) == length && memcmp(begin, key, length) == 0;
}

/// Reads dotted IPv4 notation with four decimal numbers of 0 to 255. Gives the address in network byte order.
bool parseDottedAddress(const char *text, unsigned int &addressOut)
{
	unsigned char octets[4];
	const char *p = text;
	for (int i = 0; i < 4; ++i)
	{
		if (!isDigit(*p))
			return false;
		unsigned int value = 0;
		int digits = 0;
		while (isDigit(*p))
		{
			value = value * 10 + (unsigned int)(*p - '0');
			++p;
			if (++digits > 3 || value > 255)
				return false;
		}
		octets[i] = (unsigned char)value;
		if (i < 3)
		{
			if (*p != '.')
				return false;
			++p;
		}
	}
	if (*p != '\0')
		return false;

	memcpy(&addressOut, octets, 4);
	return true;
}

} // namespace

////////////////////////////////////////////////////////////////////////////////
// ServerRegions
////////////////////////////////////////////////////////////////////////////////

struct ServerRegions::Impl
{
	enum
	{
		HAS_NUMBER = 1 << 0,
		HAS_NAME = 1 << 1,
		HAS_RELAY = 1 << 2,
		HAS_TAG = 1 << 3,
		HAS_TAG_PORT = 1 << 4,
		HAS_ALL = (1 << 5) - 1,
	};

	std::vector<ServerRegion> regions;
	char error[128];

	// State of the block being read
	ServerRegion current;
	unsigned int seen;
	bool inBlock;
	unsigned int line;

	void clear()
	{
		regions.clear();
		error[0] = '\0';
		inBlock = false;
		seen = 0;
		line = 0;
	}

	void fail(const char *what)
	{
		_snprintf_s(error, sizeof(error), _TRUNCATE, "line %u: %s", line, what);
	}

	bool startBlock()
	{
		memset(&current, 0, sizeof(current));
		seen = 0;
		inBlock = true;
		return true;
	}

	bool finishBlock()
	{
		if (!inBlock)
			return true;
		inBlock = false;

		if (seen != HAS_ALL)
		{
			fail("region block is missing a key");
			return false;
		}
		for (size_t i = 0; i < regions.size(); ++i)
		{
			if (regions[i].number == current.number)
			{
				fail("region number is used twice");
				return false;
			}
		}
		regions.push_back(current);
		return true;
	}

	bool setNumber(const char *begin, const char *end)
	{
		return parsePort(begin, end, current.number);
	}

	bool setRelay(const char *begin, const char *end)
	{
		const char *colon = end;
		while (colon > begin && *(colon - 1) != ':')
			--colon;
		if (colon == begin)
			return false;

		const char *hostEnd = colon - 1;
		if (hostEnd == begin)
			return false;
		for (const char *p = begin; p != hostEnd; ++p)
		{
			if (!isHostChar(*p))
				return false;
		}
		return copyText(current.relayHost, ServerRegion::HOST_SIZE, begin, hostEnd)
			&& parsePort(colon, end, current.relayPort);
	}

	bool setTag(const char *begin, const char *end)
	{
		if (end - begin > (int)MAX_TAG_LENGTH)
			return false;
		for (const char *p = begin; p != end; ++p)
		{
			if (!isTagChar(*p))
				return false;
		}
		return copyText(current.tag, ServerRegion::TAG_SIZE, begin, end);
	}

	bool setName(const char *begin, const char *end)
	{
		for (const char *p = begin; p != end; ++p)
		{
			if ((unsigned char)*p < 0x20)
				return false;
		}
		return copyText(current.name, ServerRegion::NAME_SIZE, begin, end);
	}

	/// Applies one key and value to the current block. Each key may appear once.
	bool setKey(const char *keyBegin, const char *keyEnd, const char *valueBegin, const char *valueEnd)
	{
		unsigned int bit = 0;
		bool ok = false;

		if (equalsKey(keyBegin, keyEnd, "number"))
		{
			bit = HAS_NUMBER;
			ok = !(seen & bit) && setNumber(valueBegin, valueEnd);
		}
		else if (equalsKey(keyBegin, keyEnd, "name"))
		{
			bit = HAS_NAME;
			ok = !(seen & bit) && setName(valueBegin, valueEnd);
		}
		else if (equalsKey(keyBegin, keyEnd, "relay"))
		{
			bit = HAS_RELAY;
			ok = !(seen & bit) && setRelay(valueBegin, valueEnd);
		}
		else if (equalsKey(keyBegin, keyEnd, "tag"))
		{
			bit = HAS_TAG;
			ok = !(seen & bit) && setTag(valueBegin, valueEnd);
		}
		else if (equalsKey(keyBegin, keyEnd, "tag_port"))
		{
			bit = HAS_TAG_PORT;
			ok = !(seen & bit) && parsePort(valueBegin, valueEnd, current.tagPort);
		}
		else
		{
			fail("unknown key");
			return false;
		}

		if (!ok)
		{
			fail("invalid or repeated value");
			return false;
		}
		seen |= bit;
		return true;
	}

	/// Handles one trimmed, non-empty, non-comment line.
	bool handleLine(const char *begin, const char *end)
	{
		if (equalsKey(begin, end, "[region]"))
			return finishBlock() && startBlock();

		if (!inBlock)
		{
			fail("expected [region]");
			return false;
		}

		const char *equals = begin;
		while (equals != end && *equals != '=')
			++equals;
		if (equals == end)
		{
			fail("expected key = value");
			return false;
		}

		const char *keyBegin = begin;
		const char *keyEnd = equals;
		const char *valueBegin = equals + 1;
		const char *valueEnd = end;
		trim(keyBegin, keyEnd);
		trim(valueBegin, valueEnd);
		return setKey(keyBegin, keyEnd, valueBegin, valueEnd);
	}

	bool parse(const char *text, unsigned int length)
	{
		clear();

		if (text == nullptr || length > MAX_FILE_SIZE)
		{
			error[0] = '\0';
			_snprintf_s(error, sizeof(error), _TRUNCATE, "region file is missing or too large");
			return false;
		}

		const char *p = text;
		const char *const fileEnd = text + length;

		// Skip a UTF-8 byte order mark.
		if (length >= 3 && (unsigned char)p[0] == 0xEF && (unsigned char)p[1] == 0xBB && (unsigned char)p[2] == 0xBF)
			p += 3;

		bool ok = true;
		while (ok && p < fileEnd)
		{
			const char *lineBegin = p;
			while (p < fileEnd && *p != '\n')
				++p;
			const char *lineEnd = p;
			if (p < fileEnd)
				++p;
			++line;

			trim(lineBegin, lineEnd);
			if (lineBegin == lineEnd || *lineBegin == '#')
				continue;

			ok = handleLine(lineBegin, lineEnd);
		}

		if (ok)
			ok = finishBlock();
		if (ok && regions.empty())
		{
			_snprintf_s(error, sizeof(error), _TRUNCATE, "region file has no regions");
			ok = false;
		}

		if (!ok)
			regions.clear();
		return ok;
	}
};

ServerRegions::ServerRegions()
	: m_impl(new Impl)
{
	m_impl->clear();
}

ServerRegions::~ServerRegions()
{
	delete m_impl;
}

bool ServerRegions::parse(const char *text, unsigned int length)
{
	return m_impl->parse(text, length);
}

bool ServerRegions::load(const char *path)
{
	m_impl->clear();

	FILE *file = nullptr;
	if (path == nullptr || fopen_s(&file, path, "rb") != 0 || file == nullptr)
	{
		_snprintf_s(m_impl->error, sizeof(m_impl->error), _TRUNCATE, "cannot open the region file");
		return false;
	}

	std::vector<char> buffer(MAX_FILE_SIZE + 1);
	const size_t got = fread(&buffer[0], 1, buffer.size(), file);
	const bool readFailed = ferror(file) != 0;
	fclose(file);

	if (readFailed)
	{
		_snprintf_s(m_impl->error, sizeof(m_impl->error), _TRUNCATE, "cannot read the region file");
		return false;
	}
	// One byte more than the limit was requested, so a file over the limit is detected, not cut.
	return m_impl->parse(&buffer[0], (unsigned int)got);
}

unsigned int ServerRegions::getCount() const
{
	return (unsigned int)m_impl->regions.size();
}

const ServerRegion *ServerRegions::getRegion(unsigned int index) const
{
	return index < m_impl->regions.size() ? &m_impl->regions[index] : nullptr;
}

const ServerRegion *ServerRegions::findByNumber(unsigned short number) const
{
	for (size_t i = 0; i < m_impl->regions.size(); ++i)
	{
		if (m_impl->regions[i].number == number)
			return &m_impl->regions[i];
	}
	return nullptr;
}

const ServerRegion *ServerRegions::findByTag(const char *tag, unsigned short tagPort) const
{
	if (tag == nullptr)
		return nullptr;

	for (size_t i = 0; i < m_impl->regions.size(); ++i)
	{
		const ServerRegion &region = m_impl->regions[i];
		if (region.tagPort == tagPort && strcmp(region.tag, tag) == 0)
			return &region;
	}
	return nullptr;
}

const char *ServerRegions::getError() const
{
	return m_impl->error;
}

////////////////////////////////////////////////////////////////////////////////
// ServerHostResolver
////////////////////////////////////////////////////////////////////////////////

namespace
{

/// Shared between the resolver and its worker thread. Whoever drops the last reference frees it.
struct ResolveJob
{
	volatile LONG references;
	volatile LONG state;
	volatile LONG address;
	char host[ServerRegion::HOST_SIZE];
};

void releaseJob(ResolveJob *job)
{
	if (InterlockedDecrement(&job->references) == 0)
		delete job;
}

DWORD WINAPI resolveThread(LPVOID param)
{
	ResolveJob *job = static_cast<ResolveJob *>(param);

	LONG result = ServerHostResolver::STATE_FAILED;
	LONG address = 0;

	addrinfo hints;
	memset(&hints, 0, sizeof(hints));
	hints.ai_family = AF_INET;
	hints.ai_socktype = SOCK_DGRAM;

	addrinfo *found = nullptr;
	if (getaddrinfo(job->host, nullptr, &hints, &found) == 0 && found != nullptr)
	{
		if (found->ai_family == AF_INET && found->ai_addr != nullptr)
		{
			address = (LONG)reinterpret_cast<sockaddr_in *>(found->ai_addr)->sin_addr.S_un.S_addr;
			result = ServerHostResolver::STATE_RESOLVED;
		}
		freeaddrinfo(found);
	}

	InterlockedExchange(&job->address, address);
	InterlockedExchange(&job->state, result);
	WSACleanup();
	releaseJob(job);
	return 0;
}

} // namespace

struct ServerHostResolver::Impl
{
	ResolveJob *job;
	State state;
	unsigned int address;

	Impl() : job(nullptr), state(STATE_IDLE), address(0) {}

	void drop()
	{
		if (job != nullptr)
		{
			releaseJob(job);
			job = nullptr;
		}
		state = STATE_IDLE;
		address = 0;
	}

	/// Moves the state forward when the worker thread has finished.
	void poll()
	{
		if (job == nullptr || state != STATE_PENDING)
			return;

		const LONG jobState = job->state;
		if (jobState == STATE_RESOLVED)
		{
			address = (unsigned int)job->address;
			state = STATE_RESOLVED;
		}
		else if (jobState == STATE_FAILED)
		{
			state = STATE_FAILED;
		}
	}

	bool startThread(const char *host)
	{
		WSADATA data;
		if (WSAStartup(MAKEWORD(2, 2), &data) != 0)
			return false;

		job = new ResolveJob;
		job->references = 2;	// the resolver and the worker thread
		job->state = STATE_PENDING;
		job->address = 0;
		strcpy_s(job->host, sizeof(job->host), host);

		HANDLE thread = CreateThread(nullptr, 0, resolveThread, job, 0, nullptr);
		if (thread == nullptr)
		{
			WSACleanup();
			delete job;
			job = nullptr;
			return false;
		}
		CloseHandle(thread);
		state = STATE_PENDING;
		return true;
	}
};

ServerHostResolver::ServerHostResolver()
	: m_impl(new Impl)
{
}

ServerHostResolver::~ServerHostResolver()
{
	m_impl->drop();
	delete m_impl;
}

bool ServerHostResolver::start(const char *host)
{
	m_impl->drop();

	if (host == nullptr || host[0] == '\0' || strlen(host) >= ServerRegion::HOST_SIZE)
		return false;

	unsigned int address = 0;
	if (parseDottedAddress(host, address))
	{
		m_impl->address = address;
		m_impl->state = STATE_RESOLVED;
		return true;
	}

	if (!m_impl->startThread(host))
	{
		m_impl->state = STATE_FAILED;
	}
	return true;
}

ServerHostResolver::State ServerHostResolver::getState() const
{
	m_impl->poll();
	return m_impl->state;
}

unsigned int ServerHostResolver::getAddress() const
{
	m_impl->poll();
	return m_impl->state == STATE_RESOLVED ? m_impl->address : 0;
}

void ServerHostResolver::reset()
{
	m_impl->drop();
}

#endif // RTS_SERVERS
