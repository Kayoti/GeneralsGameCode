// This file holds the JSON shapes of a lobby in GET Lobbies and GET Lobby/{id}
// (CLIENT_HTTP.md 5.1 and 5.2). The client reads every key through a const
// reference, so every key is always present, in every lobby and member entry.

package lobbies

// LobbyType of an Online lobby and of a server lobby (SERVICE.md section 7.1).
const (
	lobbyTypeOnline = 0
	lobbyTypeServer = 2
)

// lobbyHeader holds the lobby-level keys that the list and the detail share.
// The key spelling is the client's.
type lobbyHeader struct {
	LobbyID             int64  `json:"LobbyID"`
	Owner               int64  `json:"Owner"`
	Name                string `json:"Name"`
	MapName             string `json:"MapName"`
	MapPath             string `json:"MapPath"`
	IsMapOfficial       bool   `json:"IsMapOfficial"`
	NumCurrentPlayers   int    `json:"NumCurrentPlayers"`
	MaxPlayers          int    `json:"MaxPlayers"`
	IsVanillaTeamsOnly  bool   `json:"IsVanillaTeamsOnly"`
	StartingCash        uint32 `json:"StartingCash"`
	IsLimitSuperweapons bool   `json:"IsLimitSuperweapons"`
	IsTrackingStats     bool   `json:"IsTrackingStats"`
	IsPassworded        bool   `json:"IsPassworded"`
	AllowObservers      bool   `json:"AllowObservers"`
	MaximumCameraHeight uint16 `json:"MaximumCameraHeight"`
	ExeCRC              uint32 `json:"ExeCRC"`
	IniCRC              uint32 `json:"IniCRC"`
	MatchID             uint64 `json:"MatchID"`
	LobbyType           int    `json:"LobbyType"`
	Region              string `json:"Region"`
}

// ListMember is one member entry of GET Lobbies (CLIENT_HTTP.md 5.1).
type ListMember struct {
	UserID      int64  `json:"UserID"`
	DisplayName string `json:"DisplayName"`
	IsReady     bool   `json:"IsReady"`
	SlotIndex   int    `json:"SlotIndex"`
	SlotState   int    `json:"SlotState"`
	Region      string `json:"Region"`
}

// DetailMember is one member entry of GET Lobby/{id} (CLIENT_HTTP.md 5.2).
type DetailMember struct {
	UserID           int64  `json:"UserID"`
	DisplayName      string `json:"DisplayName"`
	IsReady          bool   `json:"IsReady"`
	Port             int    `json:"Port"`
	Side             int    `json:"Side"`
	Color            int    `json:"Color"`
	Team             int    `json:"Team"`
	StartingPosition int    `json:"StartingPosition"`
	HasMap           bool   `json:"HasMap"`
	SlotState        int    `json:"SlotState"`
	SlotIndex        int    `json:"SlotIndex"`
	Region           string `json:"Region"`
	MiddlewareUserID string `json:"MiddlewareUserID"`
}

// ListLobby is one lobby of GET Lobbies.
type ListLobby struct {
	lobbyHeader
	Members []ListMember `json:"Members"`
}

// DetailLobby is the lobby of GET Lobby/{id}.
type DetailLobby struct {
	lobbyHeader
	RNGSeed int            `json:"RNGSeed"`
	Members []DetailMember `json:"Members"`
}

// header returns the lobby-level keys of the lobby. NumCurrentPlayers counts
// the slots that hold a human or an AI.
func (lobby *Lobby) header() lobbyHeader {
	occupied := 0
	for _, entry := range lobby.slots {
		if entry.isHuman() || entry.isAI() {
			occupied++
		}
	}
	header := lobbyHeader{
		LobbyID:             lobby.id,
		Owner:               lobby.owner,
		Name:                lobby.name,
		MapName:             lobby.mapName,
		MapPath:             lobby.mapPath,
		IsMapOfficial:       lobby.isMapOfficial,
		NumCurrentPlayers:   occupied,
		MaxPlayers:          lobby.maxPlayers,
		IsVanillaTeamsOnly:  lobby.vanillaTeamsOnly,
		StartingCash:        lobby.startingCash,
		IsLimitSuperweapons: lobby.limitSuperweapons,
		IsTrackingStats:     lobby.tracksStats,
		IsPassworded:        lobby.password != "",
		AllowObservers:      lobby.allowObservers,
		MaximumCameraHeight: lobby.maxCameraHeight,
		ExeCRC:              lobby.exeCRC,
		IniCRC:              lobby.iniCRC,
		MatchID:             lobby.matchID,
		LobbyType:           lobbyTypeOnline,
		Region:              "",
	}
	if lobby.isServer {
		header.LobbyType = lobbyTypeServer
		header.Region = lobby.regionTag
	}
	return header
}

// displayName returns the name shown for a slot: the player's name, or empty
// for a slot that holds no human.
func (entry slot) displayName(nameOf func(int64) string) string {
	if !entry.isHuman() {
		return ""
	}
	return nameOf(entry.userID)
}

// ListEntry returns the lobby as GET Lobbies shows it, with one member entry
// per slot. nameOf gives the display name of a user id.
func (lobby *Lobby) ListEntry(nameOf func(int64) string) ListLobby {
	members := make([]ListMember, 0, SlotCount)
	for index, entry := range lobby.slots {
		members = append(members, ListMember{
			UserID:      entry.userID,
			DisplayName: entry.displayName(nameOf),
			IsReady:     entry.isReady,
			SlotIndex:   index,
			SlotState:   entry.state,
		})
	}
	return ListLobby{lobbyHeader: lobby.header(), Members: members}
}

// DetailEntry returns the lobby as GET Lobby/{id} shows it, with one member
// entry per slot. nameOf gives the display name of a user id.
func (lobby *Lobby) DetailEntry(nameOf func(int64) string) DetailLobby {
	members := make([]DetailMember, 0, SlotCount)
	for index, entry := range lobby.slots {
		members = append(members, DetailMember{
			UserID:           entry.userID,
			DisplayName:      entry.displayName(nameOf),
			IsReady:          entry.isReady,
			Side:             entry.side,
			Color:            entry.color,
			Team:             entry.team,
			StartingPosition: entry.startPos,
			HasMap:           entry.hasMap,
			SlotState:        entry.state,
			SlotIndex:        index,
		})
	}
	return DetailLobby{lobbyHeader: lobby.header(), RNGSeed: lobby.rngSeed, Members: members}
}
