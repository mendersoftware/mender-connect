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
	"github.com/mendersoftware/mender-shell/connection"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"

	"github.com/mendersoftware/mender-shell/procps"
	"github.com/mendersoftware/mender-shell/shell"
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

func TestMenderShellStartStopShell(t *testing.T) {
	MaxUserSessions = 2
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

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

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567ff", defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	t.Logf("created session:\n id:%s,\n createdAt:%s,\n expiresAt:%s\n now:%s",
		s.id,
		s.createdAt.Format(defaultTimeFormat),
		s.expiresAt.Format(defaultTimeFormat),
		time.Now().Format(defaultTimeFormat))
	assert.NoError(t, err)
	assert.True(t, procps.ProcessExists(s.shellPid))
	assert.Equal(t, s.status, s.GetStatus())
	assert.Equal(t, s.GetStatus(), ActiveSession)
	assert.True(t, s.GetExpiresAtFmt() != "")
	assert.True(t, s.GetStartedAtFmt() != "")
	assert.True(t, s.GetActiveAtFmt() != "")
	nowUpToHours := strings.Split(time.Now().UTC().Format(defaultTimeFormat), ":")[0]
	nowUpToHoursExpires := strings.Split(time.Now().UTC().Add(defaultSessionExpiredTimeout).Format(defaultTimeFormat), ":")[0]
	assert.Equal(t, strings.Split(s.GetExpiresAtFmt(), ":")[0], nowUpToHoursExpires)
	assert.Equal(t, strings.Split(s.GetStartedAtFmt(), ":")[0], nowUpToHours)
	assert.Equal(t, strings.Split(s.GetActiveAtFmt(), ":")[0], nowUpToHours)
	assert.Equal(t, "/bin/sh", s.GetShellCommandPath())

	sNew, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567ff", defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	assert.True(t, procps.ProcessExists(s.shellPid))

	err = s.StopShell()
	if err != nil {
		assert.Equal(t, err.Error(), "error waiting for the process: signal: interrupt")
	}
	assert.False(t, procps.ProcessExists(s.shellPid))

	count, err := MenderShellStopByUserId("user-id-f435678-f4567ff")
	assert.NoError(t, err)
	assert.Equal(t, uint(2), count) //the reason for 2 here: s.StopShell does not intrinsically remove the session

	count, err = MenderShellStopByUserId("not-really-there")
	assert.Error(t, err)
}

func TestMenderShellCommand(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

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

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, uuid.NewV4().String(), defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	assert.True(t, procps.ProcessExists(s.shellPid))
	err = s.ShellCommand(&shell.MenderShellMessage{
		Type:      wsshell.MessageTypeShellCommand,
		SessionId: s.GetId(),
		Status:    0,
		Data:      []byte("echo ok;\n"),
	})
	assert.NoError(t, err)

	//close terminal controlling handle, for error from inside ShellCommand
	s.pseudoTTY.Close()
	err = s.ShellCommand(&shell.MenderShellMessage{
		Type:      wsshell.MessageTypeShellCommand,
		SessionId: s.GetId(),
		Status:    0,
		Data:      []byte("echo ok;\n"),
	})
	assert.Error(t, err)
}

func TestMenderShellShellAlreadyStartedFailedToStart(t *testing.T) {
	MaxUserSessions = 2
	t.Log("starting mock httpd with websockets")
	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

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

	var mutex sync.Mutex
	userId := uuid.NewV4().String()
	s, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	assert.True(t, procps.ProcessExists(s.shellPid))

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
	assert.True(t, procps.ProcessExists(s.shellPid))

	sNew, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
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

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567f2", defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	assert.True(t, s.IsExpired(false))
	assert.True(t, s.IsExpired(true))
}

func TestMenderShellSessionUpdateWS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f451212", defaultSessionExpiredTimeout, NoExpirationTimeout)
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	newWS, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	assert.True(t, s.ws == ws)
	err = UpdateWSConnection(newWS)
	assert.NoError(t, err)
	assert.True(t, s.ws == newWS)
}

func TestMenderShellSessionGetByUserId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	userId := "user-id-f431212-f4567ff"
	s, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)

	anotherUserId := "user-id-f4433528-43b342b234b"
	anotherUserSession, err := NewMenderShellSession(&mutex, ws, anotherUserId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)

	assert.NotEqual(t, anotherUserId, userId)

	userSessions := MenderShellSessionsGetByUserId(userId)
	assert.True(t, len(userSessions) > 0)
	assert.NotNil(t, userSessions[0])
	assert.True(t, userSessions[0].GetId() == s.id)
	assert.True(t, userSessions[0].GetShellPid() == s.shellPid)

	anotherUserSessions := MenderShellSessionsGetByUserId(anotherUserId)
	assert.True(t, len(anotherUserSessions) > 0)
	assert.NotNil(t, anotherUserSessions[0])
	assert.True(t, anotherUserSessions[0].GetId() == anotherUserSession.id)
	assert.True(t, anotherUserSessions[0].GetShellPid() == anotherUserSession.shellPid)

	userSessions = MenderShellSessionsGetByUserId(userId + "-different")
	assert.False(t, len(userSessions) > 0)

	anotherUserSessions = MenderShellSessionsGetByUserId(anotherUserId + "-different")
	assert.False(t, len(anotherUserSessions) > 0)
}

func TestMenderShellSessionGetById(t *testing.T) {
	MaxUserSessions = 2
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	userId := "user-id-8989-f431212-f4567ff"
	s, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)
	r, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)

	anotherUserId := "user-id-8989-f4433528-43b342b234b"
	anotherUserSession, err := NewMenderShellSession(&mutex, ws, anotherUserId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)
	andAnotherUserSession, err := NewMenderShellSession(&mutex, ws, anotherUserId, defaultSessionExpiredTimeout, NoExpirationTimeout)
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
	assert.Nil(t, MenderShellSessionGetById("not-really-there"))

	var ids []string

	ids = []string{anotherUserSession.id, andAnotherUserSession.id}
	assert.Contains(t, ids, MenderShellSessionGetById(anotherUserSessions[0].GetId()).id)
	assert.Contains(t, ids, MenderShellSessionGetById(anotherUserSessions[1].GetId()).id)
	ids = []string{s.id, r.id}
	assert.Contains(t, ids, MenderShellSessionGetById(userSessions[0].GetId()).id)
	assert.Contains(t, ids, MenderShellSessionGetById(userSessions[1].GetId()).id)
}

func TestMenderShellDeleteById(t *testing.T) {
	MaxUserSessions = 2
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	userId := "user-id-1212-8989-f431212-f4567ff"
	s, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)
	r, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)

	anotherUserId := "user-id-1212-8989-f4433528-43b342b234b"
	anotherUserSession, err := NewMenderShellSession(&mutex, ws, anotherUserId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)
	assert.NotNil(t, anotherUserSession)
	andAnotherUserSession, err := NewMenderShellSession(&mutex, ws, anotherUserId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.NoError(t, err)
	assert.NotNil(t, anotherUserSession)

	assert.NotNil(t, MenderShellSessionGetById(anotherUserSession.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(andAnotherUserSession.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(s.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(r.GetId()))

	err = MenderShellDeleteById("not-really-here")
	assert.Error(t, err)

	err = MenderShellDeleteById(anotherUserSession.GetId())
	assert.NoError(t, err)
	assert.Nil(t, MenderShellSessionGetById(anotherUserSession.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(andAnotherUserSession.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(s.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(r.GetId()))

	err = MenderShellDeleteById("not-really-here")
	assert.Error(t, err)

	err = MenderShellDeleteById(andAnotherUserSession.GetId())
	assert.NoError(t, err)
	assert.Nil(t, MenderShellSessionGetById(anotherUserSession.GetId()))
	assert.Nil(t, MenderShellSessionGetById(andAnotherUserSession.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(s.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(r.GetId()))

	err = MenderShellDeleteById(s.GetId())
	assert.NoError(t, err)
	assert.Nil(t, MenderShellSessionGetById(anotherUserSession.GetId()))
	assert.Nil(t, MenderShellSessionGetById(andAnotherUserSession.GetId()))
	assert.Nil(t, MenderShellSessionGetById(s.GetId()))
	assert.NotNil(t, MenderShellSessionGetById(r.GetId()))

	err = MenderShellDeleteById(r.GetId())
	assert.NoError(t, err)
	assert.Nil(t, MenderShellSessionGetById(anotherUserSession.GetId()))
	assert.Nil(t, MenderShellSessionGetById(andAnotherUserSession.GetId()))
	assert.Nil(t, MenderShellSessionGetById(s.GetId()))
	assert.Nil(t, MenderShellSessionGetById(r.GetId()))

	err = MenderShellDeleteById("not-really-here")
	assert.Error(t, err)
}

func TestMenderShellNewMenderShellSession(t *testing.T) {
	MaxUserSessions = 2
	sessionsMap = map[string]*MenderShellSession{}
	sessionsByUserIdMap = map[string][]*MenderShellSession{}
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	var createdSessonsIds []string
	var s *MenderShellSession
	userId := uuid.NewV4().String()
	for i := 0; i < MaxUserSessions; i++ {
		s, err = NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
		assert.NoError(t, err)
		assert.NotNil(t, s)
		createdSessonsIds = append(createdSessonsIds, s.id)
	}
	notFoundSession, err := NewMenderShellSession(&mutex, ws, userId, defaultSessionExpiredTimeout, NoExpirationTimeout)
	assert.Error(t, err)
	assert.Nil(t, notFoundSession)

	sessionById := MenderShellSessionGetById(s.GetId())
	assert.NotNil(t, sessionById)
	assert.True(t, sessionById.id == s.GetId())

	count := MenderShellSessionGetCount()
	assert.Equal(t, MaxUserSessions, count)

	sessionsIds := MenderShellSessionGetSessionIds()
	assert.Equal(t, len(createdSessonsIds), len(sessionsIds))
	assert.ElementsMatch(t, createdSessonsIds, sessionsIds)
}

func TestMenderSessionTerminateExpired(t *testing.T) {
	defaultSessionExpiredTimeout = 8 * time.Second
	sessionsMap = map[string]*MenderShellSession{}
	sessionsByUserIdMap = map[string][]*MenderShellSession{}

	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567f2", defaultSessionExpiredTimeout, NoExpirationTimeout)
	t.Logf("created session:\n id:%s,\n createdAt:%s,\n expiresAt:%s\n now:%s",
		s.id,
		s.createdAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		s.expiresAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	shells, sessions, total, err := MenderSessionTerminateExpired()
	assert.NoError(t, err)
	assert.Equal(t, 0, shells)
	assert.Equal(t, 0, sessions)
	assert.Equal(t, 0, total)

	time.Sleep(2 * defaultSessionExpiredTimeout)
	assert.True(t, s.IsExpired(false))

	shells, sessions, total, err = MenderSessionTerminateExpired()
	assert.NoError(t, err)
	assert.Equal(t, 1, shells)
	assert.Equal(t, 1, sessions)
	assert.Equal(t, 0, total)
	assert.True(t, !procps.ProcessExists(s.shellPid))
}

func TestMenderSessionTerminateAll(t *testing.T) {
	defaultSessionExpiredTimeout = 8 * time.Second
	sessionsMap = map[string]*MenderShellSession{}
	sessionsByUserIdMap = map[string][]*MenderShellSession{}

	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	s0, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567f2", defaultSessionExpiredTimeout, NoExpirationTimeout)
	t.Logf("created session:\n id:%s,\n createdAt:%s,\n expiresAt:%s\n now:%s",
		s0.id,
		s0.createdAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		s0.expiresAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
	err = s0.StartShell(s0.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	s1, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567f3", defaultSessionExpiredTimeout, NoExpirationTimeout)
	t.Logf("created session:\n id:%s,\n createdAt:%s,\n expiresAt:%s\n now:%s",
		s1.id,
		s1.createdAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		s1.expiresAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
	err = s1.StartShell(s1.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	MenderSessionTerminateAll()
	assert.True(t, !procps.ProcessExists(s0.shellPid))
	assert.True(t, !procps.ProcessExists(s1.shellPid))
}

func TestMenderSessionTerminateIdle(t *testing.T) {
	defaultSessionExpiredTimeout = 255 * time.Second
	idleTimeOut := 4 * time.Second
	sessionsMap = map[string]*MenderShellSession{}
	sessionsByUserIdMap = map[string][]*MenderShellSession{}

	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	var mutex sync.Mutex
	s, err := NewMenderShellSession(&mutex, ws, "user-id-f435678-f4567f2", NoExpirationTimeout, idleTimeOut)
	t.Logf("created session:\n id:%s,\n createdAt:%s,\n expiresAt:%s\n now:%s",
		s.id,
		s.createdAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		s.expiresAt.Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
	err = s.StartShell(s.GetId(), MenderShellTerminalSettings{
		Uid:            500,
		Gid:            501,
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         24,
		Width:          80,
	})
	assert.NoError(t, err)

	go func(f *os.File) {
		for i := 0; i < 2*int(idleTimeOut.Seconds()); i++ {
			f.Write([]byte("echo;\n"))
			time.Sleep(time.Second)
		}
	}(s.pseudoTTY)

	shells, sessions, total, err := MenderSessionTerminateExpired()
	assert.NoError(t, err)
	assert.Equal(t, 0, shells)
	assert.Equal(t, 0, sessions)
	assert.Equal(t, 0, total)

	time.Sleep(2 * idleTimeOut)
	assert.True(t, s.IsExpired(false))

	shells, sessions, total, err = MenderSessionTerminateExpired()
	assert.NoError(t, err)
	assert.Equal(t, 1, shells)
	assert.Equal(t, 1, sessions)
	assert.Equal(t, 0, total)
	assert.True(t, !procps.ProcessExists(s.shellPid))
}

func TestMenderSessionTimeNow(t *testing.T) {
	assert.Equal(t, timeNow().Format(defaultTimeFormat), time.Now().UTC().Format(defaultTimeFormat))
}
