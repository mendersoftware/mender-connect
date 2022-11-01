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

package connectionmanager

import (
	"context"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	DefaultPingWait = 10 * time.Second
}

func newWebsocketServer() *http.Server {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	m := http.NewServeMux()
	s := http.Server{Addr: "localhost:8999", Handler: m}
	m.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		msg := ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto: ws.ProtoTypeShell,
			},
			Body: []byte("dummy"),
		}
		data, _ := msgpack.Marshal(msg)
		_ = conn.WriteMessage(websocket.BinaryMessage, data)

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				panic(err)
			}
			log.Println(data)
		}
	})
	return &s
}

func TestSetReconnectIntervalSeconds(t *testing.T) {
	SetReconnectIntervalSeconds(15)
	assert.Equal(t, 15, reconnectIntervalSeconds)
}

func TestGetWriteTimeout(t *testing.T) {
	timeOut := GetWriteTimeout()
	assert.Equal(t, writeWait, timeOut)
}

func TestGetWsScheme(t *testing.T) {
	assert.Equal(t, "wss", getWebSocketScheme("https"))
	assert.Equal(t, "ws", getWebSocketScheme("http"))
	assert.Equal(t, "wss", getWebSocketScheme("wss"))
	assert.Equal(t, "ws", getWebSocketScheme("ws"))
}

func TestConnect(t *testing.T) {
	srv := newWebsocketServer()
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	_ = Close(ws.ProtoTypeShell)

	ctx := context.Background()
	err := Reconnect(ws.ProtoTypeShell, "ws://localhost:8999", "/ws", "token", 1, ctx)
	assert.Nil(t, err)

	msg, err := Read(ws.ProtoTypeShell)
	assert.Nil(t, err)
	assert.Equal(t, []byte("dummy"), msg.Body)

	err = Write(ws.ProtoTypeShell, &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto: ws.ProtoTypeShell,
		},
		Body: []byte("dummy"),
	})
	assert.Nil(t, err)

	err = Close(ws.ProtoTypeShell)
	assert.Nil(t, err)
}

func TestReconnect(t *testing.T) {
	srv := newWebsocketServer()
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	_ = Close(ws.ProtoTypeShell)

	ctx := context.Background()
	err := Reconnect(ws.ProtoTypeShell, "ws://localhost:8999", "/ws", "token", 1, ctx)
	assert.Nil(t, err)

	err = Close(ws.ProtoTypeShell)
	assert.Nil(t, err)
}

func TestConnectFailed(t *testing.T) {
	_ = Close(ws.ProtoTypeShell)

	ctx := context.Background()
	err := Reconnect(ws.ProtoTypeShell, "wrong-url", "/ws", "token", 1, ctx)
	assert.NotNil(t, err)
}

func TestConnectRetries(t *testing.T) {
	oldReconnectIntervalSeconds := reconnectIntervalSeconds
	reconnectIntervalSeconds = 1
	defer func() {
		reconnectIntervalSeconds = oldReconnectIntervalSeconds
	}()

	_ = Close(ws.ProtoTypeShell)

	ctx := context.Background()
	err := Reconnect(ws.ProtoTypeShell, "ws://localhost:8999", "/ws", "token", 3, ctx)
	assert.Equal(t, ErrConnectionRetriesExhausted, err)
}

func TestCloseFailed(t *testing.T) {
	err := Close(12345)
	assert.Error(t, err)

}
func TestWriteFailed(t *testing.T) {
	err := Write(12345, nil)
	assert.Error(t, err)
}
