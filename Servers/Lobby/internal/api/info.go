// This file holds the calls that return configuration to a logged-in client:
// MOTD, Rooms and ServiceConfig (SERVICE.md sections 5.1 and 9).

package api

import "net/http"

// roomFlagsDefault is ERoomFlags value 0 of CLIENT_HTTP.md 4.11.
const roomFlagsDefault = 0

// roomParentNone is parent_id of a room that has no parent. It is the client's own default.
const roomParentNone = -1

// motdAnswer is the answer of GET MOTD. The key is upper case (CLIENT_HTTP.md 4.7).
type motdAnswer struct {
	MOTD string `json:"MOTD"`
}

// roomEntry is one room of GET Rooms (CLIENT_HTTP.md 4.11).
type roomEntry struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Flags    int    `json:"flags"`
	ParentID int    `json:"parent_id"`
}

// roomsAnswer is the answer of GET Rooms. Both support flags are false: the
// service answers a room change with message 4 but offers no moderation commands.
type roomsAnswer struct {
	Rooms                        []roomEntry `json:"rooms"`
	SupportsRoomSelectionResults bool        `json:"supports_room_selection_results"`
	SupportsModerationCommands   bool        `json:"supports_moderation_commands"`
}

// serviceConfigAnswer is every field of the client's ServiceConfig structure
// (CLIENT_HTTP.md section 6). The values are the client's own defaults except
// the five that SERVICE.md section 9 sets.
type serviceConfigAnswer struct {
	RetrySignalling                 bool    `json:"retry_signalling"`
	UseMappedPort                   bool    `json:"use_mapped_port"`
	MinRunAheadFrames               int     `json:"min_run_ahead_frames"`
	RAUpdateFrequencyFrames         int     `json:"ra_update_frequency_frames"`
	RelayAllTraffic                 bool    `json:"relay_all_traffic"`
	RASlackPercent                  int     `json:"ra_slack_percent"`
	FrameGroupingFrames             int     `json:"frame_grouping_frames"`
	EnableHostMigration             bool    `json:"enable_host_migration"`
	NetworkDoImmediateFlushPerFrame bool    `json:"network_do_immediate_flush_per_frame"`
	NetworkSendFlags                int     `json:"network_send_flags"`
	NetworkLatencyLogicModel        int     `json:"network_latency_logic_model"`
	UseDefaultConfig                bool    `json:"use_default_config"`
	RASlackOverridePercentInDefault int     `json:"ra_slack_override_percent_in_default"`
	DoProbes                        bool    `json:"do_probes"`
	DoReplayUpload                  bool    `json:"do_replay_upload"`
	NetworkMeshHistogramDuration    int     `json:"network_mesh_histogram_duration"`
	IbraRATweaks                    bool    `json:"ibra_ra_tweaks"`
	IbraMinSlackDefault             float64 `json:"ibra_minslack_default"`
	IbraMaxSlackDefault             float64 `json:"ibra_maxslack_default"`
	IbraMinSlackGreaterThan300ms    float64 `json:"ibra_minslack_greaterthan300ms"`
	IbraMaxSlackGreaterThan300ms    float64 `json:"ibra_maxslack_greaterthan300ms"`
	IbraMinSlackGreaterThan200ms    float64 `json:"ibra_minslack_greaterthan200ms"`
	IbraMaxSlackGreaterThan200ms    float64 `json:"ibra_maxslack_greaterthan200ms"`
	ScreenshotWidth                 int     `json:"screenshot_width"`
	ScreenshotHeight                int     `json:"screenshot_height"`
	ICEImplementation               int     `json:"ice_implementation"`
	UseNewNetworking                bool    `json:"use_new_networking"`
}

// serviceConfig is the answer of GET ServiceConfig. relay_all_traffic,
// retry_signalling, use_new_networking, do_probes and do_replay_upload are
// the values of SERVICE.md section 9; the rest are the client's defaults.
var serviceConfig = serviceConfigAnswer{
	RetrySignalling:                 true,
	UseMappedPort:                   true,
	MinRunAheadFrames:               4,
	RAUpdateFrequencyFrames:         10,
	RelayAllTraffic:                 false,
	RASlackPercent:                  20,
	FrameGroupingFrames:             2,
	EnableHostMigration:             true,
	NetworkDoImmediateFlushPerFrame: true,
	NetworkSendFlags:                -1,
	NetworkLatencyLogicModel:        0,
	UseDefaultConfig:                false,
	RASlackOverridePercentInDefault: 10,
	DoProbes:                        false,
	DoReplayUpload:                  false,
	NetworkMeshHistogramDuration:    20000,
	IbraRATweaks:                    false,
	IbraMinSlackDefault:             0.25,
	IbraMaxSlackDefault:             1.0,
	IbraMinSlackGreaterThan300ms:    0.35,
	IbraMaxSlackGreaterThan300ms:    1.0,
	IbraMinSlackGreaterThan200ms:    0.3,
	IbraMaxSlackGreaterThan200ms:    1.0,
	ScreenshotWidth:                 557,
	ScreenshotHeight:                333,
	ICEImplementation:               2,
	UseNewNetworking:                false,
}

// infoCallFor returns the handler of a call that returns configuration, when
// the method and the path name one of them. Each needs a session token.
func (routes *server) infoCallFor(method string, segments []string) (http.HandlerFunc, bool) {
	if method != http.MethodGet {
		return nil, false
	}
	switch {
	case segmentsMatch([]string{"MOTD"}, segments):
		return routes.whenLoggedIn(routes.motdCall), true
	case segmentsMatch([]string{"Rooms"}, segments):
		return routes.whenLoggedIn(routes.roomsCall), true
	case segmentsMatch([]string{"ServiceConfig"}, segments):
		return routes.whenLoggedIn(routes.serviceConfigCall), true
	}
	return nil, false
}

// whenLoggedIn wraps a handler so that a request without a valid session
// token gets 401 with an empty object (SERVICE.md section 3).
func (routes *server) whenLoggedIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !routes.isLoggedIn(r) {
			writeJSON(w, http.StatusUnauthorized, emptyObject)
			return
		}
		next(w, r)
	}
}

// motdCall answers GET MOTD with the configured text.
func (routes *server) motdCall(w http.ResponseWriter, _ *http.Request) {
	writeAnswer(w, http.StatusOK, motdAnswer{MOTD: routes.motd})
}

// roomsCall answers GET Rooms with the configured rooms.
func (routes *server) roomsCall(w http.ResponseWriter, _ *http.Request) {
	rooms := routes.hub.Rooms()
	entries := make([]roomEntry, 0, len(rooms))
	for _, room := range rooms {
		entries = append(entries, roomEntry{ID: room.ID, Name: room.Name, Flags: roomFlagsDefault, ParentID: roomParentNone})
	}
	writeAnswer(w, http.StatusOK, roomsAnswer{Rooms: entries})
}

// serviceConfigCall answers GET ServiceConfig with every field of the client's structure.
func (routes *server) serviceConfigCall(w http.ResponseWriter, _ *http.Request) {
	writeAnswer(w, http.StatusOK, serviceConfig)
}
