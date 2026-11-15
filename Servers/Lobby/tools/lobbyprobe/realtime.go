// This file holds the WebSocket step of the probe: two accounts connect, join
// the first room, chat and change a name (SERVICE.md sections 6 and 13). The
// two accounts stay connected in the probe, for the lobby step that follows.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// socketWait is the longest the probe waits for a message from the service.
const socketWait = 10 * time.Second

// Message ids of CLIENT_REALTIME.md section 1.7 that the probe uses.
const (
	msgRoomChatFromClient = 1
	msgRoomChatFromServer = 2
	msgRoomChange         = 3
	msgRoomMemberList     = 4
	msgPlayerNameChange   = 9
)

// chatText is the line the first account says in the room.
const chatText = "hello from lobbyprobe"

// account is a logged-in account of the probe and its WebSocket.
type account struct {
	userID  float64
	session string
	socket  *websocket.Conn
}

// webSocketRoom runs the WebSocket step: Rooms, two connections, joining the
// first room, a chat line and a name change.
func (p *probe) webSocketRoom() error {
	roomID, err := p.firstRoomID()
	if err != nil {
		return p.fail("Rooms", err)
	}
	if err := p.report(fmt.Sprintf("Rooms: ok, room %d", roomID)); err != nil {
		return err
	}
	first := &account{userID: p.userID, session: p.session}
	second, err := p.newAccount()
	if err != nil {
		return p.fail("second account", err)
	}
	for _, member := range []*account{first, second} {
		if err := p.connect(member); err != nil {
			return p.fail("WebSocket connect", err)
		}
		p.members = append(p.members, member)
	}
	if err := p.report("WebSocket: two accounts connected"); err != nil {
		return err
	}
	if err := p.exchangeInRoom(first, second, roomID); err != nil {
		return p.fail("WebSocket room", err)
	}
	return p.report("WebSocket: joined the room, chat line delivered, name change seen")
}

// firstRoomID calls Rooms and returns the id of its first room.
func (p *probe) firstRoomID() (int, error) {
	answer, err := p.call("Rooms", http.MethodGet, p.session, "")
	if err != nil {
		return 0, err
	}
	rooms, _ := answer["rooms"].([]any)
	if len(rooms) == 0 {
		return 0, errors.New("the service lists no rooms")
	}
	first, _ := rooms[0].(map[string]any)
	id, ok := first["id"].(float64)
	if !ok {
		return 0, errors.New("the first room has no numeric id")
	}
	return int(id), nil
}

// newAccount logs a new account in with LoginCode and CheckLogin.
func (p *probe) newAccount() (*account, error) {
	codeAnswer, err := p.call("LoginCode", http.MethodGet, "", "")
	if err != nil {
		return nil, err
	}
	code, _ := codeAnswer["login_code"].(string)
	body, err := json.Marshal(map[string]string{"code": code, "client_id": clientID})
	if err != nil {
		return nil, err
	}
	answer, err := p.call("CheckLogin", http.MethodPost, "", string(body))
	if err != nil {
		return nil, err
	}
	if err := requireLoginAnswer(answer); err != nil {
		return nil, err
	}
	session, _ := answer["session_token"].(string)
	userID, _ := answer["user_id"].(float64)
	return &account{userID: userID, session: session}, nil
}

// connect opens the account's WebSocket as a new session, the way the client
// does: the session token in the Authorization header and is-reconnect false.
func (p *probe) connect(member *account) error {
	ctx, cancel := context.WithTimeout(context.Background(), socketWait)
	defer cancel()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+member.session)
	header.Set("is-reconnect", "false")
	address := "ws" + strings.TrimPrefix(p.baseURL, "http") + "/ws"
	socket, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return fmt.Errorf("account %.0f: %w", member.userID, err)
	}
	member.socket = socket
	return nil
}

// exchangeInRoom joins both accounts to the room, has the first say a line
// that the second hears, and has the second change its name and the first see it.
func (p *probe) exchangeInRoom(first *account, second *account, roomID int) error {
	if err := first.send(map[string]any{"msg_id": msgRoomChange, "room": roomID, "request_id": 1}); err != nil {
		return err
	}
	if _, err := first.await(msgRoomMemberList, nil); err != nil {
		return err
	}
	if err := second.send(map[string]any{"msg_id": msgRoomChange, "room": roomID, "request_id": 1}); err != nil {
		return err
	}
	if _, err := first.await(msgRoomMemberList, func(message map[string]any) bool {
		members, _ := message["members"].([]any)
		return len(members) >= 2
	}); err != nil {
		return err
	}
	if err := first.send(map[string]any{"msg_id": msgRoomChatFromClient, "message": chatText, "action": false}); err != nil {
		return err
	}
	if _, err := second.await(msgRoomChatFromServer, func(message map[string]any) bool {
		text, _ := message["message"].(string)
		return strings.HasSuffix(text, chatText)
	}); err != nil {
		return err
	}
	newName := fmt.Sprintf("Probe %.0f", second.userID)
	if err := second.send(map[string]any{"msg_id": msgPlayerNameChange, "name": newName}); err != nil {
		return err
	}
	_, err := first.await(msgRoomMemberList, func(message map[string]any) bool {
		return hasMemberNamed(message, newName)
	})
	return err
}

// hasMemberNamed reports whether a member list message has a member with the name.
func hasMemberNamed(message map[string]any, name string) bool {
	members, _ := message["members"].([]any)
	for _, entry := range members {
		member, _ := entry.(map[string]any)
		if member["Name"] == name {
			return true
		}
	}
	return false
}

// send writes a message as a binary frame, as the game client does.
func (member *account) send(message map[string]any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), socketWait)
	defer cancel()
	if err := member.socket.Write(ctx, websocket.MessageBinary, data); err != nil {
		return fmt.Errorf("account %.0f: send: %w", member.userID, err)
	}
	return nil
}

// await reads messages until one has the msg_id and, when accept is not nil,
// is accepted by it. Other messages are skipped. It fails after socketWait.
func (member *account) await(msgID int, accept func(map[string]any) bool) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), socketWait)
	defer cancel()
	for {
		frameType, data, err := member.socket.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("account %.0f: waiting for message %d: %w", member.userID, msgID, err)
		}
		if frameType != websocket.MessageText {
			continue
		}
		var message map[string]any
		if json.Unmarshal(data, &message) != nil || message["msg_id"] != float64(msgID) {
			continue
		}
		if accept == nil || accept(message) {
			return message, nil
		}
	}
}
