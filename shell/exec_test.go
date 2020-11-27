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
	"github.com/vmihailenco/msgpack"
	"net/http"
	"net/http/httptest"
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

func readMessage(ws *websocket.Conn, m *MenderShellMessage) error {
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

func echoMainServerLoop(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		m := &MenderShellMessage{}
		err = readMessage(c, m)
		if err == nil {
			messages = append(messages, strings.TrimRight(string(m.Data), "\r\n"))
		}
		time.Sleep(1 * time.Second)
		m = &MenderShellMessage{}
		err = readMessage(c, m)
		if err == nil {
			messages = append(messages, strings.TrimRight(string(m.Data), "\r\n"))
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

	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ws.Close()

	var mutex sync.Mutex
	s := NewMenderShell(uuid.NewV4().String(), &mutex, ws, pseudoTTY, pseudoTTY)
	assert.NotNil(t, s)

	s.Start()
	assert.True(t, s.IsRunning())

	message := "_ok_"
	pseudoTTY.Write([]byte("echo " + message + "\n"))

	time.Sleep(8 * time.Second)

	writeWait = 10 * time.Millisecond
	wsNew, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer wsNew.Close()
	err = s.UpdateWSConnection(wsNew)
	assert.NoError(t, err)

	wsNew.Close()
	s.Stop()
	assert.False(t, s.IsRunning())

	assert.Contains(t, messages, message)
}

func TestMenderShellExecGetWriteTimeout(t *testing.T) {
	assert.Equal(t, writeWait, MenderShellExecGetWriteTimeout())
}
