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

	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/menderclient"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestMenderClientHandler(t *testing.T) {
	prevRunCommand := runCommand
	runCommand = func(command []string) error {
		return errors.New("error")
	}
	defer func() {
		runCommand = prevRunCommand
	}()
	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:   ws.ProtoTypeMenderClient,
			MsgType: menderclient.MessageTypeMenderClientCheckUpdate,
		},
	}
	w := new(testWriter)
	menderClientHandler(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp := w.Messages[0]
	assert.Equal(t, ws.ProtoTypeMenderClient, rsp.Header.Proto)
	if assert.Contains(t, rsp.Header.Properties, propertyStatus) {
		assert.Equal(t, ErrorMessage, rsp.Header.Properties[propertyStatus])
	}

	//
	w.Messages = w.Messages[:0]
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:   ws.ProtoTypeMenderClient,
			MsgType: menderclient.MessageTypeMenderClientSendInventory,
		},
	}
	menderClientHandler(msg, w)
	if !assert.Len(t, w.Messages, 1) {
		t.FailNow()
	}
	rsp = w.Messages[0]
	assert.Equal(t, ws.ProtoTypeMenderClient, rsp.Header.Proto)
	if assert.Contains(t, rsp.Header.Properties, propertyStatus) {
		assert.Equal(t, ErrorMessage, rsp.Header.Properties[propertyStatus])
	}
}

func TestRunCommand(t *testing.T) {
	err := runCommand([]string{"false"})
	assert.Error(t, err)
	//
	err = runCommand([]string{})
	assert.Error(t, err)
}
