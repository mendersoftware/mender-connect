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
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"

	"github.com/mendersoftware/mender-connect/client/dbus"
	"github.com/mendersoftware/mender-connect/client/mender"
	configuration "github.com/mendersoftware/mender-connect/config"
	"github.com/mendersoftware/mender-connect/connectionmanager"
	"github.com/mendersoftware/mender-connect/procps"
	"github.com/mendersoftware/mender-connect/session"
	"github.com/mendersoftware/mender-connect/utils"
)

var lastExpiredSessionSweep = time.Now()
var expiredSessionsSweepFrequency = time.Second * 32

const (
	EventReconnect             = "reconnect"
	EventReconnectRequest      = "reconnect-req"
	EventConnectionEstablished = "connected"
	EventConnectionError       = "connected-error"
)

const (
	propertyTerminalHeight = "terminal_height"
	propertyTerminalWidth  = "terminal_width"
	propertyUserID         = "user_id"
)

type MenderShellDaemonEvent struct {
	event string
	data  string
	id    string
}

type MenderShellDaemon struct {
	ctx                     context.Context
	ctxCancel               context.CancelFunc
	writeMutex              *sync.Mutex
	eventChan               chan MenderShellDaemonEvent
	connectionEstChan       chan MenderShellDaemonEvent
	reconnectChan           chan MenderShellDaemonEvent
	stop                    bool
	authorized              bool
	printStatus             bool
	username                string
	shell                   string
	shellArguments          []string
	serverJwt               string
	serverUrl               string
	serverCertificate       string
	skipVerify              bool
	deviceConnectUrl        string
	expireSessionsAfter     time.Duration
	expireSessionsAfterIdle time.Duration
	terminalString          string
	terminalWidth           uint16
	terminalHeight          uint16
	uid                     uint64
	gid                     uint64
	homeDir                 string
	shellsSpawned           uint
	debug                   bool
}

func NewDaemon(config *configuration.MenderShellConfig) *MenderShellDaemon {
	ctx, ctxCancel := context.WithCancel(context.Background())

	daemon := MenderShellDaemon{
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		writeMutex:              &sync.Mutex{},
		eventChan:               make(chan MenderShellDaemonEvent),
		connectionEstChan:       make(chan MenderShellDaemonEvent),
		reconnectChan:           make(chan MenderShellDaemonEvent),
		stop:                    false,
		authorized:              false,
		username:                config.User,
		shell:                   config.ShellCommand,
		shellArguments:          config.ShellArguments,
		serverUrl:               config.ServerURL,
		serverCertificate:       config.ServerCertificate,
		skipVerify:              config.SkipVerify,
		expireSessionsAfter:     time.Second * time.Duration(config.Sessions.ExpireAfter),
		expireSessionsAfterIdle: time.Second * time.Duration(config.Sessions.ExpireAfterIdle),
		deviceConnectUrl:        configuration.DefaultDeviceConnectPath,
		terminalString:          configuration.DefaultTerminalString,
		terminalWidth:           config.Terminal.Width,
		terminalHeight:          config.Terminal.Height,
		shellsSpawned:           0,
		debug:                   config.Debug,
	}

	connectionmanager.SetReconnectIntervalSeconds(config.ReconnectIntervalSeconds)
	if config.Sessions.MaxPerUser > 0 {
		session.MaxUserSessions = int(config.Sessions.MaxPerUser)
	}
	return &daemon
}

func (d *MenderShellDaemon) StopDaemon() {
	d.stop = true
	d.ctxCancel()
}

func (d *MenderShellDaemon) PrintStatus() {
	d.printStatus = true
}

func (d *MenderShellDaemon) shouldStop() bool {
	return d.stop
}

func (d *MenderShellDaemon) shouldPrintStatus() bool {
	return d.printStatus
}

func (d *MenderShellDaemon) timeToSweepSessions() bool {
	if d.expireSessionsAfter == time.Duration(0) && d.expireSessionsAfterIdle == time.Duration(0) {
		return false
	}

	now := time.Now()
	nextSweepAt := lastExpiredSessionSweep.Add(expiredSessionsSweepFrequency)
	if now.After(nextSweepAt) {
		lastExpiredSessionSweep = now
		return true
	} else {
		return false
	}
}

func (d *MenderShellDaemon) wsReconnect(token string) (err error) {
	err = connectionmanager.Reconnect(ws.ProtoTypeShell, d.serverUrl, d.deviceConnectUrl, token, d.skipVerify, d.serverCertificate, configuration.MaxReconnectAttempts, d.ctx)
	if err != nil {
		return errors.New("failed to reconnect after " + strconv.Itoa(int(configuration.MaxReconnectAttempts)) + " tries: " + err.Error())
	} else {
		return nil
	}
}

func (d *MenderShellDaemon) outputStatus() {
	log.Infof("mender-connect daemon v%s", configuration.VersionString())
	log.Info(" status: ")
	log.Infof("  sessions: %d", session.MenderShellSessionGetCount())
	sessionIds := session.MenderShellSessionGetSessionIds()
	for _, id := range sessionIds {
		s := session.MenderShellSessionGetById(id)
		log.Infof("   id:%s status:%d started:%s", id, s.GetStatus(), s.GetStartedAtFmt())
		log.Infof("   expires:%s active:%s", s.GetExpiresAtFmt(), s.GetActiveAtFmt())
		log.Infof("   shell:%s", s.GetShellCommandPath())
	}
	d.printStatus = false
}

func (d *MenderShellDaemon) messageLoop() (err error) {
	log.Trace("messageLoop: starting")
	sendConnectReq := true
	waitConnectResp := true
	for {
		if d.shouldStop() {
			log.Debug("messageLoop: returning")
			break
		}

		if sendConnectReq {
			e := MenderShellDaemonEvent{
				event: EventReconnectRequest,
			}
			log.Debugf("messageLoop: posting event: %s; waiting for response", e.event)
			d.reconnectChan <- e
			sendConnectReq = false
		}

		if waitConnectResp {
			response := <-d.connectionEstChan
			log.Tracef("messageLoop: got response: %+v", response)
			if response.event == EventConnectionEstablished {
				waitConnectResp = false
			} else {
				// The re-connection failed, retry
				sendConnectReq = true
				waitConnectResp = true
				continue
			}
		}

		log.Trace("messageLoop: calling readMessage")
		message, err := d.readMessage()
		log.Tracef("messageLoop: called readMessage: %v,%v", message, err)
		if err != nil {
			log.Errorf(
				"messageLoop: error on readMessage: %v; disconnecting, waiting for reconnect.",
				err,
			)
			// nolint:lll
			// If we used a closed connection means that it has been closed from other goroutine
			// and a reconnection is ongoing (or done). Just wait for the event and continue
			// This can happen when dbusEventLoop detects a change in ServerURL and/or JWT token.
			// It should be safe to use this string, see:
			// https://github.com/golang/go/blob/529939072eef730c82333344f321972874758be8/src/net/error_test.go#L502-L507
			if !strings.Contains(err.Error(), "use of closed network connection") {
				connectionmanager.Close(ws.ProtoTypeShell)
				sendConnectReq = true
			}
			waitConnectResp = true
			continue
		}

		log.Debugf("got message: type:%s data length:%d", message.Header.MsgType, len(message.Body))
		err = d.routeMessage(message)
		if err != nil {
			log.Debugf("error routing message: %s", err.Error())
		}
	}

	log.Debug("messageLoop: returning")
	return err
}

func (d *MenderShellDaemon) processJwtTokenStateChange(jwtToken, serverUrl string) {
	jwtTokenLength := len(jwtToken)
	if jwtTokenLength > 0 && len(serverUrl) > 0 {
		if !d.authorized {
			log.Tracef("dbusEventLoop: StateChanged from unauthorized"+
				" to authorized, len(token)=%d, ServerURL=%q", jwtTokenLength, serverUrl)
			//in here it is technically possible that we close a closed connection
			//but it is not a critical error, the important thing is not to leave
			//messageLoop waiting forever on readMessage
			connectionmanager.Close(ws.ProtoTypeShell)
		}
		d.authorized = true
	} else {
		if d.authorized {
			log.Debugf("dbusEventLoop: StateChanged from authorized to unauthorized." +
				"terminating all sessions and disconnecting.")
			shellsCount, sessionsCount, err := session.MenderSessionTerminateAll()
			if err == nil {
				log.Infof("dbusEventLoop terminated %d sessions, %d shells",
					shellsCount, sessionsCount)
			} else {
				log.Errorf("dbusEventLoop error terminating all sessions: %s",
					err.Error())
			}
		}
		connectionmanager.Close(ws.ProtoTypeShell)
		d.authorized = false
	}
}

func (d *MenderShellDaemon) needsReconnect() bool {
	select {
	case e := <-d.reconnectChan:
		log.Debugf("needsReconnect: got event: %s", e.event)
		return true
	case <-time.After(time.Second):
		return false
	}
}

func (d *MenderShellDaemon) dbusEventLoop(client mender.AuthClient) {
	needsReconnect := false
	for {
		if d.shouldStop() {
			break
		}

		if d.needsReconnect() {
			log.Debug("dbusEventLoop: daemon needs to reconnect")
			needsReconnect = true
		}

		p, err := client.WaitForJwtTokenStateChange()
		log.Tracef("dbusEventLoop: WaitForJwtTokenStateChange %v, err %v", p, err)
		if len(p) > 1 &&
			p[0].ParamType == dbus.GDBusTypeString &&
			p[1].ParamType == dbus.GDBusTypeString {
			token := p[0].ParamData.(string)
			serverURL := p[1].ParamData.(string)
			d.processJwtTokenStateChange(token, serverURL)
			if len(token) > 0 && len(serverURL) > 0 {
				log.Tracef("dbusEventLoop: got a token len=%d, ServerURL=%s", len(token), serverURL)
				if token != d.serverJwt || serverURL != d.serverUrl {
					log.Debugf(
						"dbusEventLoop: new token or ServerURL, reconnecting. len=%d, ServerURL=%s",
						len(token),
						serverURL,
					)
					needsReconnect = true

					// If the server (Mender client proxy) closed the connection, it is likely that
					// both messageLoop is asking for a reconnection and we got JwtTokenStateChange
					// signal. So drain here the reconnect channel and reconnect only once
					if d.needsReconnect() {
						log.Debug("dbusEventLoop: drained reconnect req channel")
					}

				}
				// TODO: moving these assignments one scope up would make d.authorized redundant...
				d.serverJwt = token
				d.serverUrl = serverURL
			}
		}
		if needsReconnect && d.authorized {
			jwtToken, serverURL, _ := client.GetJWTToken()
			e := MenderShellDaemonEvent{
				event: EventReconnect,
				data:  jwtToken,
				id:    "(dbusEventLoop)",
			}
			log.Debugf("(dbusEventLoop) posting Event: %s", e.event)
			d.serverUrl = serverURL
			d.serverJwt = jwtToken
			d.postEvent(e)
			needsReconnect = false
		}
	}

	log.Debug("dbusEventLoop: returning")
}

func (d *MenderShellDaemon) postEvent(event MenderShellDaemonEvent) {
	d.eventChan <- event
}

func (d *MenderShellDaemon) readEvent() MenderShellDaemonEvent {
	return <-d.eventChan
}

func (d *MenderShellDaemon) eventLoop() {
	var err error
	for {
		if d.shouldStop() {
			break
		}

		event := d.readEvent()
		log.Debugf("eventLoop: got event: %s", event.event)
		switch event.event {
		case EventReconnect:
			err = connectionmanager.Reconnect(
				ws.ProtoTypeShell,
				d.serverUrl,
				d.deviceConnectUrl,
				event.data,
				d.skipVerify,
				d.serverCertificate,
				configuration.MaxReconnectAttempts,
				d.ctx,
			)
			var event string
			if err != nil {
				log.Errorf("eventLoop: error reconnecting: %s", err.Error())
				event = EventConnectionError
			} else {
				log.Infof("eventLoop: Connection established with %s", d.serverUrl)
				event = EventConnectionEstablished
			}
			d.connectionEstChan <- MenderShellDaemonEvent{
				event: event,
			}
		}
	}

	log.Debug("eventLoop: returning")
}

//starts all needed elements of the mender-connect daemon
// * executes given shell (shell.ExecuteShell)
// * get dbus API and starts the dbus main loop (dbus.GetDBusAPI(), go dbusAPI.MainLoopRun(loop))
// * creates a new dbus client and connects to dbus (mender.NewAuthClient(dbusAPI), client.Connect(...))
// * gets the JWT token from the mender-client via dbus (client.GetJWTToken())
// * connects to the backend and returns a new websocket (deviceconnect.Connect(...))
// * starts the message flow between the shell and websocket (shell.NewMenderShell(...))
func (d *MenderShellDaemon) Run() error {
	if d.debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Debug("daemon Run starting")
	u, err := user.Lookup(d.username)
	if err == nil && u == nil {
		return errors.New("unknown error while getting a user id")
	}
	if err != nil {
		return err
	}

	d.homeDir = u.HomeDir

	d.uid, err = strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return err
	}

	d.gid, err = strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return err
	}

	log.Debug("mender-connect connecting to dbus")

	dbusAPI, err := dbus.GetDBusAPI()
	if err != nil {
		return err
	}

	//dbus main loop, required.
	loop := dbusAPI.MainLoopNew()
	go dbusAPI.MainLoopRun(loop)
	defer dbusAPI.MainLoopQuit(loop)

	//new dbus client
	client, err := mender.NewAuthClient(dbusAPI)
	if err != nil {
		log.Errorf("mender-connect dbus failed to create client, error: %s", err.Error())
		return err
	}

	//connection to dbus
	err = client.Connect(mender.DBusObjectName, mender.DBusObjectPath, mender.DBusInterfaceName)
	if err != nil {
		log.Errorf("mender-connect dbus failed to connect, error: %s", err.Error())
		return err
	}

	jwtToken, serverURL, err := client.GetJWTToken()
	if err != nil {
		log.Warnf("call to GetJWTToken on the Mender D-Bus API failed: %v", err)
	}
	d.serverJwt = jwtToken
	d.serverUrl = serverURL
	if len(d.serverJwt) > 0 && len(d.serverUrl) > 0 {
		d.authorized = true
	}

	go d.messageLoop()
	go d.dbusEventLoop(client)
	go d.eventLoop()

	log.Debug("mender-connect entering main loop.")
	for {
		if d.shouldStop() {
			break
		}

		if d.shouldPrintStatus() {
			d.outputStatus()
		}

		if d.timeToSweepSessions() {
			shellStoppedCount, sessionStoppedCount, totalExpiredLeft, err := session.MenderSessionTerminateExpired()
			if err != nil {
				log.Errorf("main-loop: failed to terminate some expired sessions, left: %d",
					totalExpiredLeft)
			} else if sessionStoppedCount != 0 {
				log.Infof("main-loop: stopped %d sessions, %d shells, expired sessions left: %d",
					shellStoppedCount, sessionStoppedCount, totalExpiredLeft)
			}
		}

		time.Sleep(time.Second)
	}

	log.Debug("mainLoop: returning")
	return nil
}

func (d *MenderShellDaemon) responseMessage(msg *ws.ProtoMsg) (err error) {
	log.Debugf("responseMessage: webSock.WriteMessage(%+v)", msg)
	return connectionmanager.Write(ws.ProtoTypeShell, msg)
}

func (d *MenderShellDaemon) routeMessage(msg *ws.ProtoMsg) error {
	switch msg.Header.Proto {
	case ws.ProtoTypeShell:
		switch msg.Header.MsgType {
		case wsshell.MessageTypeSpawnShell:
			return d.routeMessageSpawnShell(msg)
		case wsshell.MessageTypeStopShell:
			return d.routeMessageStopShell(msg)
		case wsshell.MessageTypeShellCommand:
			return d.routeMessageShellCommand(msg)
		case wsshell.MessageTypeResizeShell:
			return d.routeMessageShellResize(msg)
		case wsshell.MessageTypePongShell:
			return d.routeMessagePongShell(msg)
		}
	}
	err := errors.New(fmt.Sprintf("unknown message protocol and type: %d/%s", msg.Header.Proto, msg.Header.MsgType))
	response := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     msg.Header.Proto,
			MsgType:   msg.Header.MsgType,
			SessionID: msg.Header.SessionID,
			Properties: map[string]interface{}{
				"status": wsshell.ErrorMessage,
			},
		},
		Body: []byte(err.Error()),
	}
	if err := d.responseMessage(response); err != nil {
		log.Errorf(errors.Wrap(err, "unable to send the response message").Error())
	}
	return err
}

func (d *MenderShellDaemon) routeMessageResponse(response *ws.ProtoMsg, err error) {
	if err != nil {
		log.Errorf(err.Error())
		response.Header.Properties["status"] = wsshell.ErrorMessage
		response.Body = []byte(err.Error())
	} else if response == nil {
		return
	}
	if err := d.responseMessage(response); err != nil {
		log.Errorf(errors.Wrap(err, "unable to send the response message").Error())
	}
}

func getUserIdFromMessage(message *ws.ProtoMsg) string {
	userID, _ := message.Header.Properties["user_id"].(string)
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
	if d.shellsSpawned >= configuration.MaxShellsSpawned {
		err = session.ErrSessionTooManyShellsAlreadyRunning
		d.routeMessageResponse(response, err)
		return err
	}
	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		userId := getUserIdFromMessage(message)
		if s, err = session.NewMenderShellSession(message.Header.SessionID, userId, d.expireSessionsAfter, d.expireSessionsAfterIdle); err != nil {
			d.routeMessageResponse(response, err)
			return err
		}
		log.Debugf("created a new session: %s", s.GetId())
	}

	response.Header.SessionID = s.GetId()

	terminalHeight := d.terminalHeight
	terminalWidth := d.terminalWidth

	requestedHeight, requestedWidth := mapPropertiesToTerminalHeightAndWidth(message.Header.Properties)
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
				d.shellsSpawned -= shellsStoppedCount
			}
		}
		d.routeMessageResponse(response, err)
		return err
	}

	s := session.MenderShellSessionGetById(message.Header.SessionID)
	if s == nil {
		err = errors.New(fmt.Sprintf("routeMessage: StopShellMessage: session not found for id %s", message.Header.SessionID))
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
		err = errors.Wrapf(err, "routeMessage: shell command execution error, session_id=%s", message.Header.SessionID)
		d.routeMessageResponse(response, err)
		return err
	}
	// suppress the response message setting the variable to nil
	response = nil
	_ = response
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

	terminalHeight, terminalWidth := mapPropertiesToTerminalHeightAndWidth(message.Header.Properties)
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

func (d *MenderShellDaemon) readMessage() (*ws.ProtoMsg, error) {
	msg, err := connectionmanager.Read(ws.ProtoTypeShell)
	log.Debugf("webSock.ReadMessage()=%+v,%v", msg, err)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
