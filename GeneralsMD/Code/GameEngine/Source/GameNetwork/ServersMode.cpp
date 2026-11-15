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

#include <cstring>

#include "Common/GlobalData.h"
#include "GameClient/GameText.h"
#include "GameClient/GameWindowManager.h"
#include "GameClient/MessageBox.h"
#include "GameNetwork/GameSpy/PeerDefs.h"
#include "GameNetwork/GameSpyOverlay.h"
#include "GameNetwork/GeneralsOnline/NGMP_interfaces.h"
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

#endif // RTS_SERVERS
