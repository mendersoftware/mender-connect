// Copyright 2022 Northern.tech AS
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
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"

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
	// Channel to stop the go routines
	done chan bool
}

// Websocket connection routine. setup the ping-pong and connection settings
func NewConnection(
	ctx context.Context,
	u url.URL,
	token string,
	writeWait time.Duration,
	maxMessageSize int64,
	defaultPingWait time.Duration) (*Connection, error) {

	var ws *websocket.Conn

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	// nolint:bodyclose
	ws, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to establish websocket connection: %s", err.Error())
	}

	c := &Connection{
		connection:      ws,
		writeWait:       writeWait,
		maxMessageSize:  maxMessageSize,
		defaultPingWait: defaultPingWait,
		done:            make(chan bool),
	}
	ws.SetReadLimit(maxMessageSize)

	go c.pingPongHandler()

	return c, nil
}

func (c *Connection) pingPongHandler() {
	const pingPeriod = time.Hour
	rootCtx := context.Background()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

Loop:
	for {
		select {
		case <-c.done:
			break Loop
		case <-ticker.C:
			log.Debug("ping message")
			ctx, cancel := context.WithTimeout(rootCtx, time.Second*30)
			err := c.connection.Ping(ctx)
			cancel()
			if err != nil {
				log.Errorf("failed to ping server: %s", err.Error())
				log.Warn("terminating connection")
				_ = c.Close()
				break Loop
			}
		}
	}
}

func (c *Connection) GetWriteTimeout() time.Duration {
	return c.writeWait
}

func (c *Connection) WriteMessage(ctx context.Context, m *ws.ProtoMsg) (err error) {
	data, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	return c.connection.Write(ctx, websocket.MessageBinary, data)
}

func (c *Connection) ReadMessage(ctx context.Context) (*ws.ProtoMsg, error) {
	_, data, err := c.connection.Read(ctx)
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

func (c *Connection) Close() error {
	select {
	case c.done <- true:
	default:
	}
	return c.connection.Close(websocket.StatusNormalClosure, "disconnecting")
}
