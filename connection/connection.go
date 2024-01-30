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
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

type ConnectionFactory interface {
	NewConnection(
		ctx context.Context,
		u url.URL,
		token string,
		writeWait time.Duration,
		maxMessageSize int64,
		defaultPingWait time.Duration) (*Connection, error)
}

type backoffFactory struct {
	backoffTimer *time.Timer
	nextBackoff  time.Duration

	dialer WebsocketDialer
}

const backoffMin = time.Second

type WebsocketDialer interface {
	DialContext(
		ctx context.Context,
		url string,
		requestHeader http.Header,
	) (*websocket.Conn, *http.Response, error)
}

func NewConnectionFactory(dialer WebsocketDialer) ConnectionFactory {
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	factory := &backoffFactory{
		backoffTimer: time.NewTimer(0),
		nextBackoff:  backoffMin,

		dialer: dialer,
	}
	return factory
}

func (bf *backoffFactory) incrementTimer() {
	jitter := time.Duration(rand.Int63n(backoffMin.Nanoseconds())) -
		backoffMin/2
	bf.nextBackoff *= 2 + jitter
	bf.resetTimer(bf.nextBackoff)
}

func (bf *backoffFactory) resetTimer(d time.Duration) {
	if bf.backoffTimer.Stop() {
		select {
		case <-bf.backoffTimer.C:
		default:
		}
	}
	bf.backoffTimer.Reset(d)
}

func (bf *backoffFactory) Wait(ctx context.Context) error {
	select {
	case <-bf.backoffTimer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// Websocket connection routine. setup the ping-pong and connection settings
func (bf *backoffFactory) NewConnection(
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
	ws, _, err := bf.dialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		bf.incrementTimer()
		return nil, err
	}
	bf.resetTimer(0)
	bf.nextBackoff = backoffMin
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

var defaultFactory = NewConnectionFactory(websocket.DefaultDialer)

func NewConnection(
	ctx context.Context,
	u url.URL,
	token string,
	writeWait time.Duration,
	maxMessageSize int64,
	defaultPingWait time.Duration) (*Connection, error) {
	return defaultFactory.NewConnection(ctx, u, token,
		writeWait, maxMessageSize, defaultPingWait)
}

func (c *Connection) pingPongHandler() {
	// handle the ping-pong connection health check
	err := c.connection.SetReadDeadline(time.Now().Add(c.defaultPingWait))
	if err != nil {
		return
	}

	pingPeriod := (c.defaultPingWait * 9) / 10
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	c.connection.SetPongHandler(func(string) error {
		log.Debug("PongHandler called")
		// requires go >= 1.15
		// ticker.Reset(pingPeriod)
		return c.connection.SetReadDeadline(time.Now().Add(c.defaultPingWait))
	})

	c.connection.SetPingHandler(func(msg string) error {
		log.Debug("PingHandler called")
		// requires go >= 1.15
		// ticker.Reset(pingPeriod)
		err := c.connection.SetReadDeadline(time.Now().Add(c.defaultPingWait))
		if err != nil {
			return err
		}
		c.writeMutex.Lock()
		defer c.writeMutex.Unlock()
		return c.connection.WriteControl(
			websocket.PongMessage,
			[]byte(msg),
			time.Now().Add(c.writeWait),
		)
	})

	running := true
	for running {
		select {
		case <-c.done:
			running = false
			break
		case <-ticker.C:
			log.Debug("ping message")
			pongWaitString := strconv.Itoa(int(c.defaultPingWait.Seconds()))
			c.writeMutex.Lock()
			_ = c.connection.WriteControl(
				websocket.PingMessage,
				[]byte(pongWaitString),
				time.Now().Add(c.defaultPingWait),
			)
			c.writeMutex.Unlock()
		}
	}
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
	_ = c.connection.SetWriteDeadline(time.Now().Add(c.writeWait))
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

func (c *Connection) Close() error {
	select {
	case c.done <- true:
	default:
	}
	return c.connection.Close()
}
