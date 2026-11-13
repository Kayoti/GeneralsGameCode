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

// ServerEntryCheck.cpp ///////////////////////////////////////////////////////
// TheSuperHackers @feature Kayoti 13/11/2026 Adds the entry check client of Servers mode.

#ifdef RTS_SERVERS

// This file is built without the engine's precompiled header. ENet headers are
// included here and nowhere else, so ENet's winsock2.h never meets the engine's windows.h.
#include <enet/enet.h>
#include <windows.h>

#include <string.h>
#include <vector>

#include "GameNetwork/ServerEntryCheck.h"

namespace
{

// PROTOCOL.md 4.1 and 4.2
const unsigned short PROTO_VERSION = 1;
const size_t CHANNEL_COUNT = 2;
const enet_uint8 CHANNEL_MESSAGES = 0;

const unsigned char MSG_CHECK_HELLO = 1;
const unsigned char MSG_PING = 2;
const unsigned char MSG_PONG = 3;
const unsigned char MSG_PASS = 4;
const unsigned char MSG_REFUSE = 5;

const size_t CHECK_HELLO_SIZE = 1 + 2 + 4 + 4;
const size_t PING_SIZE = 1 + 8;
const size_t PONG_SIZE = 1 + 8;
const size_t PASS_MESSAGE_SIZE = 1 + ServerPass::SIZE;
const size_t REFUSE_SIZE = 1 + 1 + 2 + 2;

// PROTOCOL.md 4.3, byte offsets inside the pass
const size_t PASS_OFFSET_VERSION = 0;
const size_t PASS_OFFSET_REGION = 2;
const size_t PASS_OFFSET_RTT = 16;
const size_t PASS_OFFSET_ISSUED = 20;
const size_t PASS_OFFSET_EXPIRES = 24;
const unsigned char PASS_VERSION = 1;

// PROTOCOL.md 5.1: a region that has not answered within 6 seconds is skipped.
const unsigned int REGION_TIMEOUT_MS = 6000;

// Bounds the work of one update() call.
const int MAX_EVENTS_PER_UPDATE = 64;

unsigned short readU16(const unsigned char *p)
{
	return (unsigned short)(p[0] | (p[1] << 8));
}

unsigned int readU32(const unsigned char *p)
{
	return (unsigned int)p[0] | ((unsigned int)p[1] << 8) | ((unsigned int)p[2] << 16) | ((unsigned int)p[3] << 24);
}

void writeU16(unsigned char *p, unsigned short v)
{
	p[0] = (unsigned char)(v & 0xff);
	p[1] = (unsigned char)(v >> 8);
}

void writeU32(unsigned char *p, unsigned int v)
{
	p[0] = (unsigned char)(v & 0xff);
	p[1] = (unsigned char)((v >> 8) & 0xff);
	p[2] = (unsigned char)((v >> 16) & 0xff);
	p[3] = (unsigned char)(v >> 24);
}

/// The entry check of one region.
class RegionAttempt
{
public:
	enum Phase
	{
		PHASE_RESOLVING,
		PHASE_WAITING,	///< connected or connecting, no answer yet
		PHASE_PASSED,
		PHASE_REFUSED,
		PHASE_FAILED,		///< no answer, or an answer that cannot be used
	};

	RegionAttempt(const ServerRegion &regionIn, unsigned int exeCrcIn, unsigned int iniCrcIn)
		: region(regionIn)
		, phase(PHASE_RESOLVING)
		, refuseCode(SERVER_REFUSE_NO_ANSWER)
		, refuseRttMs(0)
		, refuseLimitMs(0)
		, exeCrc(exeCrcIn)
		, iniCrc(iniCrcIn)
		, host(nullptr)
		, peer(nullptr)
	{
		if (!resolver.start(region.relayHost))
			phase = PHASE_FAILED;
	}

	~RegionAttempt()
	{
		closeHost();
	}

	bool isDone() const { return phase == PHASE_PASSED || phase == PHASE_REFUSED || phase == PHASE_FAILED; }

	void update(unsigned int elapsedMs)
	{
		if (isDone())
			return;

		if (elapsedMs >= REGION_TIMEOUT_MS)
		{
			finish(PHASE_FAILED);
			return;
		}

		if (phase == PHASE_RESOLVING)
			updateResolving();
		if (phase == PHASE_WAITING)
			serviceHost();
	}

	ServerRegion region;
	Phase phase;
	ServerPass pass;
	unsigned char refuseCode;
	unsigned short refuseRttMs;
	unsigned short refuseLimitMs;

private:
	void updateResolving()
	{
		const ServerHostResolver::State state = resolver.getState();
		if (state == ServerHostResolver::STATE_FAILED)
		{
			finish(PHASE_FAILED);
			return;
		}
		if (state != ServerHostResolver::STATE_RESOLVED)
			return;

		host = enet_host_create(nullptr, 1, CHANNEL_COUNT, 0, 0);
		if (host == nullptr)
		{
			finish(PHASE_FAILED);
			return;
		}

		ENetAddress address;
		address.host = resolver.getAddress();
		address.port = region.relayPort;
		peer = enet_host_connect(host, &address, CHANNEL_COUNT, 0);
		if (peer == nullptr)
		{
			finish(PHASE_FAILED);
			return;
		}
		phase = PHASE_WAITING;
	}

	void serviceHost()
	{
		ENetEvent event;
		for (int i = 0; i < MAX_EVENTS_PER_UPDATE && phase == PHASE_WAITING; ++i)
		{
			if (enet_host_service(host, &event, 0) <= 0)
				break;

			switch (event.type)
			{
				case ENET_EVENT_TYPE_CONNECT:
					sendHello();
					break;
				case ENET_EVENT_TYPE_RECEIVE:
					handleMessage(event.packet->data, event.packet->dataLength);
					enet_packet_destroy(event.packet);
					break;
				case ENET_EVENT_TYPE_DISCONNECT:
					finish(PHASE_FAILED);
					break;
				default:
					break;
			}
		}
	}

	void sendMessage(const unsigned char *data, size_t length)
	{
		ENetPacket *packet = enet_packet_create(data, length, ENET_PACKET_FLAG_RELIABLE);
		if (packet == nullptr)
		{
			finish(PHASE_FAILED);
			return;
		}
		if (enet_peer_send(peer, CHANNEL_MESSAGES, packet) < 0)
		{
			enet_packet_destroy(packet);
			finish(PHASE_FAILED);
			return;
		}
		// The relay times the answer, so it is not left waiting for the next service call.
		enet_host_flush(host);
	}

	void sendHello()
	{
		unsigned char message[CHECK_HELLO_SIZE];
		message[0] = MSG_CHECK_HELLO;
		writeU16(message + 1, PROTO_VERSION);
		writeU32(message + 3, exeCrc);
		writeU32(message + 7, iniCrc);
		sendMessage(message, sizeof(message));
	}

	void handleMessage(const enet_uint8 *data, size_t length)
	{
		if (length == 0)
			return;

		switch (data[0])
		{
			case MSG_PING:
				if (length == PING_SIZE)
				{
					unsigned char message[PONG_SIZE];
					message[0] = MSG_PONG;
					memcpy(message + 1, data + 1, 8);
					sendMessage(message, sizeof(message));
				}
				break;
			case MSG_PASS:
				if (length == PASS_MESSAGE_SIZE)
					handlePass(data + 1);
				break;
			case MSG_REFUSE:
				if (length == REFUSE_SIZE)
					handleRefuse(data + 1);
				break;
			default:
				break;
		}
	}

	void handlePass(const unsigned char *bytes)
	{
		if (bytes[PASS_OFFSET_VERSION] != PASS_VERSION)
			return;
		if (readU16(bytes + PASS_OFFSET_REGION) != region.number)
			return;

		const unsigned int issued = readU32(bytes + PASS_OFFSET_ISSUED);
		const unsigned int expires = readU32(bytes + PASS_OFFSET_EXPIRES);
		if (expires <= issued)
			return;

		// Seconds to milliseconds, held to what 32 bits can count.
		unsigned int seconds = expires - issued;
		if (seconds > 0xFFFFFFFFu / 1000)
			seconds = 0xFFFFFFFFu / 1000;

		memcpy(pass.bytes, bytes, ServerPass::SIZE);
		pass.region = region.number;
		pass.rttMs = readU16(bytes + PASS_OFFSET_RTT);
		pass.lifetimeMs = seconds * 1000;
		pass.receivedAtMs = ServerClockMs();
		pass.valid = true;
		finish(PHASE_PASSED);
	}

	void handleRefuse(const unsigned char *body)
	{
		if (body[0] == SERVER_REFUSE_NO_ANSWER)
			return;

		refuseCode = body[0];
		if (refuseCode == SERVER_REFUSE_PING_TOO_HIGH)
		{
			refuseRttMs = readU16(body + 1);
			refuseLimitMs = readU16(body + 3);
		}
		finish(PHASE_REFUSED);
	}

	void finish(Phase result)
	{
		phase = result;
		closeHost();
	}

	void closeHost()
	{
		if (host != nullptr)
		{
			if (peer != nullptr)
				enet_peer_disconnect_now(peer, 0);
			enet_host_destroy(host);
			host = nullptr;
			peer = nullptr;
		}
	}

	ServerHostResolver resolver;
	unsigned int exeCrc;
	unsigned int iniCrc;
	ENetHost *host;
	ENetPeer *peer;
};

const ServerPass s_noPass;
const ServerRegion s_noRegion = {};

} // namespace

////////////////////////////////////////////////////////////////////////////////

unsigned int ServerClockMs()
{
	return (unsigned int)GetTickCount();
}

ServerPass::ServerPass()
	: region(0)
	, rttMs(0)
	, lifetimeMs(0)
	, receivedAtMs(0)
	, valid(false)
{
	memset(bytes, 0, sizeof(bytes));
}

bool ServerPass::isValid() const
{
	return valid;
}

unsigned int ServerPass::remainingMs() const
{
	if (!valid)
		return 0;

	const unsigned int age = ServerClockMs() - receivedAtMs;
	return age >= lifetimeMs ? 0 : lifetimeMs - age;
}

bool ServerPass::needsRenewal() const
{
	return remainingMs() < RENEW_BEFORE_MS;
}

////////////////////////////////////////////////////////////////////////////////

struct ServerEntryCheck::Impl
{
	std::vector<RegionAttempt *> attempts;
	State state;
	unsigned int startedAtMs;
	bool enetStarted;

	// Result
	const RegionAttempt *chosen;
	unsigned char refuseCode;
	unsigned short refuseRttMs;
	unsigned short refuseLimitMs;

	Impl()
		: state(STATE_IDLE)
		, startedAtMs(0)
		, enetStarted(false)
		, chosen(nullptr)
		, refuseCode(SERVER_REFUSE_NO_ANSWER)
		, refuseRttMs(0)
		, refuseLimitMs(0)
	{
	}

	void cancel()
	{
		for (size_t i = 0; i < attempts.size(); ++i)
			delete attempts[i];
		attempts.clear();

		if (enetStarted)
		{
			enet_deinitialize();
			enetStarted = false;
		}
		chosen = nullptr;
		refuseCode = SERVER_REFUSE_NO_ANSWER;
		refuseRttMs = 0;
		refuseLimitMs = 0;
		state = STATE_IDLE;
	}

	bool begin(const ServerRegion *regions, unsigned int count, unsigned int exeCrc, unsigned int iniCrc)
	{
		cancel();
		if (count == 0)
			return false;

		if (enet_initialize() != 0)
			return false;
		enetStarted = true;

		for (unsigned int i = 0; i < count; ++i)
			attempts.push_back(new RegionAttempt(regions[i], exeCrc, iniCrc));

		startedAtMs = ServerClockMs();
		state = STATE_RUNNING;
		return true;
	}

	/// Picks the pass with the lowest ping, the first listed on a tie, or else the refusal of the first region.
	void conclude()
	{
		const RegionAttempt *best = nullptr;
		for (size_t i = 0; i < attempts.size(); ++i)
		{
			const RegionAttempt *attempt = attempts[i];
			if (attempt->phase == RegionAttempt::PHASE_PASSED && (best == nullptr || attempt->pass.rttMs < best->pass.rttMs))
				best = attempt;
		}

		if (best != nullptr)
		{
			chosen = best;
			state = STATE_PASSED;
			return;
		}

		const RegionAttempt *first = attempts[0];
		if (first->phase == RegionAttempt::PHASE_REFUSED)
		{
			refuseCode = first->refuseCode;
			refuseRttMs = first->refuseRttMs;
			refuseLimitMs = first->refuseLimitMs;
		}
		state = STATE_REFUSED;
	}

	void update()
	{
		if (state != STATE_RUNNING)
			return;

		const unsigned int elapsed = ServerClockMs() - startedAtMs;
		bool allDone = true;
		for (size_t i = 0; i < attempts.size(); ++i)
		{
			attempts[i]->update(elapsed);
			allDone = allDone && attempts[i]->isDone();
		}

		if (allDone)
			conclude();
	}
};

ServerEntryCheck::ServerEntryCheck()
	: m_impl(new Impl)
{
}

ServerEntryCheck::~ServerEntryCheck()
{
	m_impl->cancel();
	delete m_impl;
}

bool ServerEntryCheck::start(const ServerRegions &regions, unsigned int exeCrc, unsigned int iniCrc)
{
	std::vector<ServerRegion> list;
	for (unsigned int i = 0; i < regions.getCount(); ++i)
		list.push_back(*regions.getRegion(i));

	return m_impl->begin(list.empty() ? nullptr : &list[0], (unsigned int)list.size(), exeCrc, iniCrc);
}

bool ServerEntryCheck::startRegion(const ServerRegion &region, unsigned int exeCrc, unsigned int iniCrc)
{
	return m_impl->begin(&region, 1, exeCrc, iniCrc);
}

void ServerEntryCheck::update()
{
	m_impl->update();
}

void ServerEntryCheck::cancel()
{
	m_impl->cancel();
}

ServerEntryCheck::State ServerEntryCheck::getState() const
{
	return m_impl->state;
}

const ServerPass &ServerEntryCheck::getPass() const
{
	return m_impl->state == STATE_PASSED ? m_impl->chosen->pass : s_noPass;
}

const ServerRegion &ServerEntryCheck::getRegion() const
{
	return m_impl->state == STATE_PASSED ? m_impl->chosen->region : s_noRegion;
}

unsigned char ServerEntryCheck::getRefuseCode() const
{
	return m_impl->refuseCode;
}

unsigned short ServerEntryCheck::getRttMs() const
{
	return m_impl->refuseRttMs;
}

unsigned short ServerEntryCheck::getLimitMs() const
{
	return m_impl->refuseLimitMs;
}

#endif // RTS_SERVERS
