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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/go-lib-micro/ws"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 4 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 8192
	// Time allowed to read the next pong message from the peer.
	defaultPingWait = time.Minute
)

func sleepyHandler(w http.ResponseWriter, r *http.Request) {
	var upgrade = websocket.Upgrader{}
	c, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		time.Sleep(4 * time.Second)
	}
}

func writeMessage(c *websocket.Conn, body []byte) {
	conn := &Connection{
		writeMutex:      sync.Mutex{},
		connection:      c,
		writeWait:       writeWait,
		maxMessageSize:  maxMessageSize,
		defaultPingWait: defaultPingWait,
	}

	m := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   "any-type",
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status:": "ok",
			},
		},
		Body: body,
	}

	conn.WriteMessage(m)
}

const (
	helloMessage = "hello"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	var upgrade = websocket.Upgrader{}
	c, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		writeMessage(c, []byte(helloMessage))
		time.Sleep(1 * time.Second)
	}
}

func TestNewConnection(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(sleepyHandler))
	assert.NotNil(t, s)
	defer s.Close()

	wsUrl := "ws" + strings.TrimPrefix(s.URL, "http")
	parsedUrl, err := url.Parse(wsUrl)
	assert.NoError(t, err)

	u := url.URL{Scheme: parsedUrl.Scheme, Host: parsedUrl.Host, Path: "/"}

	c, err := NewConnection(u, "some-token", writeWait, maxMessageSize, defaultPingWait, true, "")
	assert.NoError(t, err)
	assert.NotNil(t, c)
}

func TestConnection_ReadMessage(t *testing.T) {
	expectedMessage := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeShell,
			MsgType:   "any-type",
			SessionID: "any-session-id",
			Properties: map[string]interface{}{
				"status:": "ok",
			},
		},
		Body: []byte(helloMessage),
	}

	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(helloHandler))
	assert.NotNil(t, s)
	defer s.Close()

	wsUrl := "ws" + strings.TrimPrefix(s.URL, "http")
	parsedUrl, err := url.Parse(wsUrl)
	assert.NoError(t, err)

	u := url.URL{Scheme: parsedUrl.Scheme, Host: parsedUrl.Host, Path: "/"}

	c, err := NewConnection(u, "some-token", writeWait, maxMessageSize, defaultPingWait, true, "")
	time.Sleep(time.Second)
	m, err := c.ReadMessage()
	assert.NoError(t, err)
	assert.NotNil(t, m)
	t.Logf("read: '%s'", string(m.Body))
	assert.Equal(t, []byte(helloMessage), m.Body)
	assert.Equal(t, expectedMessage, m)
}

func TestConnection_WriteMessage(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(helloHandler))
	assert.NotNil(t, s)
	defer s.Close()

	wsUrl := "ws" + strings.TrimPrefix(s.URL, "http")
	parsedUrl, err := url.Parse(wsUrl)
	assert.NoError(t, err)

	u := url.URL{Scheme: parsedUrl.Scheme, Host: parsedUrl.Host, Path: "/"}

	c, err := NewConnection(u, "some-token", writeWait, maxMessageSize, defaultPingWait, true, "")
	time.Sleep(time.Second)
	m, err := c.ReadMessage()
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.Equal(t, []byte(helloMessage), m.Body)

	m.Body = []byte("hello")
	err = c.WriteMessage(m)
	assert.NoError(t, err)
}

func TestConnection_Close(t *testing.T) {
	t.Log("starting mock httpd with websockets")
	s := httptest.NewServer(http.HandlerFunc(sleepyHandler))
	assert.NotNil(t, s)
	defer s.Close()

	wsUrl := "ws" + strings.TrimPrefix(s.URL, "http")
	parsedUrl, err := url.Parse(wsUrl)
	assert.NoError(t, err)

	u := url.URL{Scheme: parsedUrl.Scheme, Host: parsedUrl.Host, Path: "/"}

	c, err := NewConnection(u, "some-token", writeWait, maxMessageSize, defaultPingWait, true, "")
	assert.NotNil(t, c)

	assert.True(t, c.GetWriteTimeout() > 0)

	time.Sleep(time.Second)
	err = c.Close()
	assert.NoError(t, err)
}

func TestMenderShellConnectionLoadServerTrust(t *testing.T) {
	testCases := map[string]struct {
		certificate string
	}{
		"ok-with-given-certificate": {
			certificate: "testdata/server.crt",
		},
		"ok-with-given-un-parseable-pem-certificate": {
			certificate: "testdata/server-broken.crt",
		},
		"ok-with-given-rubbish-certificate": {
			certificate: "testdata/rubbish.txt",
		},
		"nil-without-certificate": {
			certificate: "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			pool := loadServerTrust(tc.certificate)
			assert.True(t, pool != nil)
		})
	}
}
