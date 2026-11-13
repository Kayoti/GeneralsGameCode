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

// FILE: ServersMode.cpp //////////////////////////////////////////////////////
// The state of Servers mode in the online lobby, and the entry check that the
// Servers button runs.

// TheSuperHackers @feature Kayoti 13/11/2026 Adds the Servers mode state and the entry check run by the Servers button.

#include "PreRTS.h"	// This must go first in EVERY cpp file in the GameEngine

#ifdef RTS_SERVERS

#include <cstdlib>

#include "Common/GlobalData.h"
#include "GameClient/GameText.h"
#include "GameClient/GameWindowManager.h"
#include "GameClient/MessageBox.h"
#include "GameNetwork/GameSpy/BuddyThread.h"
#include "GameNetwork/GameSpy/PeerDefs.h"
#include "GameNetwork/GameSpy/PeerThread.h"
#include "GameNetwork/GameSpy/ServersMode.h"
#include "GameNetwork/GameSpyOverlay.h"
#include "GameNetwork/ServerLink.h"

// The region file is read from the working directory of the game.
static const char *const SERVERS_REGION_FILE = "regions.txt";

// The check run on player actions. The main menu drives the entry check below, so this one is kept apart.
static ServerEntryCheck *s_gateCheck = nullptr;
static GameWindow *s_gateWindow = nullptr;
static ServersGate s_gateAction = SERVERS_GATE_NONE;
static Bool s_gateCancelled = FALSE;
static Bool s_tearDownPending = FALSE;	// ServersLogoutOnline() has been called and the session is not yet torn down

// The match-start handshake (2.1). The relay link it owns is lent to the networking code at match launch.
static const UnsignedInt SERVERS_MATCH_TIMEOUT_MS = 15 * 1000;	// Time the relay has to answer CREATE or JOIN with WELCOME.
static ServerLink *s_matchLink = nullptr;
static GameWindow *s_matchWindow = nullptr;
static Bool s_matchIsHost = FALSE;
static Bool s_matchCreatedSeen = FALSE;				// Host: the CREATED event has been read
static Bool s_matchResultGiven = FALSE;				// HOST_READY or CONNECTED has already been returned once
static Bool s_matchCancelled = FALSE;
static UnsignedInt64 s_matchSessionId = 0;
static UnsignedInt64 s_matchTokens[ServerLinkEvent::MAX_SLOTS] = { 0 };

// The private start message this member was given (PROTOCOL.md 5.4).
static Bool s_privateHave = FALSE;
static UnsignedInt64 s_privateSessionId = 0;
static UnsignedByte s_privateSlot = 0;
static UnsignedInt64 s_privateToken = 0;

static void closeGateWindow()
{
	if( s_gateWindow )
	{
		TheWindowManager->winDestroy( s_gateWindow );
		s_gateWindow = nullptr;
	}
}

static void releaseGate()
{
	closeGateWindow();
	delete s_gateCheck;
	s_gateCheck = nullptr;
	s_gateAction = SERVERS_GATE_NONE;
	s_gateCancelled = FALSE;
}

// State ------------------------------------------------------------------------------------------

struct ServersModeState
{
	ServersModeState() : on( FALSE ) { memset( &region, 0, sizeof( region ) ); }

	Bool on;
	ServerRegion region;
	ServerPass pass;
};

static ServersModeState s_state;

Bool ServersModeIsOn()
{
	return s_state.on;
}

const ServerRegion &ServersModeGetRegion()
{
	return s_state.region;
}

const ServerPass &ServersModeGetPass()
{
	return s_state.pass;
}

void ServersModeSet( const ServerRegion &region, const ServerPass &pass )
{
	s_state.region = region;
	s_state.pass = pass;
	s_state.on = TRUE;
}

void ServersModeClear()
{
	releaseGate();
	ServersMatchAbort();
	ServersMatchForgetPrivate();
	s_tearDownPending = FALSE;
	s_state.on = FALSE;
	memset( &s_state.region, 0, sizeof( s_state.region ) );
	s_state.pass = ServerPass();
}

UnicodeString ServersModeRefusalText( UnsignedByte code, UnsignedShort rttMs, UnsignedShort limitMs )
{
	switch( code )
	{
		case SERVER_REFUSE_VPN_DETECTED:
			return TheGameText->fetch( "GUI:ServersVpnDetected" );
		case SERVER_REFUSE_PING_TOO_HIGH:
			return TheGameText->fetchFormat( "GUI:ServersPingTooHigh", (Int)rttMs, (Int)limitMs );
		default:
			// Includes SERVER_REFUSE_NO_ANSWER, which is not a relay code
			return TheGameText->fetch( "GUI:ServersCannotConnect" );
	}
}

// Lobby tag --------------------------------------------------------------------------------------

// The region file as the lobby list filter sees it. It is read once, on first use.
static ServerRegions *s_tagRegions = nullptr;
static Bool s_tagRegionsLoaded = FALSE;

static const ServerRegions *getTagRegions()
{
	if( !s_tagRegionsLoaded )
	{
		s_tagRegionsLoaded = TRUE;
		s_tagRegions = NEW ServerRegions;
		if( !s_tagRegions->load( SERVERS_REGION_FILE ) )
		{
			DEBUG_LOG(( "Servers: no lobby tags from %s: %s", SERVERS_REGION_FILE, s_tagRegions->getError() ));
		}
	}
	return s_tagRegions;
}

Bool ServersModeIsAnyTag( const AsciiString &ladderIP, UnsignedShort ladderPort )
{
	if( ladderPort == 0 )
		return FALSE;

	const ServerRegions *regions = getTagRegions();
	return regions != nullptr && regions->findByTag( ladderIP.str(), ladderPort ) != nullptr;
}

Bool ServersModeIsOwnTag( const AsciiString &ladderIP, UnsignedShort ladderPort )
{
	return s_state.on && ladderPort != 0
		&& ladderPort == s_state.region.tagPort
		&& ladderIP == s_state.region.tag;
}

Bool ServersModeShowsLobby( const AsciiString &ladderIP, UnsignedShort ladderPort )
{
	if( s_state.on )
		return ServersModeIsOwnTag( ladderIP, ladderPort );

	return !ServersModeIsAnyTag( ladderIP, ladderPort );
}

// Entry check ------------------------------------------------------------------------------------

static ServerRegions *s_regions = nullptr;
static ServerEntryCheck *s_check = nullptr;
static GameWindow *s_checkingWindow = nullptr;

static void releaseCheck()
{
	delete s_check;
	s_check = nullptr;
	delete s_regions;
	s_regions = nullptr;
}

static void closeCheckingWindow()
{
	if( s_checkingWindow )
	{
		TheWindowManager->winDestroy( s_checkingWindow );
		s_checkingWindow = nullptr;
	}
}

// The message box destroys its window after this returns
static void checkCancelledCallback()
{
	s_checkingWindow = nullptr;
	releaseCheck();
}

static void showRefusal( UnsignedByte code, UnsignedShort rttMs, UnsignedShort limitMs )
{
	MessageBoxOk( TheGameText->fetch( "GUI:ServersTitle" ), ServersModeRefusalText( code, rttMs, limitMs ), nullptr );
}

void ServersEntryCancel()
{
	closeCheckingWindow();
	releaseCheck();
}

Bool ServersEntryStart()
{
	ServersEntryCancel();
	ServersModeClear();

	s_regions = NEW ServerRegions;
	if( !s_regions->load( SERVERS_REGION_FILE ) )
	{
		DEBUG_LOG(( "Servers: cannot use %s: %s", SERVERS_REGION_FILE, s_regions->getError() ));
		releaseCheck();
		showRefusal( SERVER_REFUSE_NO_ANSWER, 0, 0 );
		return FALSE;
	}

	s_check = NEW ServerEntryCheck;
	if( !s_check->start( *s_regions, TheGlobalData->m_exeCRC, TheGlobalData->m_iniCRC ) )
	{
		DEBUG_LOG(( "Servers: %s lists no region", SERVERS_REGION_FILE ));
		releaseCheck();
		showRefusal( SERVER_REFUSE_NO_ANSWER, 0, 0 );
		return FALSE;
	}

	s_checkingWindow = MessageBoxCancel( TheGameText->fetch( "GUI:ServersChecking" ),
		TheGameText->fetch( "GUI:ServersChecking" ), checkCancelledCallback );
	return TRUE;
}

ServersEntryResult ServersEntryUpdate()
{
	if( s_check == nullptr )
		return SERVERS_ENTRY_IDLE;

	s_check->update();

	switch( s_check->getState() )
	{
		case ServerEntryCheck::STATE_RUNNING:
			return SERVERS_ENTRY_CHECKING;

		case ServerEntryCheck::STATE_PASSED:
			ServersModeSet( s_check->getRegion(), s_check->getPass() );
			DEBUG_LOG(( "Servers: entry check passed, region %d, ping %d ms",
				(Int)s_check->getRegion().number, (Int)s_check->getPass().rttMs ));
			closeCheckingWindow();
			releaseCheck();
			return SERVERS_ENTRY_PASSED;

		case ServerEntryCheck::STATE_REFUSED:
		{
			const UnsignedByte code = s_check->getRefuseCode();
			const UnsignedShort rttMs = s_check->getRttMs();
			const UnsignedShort limitMs = s_check->getLimitMs();
			DEBUG_LOG(( "Servers: entry check refused, code %d", (Int)code ));
			closeCheckingWindow();
			releaseCheck();
			showRefusal( code, rttMs, limitMs );
			return SERVERS_ENTRY_REFUSED;
		}

		default:
			// Idle again after a start: the check has no result, so treat it as no answer
			closeCheckingWindow();
			releaseCheck();
			showRefusal( SERVER_REFUSE_NO_ANSWER, 0, 0 );
			return SERVERS_ENTRY_REFUSED;
	}
}

// Entry check on player actions -----------------------------------------------------------------

// The message box destroys its window after this returns
static void gateCancelledCallback()
{
	s_gateWindow = nullptr;
	delete s_gateCheck;
	s_gateCheck = nullptr;
	s_gateCancelled = TRUE;
}

Bool ServersGateStart( ServersGate action )
{
	releaseGate();

	if( !s_state.on )
		return FALSE;

	s_gateCheck = NEW ServerEntryCheck;
	if( !s_gateCheck->startRegion( s_state.region, TheGlobalData->m_exeCRC, TheGlobalData->m_iniCRC ) )
	{
		DEBUG_LOG(( "Servers: cannot start the entry check for action %d", (Int)action ));
		releaseGate();
		showRefusal( SERVER_REFUSE_NO_ANSWER, 0, 0 );
		return FALSE;
	}

	s_gateAction = action;
	s_gateWindow = MessageBoxCancel( TheGameText->fetch( "GUI:ServersChecking" ),
		TheGameText->fetch( "GUI:ServersChecking" ), gateCancelledCallback );
	return TRUE;
}

Bool ServersGateIsPending()
{
	return s_gateAction != SERVERS_GATE_NONE;
}

ServersGateResult ServersGateUpdate( UnsignedInt handled, ServersGate *action )
{
	*action = s_gateAction;
	if( s_gateAction == SERVERS_GATE_NONE )
		return SERVERS_GATE_IDLE;

	if( s_gateCancelled )
	{
		releaseGate();
		return SERVERS_GATE_CANCELLED;
	}

	s_gateCheck->update();

	switch( s_gateCheck->getState() )
	{
		case ServerEntryCheck::STATE_RUNNING:
			return SERVERS_GATE_CHECKING;

		case ServerEntryCheck::STATE_PASSED:
			if( ( handled & s_gateAction ) == 0 )
				return SERVERS_GATE_IDLE;
			ServersModeSet( s_gateCheck->getRegion(), s_gateCheck->getPass() );
			DEBUG_LOG(( "Servers: entry check for action %d passed, ping %d ms",
				(Int)s_gateAction, (Int)s_gateCheck->getPass().rttMs ));
			releaseGate();
			return SERVERS_GATE_PASSED;

		case ServerEntryCheck::STATE_REFUSED:
		{
			if( ( handled & s_gateAction ) == 0 )
				return SERVERS_GATE_IDLE;
			const UnsignedByte code = s_gateCheck->getRefuseCode();
			const UnsignedShort rttMs = s_gateCheck->getRttMs();
			const UnsignedShort limitMs = s_gateCheck->getLimitMs();
			DEBUG_LOG(( "Servers: entry check for action %d refused, code %d", (Int)s_gateAction, (Int)code ));
			releaseGate();
			showRefusal( code, rttMs, limitMs );
			return SERVERS_GATE_REFUSED;
		}

		default:
			// Idle again after a start: the check has no result, so it is dropped
			releaseGate();
			return SERVERS_GATE_CANCELLED;
	}
}

void ServersLogoutOnline()
{
	GameSpyCloseAllOverlays();
	s_tearDownPending = TRUE;

	if( TheGameSpyPeerMessageQueue )
	{
		PeerRequest req;
		req.peerRequestType = PeerRequest::PEERREQUEST_LOGOUT;
		TheGameSpyPeerMessageQueue->addRequest( req );
	}
	if( TheGameSpyBuddyMessageQueue )
	{
		BuddyRequest breq;
		breq.buddyRequestType = BuddyRequest::BUDDYREQUEST_LOGOUT;
		TheGameSpyBuddyMessageQueue->addRequest( breq );
	}
}

void ServersTearDownOnline()
{
	if( !s_tearDownPending )
		return;

	s_tearDownPending = FALSE;
	TearDownGameSpy();
}

// Match start ------------------------------------------------------------------------------------

static void closeMatchWindow()
{
	if( s_matchWindow )
	{
		TheWindowManager->winDestroy( s_matchWindow );
		s_matchWindow = nullptr;
	}
}

// The message box destroys its window after this returns
static void matchCancelledCallback()
{
	s_matchWindow = nullptr;
	s_matchCancelled = TRUE;
}

static void showContacting()
{
	s_matchWindow = MessageBoxCancel( TheGameText->fetch( "GUI:ServersTitle" ),
		TheGameText->fetch( "GUI:ServersContacting" ), matchCancelledCallback );
}

void ServersMatchAbort()
{
	closeMatchWindow();
	if( s_matchLink != nullptr )
	{
		s_matchLink->close();
		delete s_matchLink;
		s_matchLink = nullptr;
	}
	s_matchIsHost = FALSE;
	s_matchCreatedSeen = FALSE;
	s_matchResultGiven = FALSE;
	s_matchCancelled = FALSE;
	s_matchSessionId = 0;
	for( Int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i )
		s_matchTokens[i] = 0;
}

Bool ServersMatchHostBegin( UnsignedByte hostSlot, UnsignedByte slotMask )
{
	ServersMatchAbort();

	if( !s_state.on )
		return FALSE;
	if( hostSlot >= ServerLinkEvent::MAX_SLOTS )
		return FALSE;
	// slotMask must include the host slot and at least one other slot (PROTOCOL.md 4.5).
	if( ( slotMask & ( 1 << hostSlot ) ) == 0 )
		return FALSE;
	if( ( slotMask & ~( 1 << hostSlot ) ) == 0 )
		return FALSE;

	s_matchLink = NEW ServerLink;
	if( !s_matchLink->create( s_state.region, s_state.pass, hostSlot, slotMask, SERVERS_MATCH_TIMEOUT_MS ) )
	{
		delete s_matchLink;
		s_matchLink = nullptr;
		return FALSE;
	}

	s_matchIsHost = TRUE;
	showContacting();
	return TRUE;
}

Bool ServersMatchJoinBegin()
{
	ServersMatchAbort();

	if( !s_state.on )
		return FALSE;
	if( !s_privateHave )
		return FALSE;

	s_matchLink = NEW ServerLink;
	if( !s_matchLink->join( s_state.region, s_state.pass, s_privateSessionId, s_privateSlot, s_privateToken, SERVERS_MATCH_TIMEOUT_MS ) )
	{
		delete s_matchLink;
		s_matchLink = nullptr;
		return FALSE;
	}

	s_matchIsHost = FALSE;
	showContacting();
	return TRUE;
}

Bool ServersMatchStorePrivate( const char *payload )
{
	if( payload == nullptr )
		return FALSE;

	// Parse "<session_id as 16 hex>/<slot as 1 digit>/<join_token as 16 hex>" (PROTOCOL.md 5.4).
	char *end = nullptr;
	const char *cursor = payload;

	UnsignedInt64 sessionId = (UnsignedInt64)std::strtoull( cursor, &end, 16 );
	if( end == cursor || *end != '/' )
		return FALSE;

	cursor = end + 1;
	unsigned long slot = std::strtoul( cursor, &end, 10 );
	if( end == cursor || *end != '/' || slot >= ServerLinkEvent::MAX_SLOTS )
		return FALSE;

	cursor = end + 1;
	UnsignedInt64 token = (UnsignedInt64)std::strtoull( cursor, &end, 16 );
	if( end == cursor || *end != '\0' )
		return FALSE;

	s_privateSessionId = sessionId;
	s_privateSlot = (UnsignedByte)slot;
	s_privateToken = token;
	s_privateHave = TRUE;
	return TRUE;
}

Bool ServersMatchHasPrivate()
{
	return s_privateHave;
}

void ServersMatchForgetPrivate()
{
	s_privateHave = FALSE;
	s_privateSessionId = 0;
	s_privateSlot = 0;
	s_privateToken = 0;
}

Bool ServersMatchBuildPrivate( UnsignedByte slot, AsciiString &outPayload )
{
	if( !s_matchCreatedSeen )
		return FALSE;
	if( slot >= ServerLinkEvent::MAX_SLOTS )
		return FALSE;
	// A zero token means the relay expects no join for that slot (the host slot and unexpected slots).
	if( s_matchTokens[slot] == 0 )
		return FALSE;

	outPayload.format( "%016I64x/%u/%016I64x", s_matchSessionId, (UnsignedInt)slot, s_matchTokens[slot] );
	return TRUE;
}

ServersMatchResult ServersMatchUpdate()
{
	if( s_matchLink == nullptr )
		return SERVERS_MATCH_IDLE;

	if( s_matchCancelled )
	{
		ServersMatchAbort();
		return SERVERS_MATCH_CANCELLED;
	}

	s_matchLink->update();

	// Host only: read the CREATED event (it arrives before WELCOME) so the private messages can be built.
	// Stop at CREATED so WELCOME, ROSTER and START stay queued for the networking code once the link is lent out.
	if( s_matchIsHost && !s_matchCreatedSeen )
	{
		ServerLinkEvent event;
		while( s_matchLink->nextEvent( event ) )
		{
			if( event.type == ServerLinkEvent::TYPE_CREATED )
			{
				s_matchSessionId = event.sessionId;
				for( Int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i )
					s_matchTokens[i] = event.joinTokens[i];
				s_matchCreatedSeen = TRUE;
				break;
			}
			if( event.type == ServerLinkEvent::TYPE_REFUSE || event.type == ServerLinkEvent::TYPE_LOST )
				break;
		}
	}

	switch( s_matchLink->getState() )
	{
		case ServerLink::STATE_CONNECTING:
			return SERVERS_MATCH_CONTACTING;

		case ServerLink::STATE_CONNECTED:
			if( s_matchResultGiven )
				return SERVERS_MATCH_CONTACTING;	// already reported; waiting for the caller to launch
			if( s_matchIsHost && !s_matchCreatedSeen )
				return SERVERS_MATCH_CONTACTING;	// hold until CREATED's join tokens have been read
			closeMatchWindow();
			s_matchResultGiven = TRUE;
			return s_matchIsHost ? SERVERS_MATCH_HOST_READY : SERVERS_MATCH_CONNECTED;

		default:
			// STATE_CLOSED, STATE_LEAVING, STATE_IDLE: the handshake ended before it connected.
			ServersMatchAbort();
			return SERVERS_MATCH_FAILED;
	}
}

ServerLink *ServersMatchLink()
{
	return s_matchLink;
}

#endif // RTS_SERVERS
