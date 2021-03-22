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
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"
	"github.com/pkg/errors"

	"github.com/mendersoftware/mender-connect/client/dbus"
	"github.com/mendersoftware/mender-connect/client/mender"
	"github.com/mendersoftware/mender-connect/config"
	"github.com/mendersoftware/mender-connect/connectionmanager"
	"github.com/mendersoftware/mender-connect/session"
)

var lastExpiredSessionSweep = time.Now()
var expiredSessionsSweepFrequency = time.Second * 32

const (
	EventReconnect             = "reconnect"
	EventReconnectRequest      = "reconnect-req"
	EventConnectionEstablished = "connected"
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
	serverUrl               string
	serverCertificate       string
	skipVerify              bool
	deviceConnectUrl        string
	expireSessionsAfter     time.Duration
	expireSessionsAfterIdle time.Duration
	terminalString          string
	uid                     uint64
	gid                     uint64
	homeDir                 string
	shellsSpawned           uint
	debug                   bool
	trace                   bool
	router                  session.Router
	config.TerminalConfig
	config.FileTransferConfig
	config.PortForwardConfig
	config.MenderClientConfig
}

func NewDaemon(conf *config.MenderShellConfig) *MenderShellDaemon {
	ctx, ctxCancel := context.WithCancel(context.Background())

	// Setup ProtoMsg routes.
	routes := make(session.ProtoRoutes)
	if !conf.Terminal.Disable {
		// Shell message is not handled by the Session, but the map
		// entry must be set to give the correct 'accept' response.
		routes[ws.ProtoTypeShell] = nil
	}
	if !conf.FileTransfer.Disable {
		routes[ws.ProtoTypeFileTransfer] = session.FileTransfer()
	}
	if !conf.PortForward.Disable {
		routes[ws.ProtoTypePortForward] = session.PortForward()
	}
	if !conf.MenderClient.Disable {
		routes[ws.ProtoTypeMenderClient] = session.MenderClient()
	}
	router := session.NewRouter(
		routes, session.Config{
			IdleTimeout: connectionmanager.DefaultPingWait,
		},
	)

	daemon := MenderShellDaemon{
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		writeMutex:              &sync.Mutex{},
		eventChan:               make(chan MenderShellDaemonEvent),
		connectionEstChan:       make(chan MenderShellDaemonEvent),
		reconnectChan:           make(chan MenderShellDaemonEvent),
		stop:                    false,
		authorized:              false,
		username:                conf.User,
		shell:                   conf.ShellCommand,
		serverUrl:               conf.ServerURL,
		serverCertificate:       conf.ServerCertificate,
		skipVerify:              conf.SkipVerify,
		expireSessionsAfter:     time.Second * time.Duration(conf.Sessions.ExpireAfter),
		expireSessionsAfterIdle: time.Second * time.Duration(conf.Sessions.ExpireAfterIdle),
		deviceConnectUrl:        config.DefaultDeviceConnectPath,
		terminalString:          config.DefaultTerminalString,
		TerminalConfig:          conf.Terminal,
		FileTransferConfig:      conf.FileTransfer,
		PortForwardConfig:       conf.PortForward,
		MenderClientConfig:      conf.MenderClient,
		shellsSpawned:           0,
		debug:                   conf.Debug,
		trace:                   conf.Trace,
		router:                  router,
	}

	connectionmanager.SetReconnectIntervalSeconds(conf.ReconnectIntervalSeconds)
	if conf.Sessions.MaxPerUser > 0 {
		session.MaxUserSessions = int(conf.Sessions.MaxPerUser)
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
	err = connectionmanager.Reconnect(
		ws.ProtoTypeShell, d.serverUrl,
		d.deviceConnectUrl, token,
		d.skipVerify, d.serverCertificate,
		config.MaxReconnectAttempts, d.ctx,
	)
	if err != nil {
		return errors.New("failed to reconnect after " + strconv.Itoa(int(config.MaxReconnectAttempts)) + " tries: " + err.Error())
	} else {
		return nil
	}
}

func (d *MenderShellDaemon) outputStatus() {
	log.Infof("mender-connect daemon v%s", config.VersionString())
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
	for {
		if d.shouldStop() {
			log.Trace("messageLoop: returning")
			break
		}

		log.Trace("messageLoop: calling readMessage")
		message, err := d.readMessage()
		log.Tracef("messageLoop: called readMessage: %v,%v", message, err)
		if err != nil {
			log.Errorf("messageLoop: error on readMessage: %v; disconnecting, waiting for reconnect.", err)
			connectionmanager.Close(ws.ProtoTypeShell)
			e := MenderShellDaemonEvent{
				event: EventReconnectRequest,
			}
			log.Tracef("messageLoop: posting event: %s; waiting for response", e.event)
			d.reconnectChan <- e
			response := <-d.connectionEstChan
			log.Tracef("messageLoop: got response: %+v", response)
			continue
		}

		log.Tracef("got message: type:%s data length:%d", message.Header.MsgType, len(message.Body))
		err = d.routeMessage(message)
		if err != nil {
			log.Tracef("error routing message: %s", err.Error())
		}
	}

	log.Trace("messageLoop: returning")
	return err
}

func (d *MenderShellDaemon) waitForJWTToken(client mender.AuthClient) (string, error) {
	tokenStateChange := client.GetJwtTokenStateChangeChannel()
	for {
		select {
		case p := <-tokenStateChange:
			if len(p) > 1 && p[1].ParamType == dbus.GDBusTypeString && len(p[1].ParamData.(string)) > 0 {
				d.serverUrl = p[1].ParamData.(string)
			}
			if len(p) > 0 && p[0].ParamType == dbus.GDBusTypeString && len(p[0].ParamData.(string)) > 0 {
				return p[0].ParamData.(string), nil
			}
		case <-d.ctx.Done():
			return "", errors.New("unable to get the JWT token")
		}
	}
}

func (d *MenderShellDaemon) gotAuthToken(p []dbus.SignalParams, needsReconnect bool) string {
	jwtToken := p[0].ParamData.(string)
	jwtTokenLength := len(jwtToken)
	if jwtTokenLength > 0 {
		if !d.authorized {
			log.Tracef("dbusEventLoop: StateChanged from unauthorized"+
				" to authorized, len(token)=%d", jwtTokenLength)
			//in hereT technically it is possible we close a closed connection
			//but it is not a critical error, the important thing is not to leave
			//messageLoop waiting forever on readMessage
			connectionmanager.Close(ws.ProtoTypeShell)
			if needsReconnect {
				e := MenderShellDaemonEvent{
					event: EventReconnect,
					data:  jwtToken,
					id:    "(gotAuthToken)",
				}
				log.Tracef("(gotAuthToken) posting Event: %s", e.event)
				d.postEvent(e)
			}
		}
		d.authorized = true
	} else {
		if d.authorized {
			log.Tracef("dbusEventLoop: StateChanged from authorized to unauthorized." +
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
	return jwtToken
}

func (d *MenderShellDaemon) needsReconnect() bool {
	select {
	case e := <-d.reconnectChan:
		log.Tracef("needsReconnect: got event: %s", e.event)
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
			log.Trace("dbusEventLoop: daemon needs to reconnect")
			needsReconnect = true
		}

		p, _ := client.WaitForJwtTokenStateChange()
		if len(p) > 0 && p[0].ParamType == dbus.GDBusTypeString {
			token := d.gotAuthToken(p, needsReconnect)
			if len(token) > 0 {
				log.Tracef("dbusEventLoop: got a token len=%d", len(token))
				needsReconnect = false
			}
		}
		if needsReconnect && d.authorized {
			jwtToken, serverURL, _ := client.GetJWTToken()
			e := MenderShellDaemonEvent{
				event: EventReconnect,
				data:  jwtToken,
				id:    "(dbusEventLoop)",
			}
			log.Tracef("(dbusEventLoop) posting Event: %s", e.event)
			d.serverUrl = serverURL
			d.postEvent(e)
			needsReconnect = false
		}
	}

	log.Trace("dbusEventLoop: returning")
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
		log.Tracef("eventLoop: got event: %s", event.event)
		switch event.event {
		case EventReconnect:
			err = connectionmanager.Reconnect(
				ws.ProtoTypeShell, d.serverUrl,
				d.deviceConnectUrl, event.data,
				d.skipVerify, d.serverCertificate,
				config.MaxReconnectAttempts, d.ctx,
			)
			if err != nil {
				log.Errorf("eventLoop: event: error reconnecting: %s", err.Error())
			} else {
				log.Trace("eventLoop: reconnected")
				d.connectionEstChan <- MenderShellDaemonEvent{
					event: EventConnectionEstablished,
				}
			}
		}
	}

	log.Trace("eventLoop: returning")
}

func (d *MenderShellDaemon) setupLogging() {
	if d.trace {
		log.SetLevel(log.TraceLevel)
	} else if d.debug {
		log.SetLevel(log.DebugLevel)
	}
}

//starts all needed elements of the mender-connect daemon
// * executes given shell (shell.ExecuteShell)
// * get dbus API and starts the dbus main loop (dbus.GetDBusAPI(), go dbusAPI.MainLoopRun(loop))
// * creates a new dbus client and connects to dbus (mender.NewAuthClient(dbusAPI), client.Connect(...))
// * gets the JWT token from the mender-client via dbus (client.GetJWTToken())
// * connects to the backend and returns a new websocket (deviceconnect.Connect(...))
// * starts the message flow between the shell and websocket (shell.NewMenderShell(...))
func (d *MenderShellDaemon) Run() error {
	d.setupLogging()

	log.Trace("daemon Run starting")
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

	log.Trace("mender-connect connecting to dbus")

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
		log.Errorf("mender-shall dbus failed to create client, error: %s", err.Error())
		return err
	}

	//connection to dbus
	err = client.Connect(mender.DBusObjectName, mender.DBusObjectPath, mender.DBusInterfaceName)
	if err != nil {
		log.Errorf("mender-shall dbus failed to connect, error: %s", err.Error())
		return err
	}

	jwtToken, serverURL, err := client.GetJWTToken()
	if err != nil {
		log.Warnf("call to GetJWTToken on the Mender D-Bus API failed: %v", err)
	}
	d.serverUrl = serverURL

	log.Tracef("GetJWTToken().len=%d", len(jwtToken))
	if len(jwtToken) < 1 {
		log.Info("waiting for JWT token (waitForJWTToken)")
		jwtToken, err = d.waitForJWTToken(client)
		if err != nil {
			return err
		}
		d.authorized = true
	} else {
		d.authorized = true
	}
	log.Tracef("mender-connect got len(JWT)=%d", len(jwtToken))

	err = connectionmanager.Connect(ws.ProtoTypeShell,
		d.serverUrl,
		d.deviceConnectUrl,
		jwtToken,
		d.skipVerify,
		d.serverCertificate,
		0,
		d.ctx,
	)
	if err != nil {
		log.Errorf("error on connecting, probably interrupted: %s", err.Error())
		return err
	}

	go d.messageLoop()
	go d.dbusEventLoop(client)
	go d.eventLoop()

	log.Trace("mender-connect entering main loop.")
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

	log.Trace("mainLoop: returning")
	return nil
}

func (d *MenderShellDaemon) responseMessage(msg *ws.ProtoMsg) (err error) {
	log.Tracef("responseMessage: webSock.WriteMessage(%+v)", msg)
	return connectionmanager.Write(ws.ProtoTypeShell, msg)
}

func (d *MenderShellDaemon) routeMessage(msg *ws.ProtoMsg) error {
	var err error
	// NOTE: the switch is required for backward compatibility, otherwise
	//       routing is performed and managed by the session.Router.
	//       Use the new API in sessions package (see filetransfer.go for an example)
	switch msg.Header.Proto {
	case ws.ProtoTypeShell:
		if d.TerminalConfig.Disable {
			break
		}
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
	default:
		return d.router.RouteMessage(
			msg, session.ResponseWriterFunc(d.responseMessage),
		)
	}
	err = errors.New(fmt.Sprintf("unknown message protocol and type: %d/%s", msg.Header.Proto, msg.Header.MsgType))
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

func (d *MenderShellDaemon) readMessage() (*ws.ProtoMsg, error) {
	msg, err := connectionmanager.Read(ws.ProtoTypeShell)
	log.Tracef("webSock.ReadMessage()=%+v,%v", msg, err)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
