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
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	wspf "github.com/mendersoftware/go-lib-micro/ws/portforward"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	portForwardBuffSize          = 4096
	portForwardConnectionTimeout = time.Second * 600

	propertyConnectionID      = "connection_id"
	MessageTypePortForwardAck = "ack"
)

var (
	errPortForwardInvalidMessage     = errors.New("invalid port-forward message: missing connection_id, remort_port or protocol")
	errPortForwardUnkonwnMessageType = errors.New("unknown message type")
	errPortForwardUnkonwnConnection  = errors.New("unknown connection")
)

type MenderPortForwarder struct {
	SessionID      string
	ConnectionID   string
	ResponseWriter ResponseWriter
	conn           net.Conn
	closed         bool
	ctx            context.Context
	ctxCancel      context.CancelFunc
	mutexAck       *sync.Mutex
	portForwarders map[string]*MenderPortForwarder
}

func (f *MenderPortForwarder) Connect(protocol string, host string, portNumber uint) error {
	log.Debugf("port-forward[%s/%s] connect: %s/%s:%d", f.SessionID, f.ConnectionID, protocol, host, portNumber)

	if protocol == wspf.PortForwardProtocolTCP || protocol == wspf.PortForwardProtocolUDP {
		conn, err := net.Dial(protocol, host+":"+strconv.Itoa(int(portNumber)))
		if err != nil {
			return err
		}
		f.conn = conn
	} else {
		return errors.New("unknown protocol: " + protocol)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	f.ctx = ctx
	f.ctxCancel = cancelFunc

	go f.Read()

	return nil
}

func (f *MenderPortForwarder) Close(sendStopMessage bool) error {
	if f.closed {
		return nil
	}
	f.closed = true
	log.Debugf("port-forward[%s/%s] close", f.SessionID, f.ConnectionID)
	if sendStopMessage {
		m := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypePortForward,
				MsgType:   wspf.MessageTypePortForwardStop,
				SessionID: f.SessionID,
				Properties: map[string]interface{}{
					propertyConnectionID: f.ConnectionID,
				},
			},
		}
		if err := f.ResponseWriter.WriteProtoMsg(m); err != nil {
			log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
		}
	}
	defer delete(f.portForwarders, f.SessionID)
	f.ctxCancel()
	return f.conn.Close()
}

func (f *MenderPortForwarder) Read() {
	errChan := make(chan error)
	dataChan := make(chan []byte)

	go func() {
		data := make([]byte, portForwardBuffSize)

		for {
			n, err := f.conn.Read(data)
			if err != nil {
				errChan <- err
				break
			}
			if n > 0 {
				tmp := make([]byte, n)
				copy(tmp, data[:n])
				dataChan <- tmp
			}
		}
	}()

	for {
		select {
		case err := <-errChan:
			if err != io.EOF {
				log.Errorf("port-forward[%s/%s] error: %v\n", f.SessionID, f.ConnectionID, err.Error())
			}
			f.Close(true)
		case data := <-dataChan:
			log.Debugf("port-forward[%s/%s] read %d bytes", f.SessionID, f.ConnectionID, len(data))

			// lock the ack mutex, we don't allow more than one in-flight message
			f.mutexAck.Lock()

			m := &ws.ProtoMsg{
				Header: ws.ProtoHdr{
					Proto:     ws.ProtoTypePortForward,
					MsgType:   wspf.MessageTypePortForward,
					SessionID: f.SessionID,
					Properties: map[string]interface{}{
						propertyConnectionID: f.ConnectionID,
					},
				},
				Body: data,
			}
			if err := f.ResponseWriter.WriteProtoMsg(m); err != nil {
				log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
			}
		case <-time.After(portForwardConnectionTimeout):
			f.Close(true)
		case <-f.ctx.Done():
			return
		}
	}
}

func (f *MenderPortForwarder) Write(body []byte) error {
	log.Debugf("port-forward[%s/%s] write %d bytes", f.SessionID, f.ConnectionID, len(body))
	_, err := f.conn.Write(body)
	if err != nil {
		return err
	}
	return nil
}

type PortForwardHandler struct {
	portForwarders map[string]*MenderPortForwarder
}

func PortForward() Constructor {
	return func() SessionHandler {
		return &PortForwardHandler{
			portForwarders: make(map[string]*MenderPortForwarder),
		}
	}
}

func (h *PortForwardHandler) Close() error {
	for _, f := range h.portForwarders {
		f.Close(true)
	}
	return nil
}

func (h *PortForwardHandler) ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter) {
	var err error
	switch msg.Header.MsgType {
	case wspf.MessageTypePortForwardNew:
		err = h.portForwardHandlerNew(msg, w)
	case wspf.MessageTypePortForwardStop:
		err = h.portForwardHandlerStop(msg, w)
	case wspf.MessageTypePortForward:
		err = h.portForwardHandlerForward(msg, w)
	case MessageTypePortForwardAck:
		err = h.portForwardHandlerAck(msg, w)
	default:
		err = errPortForwardUnkonwnMessageType
	}
	if err != nil {
		log.Errorf("portForwardHandler(%+v)", err)

		errMessage := err.Error()
		body, err := msgpack.Marshal(&wspf.Error{
			Error:       &errMessage,
			MessageType: &msg.Header.MsgType,
		})
		if err != nil {
			log.Errorf("portForwardHandler: msgpack.Marshal(%+v)", err)
		}
		response := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypePortForward,
				MsgType:   wspf.MessageTypeError,
				SessionID: msg.Header.SessionID,
			},
			Body: body,
		}
		if err := w.WriteProtoMsg(response); err != nil {
			log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
		}
	}
}

func (h *PortForwardHandler) portForwardHandlerNew(message *ws.ProtoMsg, w ResponseWriter) error {
	req := &wspf.PortForwardNew{}
	err := msgpack.Unmarshal(message.Body, req)
	if err != nil {
		return err
	}

	protocol := req.Protocol
	host := req.RemoteHost
	portNumber := req.RemotePort
	connectionID, _ := message.Header.Properties[propertyConnectionID].(string)

	if protocol == nil || *protocol == "" || host == nil || *host == "" || portNumber == nil || *portNumber == 0 || connectionID == "" {
		return errPortForwardInvalidMessage
	}

	portForwarder := &MenderPortForwarder{
		SessionID:      message.Header.SessionID,
		ConnectionID:   connectionID,
		ResponseWriter: w,
		mutexAck:       &sync.Mutex{},
		portForwarders: h.portForwarders,
	}

	key := message.Header.SessionID + "/" + connectionID
	h.portForwarders[key] = portForwarder

	log.Infof("port-forward: new %s: %s/%s:%d", key, *protocol, *host, *portNumber)
	err = portForwarder.Connect(string(*protocol), *host, *portNumber)
	if err != nil {
		delete(h.portForwarders, key)
		return err
	}

	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     message.Header.Proto,
			MsgType:   message.Header.MsgType,
			SessionID: message.Header.SessionID,
			Properties: map[string]interface{}{
				propertyConnectionID: connectionID,
			},
		},
	}
	if err := w.WriteProtoMsg(response); err != nil {
		log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
	}

	return nil
}

func (h *PortForwardHandler) portForwardHandlerStop(message *ws.ProtoMsg, w ResponseWriter) error {
	connectionID, _ := message.Header.Properties[propertyConnectionID].(string)
	key := message.Header.SessionID + "/" + connectionID
	if portForwarder, ok := h.portForwarders[key]; ok {
		log.Infof("port-forward: stop %s", key)
		defer delete(h.portForwarders, key)
		if err := portForwarder.Close(false); err != nil {
			return err
		}

		response := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     message.Header.Proto,
				MsgType:   message.Header.MsgType,
				SessionID: message.Header.SessionID,
				Properties: map[string]interface{}{
					propertyConnectionID: connectionID,
				},
			},
		}
		if err := w.WriteProtoMsg(response); err != nil {
			log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
		}

		return nil
	} else {
		return errPortForwardUnkonwnConnection
	}
}

func (h *PortForwardHandler) portForwardHandlerForward(message *ws.ProtoMsg, w ResponseWriter) error {
	connectionID, _ := message.Header.Properties[propertyConnectionID].(string)
	key := message.Header.SessionID + "/" + connectionID
	if portForwarder, ok := h.portForwarders[key]; ok {
		err := portForwarder.Write(message.Body)
		// send ack
		response := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     message.Header.Proto,
				MsgType:   MessageTypePortForwardAck,
				SessionID: message.Header.SessionID,
				Properties: map[string]interface{}{
					propertyConnectionID: connectionID,
				},
			},
		}
		if err := w.WriteProtoMsg(response); err != nil {
			log.Errorf("portForwardHandler: webSock.WriteMessage(%+v)", err)
		}
		return err
	} else {
		return errPortForwardUnkonwnConnection
	}
}

func (h *PortForwardHandler) portForwardHandlerAck(message *ws.ProtoMsg, w ResponseWriter) error {
	connectionID, _ := message.Header.Properties[propertyConnectionID].(string)
	key := message.Header.SessionID + "/" + connectionID
	if portForwarder, ok := h.portForwarders[key]; ok {
		// unlock the ack mutex, do not panic if it is not locked
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("portForwardHandlerAck: recover(%+v)", r)
			}
		}()
		portForwarder.mutexAck.Unlock()
		return nil
	}
	return errPortForwardUnkonwnConnection
}
