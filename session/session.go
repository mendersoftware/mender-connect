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

// Package session provides ProtoMsg Session abstraction. A session is a
// persistent communication channel with it's own set of control packets
// for opening and closing channels and performing end-to-end health checks.
// This package provides three levels of abstractions. On top, the Router
// manages creation and deletion as well as routing of messages to Sessions
// based on the ProtoMsg 'sid' (SessionID) header. The Session abstraction
// manages persistent session context such as creating and deleting as well as
// routing messages to SessionHandlers based on the 'proto' (ProtoType) header.
// The Session also takes care of session control messages. The SessionHandler
// is the application specific handler interface that takes care of the
// application specific messages. The ServeProtoMsg function is called for
// every inbound message with the associated ProtoType for the registered
// handler. If the SessionHandler requires persisting resources, Close MUST
// free these resources when the session shuts down.
// NOTE: ProtoRoutes that are used to map ProtoTypes to SessionHandlers maps
//       ProtoTypes to Constructors, since the Router must be able to regenerate
//       new Sessions with independent sets of SessionHandlers.
package session

//             +--------+ #sid +---------+ #proto +----------------+
// ProtoMsg -->| Router |----->| Session |------->| SessionHandler |
//             +--------+      +---------+        +----------------+

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"

	"github.com/mendersoftware/go-lib-micro/ws"
)

type ResponseWriter interface {
	WriteProtoMsg(msg *ws.ProtoMsg) error
}

type ResponseWriterFunc func(msg *ws.ProtoMsg) error

func (f ResponseWriterFunc) WriteProtoMsg(msg *ws.ProtoMsg) error { return f(msg) }

// SessionHandler defines the interface for application specific ProtoMsg handlers.
type SessionHandler interface {
	// ServeProtoMsg handles individual messages within the associated
	// class of ProtoTypes.
	ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter)
	// Close frees allocated resources when the session closes. It SHOULD
	// return an error if the session closes unexpectedly.
	Close() error
}

// Constructor is a function for SessionHandler initializers. To create a
// Router, all ProtoType Routes must route to a SessionHandler Constructor
// (factory).
type Constructor func() SessionHandler

// Config is the static configuration for Sessions and Routers.
type Config struct {
	// IdleTimeout is the duration a session can remain inactive before
	// it shuts down.
	IdleTimeout time.Duration
}

type Session struct {
	Config
	ID       string
	Routes   ProtoRoutes
	handlers map[ws.ProtoType]SessionHandler
	msgChan  chan *ws.ProtoMsg
	done     chan struct{}
	w        ResponseWriter
}

func New(
	sessionID string,
	msgChan chan *ws.ProtoMsg,
	w ResponseWriter,
	routes ProtoRoutes,
	config Config,
) *Session {
	return &Session{
		Config:   config,
		ID:       sessionID,
		Routes:   routes,
		handlers: make(map[ws.ProtoType]SessionHandler),
		msgChan:  msgChan,
		done:     make(chan struct{}),
		w:        w,
	}
}

func (sess *Session) Done() <-chan struct{} {
	return sess.done
}

func (sess *Session) MsgChan() chan<- *ws.ProtoMsg {
	return sess.msgChan
}

func (sess *Session) Error(msg *ws.ProtoMsg, close bool, errMessage string) {
	errSchema := ws.Error{
		Error:        errMessage,
		MessageProto: msg.Header.Proto,
		MessageType:  msg.Header.MsgType,
		Close:        close,
	}
	msgID, ok := msg.Header.Properties["msgid"].(string)
	if ok {
		errSchema.MessageID = msgID
	}
	b, _ := msgpack.Marshal(errSchema)
	err := sess.w.WriteProtoMsg(&ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeControl,
			MsgType:   ws.MessageTypeError,
			SessionID: sess.ID,
		},
		Body: b,
	})
	if err != nil {
		log.Errorf("failed to write response to client: %s", err.Error())
	}
}

func (sess *Session) HandleControl(msg *ws.ProtoMsg) (close bool) {
	switch msg.Header.MsgType {
	case ws.MessageTypePing:
		// Send pong
		pong := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeControl,
				MsgType:   ws.MessageTypePong,
				SessionID: msg.Header.SessionID,
			},
		}
		close = sess.w.WriteProtoMsg(pong) != nil

	case ws.MessageTypePong, ws.MessageTypeOpen:
		// No-op

	case ws.MessageTypeClose:
		close = true
		log.Infof("session: closed %s", msg.Header.SessionID)

	case ws.MessageTypeError:
		var errMsg ws.Error
		msgpack.Unmarshal(msg.Body, &errMsg) //nolint:errcheck
		log.Errorf("session: received error from client: %s", errMsg.Error)
		close = errMsg.Close

	default:
		sess.Error(msg, false, fmt.Sprintf(
			"session: control type message not understood: '%s'",
			msg.Header.MsgType,
		))
	}
	return
}

func (sess *Session) Ping() error {
	ping := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeControl,
			MsgType:   ws.MessageTypePing,
			SessionID: sess.ID,
		},
	}
	return sess.w.WriteProtoMsg(ping)
}

func funcname(fn string) string {
	// strip package path
	i := strings.LastIndex(fn, "/")
	fn = fn[i+1:]
	// strip package name.
	i = strings.Index(fn, ".")
	fn = fn[i+1:]
	return fn
}

// handlePanic recover from panics within session handlers by responding with
// an internal error message and dumping a log entry with the panic message and
// a stack trace.
func (sess *Session) handlePanic() {
	if r := recover(); r != nil {
		var stacktrace strings.Builder
		var trace [MaxTraceback]uintptr
		num := runtime.Callers(3, trace[:])
		for i := 0; i < num; i++ {
			fn := runtime.FuncForPC(trace[i])
			if fn == nil {
				fmt.Fprintf(&stacktrace, "\n???")
				continue
			}
			file, line := fn.FileLine(trace[i])
			fmt.Fprintf(&stacktrace, "\n%s:%d.%s",
				file, line, funcname(fn.Name()),
			)
		}
		log.WithField("trace", stacktrace.String()).
			Errorf("[panic] %s", r)
		sess.Error(&ws.ProtoMsg{}, true, "internal error")
	}
	close(sess.done)
}

func (sess *Session) ListenAndServe() {
	defer sess.handlePanic()
	var (
		msg       *ws.ProtoMsg
		open      bool
		sessIdle  bool
		pingWait  = (sess.Config.IdleTimeout * 4) / 5
		pongWait  = sess.Config.IdleTimeout - pingWait
		timerPing = time.NewTimer(pingWait)
	)
	select {
	case <-sess.done:
		panic("session already finished")
	default:
	}
	for {
		select {
		case <-timerPing.C:
			if sessIdle {
				// If the timer triggers twice without receiving
				// messages, we know the session timed out.
				sess.Error(&ws.ProtoMsg{}, true, "session timeout")
				return
			} else {
				// Send a ping and set the sessIdle flag, and
				// expect a new message to arrive before
				// pongWait (the remaining time before
				// Config.IdleTimeout)
				err := sess.Ping()
				if err != nil {
					log.Errorf("failed to ping client: %s", err.Error())
					return
				}
				sessIdle = true
				timerPing.Reset(pongWait)
			}
			continue

		case msg, open = <-sess.msgChan:
			if !open {
				return
			}
			// Always reset the ping timer and clear sessIdle flag
			// on incoming messages from the other peer.
			timerPing.Reset(pingWait)
			sessIdle = false
		}

		if msg.Header.Proto == ws.ProtoTypeControl {
			// Handle session control message.
			if sess.HandleControl(msg) {
				return
			}
			continue
		}

		// Lookup existing handlers for this session.
		handler, ok := sess.handlers[msg.Header.Proto]
		if !ok {
			// Try to create a new SessionHandler if the route exist.
			constructor, ok := sess.Routes[msg.Header.Proto]
			if !ok {
				sess.Error(msg, false, fmt.Sprintf(
					"no handler registered for protocol: 0x%04X",
					msg.Header.Proto,
				))
				continue
			}
			handler = constructor()
			defer handler.Close()
			sess.handlers[msg.Header.Proto] = handler
		}
		// Apply the SessionHandler.
		handler.ServeProtoMsg(msg, sess.w)
	}
}
