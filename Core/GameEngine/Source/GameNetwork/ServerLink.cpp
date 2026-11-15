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

// ServerLink.cpp /////////////////////////////////////////////////////////////
// TheSuperHackers @feature Kayoti 13/11/2026 Adds the relay match connection of Servers mode.

#ifdef RTS_SERVERS

// This file is built without the engine's precompiled header. ENet headers are
// included here and nowhere else, so ENet's winsock2.h never meets the engine's windows.h.
#include <enet/enet.h>
#include <windows.h>

#include <string.h>
#include <deque>
#include <vector>

#include "GameNetwork/ServerLink.h"

namespace
{

// PROTOCOL.md 4.1 and 4.2
const unsigned short PROTO_VERSION = 1;
const size_t CHANNEL_COUNT = 2;
const enet_uint8 CHANNEL_MESSAGES = 0;

const unsigned char MSG_REFUSE = 5;
const unsigned char MSG_CREATE = 6;
const unsigned char MSG_CREATED = 7;
const unsigned char MSG_JOIN = 8;
const unsigned char MSG_WELCOME = 9;
const unsigned char MSG_ROSTER = 10;
const unsigned char MSG_START = 11;
const unsigned char MSG_DROP = 12;
const unsigned char MSG_LEAVE = 13;
const unsigned char MSG_DATA = 16;

const size_t CREATE_SIZE = 1 + 2 + ServerPass::SIZE + 1 + 1;
const size_t JOIN_SIZE = 1 + 2 + ServerPass::SIZE + 8 + 1 + 8;
const size_t FIRST_MESSAGE_MAX = JOIN_SIZE;
const size_t CREATED_SIZE = 1 + 8 + 8 * ServerLinkEvent::MAX_SLOTS;
const size_t WELCOME_SIZE = 1 + 8 + 1 + 1 + 1;
const size_t ROSTER_SIZE = 1 + 1 + 1;
const size_t START_SIZE = 1 + 1 + 1;
const size_t DROP_SIZE = 1 + 1 + 1 + 4;
const size_t REFUSE_SIZE = 1 + 1 + 2 + 2;

// PROTOCOL.md 4.6 and 6
const unsigned int RECORD_MIN = 7;
const unsigned int RECORD_MAX = 4096;
const unsigned int DATA_MESSAGE_MAX = 16384;

// Bounds the work of one update() call.
const int MAX_EVENTS_PER_UPDATE = 256;

unsigned short readU16(const unsigned char *p)
{
	return (unsigned short)(p[0] | (p[1] << 8));
}

unsigned int readU32(const unsigned char *p)
{
	return (unsigned int)p[0] | ((unsigned int)p[1] << 8) | ((unsigned int)p[2] << 16) | ((unsigned int)p[3] << 24);
}

unsigned long long readU64(const unsigned char *p)
{
	return (unsigned long long)readU32(p) | ((unsigned long long)readU32(p + 4) << 32);
}

void writeU16(unsigned char *p, unsigned short v)
{
	p[0] = (unsigned char)(v & 0xff);
	p[1] = (unsigned char)(v >> 8);
}

void writeU64(unsigned char *p, unsigned long long v)
{
	for (int i = 0; i < 8; ++i)
		p[i] = (unsigned char)((v >> (8 * i)) & 0xff);
}

/// Checks that a buffer is a whole number of records, each of a valid size. Used in both directions.
bool recordsAreWellFormed(const unsigned char *records, unsigned int length)
{
	if (records == nullptr || length == 0 || length > DATA_MESSAGE_MAX - 1)
		return false;

	unsigned int offset = 0;
	while (offset < length)
	{
		if (length - offset < 2)
			return false;
		const unsigned int recordLength = readU16(records + offset);
		offset += 2;
		if (recordLength < RECORD_MIN || recordLength > RECORD_MAX || recordLength > length - offset)
			return false;
		offset += recordLength;
	}
	return true;
}

struct PendingEvent
{
	ServerLinkEvent event;
	std::vector<unsigned char> payload;
};

} // namespace

ServerLinkEvent::ServerLinkEvent()
	: type(TYPE_NONE)
	, sessionId(0)
	, slot(0)
	, connectedMask(0)
	, leaderSlot(0)
	, dropReason(0)
	, lastFrame(0)
	, refuseCode(0)
	, rttMs(0)
	, limitMs(0)
	, data(nullptr)
	, dataLength(0)
{
	for (int i = 0; i < MAX_SLOTS; ++i)
		joinTokens[i] = 0;
}

////////////////////////////////////////////////////////////////////////////////

struct ServerLink::Impl
{
	ServerRegion region;
	ServerHostResolver resolver;
	State state;
	ENetHost *host;
	ENetPeer *peer;
	bool enetStarted;

	unsigned char firstMessage[FIRST_MESSAGE_MAX];
	size_t firstLength;
	bool isHost;
	unsigned char expectedSlot;
	unsigned long long expectedSession;	// the session a joining player asked for
	unsigned int timeoutMs;
	unsigned int openedAtMs;

	bool created;			// host: CREATED received
	unsigned long long createdSession;
	bool started;			// START received

	std::deque<PendingEvent> events;
	std::vector<unsigned char> delivered;	// payload of the event last returned by nextEvent()

	Impl()
		: state(STATE_IDLE)
		, host(nullptr)
		, peer(nullptr)
		, enetStarted(false)
		, firstLength(0)
		, isHost(false)
		, expectedSlot(0)
		, expectedSession(0)
		, timeoutMs(0)
		, openedAtMs(0)
		, created(false)
		, createdSession(0)
		, started(false)
	{
		memset(&region, 0, sizeof(region));
		memset(firstMessage, 0, sizeof(firstMessage));
	}

	bool isOpen() const
	{
		return state == STATE_CONNECTING || state == STATE_CONNECTED || state == STATE_LEAVING;
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
		if (enetStarted)
		{
			enet_deinitialize();
			enetStarted = false;
		}
		resolver.reset();
	}

	void close()
	{
		closeHost();
		if (state != STATE_IDLE)
			state = STATE_CLOSED;
	}

	void reset()
	{
		closeHost();
		events.clear();
		delivered.clear();
		created = false;
		createdSession = 0;
		started = false;
		state = STATE_IDLE;
	}

	void push(const ServerLinkEvent &event, const unsigned char *payload = nullptr, unsigned int payloadLength = 0)
	{
		PendingEvent pending;
		pending.event = event;
		if (payload != nullptr && payloadLength > 0)
			pending.payload.assign(payload, payload + payloadLength);
		events.push_back(pending);
	}

	void lose()
	{
		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_LOST;
		push(event);
		close();
	}

	bool open(const ServerRegion &regionIn, const unsigned char *first, size_t firstLen, unsigned int timeout)
	{
		reset();

		if (timeout == 0 || regionIn.relayHost[0] == '\0' || regionIn.relayPort == 0)
			return false;
		if (!resolver.start(regionIn.relayHost))
			return false;
		if (enet_initialize() != 0)
		{
			resolver.reset();
			return false;
		}
		enetStarted = true;

		region = regionIn;
		memcpy(firstMessage, first, firstLen);
		firstLength = firstLen;
		timeoutMs = timeout;
		openedAtMs = ServerClockMs();
		state = STATE_CONNECTING;
		return true;
	}

	void connectToRelay()
	{
		host = enet_host_create(nullptr, 1, CHANNEL_COUNT, 0, 0);
		if (host == nullptr)
		{
			lose();
			return;
		}
		// No message of this protocol is larger than one DATA message.
		host->maximumPacketSize = DATA_MESSAGE_MAX;

		ENetAddress address;
		address.host = resolver.getAddress();
		address.port = region.relayPort;
		peer = enet_host_connect(host, &address, CHANNEL_COUNT, 0);
		if (peer == nullptr)
			lose();
	}

	bool sendMessage(const unsigned char *data, size_t length)
	{
		if (host == nullptr || peer == nullptr)
			return false;

		ENetPacket *packet = enet_packet_create(data, length, ENET_PACKET_FLAG_RELIABLE);
		if (packet == nullptr)
			return false;
		if (enet_peer_send(peer, CHANNEL_MESSAGES, packet) < 0)
		{
			enet_packet_destroy(packet);
			return false;
		}
		return true;
	}

	////////////////////////////////////////////////////////////////////////////
	// Messages from the relay. A message that is the wrong size or not valid in the current state is dropped.

	void handleCreated(const unsigned char *body)
	{
		if (!isHost || created || state != STATE_CONNECTING)
			return;

		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_CREATED;
		event.sessionId = readU64(body);
		for (int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i)
			event.joinTokens[i] = readU64(body + 8 + 8 * i);

		created = true;
		createdSession = event.sessionId;
		push(event);
	}

	void handleWelcome(const unsigned char *body)
	{
		if (state != STATE_CONNECTING || (isHost && !created))
			return;

		const unsigned long long session = readU64(body);
		const unsigned char slot = body[8];
		const unsigned char mask = body[9];
		const unsigned char leader = body[10];
		if (slot >= ServerLinkEvent::MAX_SLOTS || leader >= ServerLinkEvent::MAX_SLOTS)
			return;
		if (slot != expectedSlot || session != (isHost ? createdSession : expectedSession))
			return;

		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_WELCOME;
		event.sessionId = session;
		event.slot = slot;
		event.connectedMask = mask;
		event.leaderSlot = leader;
		push(event);
		state = STATE_CONNECTED;
	}

	void handleRoster(ServerLinkEvent::Type type, const unsigned char *body)
	{
		if (state != STATE_CONNECTED || body[1] >= ServerLinkEvent::MAX_SLOTS)
			return;
		if (type == ServerLinkEvent::TYPE_START)
		{
			if (started)
				return;
			started = true;
		}

		ServerLinkEvent event;
		event.type = type;
		event.connectedMask = body[0];
		event.leaderSlot = body[1];
		push(event);
	}

	void handleDrop(const unsigned char *body)
	{
		if (state != STATE_CONNECTED || body[0] >= ServerLinkEvent::MAX_SLOTS)
			return;

		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_DROP;
		event.slot = body[0];
		event.dropReason = body[1];
		event.lastFrame = readU32(body + 2);
		push(event);
	}

	void handleRefuse(const unsigned char *body)
	{
		if (state != STATE_CONNECTING && state != STATE_CONNECTED)
			return;

		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_REFUSE;
		event.refuseCode = body[0];
		if (event.refuseCode == SERVER_REFUSE_PING_TOO_HIGH)
		{
			event.rttMs = readU16(body + 1);
			event.limitMs = readU16(body + 3);
		}
		push(event);
		close();
	}

	void handleData(const unsigned char *records, unsigned int length)
	{
		if (state != STATE_CONNECTED || !recordsAreWellFormed(records, length))
			return;

		ServerLinkEvent event;
		event.type = ServerLinkEvent::TYPE_DATA;
		event.dataLength = length;
		push(event, records, length);
	}

	void handleMessage(const enet_uint8 *data, size_t length)
	{
		if (length == 0)
			return;

		const unsigned char *body = data + 1;
		switch (data[0])
		{
			case MSG_CREATED:
				if (length == CREATED_SIZE)
					handleCreated(body);
				break;
			case MSG_WELCOME:
				if (length == WELCOME_SIZE)
					handleWelcome(body);
				break;
			case MSG_ROSTER:
				if (length == ROSTER_SIZE)
					handleRoster(ServerLinkEvent::TYPE_ROSTER, body);
				break;
			case MSG_START:
				if (length == START_SIZE)
					handleRoster(ServerLinkEvent::TYPE_START, body);
				break;
			case MSG_DROP:
				if (length == DROP_SIZE)
					handleDrop(body);
				break;
			case MSG_REFUSE:
				if (length == REFUSE_SIZE)
					handleRefuse(body);
				break;
			case MSG_DATA:
				if (length <= DATA_MESSAGE_MAX)
					handleData(body, (unsigned int)(length - 1));
				break;
			default:
				break;
		}
	}

	void onDisconnect()
	{
		if (state == STATE_LEAVING)
			close();
		else if (isOpen())
			lose();
	}

	void service()
	{
		ENetEvent event;
		for (int i = 0; i < MAX_EVENTS_PER_UPDATE && host != nullptr; ++i)
		{
			if (enet_host_service(host, &event, 0) <= 0)
				break;

			switch (event.type)
			{
				case ENET_EVENT_TYPE_CONNECT:
					if (!sendMessage(firstMessage, firstLength))
						lose();
					break;
				case ENET_EVENT_TYPE_RECEIVE:
					handleMessage(event.packet->data, event.packet->dataLength);
					enet_packet_destroy(event.packet);
					break;
				case ENET_EVENT_TYPE_DISCONNECT:
					onDisconnect();
					break;
				default:
					break;
			}
		}
	}

	void update()
	{
		if (!isOpen())
			return;

		if (state == STATE_CONNECTING && ServerClockMs() - openedAtMs >= timeoutMs)
		{
			lose();
			return;
		}

		if (host == nullptr)
		{
			const ServerHostResolver::State resolved = resolver.getState();
			if (resolved == ServerHostResolver::STATE_FAILED)
				lose();
			else if (resolved == ServerHostResolver::STATE_RESOLVED)
				connectToRelay();
		}

		if (host != nullptr)
			service();
	}
};

ServerLink::ServerLink()
	: m_impl(new Impl)
{
}

ServerLink::~ServerLink()
{
	m_impl->close();
	delete m_impl;
}

bool ServerLink::create(const ServerRegion &region, const ServerPass &pass, unsigned char hostSlot, unsigned char slotMask, unsigned int timeoutMs)
{
	if (m_impl->isOpen() || !pass.isValid() || hostSlot >= ServerLinkEvent::MAX_SLOTS)
		return false;

	// The host's own slot and at least one other slot are expected.
	const unsigned int hostBit = 1u << hostSlot;
	if (!(slotMask & hostBit) || (slotMask & ~hostBit) == 0)
		return false;

	unsigned char message[FIRST_MESSAGE_MAX];
	message[0] = MSG_CREATE;
	writeU16(message + 1, PROTO_VERSION);
	memcpy(message + 3, pass.bytes, ServerPass::SIZE);
	message[3 + ServerPass::SIZE] = hostSlot;
	message[4 + ServerPass::SIZE] = slotMask;

	if (!m_impl->open(region, message, CREATE_SIZE, timeoutMs))
		return false;
	m_impl->isHost = true;
	m_impl->expectedSlot = hostSlot;
	return true;
}

bool ServerLink::join(const ServerRegion &region, const ServerPass &pass, unsigned long long sessionId, unsigned char slot, unsigned long long joinToken, unsigned int timeoutMs)
{
	if (m_impl->isOpen() || !pass.isValid() || slot >= ServerLinkEvent::MAX_SLOTS)
		return false;

	unsigned char message[FIRST_MESSAGE_MAX];
	message[0] = MSG_JOIN;
	writeU16(message + 1, PROTO_VERSION);
	memcpy(message + 3, pass.bytes, ServerPass::SIZE);
	writeU64(message + 3 + ServerPass::SIZE, sessionId);
	message[11 + ServerPass::SIZE] = slot;
	writeU64(message + 12 + ServerPass::SIZE, joinToken);

	if (!m_impl->open(region, message, JOIN_SIZE, timeoutMs))
		return false;
	m_impl->isHost = false;
	m_impl->expectedSlot = slot;
	m_impl->expectedSession = sessionId;
	return true;
}

void ServerLink::update()
{
	m_impl->update();
}

bool ServerLink::nextEvent(ServerLinkEvent &event)
{
	if (m_impl->events.empty())
		return false;

	PendingEvent &front = m_impl->events.front();
	event = front.event;
	m_impl->delivered.swap(front.payload);
	m_impl->events.pop_front();

	if (event.type == ServerLinkEvent::TYPE_DATA)
		event.data = &m_impl->delivered[0];
	return true;
}

bool ServerLink::sendData(const unsigned char *records, unsigned int length)
{
	if (m_impl->state != STATE_CONNECTED || !recordsAreWellFormed(records, length))
		return false;

	std::vector<unsigned char> message(1 + (size_t)length);
	message[0] = MSG_DATA;
	memcpy(&message[1], records, length);
	return m_impl->sendMessage(&message[0], message.size());
}

void ServerLink::leave()
{
	if (m_impl->state == STATE_CONNECTED)
	{
		const unsigned char message[1] = { MSG_LEAVE };
		if (m_impl->sendMessage(message, sizeof(message)))
		{
			enet_peer_disconnect_later(m_impl->peer, 0);
			m_impl->state = STATE_LEAVING;
			return;
		}
	}
	if (m_impl->isOpen())
		m_impl->close();
}

void ServerLink::close()
{
	m_impl->close();
}

ServerLink::State ServerLink::getState() const
{
	return m_impl->state;
}

#endif // RTS_SERVERS
