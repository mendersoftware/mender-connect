// Copyright 2021 Northern.tech AS
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
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	log "github.com/sirupsen/logrus"

	"github.com/northerntechhq/nt-connect/connection"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 4 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 8192

	httpsProtocol = "https"
	httpProtocol  = "http"
	wssProtocol   = "wss"
	wsProtocol    = "ws"
)

var (
	ErrHandlerNotRegistered       = errors.New("protocol handler not registered")
	ErrHandlerAlreadyRegistered   = errors.New("protocol handler already registered")
	ErrConnectionRetriesExhausted = errors.New("failed to connect after max number of retries")
)

var (
	reconnectingMutex   = sync.Mutex{}
	reconnecting        = map[ws.ProtoType]bool{}
	cancelReconnectChan = map[ws.ProtoType]chan bool{}
)

type ProtocolHandler struct {
	proto      ws.ProtoType
	connection *connection.Connection
	mutex      *sync.Mutex
}

var handlersByTypeMutex = &sync.Mutex{}
var handlersByType = map[ws.ProtoType]*ProtocolHandler{}
var reconnectIntervalSeconds = 5
var DefaultPingWait = time.Minute

func GetWriteTimeout() time.Duration {
	return writeWait
}

func SetReconnectIntervalSeconds(i int) {
	reconnectIntervalSeconds = i
}

func connect(
	proto ws.ProtoType,
	serverUrl, connectUrl, token string,
	retries uint,
	ctx context.Context,
) error {
	parsedUrl, err := url.Parse(serverUrl)
	if err != nil {
		return err
	}

	scheme := getWebSocketScheme(parsedUrl.Scheme)
	u := url.URL{Scheme: scheme, Host: parsedUrl.Host, Path: connectUrl}

	cancelReconnectChan[proto] = make(chan bool)
	var c *connection.Connection
	var i uint = 0
	defer func() {
		setReconnecting(proto, false)
	}()
	setReconnecting(proto, true)
	for IsReconnecting(proto) {
		i++
		c, err = connection.NewConnection(u, token, writeWait, maxMessageSize, DefaultPingWait)
		if err != nil || c == nil {
			if retries == 0 || i < retries {
				if err == nil {
					err = errors.New(
						"unknown error: connection was nil but no error provided by" +
							" connection.NewConnection",
					)
				}
				log.Errorf("connection manager failed to connect to %s%s: %s; "+
					"reconnecting in %ds (try %d/%d); len(token)=%d", serverUrl, connectUrl,
					err.Error(), reconnectIntervalSeconds, i, retries, len(token))
				select {
				case cancelFlag := <-cancelReconnectChan[proto]:
					log.Tracef("connectionmanager connect got cancelFlag=%+v", cancelFlag)
					if cancelFlag {
						return nil
					}
				case <-ctx.Done():
					return nil
				case <-time.After(time.Second * time.Duration(reconnectIntervalSeconds)):
					break
				}
				continue
			} else if i >= retries {
				return ErrConnectionRetriesExhausted
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

	// There could be a pending cancelReconnectChan request from CancelReconnection unprocessed
	select {
	case <-cancelReconnectChan[proto]:
		log.Trace("connectionmanager drained cancelReconnectChan")
	default:
	}

	return nil
}

func Connect(
	proto ws.ProtoType,
	serverUrl, connectUrl, token string,
	retries uint,
	ctx context.Context,
) error {
	handlersByTypeMutex.Lock()
	defer handlersByTypeMutex.Unlock()

	if _, exists := handlersByType[proto]; exists {
		return ErrHandlerAlreadyRegistered
	}

	return connect(proto, serverUrl, connectUrl, token, retries, ctx)
}

func Reconnect(
	proto ws.ProtoType,
	serverUrl, connectUrl, token string,
	retries uint,
	ctx context.Context,
) error {
	handlersByTypeMutex.Lock()
	defer handlersByTypeMutex.Unlock()

	if h, exists := handlersByType[proto]; exists {
		if h != nil && h.connection != nil {
			h.connection.Close()
		}
	}

	delete(handlersByType, proto)
	return connect(proto, serverUrl, connectUrl, token, retries, ctx)
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

func IsReconnecting(proto ws.ProtoType) bool {
	reconnectingMutex.Lock()
	defer reconnectingMutex.Unlock()
	return reconnecting[proto]
}

func setReconnecting(proto ws.ProtoType, v bool) bool {
	reconnectingMutex.Lock()
	defer reconnectingMutex.Unlock()
	reconnecting[proto] = v
	return v
}

func CancelReconnection(proto ws.ProtoType) bool {
	maxWaitSeconds := 8
	go func() {
		cancelReconnectChan[proto] <- true
	}()
	for maxWaitSeconds > 0 {
		time.Sleep(time.Second)
		if !IsReconnecting(proto) {
			break
		}
		maxWaitSeconds--
	}
	if IsReconnecting(proto) {
		log.Error("failed to cancel reconnection")
		return false
	}
	return true
}

func Close(proto ws.ProtoType) error {
	if IsReconnecting(proto) {
		if !CancelReconnection(proto) {
			return errors.New("failed to cancel ongoing reconnection")
		}
	}

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
