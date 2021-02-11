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
	"os/exec"

	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/menderclient"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/mender-connect/connectionmanager"
)

const (
	NormalMessage int = iota + 1
	ErrorMessage
)

const propertyStatus = "status"

var runCommand = func(command []string) error {
	if len(command) > 0 {
		cmd := exec.Command(command[0], command[1:]...)
		return cmd.Run()
	}
	return errors.New("no command provided")
}

func processMessageMenderClient(message *ws.ProtoMsg) error {
	command := ""
	switch message.Header.MsgType {
	case menderclient.MessageTypeMenderClientCheckUpdate:
		command = "check-update"
	case menderclient.MessageTypeMenderClientSendInventory:
		command = "send-inventory"
	}
	var err error
	if command != "" {
		err = runCommand([]string{"mender", command})
	} else {
		err = errors.New("unknown message type")
	}
	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     message.Header.Proto,
			MsgType:   message.Header.MsgType,
			SessionID: message.Header.SessionID,
			Properties: map[string]interface{}{
				propertyStatus: int(NormalMessage),
			},
		},
	}
	if err != nil {
		response.Header.Properties[propertyStatus] = ErrorMessage
		response.Body = []byte(err.Error())
	}
	if err := connectionmanager.Write(ws.ProtoTypeShell, response); err != nil {
		log.Errorf("processMessageMenderClient: webSock.WriteMessage(%+v)", err)
	}
	return err
}
