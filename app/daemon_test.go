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
	"github.com/mendersoftware/mender-shell/session"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/mender-shell/config"
	"github.com/mendersoftware/mender-shell/shell"
)

var (
	testFileNameTemporary string
	testData              string
)

func sendMessage(ws *websocket.Conn, t string, sessionId string, data string) error {
	m := &shell.MenderShellMessage{
		Type:      t,
		SessionId: sessionId,
		Data:      []byte(data),
	}
	d, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	ws.SetWriteDeadline(time.Now().Add(2 * time.Second))
	ws.WriteMessage(websocket.BinaryMessage, d)
	return nil
}

func readMessage(ws *websocket.Conn, m *shell.MenderShellMessage) error {
	_, data, err := ws.ReadMessage()
	if err != nil {
		return err
	}

	err = msgpack.Unmarshal(data, m)
	if err != nil {
		return err
	}
	return nil
}

func newShellTransaction(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	sendMessage(c, shell.MessageTypeSpawnShell, "", "user-id-unit-tests-f6723467-561234ff")
	time.Sleep(4 * time.Second)
	m := &shell.MenderShellMessage{}
	readMessage(c, m)
	sendMessage(c, shell.MessageTypeShellCommand, m.SessionId, "echo "+testData+" > "+testFileNameTemporary+"\n")
	sendMessage(c, shell.MessageTypeShellCommand, "undefined-session-id", "rm -f "+testFileNameTemporary+"\n")
	sendMessage(c, shell.MessageTypeShellCommand, m.SessionId, "thiscommand probably does not exist\n")
	sendMessage(c, shell.MessageTypeStopShell, "undefined-session-id", "")
	time.Sleep(4 * time.Second)
	sendMessage(c, shell.MessageTypeStopShell, m.SessionId, "")
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

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

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
	sendMessage(c, shell.MessageTypeSpawnShell, "", "user-id-unit-tests-a00908-f6723467-561234ff")
	time.Sleep(1 * time.Second)
	m := &shell.MenderShellMessage{}
	readMessage(c, m)
	time.Sleep(1 * time.Second)
	sendMessage(c, shell.MessageTypeStopShell, "", "")
	sendMessage(c, shell.MessageTypeStopShell, "", "user-id-unit-tests-a00908-f6723467-561234ff")
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

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

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
		sendMessage(c, shell.MessageTypeSpawnShell, "", "user-id-unit-tests-7f00f6723467-561234ff")
	}
	sendMessage(c, shell.MessageTypeSpawnShell, "", "user-id-unit-tests-7f00f6723467-561234ff")
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

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

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
		time.Sleep(time.Second)
		message, err := d.readMessage(ws)
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
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	d := &MenderShellDaemon{writeMutex: &sync.Mutex{}}
	d.responseMessage(c, &shell.MenderShellMessage{
		Type:      shell.MessageTypeShellCommand,
		SessionId: "some-session_id",
		Status:    shell.NormalMessage,
		Data:      []byte("hello ws"),
	})
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

	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

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

	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()
	assert.NotNil(t, ws)
	assert.NoError(t, err)

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

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

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
