// This file holds the shapes of the lobby requests a client sends over HTTP
// (CLIENT_HTTP.md 4.13, 4.15 and 5.4). Every key is optional in the shape;
// the lobby checks what it needs, because the request is hostile input.

package lobbies

// CreateRequest is the body of PUT Lobbies (CLIENT_HTTP.md 4.13). Name and
// MaxPlayers are pointers so that a missing key is told from an empty one.
// Keys the service does not use (preferred_port, anticheat_id) are not read.
type CreateRequest struct {
	Name           *string `json:"name"`
	MapName        string  `json:"map_name"`
	MapPath        string  `json:"map_path"`
	MapOfficial    bool    `json:"map_official"`
	MaxPlayers     *int    `json:"max_players"`
	VanillaTeams   bool    `json:"vanilla_teams"`
	TrackStats     bool    `json:"track_stats"`
	StartingCash   uint32  `json:"starting_cash"`
	Passworded     bool    `json:"passworded"`
	Password       string  `json:"password"`
	AllowObservers bool    `json:"allow_observers"`
	ExeCRC         uint32  `json:"exe_crc"`
	IniCRC         uint32  `json:"ini_crc"`
	MaxCamHeight   float64 `json:"max_cam_height"`
	// ServerMode asks for a server lobby (SERVICE.md section 10).
	ServerMode bool `json:"server_mode"`
	// RegionTag is the tag of the region of the pass that the caller has
	// already checked. It is set by the service, never read from the request.
	RegionTag string `json:"-"`
}

// JoinRequest is the body of PUT Lobby/{id} (CLIENT_HTTP.md 4.15). The
// password is absent when the client has none to give.
type JoinRequest struct {
	HasMap   bool   `json:"has_map"`
	Password string `json:"password"`
	// Admit says whether the caller holds a valid pass for a region and
	// build fingerprints. It is set by the service, never read from the
	// request, and is asked only when the lobby is a server lobby. Nil refuses.
	Admit func(regionTag string, exeCRC uint32, iniCRC uint32) bool `json:"-"`
}

// FieldRequest is the body of POST Lobby/{id} (CLIENT_HTTP.md 5.4). Field says
// which keys matter. The key spellings are the client's, including startpos
// and start_pos, which belong to different fields.
type FieldRequest struct {
	Field             *int           `json:"field"`
	Map               *string        `json:"map"`
	MapPath           *string        `json:"map_path"`
	MapOfficial       *bool          `json:"map_official"`
	MaxPlayers        *int           `json:"max_players"`
	Side              *int           `json:"side"`
	StartPos          *int           `json:"start_pos"`
	MyStartPos        *int           `json:"startpos"`
	Color             *int           `json:"color"`
	Team              *int           `json:"team"`
	StartingCash      *int64         `json:"startingcash"`
	LimitSuperweapons *bool          `json:"limit_superweapons"`
	HasMap            *bool          `json:"has_map"`
	UserID            *int64         `json:"userid"`
	SlotIndex         *int           `json:"slot_index"`
	SlotState         *int           `json:"slot_state"`
	Slot              *int           `json:"slot"`
	MaxCameraHeight   *int           `json:"max_camera_height"`
	Joinability       *int           `json:"joinability"`
	ServerSession     *string        `json:"server_session"`
	Tokens            []SessionToken `json:"tokens"`
}

// SessionToken is one entry of the tokens of field 19 (SERVICE.md section 10).
type SessionToken struct {
	Slot  *int    `json:"slot"`
	Token *string `json:"token"`
}
