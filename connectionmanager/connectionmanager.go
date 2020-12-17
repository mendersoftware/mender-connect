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

package connectionmanager

import (
	"errors"
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/mender-shell/connection"
	log "github.com/sirupsen/logrus"
	"net/url"
	"sync"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 4 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 8192
	// Time allowed to read the next pong message from the peer.
	defaultPingWait = 10 * time.Second
	httpsProtocol   = "https"
	httpProtocol    = "http"
	wssProtocol     = "wss"
	wsProtocol      = "ws"
)

var (
	ErrHandlerNotRegistered       = errors.New("protocol handler not registered")
	ErrHandlerAlreadyRegistered   = errors.New("protocol handler already registered")
	ErrConnectionRetriesExhausted = errors.New("failed to connect after max number of retries")
)

type ProtocolHandler struct {
	proto      ws.ProtoType
	connection *connection.Connection
	mutex      *sync.Mutex
}

var handlersByTypeMutex = &sync.Mutex{}
var handlersByType = map[ws.ProtoType]*ProtocolHandler{}
var reconnectIntervalSeconds = 300

func GetWriteTimeout() time.Duration {
	return writeWait
}

func SetReconnectIntervalSeconds(i int) {
	reconnectIntervalSeconds = i
}

func connect(proto ws.ProtoType, serverUrl, connectUrl, token string, skipVerify bool, serverCertificate string, retries uint) error {
	parsedUrl, err := url.Parse(serverUrl)
	if err != nil {
		return err
	}

	scheme := getWebSocketScheme(parsedUrl.Scheme)
	u := url.URL{Scheme: scheme, Host: parsedUrl.Host, Path: connectUrl}

	var c *connection.Connection
	var i uint = 1
	for {
		c, err = connection.NewConnection(u, token, writeWait, maxMessageSize, defaultPingWait, skipVerify, serverCertificate)
		if err != nil || c == nil {
			if retries > 0 {
				if i >= retries {
					return ErrConnectionRetriesExhausted
				}
				i++
				time.Sleep(time.Second)
			}
			if i < retries || retries == 0 {
				if err == nil {
					err = errors.New("unknown error: connection was nil but no error provided by connection.NewConnection")
				}
				log.Errorf("try:%d/%d connection manager failed to connect to %s%s, error: %s;"+
					"reconnecting in 1s; len(token)=%d", i, retries, serverUrl, connectUrl, err.Error(), len(token))
				time.Sleep(time.Second * time.Duration(reconnectIntervalSeconds))
				continue
			}
			return err
		} else {
			break
		}
	}

	handlersByType[proto] = &ProtocolHandler{
		proto:      proto,
		connection: c,
		mutex:      &sync.Mutex{},
	}
	return nil
}

func Connect(proto ws.ProtoType, serverUrl, connectUrl, token string, skipVerify bool, serverCertificate string, retries uint) error {
	handlersByTypeMutex.Lock()
	defer handlersByTypeMutex.Unlock()

	if _, exists := handlersByType[proto]; exists {
		return ErrHandlerAlreadyRegistered
	}

	return connect(proto, serverUrl, connectUrl, token, skipVerify, serverCertificate, retries)
}

func Reconnect(proto ws.ProtoType, serverUrl, connectUrl, token string, skipVerify bool, serverCertificate string, retries uint) error {
	handlersByTypeMutex.Lock()
	defer handlersByTypeMutex.Unlock()

	if h, exists := handlersByType[proto]; exists {
		if h != nil && h.connection != nil {
			h.connection.Close()
		}
	}

	delete(handlersByType, proto)
	return connect(proto, serverUrl, connectUrl, token, skipVerify, serverCertificate, retries)
}

func Read(proto ws.ProtoType) (*ws.ProtoMsg, error) {
	handlersByTypeMutex.Lock()
	h := handlersByType[proto]
	if h == nil {
		handlersByTypeMutex.Unlock()
		return nil, ErrHandlerNotRegistered
	}

	handlersByTypeMutex.Unlock()
	return h.connection.ReadMessage()
}

func Write(proto ws.ProtoType, m *ws.ProtoMsg) error {
	handlersByTypeMutex.Lock()
	h := handlersByType[proto]
	if h == nil {
		handlersByTypeMutex.Unlock()
		return ErrHandlerNotRegistered
	}

	handlersByTypeMutex.Unlock()
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.connection.WriteMessage(m)
}

func Close(proto ws.ProtoType) error {
	handlersByTypeMutex.Lock()
	defer handlersByTypeMutex.Unlock()

	h := handlersByType[proto]
	if h == nil {
		return ErrHandlerNotRegistered
	}

	return h.connection.Close()
}

func getWebSocketScheme(scheme string) string {
	if scheme == httpsProtocol {
		scheme = wssProtocol
	} else if scheme == httpProtocol {
		scheme = wsProtocol
	}
	return scheme
}
