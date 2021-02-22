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
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/stretchr/testify/assert"
)

type panicHandler struct {
	panicDelay time.Duration
}

func (p panicHandler) ServeProtoMsg(msg *ws.ProtoMsg, w ResponseWriter) {
	time.Sleep(p.panicDelay)
	panic("panicHandler")
}

func (p panicHandler) Close() error {
	return nil
}

func TestRouter(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		Routes ProtoRoutes
		Config Config

		Messages []ws.ProtoMsg

		Error error
	}{{
		Name: "ok",

		Routes: ProtoRoutes{
			ws.ProtoType(0x1234): func() SessionHandler {
				return new(echoHandler)
			},
		},
		Messages: []ws.ProtoMsg{{
			Header: ws.ProtoHdr{
				Proto:   ws.ProtoType(0x1234),
				MsgType: "hello?",
			}},
		},
		Config: Config{
			IdleTimeout: time.Second * 10,
		},
	}}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			r := NewRouter(tc.Routes, tc.Config)
			w := new(testWriter)
			var err error
			for i := range tc.Messages {
				err = r.RouteMessage(&tc.Messages[i], w)
				if i < len(tc.Messages)-1 {
					assert.NoError(t, err)
				}
			}
			if tc.Error != nil {
				assert.EqualError(t, err, tc.Error.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRouterRace(t *testing.T) {
	// Tests the race condition where the handler panics and closes while a
	// message is in the queue.
	t.Parallel()
	routes := ProtoRoutes{
		ws.ProtoType(0x4321): func() SessionHandler {
			return &panicHandler{panicDelay: time.Second}
		},
	}
	w := &testWriter{}
	router := NewRouter(routes, Config{IdleTimeout: time.Second * 30})
	err := router.RouteMessage(&ws.ProtoMsg{
		Header: ws.ProtoHdr{Proto: ws.ProtoType(0x4321)},
	}, w)
	assert.NoError(t, err)
	err = router.RouteMessage(&ws.ProtoMsg{
		Header: ws.ProtoHdr{Proto: ws.ProtoType(0x4321)},
	}, w)
	assert.EqualError(t, err, "session completed before message handoff")
}
