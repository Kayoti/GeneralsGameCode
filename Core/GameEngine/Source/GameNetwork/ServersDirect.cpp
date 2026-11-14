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

// FILE: ServersDirect.cpp ////////////////////////////////////////////////////
// Starts a Servers match from launch flags, without the lobby.
// Protocol: docs/servers/PROTOCOL.md, sections 4.4, 4.5 and 5.4.

// TheSuperHackers @feature Kayoti 14/11/2026 Adds the direct Servers match start driven by launch flags.

#include "PreRTS.h"	// This must go first in EVERY cpp file in the GameEngine

#ifdef RTS_SERVERS

#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

#include "GameNetwork/ServersDirect.h"

#include "Common/GlobalData.h"
#include "Common/MessageStream.h"
#include "Common/PlayerTemplate.h"
#include "Common/RandomValue.h"
#include "GameClient/GameText.h"
#include "GameClient/MapUtil.h"
#include "GameClient/MessageBox.h"
#include "GameClient/Shell.h"
#include "GameLogic/GameLogic.h"
#include "GameNetwork/NetworkInterface.h"
#include "GameNetwork/ServerEntryCheck.h"
#include "GameNetwork/ServerLink.h"
#include "GameNetwork/ServerRegions.h"
#include "GameNetwork/GameSpy/ServersMode.h"
#include "GameNetwork/GameSpy/StagingRoomGameInfo.h"

// The seed every instance loads the match with. Documented in docs/servers/TESTING.md.
static const Int SERVERS_DIRECT_SEED = 12345;

// The number of the region the flags describe. The relay answers the entry check whatever the number is.
static const UnsignedShort SERVERS_DIRECT_REGION_NUMBER = 1;

// The host writes how each other instance joins to this file, in the working directory of the game.
static const char *const SERVERS_DIRECT_MATCH_FILE = "ServersDirectMatch.txt";

static const Int SERVERS_DIRECT_MAP_SIZE = 260;

// Flags -------------------------------------------------------------------------------------------

struct ServersDirectParams
{
	Bool host;
	Bool join;
	Bool invalid;												///< A flag had a bad value. No direct start is attempted.
	Bool hasRelay;
	Bool hasPort;
	Bool hasSlot;
	Bool hasPlayers;
	Bool hasMap;
	Bool hasSession;
	Bool hasToken;
	char relay[ServerRegion::HOST_SIZE];
	UnsignedShort port;
	UnsignedByte slot;
	UnsignedByte players;
	char map[SERVERS_DIRECT_MAP_SIZE];
	UnsignedInt64 session;
	UnsignedInt64 token;
};

static ServersDirectParams s_params;	// zero until a flag is given

static void rejectFlag( const char *name, const char *reason )
{
	DEBUG_LOG(( "Servers direct: flag %s %s", name, reason ));
	s_params.invalid = TRUE;
}

static Bool isHexDigit( char c )
{
	return ( c >= '0' && c <= '9' ) || ( c >= 'a' && c <= 'f' ) || ( c >= 'A' && c <= 'F' );
}

/// Reads a whole number in the given base. A sign, a space, trailing text or a value above maxValue is refused.
static Bool parseNumber( const char *text, Int base, UnsignedInt64 maxValue, UnsignedInt64 &out )
{
	if( text == nullptr || !isHexDigit( text[0] ) )
		return FALSE;

	char *end = nullptr;
	errno = 0;
	const unsigned long long value = std::strtoull( text, &end, base );
	if( end == text || *end != '\0' || errno == ERANGE || value > maxValue )
		return FALSE;

	out = (UnsignedInt64)value;
	return TRUE;
}

static Bool isRelayHostText( const char *text )
{
	if( text == nullptr || text[0] == '\0' || std::strlen( text ) >= ServerRegion::HOST_SIZE )
		return FALSE;
	for( const char *c = text; *c != '\0'; ++c )
	{
		const Bool ok = ( *c >= '0' && *c <= '9' ) || ( *c >= 'a' && *c <= 'z' ) || ( *c >= 'A' && *c <= 'Z' ) || *c == '.' || *c == '-';
		if( !ok )
			return FALSE;
	}
	return TRUE;
}

void ServersDirectSetFlag( ServersDirectFlag flag, const char *value )
{
	UnsignedInt64 number = 0;

	switch( flag )
	{
		case SERVERS_DIRECT_FLAG_HOST:
			s_params.host = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_JOIN:
			s_params.join = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_RELAY:
			if( !isRelayHostText( value ) )
			{
				rejectFlag( "-serversRelay", "needs a host name or an IPv4 address" );
				break;
			}
			strlcpy( s_params.relay, value, ARRAY_SIZE( s_params.relay ) );
			s_params.hasRelay = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_PORT:
			if( !parseNumber( value, 10, 65535, number ) || number == 0 )
			{
				rejectFlag( "-serversPort", "needs a port from 1 to 65535" );
				break;
			}
			s_params.port = (UnsignedShort)number;
			s_params.hasPort = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_SLOT:
			if( !parseNumber( value, 10, ServerLinkEvent::MAX_SLOTS - 1, number ) )
			{
				rejectFlag( "-serversSlot", "needs a slot from 0 to 7" );
				break;
			}
			s_params.slot = (UnsignedByte)number;
			s_params.hasSlot = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_PLAYERS:
			if( !parseNumber( value, 10, ServerLinkEvent::MAX_SLOTS, number ) || number < 2 )
			{
				rejectFlag( "-serversPlayers", "needs a player count from 2 to 8" );
				break;
			}
			s_params.players = (UnsignedByte)number;
			s_params.hasPlayers = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_MAP:
			if( value == nullptr || value[0] == '\0' || std::strlen( value ) >= SERVERS_DIRECT_MAP_SIZE )
			{
				rejectFlag( "-serversMap", "needs a map name" );
				break;
			}
			strlcpy( s_params.map, value, ARRAY_SIZE( s_params.map ) );
			s_params.hasMap = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_SESSION:
			if( !parseNumber( value, 16, ~(UnsignedInt64)0, number ) || number == 0 )
			{
				rejectFlag( "-serversSession", "needs a session id of up to 16 hex digits, not zero" );
				break;
			}
			s_params.session = number;
			s_params.hasSession = TRUE;
			break;

		case SERVERS_DIRECT_FLAG_TOKEN:
			if( !parseNumber( value, 16, ~(UnsignedInt64)0, number ) || number == 0 )
			{
				rejectFlag( "-serversToken", "needs a join token of up to 16 hex digits, not zero" );
				break;
			}
			s_params.token = number;
			s_params.hasToken = TRUE;
			break;
	}
}

/// True if the flags that were given are a complete and consistent description of a host or a joiner.
static Bool paramsAreUsable()
{
	if( s_params.invalid )
		return FALSE;

	if( s_params.host == s_params.join )
	{
		DEBUG_LOG(( "Servers direct: give exactly one of -serversHost and -serversJoin" ));
		return FALSE;
	}
	if( !s_params.hasRelay || !s_params.hasPort || !s_params.hasSlot || !s_params.hasPlayers || !s_params.hasMap )
	{
		DEBUG_LOG(( "Servers direct: -serversRelay, -serversPort, -serversSlot, -serversPlayers and -serversMap are all needed" ));
		return FALSE;
	}
	if( s_params.slot >= s_params.players )
	{
		DEBUG_LOG(( "Servers direct: -serversSlot %d is not below -serversPlayers %d", (Int)s_params.slot, (Int)s_params.players ));
		return FALSE;
	}
	if( s_params.join && ( !s_params.hasSession || !s_params.hasToken ) )
	{
		DEBUG_LOG(( "Servers direct: -serversJoin needs -serversSession and -serversToken" ));
		return FALSE;
	}
	if( s_params.host && ( s_params.hasSession || s_params.hasToken ) )
	{
		DEBUG_LOG(( "Servers direct: -serversHost does not take -serversSession or -serversToken" ));
		return FALSE;
	}
	return TRUE;
}

// The game of a direct match -----------------------------------------------------------------------

/**
 * The slot list of a direct match. The engine finds the game of a network match without a LAN session
 * through TheGameSpyGame, so this takes that role. Nothing of the online lobby is used: the local slot
 * comes from the flags, not from a login name.
 */
class ServersDirectGameInfo : public GameSpyStagingRoom
{
public:
	ServersDirectGameInfo() : m_localSlot( -1 ), m_isHost( FALSE ) {}
	virtual ~ServersDirectGameInfo() override {}

	virtual Int getLocalSlotNum() const override { return m_localSlot; }
	virtual Bool amIHost() const override { return m_isHost; }

	/// Fills the slot list from the flags. Every slot is a human player with a fixed faction and colour.
	void setUp( const AsciiString &map, const std::vector<Int> &templates );

private:
	Int m_localSlot;
	Bool m_isHost;
};

void ServersDirectGameInfo::setUp( const AsciiString &map, const std::vector<Int> &templates )
{
	enterGame();
	setMap( map );
	setSeed( SERVERS_DIRECT_SEED );

	for( Int i = 0; i < s_params.players; ++i )
	{
		UnicodeString name;
		name.format( L"Player %d", i + 1 );

		GameSlot *slot = getSlot( i );
		slot->setState( SLOT_PLAYER, name );
		slot->setAccept();
		// The faction of slot N is the Nth playable template, wrapping around. The colour is the slot number.
		slot->setPlayerTemplate( templates.empty() ? (Int)PLAYERTEMPLATE_RANDOM : templates[i % templates.size()] );
		slot->setColor( i );
		slot->setTeamNumber( -1 );
	}

	m_localSlot = s_params.slot;
	m_isHost = s_params.host;

	// Closes the slots above the player count and marks the game as started, as a started LAN game does.
	GameInfo::startGame( 0 );
}

// The player templates a player can pick, as the lobby offers them.
static void collectPlayableTemplates( std::vector<Int> &out )
{
	const Int count = ThePlayerTemplateStore->getPlayerTemplateCount();
	for( Int i = 0; i < count; ++i )
	{
		const PlayerTemplate *pt = ThePlayerTemplateStore->getNthPlayerTemplate( i );
		if( pt != nullptr && pt->isPlayableSide() && pt->getSide().compare( "Boss" ) != 0 )
			out.push_back( i );
	}
}

/// Finds the map in the map cache. The name may be the full cache name or a shorter form of it.
static Bool resolveMap( AsciiString &outMap )
{
	char name[SERVERS_DIRECT_MAP_SIZE];
	strlcpy( name, s_params.map, ARRAY_SIZE( name ) );
	for( char *c = name; *c != '\0'; ++c )
	{
		if( *c == '/' )
			*c = '\\';
	}

	AsciiString candidates[3];
	candidates[0] = name;
	candidates[1].format( "Maps\\%s", name );
	if( std::strchr( name, '\\' ) == nullptr )
	{
		AsciiString stem = name;
		if( stem.endsWithNoCase( ".map" ) )
			stem.truncateBy( 4 );
		candidates[2].format( "Maps\\%s\\%s.map", stem.str(), stem.str() );
	}

	TheMapCache->updateCache();
	for( Int i = 0; i < 3; ++i )
	{
		if( candidates[i].isNotEmpty() && TheMapCache->findMap( candidates[i] ) != nullptr )
		{
			outMap = candidates[i];
			return TRUE;
		}
	}
	return FALSE;
}

// Driving the start ----------------------------------------------------------------------------------

enum ServersDirectPhase
{
	PHASE_START,				///< The flags have not been looked at yet
	PHASE_WAIT_ENGINE,	///< Waiting for the main menu
	PHASE_ENTRY,				///< The entry check is running
	PHASE_MATCH,				///< The CREATE or JOIN handshake is running
	PHASE_DONE,					///< Nothing more to do, because of no flags, a launch or a failure
};

static ServersDirectPhase s_phase = PHASE_START;
static ServerEntryCheck *s_check = nullptr;
static ServersDirectGameInfo *s_game = nullptr;	// Lives until the process ends, as the engine keeps pointing at it.

static void showError( const UnicodeString &text )
{
	MessageBoxOk( TheGameText->fetch( "GUI:ServersTitle" ), text, nullptr );
}

static void showStartFailed()
{
	showError( UnicodeString( L"Servers direct start failed. See the log." ) );
}

static void finish( Bool failed )
{
	delete s_check;
	s_check = nullptr;
	if( failed )
		ServersModeClear();
	s_phase = PHASE_DONE;
}

static Bool engineIsReady()
{
	return TheGlobalData != nullptr && TheShell != nullptr && TheShell->top() != nullptr && TheShell->isAnimFinished()
		&& TheGameLogic != nullptr && !TheGameLogic->isInGame() && TheNetwork == nullptr
		&& TheMapCache != nullptr && ThePlayerTemplateStore != nullptr && TheGameText != nullptr && TheMessageStream != nullptr;
}

static Bool startEntryCheck()
{
	ServerRegion region;
	std::memset( &region, 0, sizeof( region ) );
	region.number = SERVERS_DIRECT_REGION_NUMBER;
	strlcpy( region.name, "Direct", ARRAY_SIZE( region.name ) );
	strlcpy( region.relayHost, s_params.relay, ARRAY_SIZE( region.relayHost ) );
	region.relayPort = s_params.port;

	ServersModeClear();
	s_check = NEW ServerEntryCheck;
	if( !s_check->startRegion( region, TheGlobalData->m_exeCRC, TheGlobalData->m_iniCRC ) )
	{
		DEBUG_LOG(( "Servers direct: the entry check could not start" ));
		return FALSE;
	}
	return TRUE;
}

static Bool beginMatch()
{
	if( s_params.host )
	{
		const UnsignedByte slotMask = (UnsignedByte)( ( 1 << s_params.players ) - 1 );
		if( !ServersMatchHostBegin( s_params.slot, slotMask ) )
		{
			DEBUG_LOG(( "Servers direct: CREATE could not be sent" ));
			return FALSE;
		}
		return TRUE;
	}

	// The joiner hands the flags to the same private message parser that the staging room uses.
	AsciiString payload;
	payload.format( "%016I64x/%u/%016I64x", s_params.session, (UnsignedInt)s_params.slot, s_params.token );
	const Bool stored = ServersMatchStorePrivate( payload.str() );
	const Bool begun = stored && ServersMatchJoinBegin();
	ServersMatchForgetPrivate();
	if( !begun )
		DEBUG_LOG(( "Servers direct: JOIN could not be sent" ));
	return begun;
}

/// The host writes the session id and each join token to the log and to the match file.
static void reportJoinTokens()
{
	FILE *file = std::fopen( SERVERS_DIRECT_MATCH_FILE, "w" );

	for( Int i = 0; i < s_params.players; ++i )
	{
		if( i == s_params.slot )
			continue;

		AsciiString payload;
		if( !ServersMatchBuildPrivate( (UnsignedByte)i, payload ) )
			continue;

		char session[17] = { 0 };
		char token[17] = { 0 };
		unsigned int slot = 0;
		if( std::sscanf( payload.str(), "%16[0-9a-f]/%u/%16[0-9a-f]", session, &slot, token ) != 3 )
			continue;

		DEBUG_LOG(( "Servers direct: slot %u joins with -serversJoin -serversSlot %u -serversSession %s -serversToken %s",
			slot, slot, session, token ));
		if( file != nullptr )
			std::fprintf( file, "-serversJoin -serversSlot %u -serversSession %s -serversToken %s\n", slot, session, token );
	}

	if( file != nullptr )
		std::fclose( file );
}

/// Loads the match the way a started match does: the slot list, the network, then the new game message.
static Bool launchMatch()
{
	if( TheGameSpyGame != nullptr || s_game != nullptr )
	{
		DEBUG_LOG(( "Servers direct: a game is already set up" ));
		return FALSE;
	}

	AsciiString map;
	if( !resolveMap( map ) )
	{
		DEBUG_LOG(( "Servers direct: map %s is not in the map cache", s_params.map ));
		return FALSE;
	}

	std::vector<Int> templates;
	collectPlayableTemplates( templates );

	s_game = NEW ServersDirectGameInfo;
	s_game->setUp( map, templates );
	TheGameSpyGame = s_game;

	// There are no player-to-player connections and no transport in a relay match. parseUserList hands
	// the match link to the connection manager and turns server mode on.
	TheNetwork = NetworkInterface::createNetwork();
	TheNetwork->init();
	TheNetwork->parseUserList( s_game );

	TheWritableGlobalData->m_pendingFile = s_game->getMap();

	GameMessage *msg = TheMessageStream->appendMessage( GameMessage::MSG_NEW_GAME );
	msg->appendIntegerArgument( GAME_LAN );

	TheWritableGlobalData->m_useFpsLimit = false;

	InitRandom( s_game->getSeed() );
	DEBUG_LOG(( "Servers direct: launching %s, seed %d, local slot %d", map.str(), (Int)s_game->getSeed(), (Int)s_game->getLocalSlotNum() ));
	return TRUE;
}

static void launchOrFail()
{
	if( launchMatch() )
	{
		finish( FALSE );
		return;
	}
	showStartFailed();
	finish( TRUE );
}

static void stepEntryCheck()
{
	s_check->update();

	switch( s_check->getState() )
	{
		case ServerEntryCheck::STATE_RUNNING:
			return;

		case ServerEntryCheck::STATE_PASSED:
			ServersModeSet( s_check->getRegion(), s_check->getPass() );
			DEBUG_LOG(( "Servers direct: entry check passed, ping %d ms", (Int)s_check->getPass().rttMs ));
			delete s_check;
			s_check = nullptr;
			if( beginMatch() )
			{
				s_phase = PHASE_MATCH;
				return;
			}
			showStartFailed();
			finish( TRUE );
			return;

		case ServerEntryCheck::STATE_REFUSED:
			DEBUG_LOG(( "Servers direct: entry check refused, code %d", (Int)s_check->getRefuseCode() ));
			showError( ServersModeRefusalText( s_check->getRefuseCode(), s_check->getRttMs(), s_check->getLimitMs() ) );
			finish( TRUE );
			return;

		default:
			DEBUG_LOG(( "Servers direct: entry check ended without a result" ));
			showError( ServersModeRefusalText( SERVER_REFUSE_NO_ANSWER, 0, 0 ) );
			finish( TRUE );
			return;
	}
}

static void stepMatch()
{
	switch( ServersMatchUpdate() )
	{
		case SERVERS_MATCH_CONTACTING:
			return;

		case SERVERS_MATCH_HOST_READY:
			reportJoinTokens();
			launchOrFail();
			return;

		case SERVERS_MATCH_CONNECTED:
			launchOrFail();
			return;

		default:
			DEBUG_LOG(( "Servers direct: the relay handshake did not finish" ));
			showError( ServersModeRefusalText( SERVER_REFUSE_NO_ANSWER, 0, 0 ) );
			finish( TRUE );
			return;
	}
}

void ServersDirectUpdate()
{
	switch( s_phase )
	{
		case PHASE_DONE:
			return;

		case PHASE_START:
			if( !s_params.host && !s_params.join && !s_params.invalid )
			{
				s_phase = PHASE_DONE;	// No flags: this is never looked at again
				return;
			}
			s_phase = paramsAreUsable() ? PHASE_WAIT_ENGINE : PHASE_DONE;
			return;

		case PHASE_WAIT_ENGINE:
			if( !engineIsReady() )
				return;
			if( !startEntryCheck() )
			{
				showStartFailed();
				finish( TRUE );
				return;
			}
			s_phase = PHASE_ENTRY;
			return;

		case PHASE_ENTRY:
			stepEntryCheck();
			return;

		case PHASE_MATCH:
			stepMatch();
			return;
	}
}

#endif // RTS_SERVERS
