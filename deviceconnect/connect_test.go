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

package deviceconnect

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
)

func TestGetWebSocketScheme(t *testing.T) {
	testCases := map[string]struct {
		scheme string
		result string
	}{
		"http": {
			scheme: httpProtocol,
			result: wsProtocol,
		},
		"https": {
			scheme: httpsProtocol,
			result: wssProtocol,
		},
		"ws": {
			scheme: wsProtocol,
			result: wsProtocol,
		},
		"wss": {
			scheme: wssProtocol,
			result: wssProtocol,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := getWebSocketScheme(tc.scheme)
			assert.Equal(t, tc.result, result)
		})
	}
}

func noopMainServerLoop(w http.ResponseWriter, r *http.Request) {
	var upgrade = websocket.Upgrader{}
	c, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	for {
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		m := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeShell,
				MsgType:   "any",
				SessionID: "any",
				Properties: map[string]interface{}{
					"status": "any",
				},
			},
			Body: []byte("echo"),
		}

		data, _ := msgpack.Marshal(m)
		c.WriteMessage(websocket.BinaryMessage, data)

		time.Sleep(4 * time.Second)
	}
}

func TestMenderShellDeviceConnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(noopMainServerLoop))
	defer server.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.
	u := "ws" + strings.TrimPrefix(server.URL, "http")

	wsValid, err := Connect(u, "/", false, "", "token")
	assert.Nil(t, err)
	assert.NotNil(t, wsValid)
	defer wsValid.Close()

	t.Log("reading a message")
	m, err := wsValid.ReadMessage()
	assert.NoError(t, err)
	assert.True(t, len(m.Body) > 1)
	assert.Equal(t, "echo", string(m.Body))

	t.Log("waiting for timeout")
	time.Sleep(20 * time.Second)
	_, err = wsValid.ReadMessage()
	assert.Error(t, err)

	ws0, err := Connect("%2Casdads://:/sadfa//a", " same here", false, "", "token")
	assert.Error(t, err)
	assert.Nil(t, ws0)

	t.Log("waiting for connection timeout")
	ws1, err := Connect("wss://127.1.1.1:65534", "/this", false, "", "token")
	assert.Error(t, err)
	assert.Nil(t, ws1)
}

func TestMenderShellDeviceConnectTLSCerts(t *testing.T) {
	testCases := map[string]struct {
		certificate string
		skipVerify  bool
		noUpgrade   bool
		err         error
	}{
		"ok-with-given-certificate": {
			certificate: "testdata/server.crt",
		},
		"ok-without-verify": {
			certificate: "",
			skipVerify:  true,
		},
		"error-without-certificate": {
			certificate: "",
			err:         errors.New("x509: certificate signed by unknown authority"),
		},
		"error-bad-handshake": {
			certificate: "testdata/server.crt",
			noUpgrade:   true,
			err:         errors.New("websocket: bad handshake"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.noUpgrade {
					var upgrade = websocket.Upgrader{}
					c, err := upgrade.Upgrade(w, r, nil)
					if err != nil {
						return
					}
					defer c.Close()

					for {
						c.SetWriteDeadline(time.Now().Add(2 * time.Second))
						c.WriteMessage(websocket.BinaryMessage, []byte("echo"))

						time.Sleep(4 * time.Second)
					}
				}
			}))
			defer server.Close()

			// Convert http://127.0.0.1 to wss://127.0.0.
			u := "wss" + strings.TrimPrefix(server.URL, "https")
			wsValid, err := Connect(u, "/", tc.skipVerify, tc.certificate, "token")
			if tc.err != nil {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.err.Error())
				assert.Nil(t, wsValid)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, wsValid)
				defer wsValid.Close()
			}
		})
	}
}
