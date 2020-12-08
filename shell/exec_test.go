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
package shell

import (
	"fmt"
	"github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"
	"github.com/mendersoftware/mender-shell/connection"
	"github.com/vmihailenco/msgpack"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

var messages []string

func TestNewMenderShell(t *testing.T) {
	var mutex sync.Mutex
	s := NewMenderShell("", &mutex, nil, nil, nil)
	assert.NotNil(t, s)
}

func readMessage(webSock *websocket.Conn) (*MenderShellMessage, error) {
	_, data, err := webSock.ReadMessage()
	if err != nil {
		return nil, err
	}

	msg := &ws.ProtoMsg{}
	err = msgpack.Unmarshal(data, msg)
	if err != nil {
		return nil, err
	}

	m := &MenderShellMessage{
		Type:      msg.Header.MsgType,
		SessionId: msg.Header.SessionID,
		Status:    wsshell.NormalMessage,
		Data:      msg.Body,
	}

	return m, nil
}

func echoMainServerLoop(w http.ResponseWriter, r *http.Request) {
	var upgrade = websocket.Upgrader{}
	c, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		m, err := readMessage(c)
		if err == nil {
			lines := strings.Split(string(m.Data), "\r\n")
			for _, l := range lines {
				messages = append(messages, l)
			}
		}
		time.Sleep(1 * time.Second)
		m, err = readMessage(c)
		if err == nil {
			lines := strings.Split(string(m.Data), "\r\n")
			for _, l := range lines {
				messages = append(messages, l)
			}
		}
	}
}

func TestNewMenderShellReadStdIn(t *testing.T) {
	messages = []string{}
	cmd := exec.Command("/bin/sh")
	if cmd == nil {
		t.Fatal("cant execute shell")
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", "xterm-256color"))
	pseudoTTY, err := pty.Start(cmd)
	if err != nil {
		t.Fatal("cant execute shell")
	}

	_, _, err = syscall.Syscall(syscall.SYS_IOCTL, pseudoTTY.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct {
			h, w, x, y uint16
		}{
			24, 80, 0, 0,
		})))

	server := httptest.NewServer(http.HandlerFunc(echoMainServerLoop))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	webSock, err := connection.NewConnection(*urlString, "token", time.Second, 526, time.Second, false, "")
	assert.NoError(t, err)
	assert.NotNil(t, webSock)

	var mutex sync.Mutex
	s := NewMenderShell(uuid.NewV4().String(), &mutex, webSock, pseudoTTY, pseudoTTY)
	assert.NotNil(t, s)

	timeout := s.GetWriteTimeout()
	assert.True(t, timeout > 0)

	s.Start()
	assert.True(t, s.IsRunning())

	message := "_ok_"
	pseudoTTY.Write([]byte("echo " + message + "\n"))

	time.Sleep(8 * time.Second)

	webSock, err = connection.NewConnection(*urlString, "token", time.Second, 526, time.Second, true, "")
	assert.NoError(t, err)
	assert.NotNil(t, webSock)

	err = s.UpdateWSConnection(webSock)
	assert.NoError(t, err)

	webSock.Close()
	s.Stop()
	assert.False(t, s.IsRunning())

	assert.Contains(t, messages, message)
}

func TestPipeStdout(t *testing.T) {
	reader, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}

	writer, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(echoMainServerLoop))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")
	urlString, err := url.Parse(u)
	assert.NoError(t, err)
	assert.NotNil(t, urlString)

	webSock, err := connection.NewConnection(*urlString, "token", time.Second, 526, time.Second, true, "")
	assert.NoError(t, err)
	assert.NotNil(t, webSock)

	shell := &MenderShell{
		sessionId: "unit-tests-sessions-id",
		ws:        webSock,
		r:         reader,
		w:         writer,
		running:   false,
	}

	rc := shell.IsRunning()
	assert.False(t, rc)

	shell.Start()
	rc = shell.IsRunning()
	assert.True(t, rc)

	time.Sleep(4 * time.Second)
	shell.Stop()
	rc = shell.IsRunning()
	assert.False(t, rc)

	shell.Start()
	rc = shell.IsRunning()
	assert.True(t, rc)
	reader.Close()
	writer.Close()
}
