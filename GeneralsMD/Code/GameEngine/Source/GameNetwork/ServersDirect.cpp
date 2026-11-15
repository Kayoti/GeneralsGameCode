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
// Starts a Servers match from launch flags, without the lobby. The match is built around an
// NGMPGame whose slots are filled from the flags, because this client's game logic takes its
// player list from TheNGMPGame when there is no LAN game (GameLogic.cpp).
// Protocol: docs/servers/PROTOCOL.md, sections 4.4, 4.5 and 4.6.

// TheSuperHackers @feature Kayoti 15/11/2026 Adds the direct Servers match start driven by launch flags.

#include "PreRTS.h"	// This must go first in EVERY cpp file in the GameEngine

#ifdef RTS_SERVERS

#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

#include "GameNetwork/ServersDirect.h"

#include "Common/GlobalData.h"
#include "Common/PlayerTemplate.h"
#include "GameClient/GameText.h"
#include "GameClient/MapUtil.h"
#include "GameClient/MessageBox.h"
#include "GameClient/Shell.h"
#include "GameLogic/GameLogic.h"
#include "GameNetwork/NetworkInterface.h"
#include "GameNetwork/ServerEntryCheck.h"
#include "GameNetwork/ServerLink.h"
#include "GameNetwork/ServerRegions.h"
#include "GameNetwork/ServersMode.h"
#include "GameNetwork/GeneralsOnline/NGMPGame.h"

// GameLogic.cpp chooses TheGameInfo = TheNGMPGame when TheNetwork is set and TheLAN is null, so the
// direct match installs its game here.
extern NGMPGame *TheNGMPGame;

// The seed every instance loads the match with. Documented in docs/servers/TESTING.md.
static const Int SERVERS_DIRECT_SEED = 12345;

// The number of the region the flags describe. The relay answers the entry check whatever the number is.
static const UnsignedShort SERVERS_DIRECT_REGION_NUMBER = 1;

// The relay must answer CREATE or JOIN with WELCOME within this time, or the link ends with LOST.
static const UnsignedInt SERVERS_DIRECT_LINK_TIMEOUT_MS = 15000;

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
	if( text == NULL || !isHexDigit( text[0] ) )
		return FALSE;

	char *end = NULL;
	errno = 0;
	const unsigned long long value = std::strtoull( text, &end, base );
	if( end == text || *end != '\0' || errno == ERANGE || value > maxValue )
		return FALSE;

	out = (UnsignedInt64)value;
	return TRUE;
}

static Bool isRelayHostText( const char *text )
{
	if( text == NULL || text[0] == '\0' || std::strlen( text ) >= ServerRegion::HOST_SIZE )
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
			if( value == NULL || value[0] == '\0' || std::strlen( value ) >= SERVERS_DIRECT_MAP_SIZE )
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
 * The game of a direct match. It derives from NGMPGame so that GameLogic.cpp uses it as TheGameInfo, but
 * its slots and local slot come from the flags, not from the lobby. The base NGMPGame constructor reads
 * the lobby interface, which is null on a flag start, so it returns early and leaves the slot pointers
 * unset; this constructor sets them.
 */
class ServersDirectGameInfo : public NGMPGame
{
public:
	ServersDirectGameInfo() : m_localSlot( -1 ), m_isHost( FALSE )
	{
		cleanUpSlotPointers();
	}

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

	// Closes the slots above the player count and marks the game as started, as a started game does.
	// The base NGMPGame::startGame runs the lobby launch, so the GameInfo version is called directly.
	GameInfo::startGame( 0 );
}

// The player templates a player can pick, as the lobby offers them.
static void collectPlayableTemplates( std::vector<Int> &out )
{
	const Int count = ThePlayerTemplateStore->getPlayerTemplateCount();
	for( Int i = 0; i < count; ++i )
	{
		const PlayerTemplate *pt = ThePlayerTemplateStore->getNthPlayerTemplate( i );
		if( pt != NULL && pt->isPlayableSide() && pt->getSide().compare( "Boss" ) != 0 )
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
	if( std::strchr( name, '\\' ) == NULL )
	{
		AsciiString stem = name;
		if( stem.endsWithNoCase( ".map" ) )
			stem.truncateBy( 4 );
		candidates[2].format( "Maps\\%s\\%s.map", stem.str(), stem.str() );
	}

	TheMapCache->updateCache();
	for( Int i = 0; i < 3; ++i )
	{
		if( candidates[i].isNotEmpty() && TheMapCache->findMap( candidates[i] ) != NULL )
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
	PHASE_HANDSHAKE,		///< The CREATE or JOIN handshake is running on the relay link
	PHASE_DONE,					///< Nothing more to do, because of no flags, a launch or a failure
};

static ServersDirectPhase s_phase = PHASE_START;
static ServerEntryCheck *s_check = NULL;
static ServerLink *s_link = NULL;								// Owned here; handed to the connection manager on launch, which does not own it.
static ServersDirectGameInfo *s_game = NULL;		// Lives until the process ends, as the engine keeps pointing at it.
static ServerRegion s_region;
static ServerPass s_pass;
static UnsignedInt64 s_sessionId = 0;
static UnsignedInt64 s_joinTokens[ServerLinkEvent::MAX_SLOTS];

static void showError( const UnicodeString &text )
{
	MessageBoxOk( TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersTitle", L"Servers" ), text, NULL );
}

static void showStartFailed()
{
	showError( UnicodeString( L"Servers direct start failed. See the log." ) );
}

/// Ends the attempt. On a failure the link is deleted (it was never handed to the connection manager);
/// on a launch finish( FALSE ) leaves the attached link alone.
static void finish( Bool failed )
{
	delete s_check;
	s_check = NULL;
	if( failed )
	{
		delete s_link;
		s_link = NULL;
		ServersModeClear();
	}
	s_phase = PHASE_DONE;
}

static Bool engineIsReady()
{
	return TheGlobalData != NULL && TheShell != NULL && TheShell->top() != NULL && TheShell->isAnimFinished()
		&& TheGameLogic != NULL && !TheGameLogic->isInGame() && TheNetwork == NULL
		&& TheMapCache != NULL && ThePlayerTemplateStore != NULL && TheGameText != NULL;
}

static Bool openLink()
{
	s_link = NEW ServerLink;

	if( s_params.host )
	{
		const UnsignedByte slotMask = (UnsignedByte)( ( 1 << s_params.players ) - 1 );
		if( !s_link->create( s_region, s_pass, s_params.slot, slotMask, SERVERS_DIRECT_LINK_TIMEOUT_MS ) )
		{
			DEBUG_LOG(( "Servers direct: CREATE could not be started" ));
			delete s_link;
			s_link = NULL;
			return FALSE;
		}
		DEBUG_LOG(( "Servers direct: CREATE sent, host slot %d, %d players", (Int)s_params.slot, (Int)s_params.players ));
		return TRUE;
	}

	if( !s_link->join( s_region, s_pass, s_params.session, s_params.slot, s_params.token, SERVERS_DIRECT_LINK_TIMEOUT_MS ) )
	{
		DEBUG_LOG(( "Servers direct: JOIN could not be started" ));
		delete s_link;
		s_link = NULL;
		return FALSE;
	}
	DEBUG_LOG(( "Servers direct: JOIN sent, slot %d", (Int)s_params.slot ));
	return TRUE;
}

/// The host writes the session id and each join token to the log and to the match file.
static void reportJoinTokens()
{
	FILE *file = std::fopen( SERVERS_DIRECT_MATCH_FILE, "w" );

	for( Int i = 0; i < s_params.players; ++i )
	{
		if( i == s_params.slot )
			continue;

		const UnsignedInt64 token = s_joinTokens[i];
		if( token == 0 )
			continue;

		DEBUG_LOG(( "Servers direct: slot %d joins with -serversJoin -serversSlot %d -serversSession %016llx -serversToken %016llx",
			i, i, (unsigned long long)s_sessionId, (unsigned long long)token ));
		if( file != NULL )
			std::fprintf( file, "-serversJoin -serversSlot %d -serversSession %016llx -serversToken %016llx\n",
				i, (unsigned long long)s_sessionId, (unsigned long long)token );
	}

	if( file != NULL )
		std::fclose( file );
}

/// Builds the game from the flags, installs it as TheNGMPGame and launches it on the relay link. The link
/// is kept in server mode by the connection manager from here on, so this does not poll it again.
static Bool launchMatch()
{
	if( TheNGMPGame != NULL || s_game != NULL )
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
	TheNGMPGame = s_game;

	s_game->launchServersDirect( s_link );
	return TRUE;
}

static void launchOrFail()
{
	if( launchMatch() )
	{
		finish( FALSE );	// The link is now attached to the connection manager; keep it.
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
			s_region = s_check->getRegion();
			s_pass = s_check->getPass();
			ServersModeSet( s_region, s_pass );
			DEBUG_LOG(( "Servers direct: entry check passed, ping %d ms", (Int)s_pass.rttMs ));
			delete s_check;
			s_check = NULL;
			if( openLink() )
			{
				s_phase = PHASE_HANDSHAKE;
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

static void stepHandshake()
{
	s_link->update();

	ServerLinkEvent event;
	while( s_link->nextEvent( event ) )
	{
		switch( event.type )
		{
			case ServerLinkEvent::TYPE_CREATED:
				// Host only. The remaining events (WELCOME, ROSTER, START) stay queued for the connection manager.
				if( !s_params.host )
					break;
				s_sessionId = event.sessionId;
				for( Int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i )
					s_joinTokens[i] = event.joinTokens[i];
				reportJoinTokens();
				launchOrFail();
				return;

			case ServerLinkEvent::TYPE_WELCOME:
				// Joiner only. The host already launched on CREATED and left its WELCOME for the connection manager.
				if( !s_params.join )
					break;
				launchOrFail();
				return;

			case ServerLinkEvent::TYPE_REFUSE:
				DEBUG_LOG(( "Servers direct: the relay refused the match, code %d", (Int)event.refuseCode ));
				showError( ServersModeRefusalText( event.refuseCode, event.rttMs, event.limitMs ) );
				finish( TRUE );
				return;

			case ServerLinkEvent::TYPE_LOST:
				DEBUG_LOG(( "Servers direct: the relay connection was lost before the match started" ));
				showError( ServersModeRefusalText( SERVER_REFUSE_NO_ANSWER, 0, 0 ) );
				finish( TRUE );
				return;

			case ServerLinkEvent::TYPE_DROP:
				DEBUG_LOG(( "Servers direct: dropped before the match started, slot %d reason %d", (Int)event.slot, (Int)event.dropReason ));
				showStartFailed();
				finish( TRUE );
				return;

			default:
				// ROSTER, START, DATA, NONE: nothing to do while still waiting to launch.
				break;
		}
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
			if( s_check != NULL )
			{
				delete s_check;
				s_check = NULL;
			}
			s_check = NEW ServerEntryCheck;
			{
				ServerRegion region;
				std::memset( &region, 0, sizeof( region ) );
				region.number = SERVERS_DIRECT_REGION_NUMBER;
				strlcpy( region.name, "Direct", ARRAY_SIZE( region.name ) );
				strlcpy( region.relayHost, s_params.relay, ARRAY_SIZE( region.relayHost ) );
				region.relayPort = s_params.port;

				ServersModeClear();
				if( !s_check->startRegion( region, TheGlobalData->m_exeCRC, TheGlobalData->m_iniCRC ) )
				{
					DEBUG_LOG(( "Servers direct: the entry check could not start" ));
					showStartFailed();
					finish( TRUE );
					return;
				}
			}
			s_phase = PHASE_ENTRY;
			return;

		case PHASE_ENTRY:
			stepEntryCheck();
			return;

		case PHASE_HANDSHAKE:
			stepHandshake();
			return;
	}
}

#endif // RTS_SERVERS
