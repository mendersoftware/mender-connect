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
package app

import (
	"errors"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/user"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"

	dbusmocks "github.com/mendersoftware/mender-shell/client/dbus/mocks"
	authmocks "github.com/mendersoftware/mender-shell/client/mender/mocks"

	"github.com/mendersoftware/mender-shell/config"
	"github.com/mendersoftware/mender-shell/connection"
	"github.com/mendersoftware/mender-shell/session"
	"github.com/mendersoftware/mender-shell/shell"
)

var (
	testFileNameTemporary string
	testData              string
)

func sendMessage(webSock *websocket.Conn, t string, sessionId string, data string) error {
	m := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   t,
			SessionID: sessionId,
			Properties: map[string]interface{}{
				"status": wsshell.NormalMessage,
			},
		},
		Body: []byte(data),
	}
	d, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	webSock.SetWriteDeadline(time.Now().Add(2 * time.Second))
	webSock.WriteMessage(websocket.BinaryMessage, d)
	return nil
}

func readMessage(webSock *websocket.Conn) (*shell.MenderShellMessage, error) {
	_, data, err := webSock.ReadMessage()
	if err != nil {
		return nil, err
	}

	msg := &ws.ProtoMsg{}
	err = msgpack.Unmarshal(data, msg)
	if err != nil {
		return nil, err
	}

	m := &shell.MenderShellMessage{
		Type:      msg.Header.MsgType,
		SessionId: msg.Header.SessionID,
		Status:    wsshell.NormalMessage,
		Data:      msg.Body,
	}

	return m, nil
}

func newShellTransaction(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	sendMessage(c, wsshell.MessageTypeSpawnShell, "", "user-id-unit-tests-f6723467-561234ff")
	time.Sleep(4 * time.Second)
	//m := &shell.MenderShellMessage{}
	m, err := readMessage(c)
	sendMessage(c, wsshell.MessageTypeShellCommand, m.SessionId, "echo "+testData+" > "+testFileNameTemporary+"\n")
	sendMessage(c, wsshell.MessageTypeShellCommand, "undefined-session-id", "rm -f "+testFileNameTemporary+"\n")
	sendMessage(c, wsshell.MessageTypeShellCommand, m.SessionId, "thiscommand probably does not exist\n")
	sendMessage(c, wsshell.MessageTypeStopShell, "undefined-session-id", "")
	time.Sleep(4 * time.Second)
	sendMessage(c, wsshell.MessageTypeStopShell, m.SessionId, "")
	for {
		time.Sleep(4 * time.Second)
	}
}

func TestMenderShellSessionStart(t *testing.T) {
	rand.Seed(time.Now().Unix())
	testData = "newShellTransaction." + strconv.Itoa(rand.Intn(6553600))
	tempFile, err := ioutil.TempFile("", "TestMenderShellExec")
	if err != nil {
		t.Error("cant create temp file")
		return
	}
	testFileNameTemporary = tempFile.Name()
	defer os.Remove(tempFile.Name())

	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 256, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})
	message, err := d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	assert.NoError(t, err)
	assert.NotNil(t, message)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	t.Log("checking command execution results")
	found := false
	for i := 0; i < 8; i++ {
		t.Logf("checking if %s contains %s", testFileNameTemporary, testData)
		data, _ := ioutil.ReadFile(testFileNameTemporary)
		trimmedData := strings.TrimRight(string(data), "\n")
		t.Logf("got: '%s'", trimmedData)
		if trimmedData == testData {
			found = true
			break
		}
		time.Sleep(time.Second)
	}
	assert.True(t, found, "file contents must match expected value")
}

func newShellStopByUserId(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	sendMessage(c, wsshell.MessageTypeSpawnShell, "", "user-id-unit-tests-a00908-f6723467-561234ff")
	//time.Sleep(1 * time.Second)
	//m := &shell.MenderShellMessage{}
	readMessage(c)
	//time.Sleep(1 * time.Second)
	sendMessage(c, wsshell.MessageTypeStopShell, "", "")
	sendMessage(c, wsshell.MessageTypeStopShell, "", "user-id-unit-tests-a00908-f6723467-561234ff")
	for {
		time.Sleep(4 * time.Second)
	}
}

func TestMenderShellStopByUserId(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(newShellStopByUserId))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 8*time.Second, 526, 8*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})
	message, err := d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	sessions := session.MenderShellSessionsGetByUserId("user-id-unit-tests-a00908-f6723467-561234ff")
	assert.True(t, len(sessions) > 0)
	assert.NotNil(t, sessions[0])
	sessionsCount := d.shellsSpawned

	message, err = d.readMessage(ws)
	assert.NoError(t, err)
	assert.NotNil(t, message)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	message, err = d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}

	time.Sleep(time.Second)
	assert.Equal(t, sessionsCount-1, d.shellsSpawned)
}

func newShellMulti(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	for i := 0; i < session.MaxUserSessions; i++ {
		sendMessage(c, wsshell.MessageTypeSpawnShell, "", "user-id-unit-tests-7f00f6723467-561234ff")
	}
	sendMessage(c, wsshell.MessageTypeSpawnShell, "", "user-id-unit-tests-7f00f6723467-561234ff")
	time.Sleep(4 * time.Second)
	for {
		time.Sleep(4 * time.Second)
	}
}

//maxUserSessions controls how many sessions user can have.
func TestMenderShellSessionLimitPerUser(t *testing.T) {
	session.MaxUserSessions = 2
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(newShellMulti))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", time.Second, 526, time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})

	for i := 0; i < session.MaxUserSessions; i++ {
		message, err := d.readMessage(ws)
		assert.NoError(t, err)
		assert.NotNil(t, message)
		t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
		err = d.routeMessage(ws, message)
		if err != nil {
			t.Logf("route message error: %s", err.Error())
		}
		assert.NoError(t, err)
	}

	message, err := d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}
	assert.Error(t, err)
}

func TestMenderShellStopDaemon(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})

	assert.True(t, !d.shouldStop())
	d.PrintStatus()
	d.StopDaemon()
	assert.True(t, d.shouldStop())
}

func oneMsgMainServerLoop(w http.ResponseWriter, r *http.Request) {
	var upgrade = websocket.Upgrader{}
	c, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	m := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   wsshell.MessageTypeShellCommand,
			SessionID: "some-session_id",
			Properties: map[string]interface{}{
				"status": wsshell.NormalMessage,
			},
		},
		Body: []byte("hello ws"),
	}

	data, err := msgpack.Marshal(m)
	c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	c.WriteMessage(websocket.BinaryMessage, data)

	for {
		time.Sleep(1 * time.Second)
	}
}

func TestMenderShellReadMessage(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(oneMsgMainServerLoop))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 526, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	time.Sleep(2 * time.Second)
	m, err := d.readMessage(ws)
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.Equal(t, m.Data, []byte("hello ws"))
	time.Sleep(2 * time.Second)

	ws.Close()
	m, err = d.readMessage(ws)
	assert.Error(t, err)
	assert.Nil(t, m)
}

func TestMenderShellWsReconnect(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(oneMsgMainServerLoop))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", time.Second, 526, time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	t.Log("attempting reconnect")
	d.serverUrl = u
	ws, err = d.wsReconnect("atoken")
	assert.NotNil(t, ws)
	assert.NoError(t, err)
}

func TestMenderShellMaxShellsLimit(t *testing.T) {
	session.MaxUserSessions = 4
	config.MaxShellsSpawned = 2
	currentUser, err := user.Current()
	if err != nil {
		t.Errorf("cant get current user: %s", err.Error())
		return
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(newShellMulti))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	ws, err := connection.NewConnection(*urlString, "token", 16*time.Second, 526, 16*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, ws)

	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         currentUser.Name,
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})

	for i := 0; i < int(config.MaxShellsSpawned); i++ {
		time.Sleep(time.Second)
		message, err := d.readMessage(ws)
		assert.NoError(t, err)
		assert.NotNil(t, message)
		t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
		err = d.routeMessage(ws, message)
		if err != nil {
			t.Logf("route message error: %s", err.Error())
		}
		assert.NoError(t, err)
	}

	message, err := d.readMessage(ws)
	t.Logf("read message: type, session_id, data %s, %s, %s", message.Type, message.SessionId, message.Data)
	err = d.routeMessage(ws, message)
	if err != nil {
		t.Logf("route message error: %s", err.Error())
	}
	assert.Error(t, err)
}

func TestOutputStatus(t *testing.T) {
	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         "mender",
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})
	assert.NotNil(t, d)

	d.outputStatus()
}

func TestTimeToSweepSessions(t *testing.T) {
	d := NewDaemon(&config.MenderShellConfig{
		MenderShellConfigFromFile: config.MenderShellConfigFromFile{
			ShellCommand: "/bin/sh",
			User:         "mender",
			Terminal: config.TerminalConfig{
				Width:  24,
				Height: 80,
			},
		},
	})
	assert.NotNil(t, d)
	assert.False(t, d.timeToSweepSessions())

	//if both expire timeout and idle expire timeout are not set it is never time to sweep
	d.expireSessionsAfter = time.Duration(0)
	d.expireSessionsAfterIdle = time.Duration(0)
	assert.False(t, d.timeToSweepSessions())

	//on the other hand when both are set it maybe time to sweep
	d.expireSessionsAfter = 32 * time.Second
	d.expireSessionsAfterIdle = 8 * time.Second
	lastExpiredSessionSweep = time.Now()
	assert.False(t, d.timeToSweepSessions())

	expiredSessionsSweepFrequency = 2 * time.Second
	time.Sleep(2 * expiredSessionsSweepFrequency)
	assert.True(t, d.timeToSweepSessions())
}

func TestWaitForJWTToken(t *testing.T) {
	testCases := []struct {
		name    string
		err     error
		token   string
		timeout time.Duration
	}{
		{
			name:  "token_returned",
			token: "the-token",
		},
		{
			name:    "token_not_returned_wait_forever",
			token:   "",
			timeout: 15 * time.Second,
		},
	}

	for _, tc := range testCases {
		if tc.name == "token_not_returned_wait_forever" {
			timeout := time.After(tc.timeout)
			done := make(chan bool)
			go func() {
				t.Run(tc.name, func(t *testing.T) {
					dbusAPI := &dbusmocks.DBusAPI{}
					defer dbusAPI.AssertExpectations(t)
					client := &authmocks.AuthClient{}
					client.On("GetJWTToken").Return(tc.token, tc.err)
					token, err := waitForJWTToken(client)
					if tc.err != nil {
						assert.Error(t, err)
					} else {
						assert.NoError(t, err)
						assert.True(t, tc.token != "")
						assert.Equal(t, tc.token, token)
					}
				})
				done <- true
			}()

			select {
			case <-timeout:
				t.Logf("ok: expected to run forever")
			case <-done:
			}
		} else {
			t.Run(tc.name, func(t *testing.T) {
				dbusAPI := &dbusmocks.DBusAPI{}
				defer dbusAPI.AssertExpectations(t)
				client := &authmocks.AuthClient{}
				client.On("GetJWTToken").Return(tc.token, tc.err)
				token, err := waitForJWTToken(client)
				if tc.err != nil {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.True(t, tc.token != "")
					assert.Equal(t, tc.token, token)
				}
			})
		}
	}
}

func TestDeviceUnauth(t *testing.T) {
	testCases := []struct {
		name  string
		err   error
		token string
		rc    bool
	}{
		{
			name:  "authorized",
			token: "the-token",
			rc:    false,
		},
		{
			name:  "unauthorized",
			token: "",
			rc:    true,
		},
		{
			name:  "unknown due to error assumed no change (authorized)",
			token: "",
			err:   errors.New("error getting the token"),
			rc:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbusAPI := &dbusmocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)
			client := &authmocks.AuthClient{}
			client.On("GetJWTToken").Return(tc.token, tc.err)
			rc := deviceUnauth(client)
			assert.Equal(t, tc.rc, rc)
		})
	}
}

func everySecondMessage(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		sendMessage(c, wsshell.MessageTypeShellCommand, "any-session-id", "echo;")
		time.Sleep(400 * time.Millisecond)
	}
}

func TestMessageMainLoop(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(everySecondMessage))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	webSock, err := connection.NewConnection(*urlString, "token", 8*time.Second, 526, 8*time.Second, true)
	assert.NoError(t, err)
	assert.NotNil(t, webSock)

	testCases := []struct {
		name       string
		ws         *connection.Connection
		token      string
		shouldStop bool
		err        error
	}{
		{
			name:       "normal-exit",
			shouldStop: true,
		},
		{
			name: "route-a-message",
			ws:   webSock,
		},
		{
			name: "ws-nil-error",
			err:  errors.New("error connecting"),
		},
	}

	config.MaxReconnectAttempts = 2
	timeout := 15 * time.Second
	for _, tc := range testCases {
		timeout := time.After(timeout)
		done := make(chan bool)
		go func() {
			t.Run(tc.name, func(t *testing.T) {
				d := &MenderShellDaemon{}
				d.stop = tc.shouldStop
				d.printStatus = true
				if tc.ws != nil {
					go func() {
						time.Sleep(4 * time.Second)
						d.stop = true
					}()
				}
				err = d.messageMainLoop(tc.ws, tc.token)
				if tc.err != nil {
					assert.Error(t, err)
				} else {
					if err != nil && err.Error() == session.ErrSessionNotFound.Error() {
						assert.Equal(t, session.ErrSessionNotFound.Error(), err.Error())
					} else {
						assert.NoError(t, err)
					}
				}
				done <- true
			})
		}()

		select {
		case <-timeout:
			t.Logf("ok: expected to run forever")
		case <-done:
		}
	}
}

func TestRun(t *testing.T) {
	d := &MenderShellDaemon{}
	d.debug = true
	to := 15 * time.Second
	timeout := time.After(to)
	done := make(chan bool)
	go func() {
		err := d.Run()
		assert.Error(t, err)
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("expected to exit with error")
	case <-done:
	}
}
