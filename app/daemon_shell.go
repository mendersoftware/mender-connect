// Copyright 2022 Northern.tech AS
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
	"fmt"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/northerntechhq/nt-connect/config"
	"github.com/northerntechhq/nt-connect/procps"
	"github.com/northerntechhq/nt-connect/session"
	"github.com/northerntechhq/nt-connect/utils"
)

const (
	propertyTerminalHeight = "terminal_height"
	propertyTerminalWidth  = "terminal_width"
	propertyUserID         = "user_id"
)

func getUserIdFromMessage(message *ws.ProtoMsg) string {
	userID, _ := message.Header.Properties[propertyUserID].(string)
	return userID
}

func (d *MenderShellDaemon) routeMessageSpawnShell(message *ws.ProtoMsg) error {
	var err error
	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     message.Header.Proto,
			MsgType:   message.Header.MsgType,
			SessionID: message.Header.SessionID,
			Properties: map[string]interface{}{
				"status": wsshell.NormalMessage,
			},
		},
		Body: []byte{},
	}
	if d.shellsSpawned >= config.MaxShellsSpawned {
		err = session.ErrSessionTooManyShellsAlreadyRunning
		d.routeMessageResponse(response, err)
		return err
	}
	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		userId := getUserIdFromMessage(message)
		if s, err = session.NewMenderShellSession(
			message.Header.SessionID,
			userId,
			d.expireSessionsAfter,
			d.expireSessionsAfterIdle,
		); err != nil {
			d.routeMessageResponse(response, err)
			return err
		}
		log.Debugf("created a new session: %s", s.GetId())
	}

	response.Header.SessionID = s.GetId()

	terminalHeight := d.TerminalConfig.Height
	terminalWidth := d.TerminalConfig.Width

	requestedHeight, requestedWidth := mapPropertiesToTerminalHeightAndWidth(
		message.Header.Properties,
	)
	if requestedHeight > 0 && requestedWidth > 0 {
		terminalHeight = requestedHeight
		terminalWidth = requestedWidth
	}

	log.Debugf("starting shell session_id=%s", s.GetId())
	if err = s.StartShell(s.GetId(), session.MenderShellTerminalSettings{
		Uid:            uint32(d.uid),
		Gid:            uint32(d.gid),
		Shell:          d.shell,
		HomeDir:        d.homeDir,
		TerminalString: d.terminalString,
		Height:         terminalHeight,
		Width:          terminalWidth,
		ShellArguments: d.shellArguments,
	}); err != nil {
		err = errors.Wrap(err, "failed to start shell")
		d.routeMessageResponse(response, err)
		return err
	}

	log.Debug("Shell started")
	d.shellsSpawned++

	response.Body = []byte("Shell started")
	d.routeMessageResponse(response, err)
	return nil
}

func (d *MenderShellDaemon) routeMessageStopShell(message *ws.ProtoMsg) error {
	var err error
	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     message.Header.Proto,
			MsgType:   message.Header.MsgType,
			SessionID: message.Header.SessionID,
			Properties: map[string]interface{}{
				"status": wsshell.NormalMessage,
			},
		},
		Body: []byte{},
	}

	if len(message.Header.SessionID) < 1 {
		userId := getUserIdFromMessage(message)
		if len(userId) < 1 {
			err = errors.New("StopShellMessage: sessionId not given and userId empty")
			d.routeMessageResponse(response, err)
			return err
		}
		shellsStoppedCount, err := session.MenderShellStopByUserId(userId)
		if err == nil {
			if shellsStoppedCount > d.shellsSpawned {
				d.shellsSpawned = 0
				err = errors.New(fmt.Sprintf("StopByUserId: the shells stopped count (%d) "+
					"greater than total shells spawned (%d). resetting shells "+
					"spawned to 0.", shellsStoppedCount, d.shellsSpawned))
				d.routeMessageResponse(response, err)
				return err
			} else {
				log.Debugf("StopByUserId: stopped %d shells.", shellsStoppedCount)
				d.DecreaseSpawnedShellsCount(shellsStoppedCount)
			}
		}
		d.routeMessageResponse(response, err)
		return err
	}

	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		err = errors.New(
			fmt.Sprintf(
				"routeMessage: StopShellMessage: session not found for id %s",
				message.Header.SessionID,
			),
		)
		d.routeMessageResponse(response, err)
		return err
	}

	err = s.StopShell()
	if err != nil {
		if procps.ProcessExists(s.GetShellPid()) {
			log.Errorf("could not terminate shell (pid %d) for session %s, user"+
				"will not be able to start another one if the limit is reached.",
				s.GetShellPid(),
				s.GetId())
			err = errors.New("could not terminate shell: " + err.Error() + ".")
			d.routeMessageResponse(response, err)
			return err
		} else {
			log.Errorf("process error on exit: %s", err.Error())
		}
	}
	if d.shellsSpawned == 0 {
		log.Warn("can't decrement shellsSpawned count: it is 0.")
	} else {
		d.shellsSpawned--
	}
	err = session.MenderShellDeleteById(s.GetId())
	d.routeMessageResponse(response, err)
	return err
}

func (d *MenderShellDaemon) routeMessageShellCommand(message *ws.ProtoMsg) error {
	var err error
	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     message.Header.Proto,
			MsgType:   message.Header.MsgType,
			SessionID: message.Header.SessionID,
			Properties: map[string]interface{}{
				"status": wsshell.NormalMessage,
			},
		},
		Body: []byte{},
	}

	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		err = session.ErrSessionNotFound
		d.routeMessageResponse(response, err)
		return err
	}
	err = s.ShellCommand(message)
	if err != nil {
		err = errors.Wrapf(
			err,
			"routeMessage: shell command execution error, session_id=%s",
			message.Header.SessionID,
		)
		d.routeMessageResponse(response, err)
		return err
	}
	return nil
}

func mapPropertiesToTerminalHeightAndWidth(properties map[string]interface{}) (uint16, uint16) {
	var terminalHeight, terminalWidth uint16
	requestedHeight, requestedHeightOk := properties[propertyTerminalHeight]
	requestedWidth, requestedWidthOk := properties[propertyTerminalWidth]
	if requestedHeightOk && requestedWidthOk {
		if val, _ := utils.Num64(requestedHeight); val > 0 {
			terminalHeight = uint16(val)
		}
		if val, _ := utils.Num64(requestedWidth); val > 0 {
			terminalWidth = uint16(val)
		}
	}
	return terminalHeight, terminalWidth
}

func (d *MenderShellDaemon) routeMessageShellResize(message *ws.ProtoMsg) error {
	var err error

	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		err = session.ErrSessionNotFound
		log.Errorf(err.Error())
		return err
	}

	terminalHeight, terminalWidth := mapPropertiesToTerminalHeightAndWidth(
		message.Header.Properties,
	)
	if terminalHeight > 0 && terminalWidth > 0 {
		s.ResizeShell(terminalHeight, terminalWidth)
	}
	return nil
}

func (d *MenderShellDaemon) routeMessagePongShell(message *ws.ProtoMsg) error {
	var err error

	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		err = session.ErrSessionNotFound
		log.Errorf(err.Error())
		return err
	}

	s.HealthcheckPong()
	return nil
}
