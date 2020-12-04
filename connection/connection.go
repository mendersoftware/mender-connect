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

package connection

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
)

type Connection struct {
	writeMutex sync.Mutex
	// the connection handler
	connection *websocket.Conn
	// Time allowed to write a message to the peer.
	writeWait time.Duration
	// Maximum message size allowed from peer.
	maxMessageSize int64
	// Time allowed to read the next pong message from the peer.
	defaultPingWait time.Duration
}

//Websocket connection routine. setup the ping-pong and connection settings
func NewConnection(u url.URL,
	token string,
	writeWait time.Duration,
	maxMessageSize int64,
	defaultPingWait time.Duration,
	skipVerify bool) (*Connection, error) {
	// skip verification of HTTPS certificate if skipVerify is set in the config file
	if skipVerify {
		websocket.DefaultDialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	var ws *websocket.Conn
	dialer := *websocket.DefaultDialer

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	ws, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		return nil, err
	}

	c := &Connection{
		connection:      ws,
		writeWait:       writeWait,
		maxMessageSize:  maxMessageSize,
		defaultPingWait: defaultPingWait,
	}
	// ping-pong
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(defaultPingWait))
	ws.SetPingHandler(func(message string) error {
		pongWait, _ := strconv.Atoi(message)
		ws.SetReadDeadline(time.Now().Add(time.Duration(pongWait) * time.Second))
		c.writeMutex.Lock()
		defer c.writeMutex.Unlock()
		return ws.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(writeWait))
	})
	return c, nil
}

func (c *Connection) GetWriteTimeout() time.Duration {
	return c.writeWait
}

func (c *Connection) WriteMessage(m *ws.ProtoMsg) (err error) {
	data, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	c.connection.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.connection.WriteMessage(websocket.BinaryMessage, data)
}

// keeping those for debugging and internal use
func (c *Connection) writeMessageRaw(data []byte) (err error) {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	c.connection.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.connection.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Connection) ReadMessage() (*ws.ProtoMsg, error) {
	_, data, err := c.connection.ReadMessage()
	if err != nil {
		return nil, err
	}

	m := &ws.ProtoMsg{}
	err = msgpack.Unmarshal(data, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// keeping those for debugging and internal use
func (c *Connection) readMessageRaw() ([]byte, error) {
	_, data, err := c.connection.ReadMessage()
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *Connection) Close() error {
	return c.connection.Close()
}
