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
	"sync"

	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/pkg/errors"
)

var (
	ErrNoSession   = errors.New("session: does not exist")
	ErrNoSessionID = errors.New("session: message does not have a session ID")
)

type ProtoRoutes map[ws.ProtoType]Constructor

func (r *ProtoRoutes) AddRoute(proto ws.ProtoType, init Constructor) *ProtoRoutes {
	if r == nil || *r == nil {
		routes := make(ProtoRoutes)
		r = &routes
	}
	(*r)[proto] = init
	return r
}

//go:generate ../utils/mockgen.sh
type Router interface {
	RouteMessage(msg *ws.ProtoMsg, w ResponseWriter) error
}

// router manages creation/deletion and routing of concurrent sessions.
type router struct {
	Config
	sessions sync.Map
	routes   ProtoRoutes
}

func NewRouter(routes ProtoRoutes, config Config) Router {
	return &router{
		Config:   config,
		sessions: sync.Map{},
		routes:   routes,
	}
}

func (mgr *router) startSession(sess *Session) {
	defer mgr.sessions.Delete(sess.ID)
	sess.ListenAndServe()
}

func (mgr *router) RouteMessage(msg *ws.ProtoMsg, w ResponseWriter) error {
	var sess *Session
	sessFace, ok := mgr.sessions.Load(msg.Header.SessionID)
	if !ok {
		msgChan := make(chan *ws.ProtoMsg)
		sess = New(msg.Header.SessionID, msgChan, w, mgr.routes, mgr.Config)
		mgr.sessions.Store(msg.Header.SessionID, sess)
		go mgr.startSession(sess)
	} else {
		sess = sessFace.(*Session)
	}
	select {
	case <-sess.Done():
		return errors.New("session completed before message handoff")
	case sess.MsgChan() <- msg:
	}
	return nil
}
