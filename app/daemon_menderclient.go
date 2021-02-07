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
)

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
	if command != "" {
		return runCommand([]string{"mender", command})
	}
	return nil
}
