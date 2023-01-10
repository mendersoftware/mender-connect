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

package session

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	wspf "github.com/mendersoftware/go-lib-micro/ws/portforward"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func getFreeTCPPort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func TestPortForwardHandler(t *testing.T) {
	handler := PortForward()()

	// unkonwn message
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:   ws.ProtoTypePortForward,
			MsgType: "dummy",
		},
	}
	w := new(testWriter)
	handler.ServeProtoMsg(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp := w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypeError, rsp.Header.MsgType)

	msgError := &wspf.Error{}
	_ = msgpack.Unmarshal(rsp.Body, msgError)
	assert.Equal(t, errPortForwardUnkonwnMessageType.Error(), *msgError.Error)

	// new
	protocol := wspf.PortForwardProtocol(wspf.PortForwardProtocolTCP)
	remoteHost := "localhost"
	remotePort := uint16(getFreeTCPPort())
	portForwardNew := &wspf.PortForwardNew{
		Protocol:   &protocol,
		RemoteHost: &remoteHost,
		RemotePort: &remotePort,
	}
	body, _ := msgpack.Marshal(portForwardNew)
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardNew,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
		Body: body,
	}
	w = new(testWriter)
	handler.ServeProtoMsg(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypeError, rsp.Header.MsgType)

	msgError = &wspf.Error{}
	_ = msgpack.Unmarshal(rsp.Body, msgError)
	assert.Contains(t, *msgError.Error, "connect: connection refused")

	// new - unknown protocol
	protocol = "dummy"
	remoteHost = "localhost"
	remotePort = uint16(getFreeTCPPort())
	portForwardNew = &wspf.PortForwardNew{
		Protocol:   &protocol,
		RemoteHost: &remoteHost,
		RemotePort: &remotePort,
	}
	body, _ = msgpack.Marshal(portForwardNew)
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardNew,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
		Body: body,
	}
	w = new(testWriter)
	handler.ServeProtoMsg(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypeError, rsp.Header.MsgType)

	msgError = &wspf.Error{}
	_ = msgpack.Unmarshal(rsp.Body, msgError)
	assert.Contains(t, *msgError.Error, "unknown protocol: dummy")

	// stop
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardStop,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	w = new(testWriter)
	handler.ServeProtoMsg(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypeError, rsp.Header.MsgType)

	msgError = &wspf.Error{}
	_ = msgpack.Unmarshal(rsp.Body, msgError)
	assert.Equal(t, errPortForwardUnkonwnConnection.Error(), *msgError.Error)

	// forward
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForward,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	w = new(testWriter)
	handler.ServeProtoMsg(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypeError, rsp.Header.MsgType)

	msgError = &wspf.Error{}
	_ = msgpack.Unmarshal(rsp.Body, msgError)
	assert.Equal(t, errPortForwardUnkonwnConnection.Error(), *msgError.Error)
}

func TestPortForwardHandlerSuccessfulConnection(t *testing.T) {
	handler := PortForward()()

	// mock echo TCP server
	tcpPort := getFreeTCPPort()
	go func(tcpPort int) {
		l, err := net.Listen(wspf.PortForwardProtocolTCP, fmt.Sprintf("localhost:%d", tcpPort))
		if err != nil {
			panic(err)
		}
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				panic(err)
			}
			go func(conn net.Conn) {
				buf := make([]byte, 1024)
				for {
					n, err := conn.Read(buf)
					if err != nil && err == io.EOF {
						return
					} else if err != nil {
						panic(err)
					}
					response := strings.ToUpper(string(buf[:n]))
					_, err = conn.Write([]byte(response))
					if err != nil {
						panic(err)
					}
					if response == "STOP" {
						time.Sleep(50 * time.Millisecond)
						conn.Close()
						return
					}
				}
			}(conn)
		}
	}(tcpPort)

	time.Sleep(2000 * time.Millisecond)

	// c1: new
	protocol := wspf.PortForwardProtocol(wspf.PortForwardProtocolTCP)
	remoteHost := "localhost"
	remotePort := uint16(tcpPort)
	portForwardNew := &wspf.PortForwardNew{
		Protocol:   &protocol,
		RemoteHost: &remoteHost,
		RemotePort: &remotePort,
	}
	body, _ := msgpack.Marshal(portForwardNew)
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardNew,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
		Body: body,
	}
	w := new(testWriter)
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}

	rsp := w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypePortForwardNew, rsp.Header.MsgType)
	assert.Nil(t, rsp.Body)

	// c1: forward
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForward,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
		Body: []byte("abcdefghi"),
	}
	w.Messages = []*ws.ProtoMsg{}
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 2) {
		t.FailNow()
	}

	for _, rsp := range w.Messages {
		if rsp.Header.MsgType == wspf.MessageTypePortForwardAck {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForwardAck, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c1", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
		} else if rsp.Header.MsgType == wspf.MessageTypePortForward {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForward, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c1", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
			assert.Equal(t, []byte("ABCDEFGHI"), rsp.Body)
		}
	}

	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardAck,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	handler.ServeProtoMsg(msg, w)

	// c2: new
	protocol = wspf.PortForwardProtocol(wspf.PortForwardProtocolTCP)
	remoteHost = "localhost"
	remotePort = uint16(tcpPort)
	portForwardNew = &wspf.PortForwardNew{
		Protocol:   &protocol,
		RemoteHost: &remoteHost,
		RemotePort: &remotePort,
	}
	body, _ = msgpack.Marshal(portForwardNew)
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardNew,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c2",
			},
		},
		Body: body,
	}
	w.Messages = []*ws.ProtoMsg{}
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}

	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypePortForwardNew, rsp.Header.MsgType)
	assert.Equal(t, "session", rsp.Header.SessionID)
	assert.Equal(t, "c2", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
	assert.Nil(t, rsp.Body)

	// c1: forward, again
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForward,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
		Body: []byte("1234"),
	}
	w.Messages = []*ws.ProtoMsg{}
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 2) {
		t.FailNow()
	}

	for _, rsp := range w.Messages {
		if rsp.Header.MsgType == wspf.MessageTypePortForwardAck {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForwardAck, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c1", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
		} else if rsp.Header.MsgType == wspf.MessageTypePortForward {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForward, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c1", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
			assert.Equal(t, []byte("1234"), rsp.Body)
		}
	}

	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardAck,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	handler.ServeProtoMsg(msg, w)

	// c1: stop
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardStop,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	w.Messages = []*ws.ProtoMsg{}
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}

	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
	assert.Equal(t, wspf.MessageTypePortForwardStop, rsp.Header.MsgType)
	assert.Equal(t, "session", rsp.Header.SessionID)
	assert.Equal(t, "c1", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
	assert.Nil(t, rsp.Body)

	// c2: forward the message with the "stop" payload
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForward,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c2",
			},
		},
		Body: []byte("stop"),
	}
	w.Messages = []*ws.ProtoMsg{}
	handler.ServeProtoMsg(msg, w)

	time.Sleep(200 * time.Millisecond)
	if !assert.Len(t, w.Messages, 3) {
		t.FailNow()
	}

	for _, rsp := range w.Messages {
		if rsp.Header.MsgType == wspf.MessageTypePortForwardAck {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForwardAck, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c2", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
		} else if rsp.Header.MsgType == wspf.MessageTypePortForward {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForward, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c2", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
			assert.Equal(t, []byte("STOP"), rsp.Body)
		} else if rsp.Header.MsgType == wspf.MessageTypePortForwardStop {
			assert.Equal(t, ws.ProtoTypePortForward, rsp.Header.Proto)
			assert.Equal(t, wspf.MessageTypePortForwardStop, rsp.Header.MsgType)
			assert.Equal(t, "session", rsp.Header.SessionID)
			assert.Equal(t, "c2", rsp.Header.Properties[wspf.PropertyConnectionID].(string))
			assert.Nil(t, rsp.Body)
		}
	}

	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypePortForward,
			MsgType:   wspf.MessageTypePortForwardAck,
			SessionID: "session",
			Properties: map[string]interface{}{
				wspf.PropertyConnectionID: "c1",
			},
		},
	}
	handler.ServeProtoMsg(msg, w)
}
