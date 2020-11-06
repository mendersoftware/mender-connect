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
	"bufio"
	"errors"
	"io"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"
)

var (
	ErrExecWriteBytesShort  = errors.New("failed to write the whole message")
	messageTypeShellCommand = "shell"
	writeWait               = 1 * time.Second
)

// MenderShellMessage represents a message between the device and the backend
type MenderShellMessage struct {
	Type string `json:"type" msgpack:"type"`
	Data []byte `json:"data" msgpack:"data"`
}

type MenderShell struct {
	ws      *websocket.Conn
	r       io.Reader
	w       io.Writer
	running bool
}

type MenderShellCommand struct {
	path string
}

//Create a new shell, note that we assume that r Reader and w Writer
//are already connected to the i/o of the shell process and ws websocket
//is connected and ws.SetReadDeadline(time.Now().Add(defaultPingWait))
//was already called and ping-pong was established
func NewMenderShell(ws *websocket.Conn, r io.Reader, w io.Writer) *MenderShell {
	shell := MenderShell{
		ws:      ws,
		r:       r,
		w:       w,
		running: false,
	}
	return &shell
}

func (s *MenderShell) Start() {
	go s.pipeStdout()
	go s.pipeStdin()
	s.running = true
}

func (s *MenderShell) Stop() {
	s.running = false
	s.ws.Close()
}

func (s *MenderShell) IsRunning() bool {
	return s.running
}

func (s *MenderShell) handlerShellCommand(data []byte) error {
	commandLine := string(data)
	n, err := s.w.Write(data)
	if err != nil && n != len(data) {
		err = ErrExecWriteBytesShort
	}
	if err != nil {
		log.Debugf("error: '%s' while running '%s'.", err.Error(), commandLine)
	} else {
		log.Debugf("executed: '%s'", commandLine)
	}
	return err
}

func (s *MenderShell) pipeStdin() {
	for {
		if !s.IsRunning() {
			return
		}

		_, data, err := s.ws.ReadMessage()
		if err != nil {
			log.Errorf("error reading message: '%s'; restart is needed.", err)
			break
		}

		m := &MenderShellMessage{}
		err = msgpack.Unmarshal(data, m)
		if err != nil {
			log.Errorf("error parsing message: '%s'; restart is needed.", err)
			continue
		}

		if !s.IsRunning() {
			return
		}
		switch m.Type {
		case messageTypeShellCommand:
			err = s.handlerShellCommand(m.Data)
			if err != nil {
				//FIXME: we need to handle the reconnect somehow
				return
			}
		}
	}
}

func (s *MenderShell) pipeStdout() {
	sr := bufio.NewReader(s.r)
	for {
		if !s.IsRunning() {
			return
		}
		raw := make([]byte, 255)
		n, err := sr.Read(raw)
		if err != nil {
			log.Errorf("error reading stdout: '%s'; restart is needed.", err)
			break
		}
		m := &MenderShellMessage{
			Type: messageTypeShellCommand,
			Data: raw[:n],
		}
		data, err := msgpack.Marshal(m)
		if err != nil {
			log.Errorf("error parsing message: '%s'; restart is needed.", err)
			continue
		}
		s.ws.SetWriteDeadline(time.Now().Add(writeWait))
		s.ws.WriteMessage(websocket.BinaryMessage, data)
	}
}
