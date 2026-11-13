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

#include "Common/GlobalData.h"
#include "GameClient/GameText.h"
#include "GameClient/GameWindowManager.h"
#include "GameClient/MessageBox.h"
#include "GameNetwork/GameSpy/ServersMode.h"

// The region file is read from the working directory of the game.
static const char *const SERVERS_REGION_FILE = "regions.txt";

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

#endif // RTS_SERVERS
