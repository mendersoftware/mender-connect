// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mendersoftware/mender-shell/shell"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newShellTransaction(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		time.Sleep(4 * time.Second)
	}
}

func psExists(pid int) bool {
	p, err := os.FindProcess(pid)
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

func TestMenderShellStartStopShell(t *testing.T) {
	MaxUserSessions = 2
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}
	uid, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	if err != nil {
		t.Errorf("cant get current uid: %s", err.Error())
		return
	}

	gid, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	if err != nil {
		t.Errorf("cant get current gid: %s", err.Error())
		return
	}

	s, err := NewMenderShellSession(ws, "user-id-f435678-f4567ff")
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.NoError(t, err)
	assert.True(t, psExists(s.shellPid))

	sNew, err := NewMenderShellSession(ws, "user-id-f435678-f4567ff")
	err = sNew.StartShell(sNew.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.NoError(t, err)
	assert.True(t, psExists(s.shellPid))

	err = s.StopShell()
	assert.NoError(t, err)
	assert.False(t, psExists(s.shellPid))

	count, err := MenderShellStopByUserId("user-id-f435678-f4567ff")
	assert.Error(t, err)
	assert.Equal(t, uint(1), count)
}

func TestMenderShellCommand(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}
	uid, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	if err != nil {
		t.Errorf("cant get current uid: %s", err.Error())
		return
	}

	gid, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	if err != nil {
		t.Errorf("cant get current gid: %s", err.Error())
		return
	}

	s, err := NewMenderShellSession(ws, uuid.NewV4().String())
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.NoError(t, err)
	assert.True(t, psExists(s.shellPid))
	err = s.ShellCommand(&shell.MenderShellMessage{
		Type:      shell.MessageTypeShellCommand,
		SessionId: s.GetId(),
		Status:    0,
		Data:      []byte("echo ok;\n"),
	})
	assert.NoError(t, err)
}

func TestMenderShellShellAlreadyStartedFailedToStart(t *testing.T) {
	MaxUserSessions = 2
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}
	uid, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	if err != nil {
		t.Errorf("cant get current uid: %s", err.Error())
		return
	}

	gid, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	if err != nil {
		t.Errorf("cant get current gid: %s", err.Error())
		return
	}

	userId := uuid.NewV4().String()
	s, err := NewMenderShellSession(ws, userId)
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.NoError(t, err)
	assert.True(t, psExists(s.shellPid))

	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.Error(t, err)
	assert.True(t, psExists(s.shellPid))

	sNew, err := NewMenderShellSession(ws, userId)
	err = sNew.StartShell(sNew.GetId(), MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "thatissomethingelse/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	if err != nil {
		log.Errorf("failed to start shell: %s", err.Error())
	}
	assert.Error(t, err)
}

func noopMainServerLoop(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		time.Sleep(4 * time.Second)
	}
}

func TestMenderShellSessionExpire(t *testing.T) {
	defaultSessionExpiredTimeout = 2

	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	s, err := NewMenderShellSession(ws, "user-id-f435678-f4567f2")
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)
	time.Sleep(2 * defaultSessionExpiredTimeout * time.Second)
	assert.True(t, s.IsExpired())
}

func TestMenderShellSessionUpdateWS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	s, err := NewMenderShellSession(ws, "user-id-f435678-f451212")
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	newWS, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer newWS.Close()

	assert.True(t, s.ws == ws)
	err = UpdateWSConnection(newWS)
	assert.NoError(t, err)
	assert.True(t, s.ws == newWS)
}

func TestMenderShellSessionGetByUserId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	userId := "user-id-f431212-f4567ff"
	s, err := NewMenderShellSession(ws, userId)
	assert.NoError(t, err)

	anotherUserId := "user-id-f4433528-43b342b234b"
	anotherUserSession, err := NewMenderShellSession(ws, anotherUserId)
	assert.NoError(t, err)

	assert.NotEqual(t, anotherUserId, userId)

	userSessions := MenderShellSessionsGetByUserId(userId)
	assert.True(t, len(userSessions) > 0)
	assert.NotNil(t, userSessions[0])
	assert.True(t, userSessions[0].GetId() == s.id)

	anotherUserSessions := MenderShellSessionsGetByUserId(anotherUserId)
	assert.True(t, len(anotherUserSessions) > 0)
	assert.NotNil(t, anotherUserSessions[0])
	assert.True(t, anotherUserSessions[0].GetId() == anotherUserSession.id)

	userSessions = MenderShellSessionsGetByUserId(userId + "-different")
	assert.False(t, len(userSessions) > 0)

	anotherUserSessions = MenderShellSessionsGetByUserId(anotherUserId + "-different")
	assert.False(t, len(anotherUserSessions) > 0)
}

func TestMenderShellSessionGetById(t *testing.T) {
	MaxUserSessions = 2
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	userId := "user-id-8989-f431212-f4567ff"
	s, err := NewMenderShellSession(ws, userId)
	assert.NoError(t, err)
	r, err := NewMenderShellSession(ws, userId)
	assert.NoError(t, err)

	anotherUserId := "user-id-8989-f4433528-43b342b234b"
	anotherUserSession, err := NewMenderShellSession(ws, anotherUserId)
	assert.NoError(t, err)
	andAnotherUserSession, err := NewMenderShellSession(ws, anotherUserId)
	assert.NoError(t, err)

	assert.NotEqual(t, anotherUserId, userId)

	userSessions := MenderShellSessionsGetByUserId(userId)
	assert.True(t, len(userSessions) == 2)
	assert.NotNil(t, userSessions[0])
	assert.True(t, userSessions[0].GetId() == s.id)

	anotherUserSessions := MenderShellSessionsGetByUserId(anotherUserId)
	assert.True(t, len(anotherUserSessions) == 2)
	assert.NotNil(t, anotherUserSessions[0])
	assert.True(t, anotherUserSessions[0].GetId() == anotherUserSession.id)

	assert.NotNil(t, MenderShellSessionGetById(userSessions[0].GetId()))
	assert.NotNil(t, MenderShellSessionGetById(userSessions[1].GetId()))
	assert.NotNil(t, MenderShellSessionGetById(anotherUserSessions[0].GetId()))
	assert.NotNil(t, MenderShellSessionGetById(anotherUserSessions[1].GetId()))

	var ids []string

	ids = []string{anotherUserSession.id, andAnotherUserSession.id}
	assert.Contains(t, ids, MenderShellSessionGetById(anotherUserSessions[0].GetId()).id)
	assert.Contains(t, ids, MenderShellSessionGetById(anotherUserSessions[1].GetId()).id)
	ids = []string{s.id, r.id}
	assert.Contains(t, ids, MenderShellSessionGetById(userSessions[0].GetId()).id)
	assert.Contains(t, ids, MenderShellSessionGetById(userSessions[1].GetId()).id)
}

func TestMenderShellNewMenderShellSession(t *testing.T) {
	MaxUserSessions = 2
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	var s *MenderShellSession
	userId := uuid.NewV4().String()
	for i := 0; i < MaxUserSessions; i++ {
		s, err = NewMenderShellSession(ws, userId)
		assert.NoError(t, err)
		assert.NotNil(t, s)
	}
	notFoundSession, err := NewMenderShellSession(ws, userId)
	assert.Error(t, err)
	assert.Nil(t, notFoundSession)

	sessionById := MenderShellSessionGetById(s.GetId())
	assert.NotNil(t, sessionById)
	assert.True(t, sessionById.id == s.GetId())
}
