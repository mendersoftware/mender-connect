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
package app

import (
	"testing"

	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/menderclient"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestProcessMessageMenderClient(t *testing.T) {
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
	err := processMessageMenderClient(msg)
	assert.Error(t, err)
	//
	msg = &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:   ws.ProtoTypeMenderClient,
			MsgType: menderclient.MessageTypeMenderClientSendInventory,
		},
	}
	err = processMessageMenderClient(msg)
	assert.Error(t, err)
}

func TestRunCommand(t *testing.T) {
	err := runCommand([]string{"false"})
	assert.Error(t, err)
	//
	err = runCommand([]string{})
	assert.Error(t, err)
}
