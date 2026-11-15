// Every outside address the client uses, and the name it shows for its online service.
// No other file contains a host name. Debug builds talk to a service on this PC;
// Release builds talk to the hosts below until the real ones are supplied.

#pragma once

namespace ServiceHosts
{
#if defined(_DEBUG)
	// Base of every HTTP call, without a trailing slash. Plain HTTP: a local service has no certificate.
	constexpr const char* API_BASE_URL = "http://127.0.0.1:9000";

	// Each list holds one entry. NetworkMesh.cpp gives the TURN credentials one value per entry.
	// The library refuses an empty STUN list, so Debug builds point it at the local coturn.
	constexpr const char* STUN_SERVER_LIST = "stun:127.0.0.1:3478";
	constexpr const char* TURN_SERVER_LIST = "turn:127.0.0.1:3478";
#else
	constexpr const char* API_BASE_URL = "https://lobby.example.net";

	constexpr const char* STUN_SERVER_LIST = "stun:stun.example.net:3478";
	constexpr const char* TURN_SERVER_LIST = "turn:turn.example.net:3478";
#endif

	// Name shown to players where the client used to name its previous service.
	// The wide form is the same text, for UnicodeString call sites.
	constexpr const char* DISPLAY_NAME = "Servers Community";
	constexpr const wchar_t* DISPLAY_NAME_WIDE = L"Servers Community";
}
