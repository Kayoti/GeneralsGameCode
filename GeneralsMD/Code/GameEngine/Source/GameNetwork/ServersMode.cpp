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
// The state of Servers mode, and the entry check that the Servers button runs.

// TheSuperHackers @feature Kayoti 15/11/2026 Adds the Servers mode state and the entry check run by the Servers button.

#include "PreRTS.h"	// This must go first in EVERY cpp file in the GameEngine

#ifdef RTS_SERVERS

#include <algorithm>
#include <cerrno>
#include <cstdlib>
#include <cstring>
#include <vector>

#include "Common/GlobalData.h"
#include "GameClient/GameText.h"
#include "GameClient/GameWindowManager.h"
#include "GameClient/MessageBox.h"
#include "GameNetwork/GameSpy/PeerDefs.h"
#include "GameNetwork/GameSpyOverlay.h"
#include "GameNetwork/GeneralsOnline/NGMP_interfaces.h"
#include "GameNetwork/ServerLink.h"
#include "GameNetwork/ServersMode.h"

// The region file is read from the working directory of the game.
static const char *const SERVERS_REGION_FILE = "regions.txt";

// The check run on player actions. The main menu drives the entry check further down, so this one is kept apart.
static ServerEntryCheck *s_gateCheck = nullptr;
static GameWindow *s_gateWindow = nullptr;
static ServersGate s_gateAction = SERVERS_GATE_NONE;
static Bool s_gateCancelled = FALSE;

static void releaseGate()
{
	if( s_gateWindow )
	{
		TheWindowManager->winDestroy( s_gateWindow );
		s_gateWindow = nullptr;
	}
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
	s_state.on = FALSE;
	memset( &s_state.region, 0, sizeof( s_state.region ) );
	s_state.pass = ServerPass();
}

const char *ServersModeGetRegionTag()
{
	return s_state.on ? s_state.region.tag : "";
}

AsciiString ServersModeGetPassHex()
{
	AsciiString text;
	if( !s_state.on || !s_state.pass.isValid() )
		return text;

	static const char digits[] = "0123456789abcdef";
	char buffer[ServerPass::SIZE * 2 + 1];
	for( Int i = 0; i < ServerPass::SIZE; ++i )
	{
		buffer[i * 2] = digits[ s_state.pass.bytes[i] >> 4 ];
		buffer[i * 2 + 1] = digits[ s_state.pass.bytes[i] & 0x0F ];
	}
	buffer[ServerPass::SIZE * 2] = '\0';
	text.set( buffer );
	return text;
}

UnicodeString ServersModeRefusalText( UnsignedByte code, UnsignedShort rttMs, UnsignedShort limitMs )
{
	switch( code )
	{
		case SERVER_REFUSE_VPN_DETECTED:
			return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersVpnDetected",
				L"VPN detected. Disable your VPN to play on Servers." );
		case SERVER_REFUSE_PING_TOO_HIGH:
			return TheGameText->FETCH_OR_SUBSTITUTE_FORMAT( "GUI:ServersPingTooHigh",
				L"Your connection to the server is too slow (%d ms, limit %d ms).", (Int)rttMs, (Int)limitMs );
		default:
			// Includes SERVER_REFUSE_NO_ANSWER, which is not a relay code
			return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersCannotConnect", L"The server cannot be reached." );
	}
}

// Lobby calls ------------------------------------------------------------------------------------

UnicodeString ServersCheckExpiredText()
{
	return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersCheckExpired",
		L"Your server check has expired. Return to the main menu and enter Servers again." );
}

void ServersShowCheckExpired()
{
	SetLobbyAttemptHostJoin( FALSE );
	GSMessageBoxOk( TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersTitle", L"Servers" ), ServersCheckExpiredText() );
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
	MessageBoxOk( TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersTitle", L"Servers" ),
		ServersModeRefusalText( code, rttMs, limitMs ), nullptr );
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

	s_checkingWindow = MessageBoxCancel( TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersTitle", L"Servers" ),
		TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersChecking", L"Checking connection" ), checkCancelledCallback );
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
	s_gateWindow = MessageBoxCancel( TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersTitle", L"Servers" ),
		TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersChecking", L"Checking connection" ), gateCancelledCallback );
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
	NGMP_OnlineServicesManager *manager = NGMP_OnlineServicesManager::GetInstance();
	if( manager )
		manager->SetPendingFullTeardown( EGOTearDownReason::USER_REQUESTED_SILENT );

	ServersModeClear();
}

// Match start from a server lobby ---------------------------------------------------------------
// PROTOCOL.md 5.4 and SERVICE.md section 10. One relay link per match, owned here until the launch
// takes it (ServersMatchTakeLink) or a failure closes it. Only the relay handshake, the service calls
// and the timeouts live here; the staging menu owns the buttons, the status lines and the launch.

// Timeouts, from PROTOCOL.md 5.4.
static const UnsignedInt SERVERS_MATCH_CREATE_TIMEOUT_MS = 10000;		///< step 4: CREATE must answer with WELCOME within 10 s
static const UnsignedInt SERVERS_MATCH_FIELD19_TIMEOUT_MS = 5000;		///< step 5: the service must answer field 19 within 5 s
static const UnsignedInt SERVERS_MATCH_JOIN_TIMEOUT_MS = 45000;			///< step 10: a member waits up to 45 s to hold its slot

enum ServersMatchPhase
{
	MATCH_PHASE_IDLE,
	MATCH_PHASE_HOST_CHECK,			///< renewing the pass before CREATE
	MATCH_PHASE_HOST_CREATE,		///< CREATE sent, waiting for CREATED and WELCOME
	MATCH_PHASE_HOST_FIELD19,		///< tokens posted to the service, waiting for the result
	MATCH_PHASE_MEMBER_CHECK,		///< renewing the pass before JOIN
	MATCH_PHASE_MEMBER_JOIN,		///< JOIN sent, waiting for WELCOME
	MATCH_PHASE_DONE,						///< a terminal outcome is waiting to be returned once
};

// field 19 result, set by the asynchronous POST callback.
enum ServersField19Result
{
	FIELD19_PENDING,
	FIELD19_STORED,
	FIELD19_REFUSED,
};

struct ServersMatchState
{
	ServersMatchPhase phase;
	ServersMatchOutcome pendingOutcome;			///< delivered once when phase == MATCH_PHASE_DONE
	Bool isHost;
	ServerLink *link;												///< owned here until taken by the launch
	ServerEntryCheck *check;								///< the silent renewal check, owned here
	UnsignedInt phaseStartedMs;							///< ServerClockMs() when the current timed phase began
	UnicodeString message;									///< shown for ABORTED or FAILED

	// Host.
	UnsignedInt64 sessionId;
	UnsignedInt64 joinTokens[ServerLinkEvent::MAX_SLOTS];
	UnsignedByte hostSlot;
	UnsignedByte slotMask;
	std::vector<Int> rosterSnapshot;				///< sorted human user ids when Start was pressed
	ServersField19Result field19;

	// Member.
	UnsignedInt64 memberSession;
	UnsignedByte memberSlot;
	UnsignedInt64 memberToken;
};

static ServersMatchState s_match;

// Fields of a received start message, kept apart from s_match so ServersMatchReset() does not clear them
// between the message arriving and the member flow reading them.
static Bool s_startFieldsValid = FALSE;
static UnsignedInt64 s_startSession = 0;
static UnsignedByte s_startSlot = 0;
static UnsignedInt64 s_startToken = 0;

static UnicodeString couldNotJoinText()
{
	return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersCouldNotJoin", L"Could not join the match." );
}

static UnicodeString contactFailedText()
{
	return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersStartFailed", L"Could not start the match through the server." );
}

static UnicodeString rosterChangedText()
{
	return TheGameText->FETCH_OR_SUBSTITUTE( "GUI:ServersRosterChanged",
		L"A player joined or left the lobby, so the match was not started." );
}

static void formatHex64( UnsignedInt64 value, AsciiString &out )
{
	out.format( "%016llx", (unsigned long long)value );
}

/// Reads up to 16 hexadecimal digits. Anything else, or trailing text, is refused. The value may be zero.
static Bool parseHex64( const char *text, UnsignedInt64 &out )
{
	if( text == NULL || text[0] == '\0' )
		return FALSE;
	for( const char *c = text; *c != '\0'; ++c )
	{
		const Bool ok = ( *c >= '0' && *c <= '9' ) || ( *c >= 'a' && *c <= 'f' ) || ( *c >= 'A' && *c <= 'F' );
		if( !ok )
			return FALSE;
	}

	char *end = NULL;
	errno = 0;
	const unsigned long long value = std::strtoull( text, &end, 16 );
	if( end == text || *end != '\0' || errno == ERANGE )
		return FALSE;

	out = (UnsignedInt64)value;
	return TRUE;
}

Bool ServersMatchIsActive()
{
	return s_match.phase != MATCH_PHASE_IDLE;
}

void ServersMatchReset()
{
	if( s_match.link != NULL )
	{
		s_match.link->close();
		delete s_match.link;
		s_match.link = NULL;
	}
	delete s_match.check;
	s_match.check = NULL;

	s_match.phase = MATCH_PHASE_IDLE;
	s_match.pendingOutcome = SERVERS_MATCH_IDLE;
	s_match.isHost = FALSE;
	s_match.phaseStartedMs = 0;
	s_match.message = UnicodeString::TheEmptyString;
	s_match.sessionId = 0;
	for( Int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i )
		s_match.joinTokens[i] = 0;
	s_match.hostSlot = 0;
	s_match.slotMask = 0;
	s_match.rosterSnapshot.clear();
	s_match.field19 = FIELD19_PENDING;
	s_match.memberSession = 0;
	s_match.memberSlot = 0;
	s_match.memberToken = 0;
}

UnicodeString ServersMatchGetMessage()
{
	return s_match.message;
}

ServerLink *ServersMatchTakeLink()
{
	ServerLink *link = s_match.link;
	s_match.link = NULL;
	return link;
}

/// Ends the flow with its final outcome. A failure closes the link we still own; a launch leaves it for the caller.
static void matchTerminal( ServersMatchOutcome outcome, const UnicodeString &message )
{
	if( outcome != SERVERS_MATCH_LAUNCH && s_match.link != NULL )
	{
		s_match.link->close();
		delete s_match.link;
		s_match.link = NULL;
	}
	delete s_match.check;
	s_match.check = NULL;

	s_match.message = message;
	s_match.pendingOutcome = outcome;
	s_match.phase = MATCH_PHASE_DONE;
}

/// True when the set of human members differs from the snapshot taken when Start was pressed (PROTOCOL.md 5.4 step 9).
static Bool rosterChanged()
{
	NGMP_OnlineServices_LobbyInterface *lobby = NGMP_OnlineServicesManager::GetInterface<NGMP_OnlineServices_LobbyInterface>();
	if( lobby == NULL )
		return TRUE;

	std::vector<Int> current;
	for( const LobbyMemberEntry &member : lobby->GetCurrentLobby().members )
	{
		if( member.IsHuman() && member.m_SlotIndex < ServerLinkEvent::MAX_SLOTS )
			current.push_back( (Int)member.user_id );
	}
	std::sort( current.begin(), current.end() );
	return current != s_match.rosterSnapshot;
}

static Bool startSilentCheck()
{
	delete s_match.check;
	s_match.check = NEW ServerEntryCheck;
	if( !s_match.check->startRegion( ServersModeGetRegion(), TheGlobalData->m_exeCRC, TheGlobalData->m_iniCRC ) )
	{
		delete s_match.check;
		s_match.check = NULL;
		return FALSE;
	}
	return TRUE;
}

enum SilentResult { SILENT_RUNNING, SILENT_PASSED, SILENT_REFUSED };

/// Drives the silent renewal check. On SILENT_PASSED the pass is replaced; the caller then deletes the check.
static SilentResult stepSilentCheck()
{
	if( s_match.check == NULL )
		return SILENT_REFUSED;

	s_match.check->update();
	const ServerEntryCheck::State state = s_match.check->getState();
	if( state == ServerEntryCheck::STATE_RUNNING )
		return SILENT_RUNNING;
	if( state == ServerEntryCheck::STATE_PASSED )
	{
		ServersModeSet( s_match.check->getRegion(), s_match.check->getPass() );
		return SILENT_PASSED;
	}
	return SILENT_REFUSED;
}

static Bool openHostLink()
{
	s_match.link = NEW ServerLink;
	if( !s_match.link->create( ServersModeGetRegion(), ServersModeGetPass(), s_match.hostSlot, s_match.slotMask,
			SERVERS_MATCH_CREATE_TIMEOUT_MS ) )
	{
		delete s_match.link;
		s_match.link = NULL;
		return FALSE;
	}
	return TRUE;
}

static Bool openMemberLink()
{
	s_match.link = NEW ServerLink;
	if( !s_match.link->join( ServersModeGetRegion(), ServersModeGetPass(), s_match.memberSession, s_match.memberSlot,
			s_match.memberToken, SERVERS_MATCH_JOIN_TIMEOUT_MS ) )
	{
		delete s_match.link;
		s_match.link = NULL;
		return FALSE;
	}
	return TRUE;
}

/// Posts the session id and one join token per other human slot (SERVICE.md section 10, field 19).
static void postServerSession()
{
	s_match.field19 = FIELD19_PENDING;

	NGMP_OnlineServices_LobbyInterface *lobby = NGMP_OnlineServicesManager::GetInterface<NGMP_OnlineServices_LobbyInterface>();
	if( lobby == NULL )
	{
		s_match.field19 = FIELD19_REFUSED;
		return;
	}

	AsciiString sessionHex;
	formatHex64( s_match.sessionId, sessionHex );

	std::vector<Int> slots;
	std::vector<AsciiString> tokens;
	for( Int slot = 0; slot < ServerLinkEvent::MAX_SLOTS; ++slot )
	{
		if( slot == (Int)s_match.hostSlot )
			continue;
		if( ( s_match.slotMask & ( 1 << slot ) ) == 0 )
			continue;
		if( s_match.joinTokens[slot] == 0 )
		{
			// The relay gives a non-zero token for every expected slot; a zero one means we cannot form the session.
			s_match.field19 = FIELD19_REFUSED;
			return;
		}
		AsciiString tokenHex;
		formatHex64( s_match.joinTokens[slot], tokenHex );
		slots.push_back( slot );
		tokens.push_back( tokenHex );
	}

	lobby->SendServerSession( sessionHex, slots, tokens, []( bool bStored )
		{
			// A late answer for a flow that was already aborted is ignored.
			if( s_match.phase == MATCH_PHASE_HOST_FIELD19 )
				s_match.field19 = bStored ? FIELD19_STORED : FIELD19_REFUSED;
		} );
}

static void updateHostCheck()
{
	if( rosterChanged() )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, rosterChangedText() );
		return;
	}

	const SilentResult result = stepSilentCheck();
	if( result == SILENT_RUNNING )
		return;
	if( result == SILENT_REFUSED )
	{
		const UnicodeString msg = ServersModeRefusalText( s_match.check->getRefuseCode(),
			s_match.check->getRttMs(), s_match.check->getLimitMs() );
		matchTerminal( SERVERS_MATCH_ABORTED, msg );
		return;
	}

	delete s_match.check;
	s_match.check = NULL;
	if( !openHostLink() )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
		return;
	}
	s_match.phase = MATCH_PHASE_HOST_CREATE;
}

static void updateHostCreate()
{
	if( rosterChanged() )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, rosterChangedText() );
		return;
	}
	if( s_match.link == NULL )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
		return;
	}

	s_match.link->update();

	ServerLinkEvent event;
	while( s_match.link->nextEvent( event ) )
	{
		switch( event.type )
		{
			case ServerLinkEvent::TYPE_CREATED:
				s_match.sessionId = event.sessionId;
				for( Int i = 0; i < ServerLinkEvent::MAX_SLOTS; ++i )
					s_match.joinTokens[i] = event.joinTokens[i];
				break;	// keep polling for WELCOME

			case ServerLinkEvent::TYPE_WELCOME:
				// We hold our slot. Post the tokens, then stop polling so ROSTER and START stay queued for the
				// connection manager, which sets the started flag on START (PROTOCOL.md 4.5 step 9).
				postServerSession();
				s_match.phaseStartedMs = ServerClockMs();
				s_match.phase = MATCH_PHASE_HOST_FIELD19;
				return;

			case ServerLinkEvent::TYPE_REFUSE:
				matchTerminal( SERVERS_MATCH_ABORTED,
					ServersModeRefusalText( event.refuseCode, event.rttMs, event.limitMs ) );
				return;

			case ServerLinkEvent::TYPE_LOST:
			case ServerLinkEvent::TYPE_DROP:
				matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
				return;

			default:
				break;	// ROSTER/START/DATA do not arrive before WELCOME
		}
	}
}

static void updateHostField19()
{
	if( rosterChanged() )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, rosterChangedText() );
		return;
	}
	if( s_match.link != NULL )
		s_match.link->update();	// keep the relay connection alive; leave the queued events for the launch

	switch( s_match.field19 )
	{
		case FIELD19_PENDING:
			if( ServerClockMs() - s_match.phaseStartedMs >= SERVERS_MATCH_FIELD19_TIMEOUT_MS )
				matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
			return;

		case FIELD19_REFUSED:
			matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
			return;

		case FIELD19_STORED:
		{
			std::shared_ptr<WebSocket> ws = NGMP_OnlineServicesManager::GetWebSocket();
			if( !ws )
			{
				matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
				return;
			}
			// The service has the tokens: send the start message and launch. The relay sends START once every
			// member has joined, and the connection manager begins the match on it.
			ws->SendData_StartGame();
			matchTerminal( SERVERS_MATCH_LAUNCH, UnicodeString::TheEmptyString );
			return;
		}
	}
}

static void updateMemberCheck()
{
	const SilentResult result = stepSilentCheck();
	if( result == SILENT_RUNNING )
		return;
	if( result == SILENT_REFUSED )
	{
		matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
		return;
	}

	delete s_match.check;
	s_match.check = NULL;
	if( !openMemberLink() )
	{
		matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
		return;
	}
	s_match.phase = MATCH_PHASE_MEMBER_JOIN;
}

static void updateMemberJoin()
{
	if( s_match.link == NULL )
	{
		matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
		return;
	}

	s_match.link->update();

	ServerLinkEvent event;
	while( s_match.link->nextEvent( event ) )
	{
		switch( event.type )
		{
			case ServerLinkEvent::TYPE_WELCOME:
				// We hold our slot. Leave ROSTER and START queued for the connection manager and launch.
				matchTerminal( SERVERS_MATCH_LAUNCH, UnicodeString::TheEmptyString );
				return;

			case ServerLinkEvent::TYPE_REFUSE:
			case ServerLinkEvent::TYPE_LOST:
			case ServerLinkEvent::TYPE_DROP:
				matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
				return;

			default:
				break;
		}
	}
}

static void matchStep()
{
	switch( s_match.phase )
	{
		case MATCH_PHASE_HOST_CHECK:		updateHostCheck();		break;
		case MATCH_PHASE_HOST_CREATE:		updateHostCreate();		break;
		case MATCH_PHASE_HOST_FIELD19:	updateHostField19();	break;
		case MATCH_PHASE_MEMBER_CHECK:	updateMemberCheck();	break;
		case MATCH_PHASE_MEMBER_JOIN:		updateMemberJoin();		break;
		default:												break;
	}
}

ServersMatchOutcome ServersMatchUpdate()
{
	if( s_match.phase == MATCH_PHASE_IDLE )
		return SERVERS_MATCH_IDLE;

	if( s_match.phase != MATCH_PHASE_DONE )
		matchStep();

	if( s_match.phase == MATCH_PHASE_DONE )
	{
		const ServersMatchOutcome outcome = s_match.pendingOutcome;
		s_match.pendingOutcome = SERVERS_MATCH_IDLE;
		s_match.phase = MATCH_PHASE_IDLE;
		return outcome;
	}
	return SERVERS_MATCH_IDLE;
}

void ServersMatchBeginHost()
{
	if( ServersMatchIsActive() )
		return;
	ServersMatchReset();

	NGMP_OnlineServices_LobbyInterface *lobby = NGMP_OnlineServicesManager::GetInterface<NGMP_OnlineServices_LobbyInterface>();
	NGMP_OnlineServices_AuthInterface *auth = NGMP_OnlineServicesManager::GetInterface<NGMP_OnlineServices_AuthInterface>();
	if( lobby == NULL || auth == NULL )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
		return;
	}

	const int64_t myUserId = auth->GetUserID();
	const UnsignedByte NO_SLOT = 0xFF;
	s_match.hostSlot = NO_SLOT;
	s_match.slotMask = 0;
	s_match.rosterSnapshot.clear();
	Int humanCount = 0;
	for( const LobbyMemberEntry &member : lobby->GetCurrentLobby().members )
	{
		if( !member.IsHuman() || member.m_SlotIndex >= ServerLinkEvent::MAX_SLOTS )
			continue;
		++humanCount;
		s_match.slotMask |= (UnsignedByte)( 1 << member.m_SlotIndex );
		s_match.rosterSnapshot.push_back( (Int)member.user_id );
		if( member.user_id == myUserId )
			s_match.hostSlot = (UnsignedByte)member.m_SlotIndex;
	}
	std::sort( s_match.rosterSnapshot.begin(), s_match.rosterSnapshot.end() );

	// A server lobby needs at least two humans (PROTOCOL.md 5.4 step 1). Start guards this, but the roster
	// can change before the countdown ends.
	if( humanCount < 2 || s_match.hostSlot == NO_SLOT )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, rosterChangedText() );
		return;
	}

	s_match.isHost = TRUE;

	// Renew the pass first if it is close to expiry (PROTOCOL.md 4.4).
	if( ServersModeGetPass().needsRenewal() )
	{
		if( !startSilentCheck() )
		{
			matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
			return;
		}
		s_match.phase = MATCH_PHASE_HOST_CHECK;
		return;
	}

	if( !openHostLink() )
	{
		matchTerminal( SERVERS_MATCH_ABORTED, contactFailedText() );
		return;
	}
	s_match.phase = MATCH_PHASE_HOST_CREATE;
}

void ServersMatchSetStartFields( Bool present, const char *serverSessionHex, Int serverSlot, const char *serverTokenHex )
{
	s_startFieldsValid = FALSE;
	s_startSession = 0;
	s_startSlot = 0;
	s_startToken = 0;

	if( !present )
		return;

	UnsignedInt64 session = 0;
	UnsignedInt64 token = 0;
	if( !parseHex64( serverSessionHex, session ) || session == 0 )
		return;
	if( serverSlot < 0 || serverSlot >= ServerLinkEvent::MAX_SLOTS )
		return;
	if( !parseHex64( serverTokenHex, token ) || token == 0 )
		return;

	s_startSession = session;
	s_startSlot = (UnsignedByte)serverSlot;
	s_startToken = token;
	s_startFieldsValid = TRUE;
}

void ServersMatchBeginMemberFromStartMessage()
{
	if( ServersMatchIsActive() )
		return;
	ServersMatchReset();
	s_match.isHost = FALSE;

	// The start message must carry a valid session, slot and token for a server lobby (PROTOCOL.md 5.4 step 7).
	if( !s_startFieldsValid )
	{
		matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
		return;
	}

	s_match.memberSession = s_startSession;
	s_match.memberSlot = s_startSlot;
	s_match.memberToken = s_startToken;

	if( ServersModeGetPass().needsRenewal() )
	{
		if( !startSilentCheck() )
		{
			matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
			return;
		}
		s_match.phase = MATCH_PHASE_MEMBER_CHECK;
		return;
	}

	if( !openMemberLink() )
	{
		matchTerminal( SERVERS_MATCH_FAILED, couldNotJoinText() );
		return;
	}
	s_match.phase = MATCH_PHASE_MEMBER_JOIN;
}

#endif // RTS_SERVERS
