// Copyright 2020 Northern.tech AS
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
	"crypto/tls"
	"errors"
	"os/user"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mendersoftware/mender-shell/client/dbus"
	"github.com/mendersoftware/mender-shell/client/mender"
	configuration "github.com/mendersoftware/mender-shell/config"
	"github.com/mendersoftware/mender-shell/deviceconnect"
	"github.com/mendersoftware/mender-shell/session"
	"github.com/mendersoftware/mender-shell/shell"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"
)

var lastExpiredSessionSweep = time.Now()
var expiredSessionsSweepFrequency = time.Second * 32

var (
	ErrNilParameterUnexpected = errors.New("unexpected nil parameter")
)

type MenderShellDaemon struct {
	writeMutex              *sync.Mutex
	stop                    bool
	printStatus             bool
	username                string
	shell                   string
	serverUrl               string
	skipVerify              bool
	deviceConnectUrl        string
	expireSessionsAfter     time.Duration
	expireSessionsAfterIdle time.Duration
	terminalString          string
	terminalWidth           uint16
	terminalHeight          uint16
	uid                     uint64
	gid                     uint64
	shellsSpawned           uint
	debug                   bool
}

func NewDaemon(config *configuration.MenderShellConfig) *MenderShellDaemon {
	daemon := MenderShellDaemon{
		writeMutex:              &sync.Mutex{},
		stop:                    false,
		username:                config.User,
		shell:                   config.ShellCommand,
		serverUrl:               config.ServerURL,
		skipVerify:              config.SkipVerify,
		expireSessionsAfter:     time.Second * time.Duration(config.Sessions.ExpireAfter),
		expireSessionsAfterIdle: time.Second * time.Duration(config.Sessions.ExpireAfterIdle),
		deviceConnectUrl:        configuration.DefaultDeviceConnectPath,
		terminalString:          configuration.DefaultTerminalString,
		terminalWidth:           config.Terminal.Width,
		terminalHeight:          config.Terminal.Height,
		shellsSpawned:           0,
		debug:                   true,
	}

	if config.Sessions.MaxPerUser > 0 {
		session.MaxUserSessions = int(config.Sessions.MaxPerUser)
	}
	return &daemon
}

func (d *MenderShellDaemon) StopDaemon() {
	d.stop = true
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

func (d *MenderShellDaemon) wsReconnect(token string) (ws *websocket.Conn, err error) {
	for reconnectAttempts := configuration.MaxReconnectAttempts; reconnectAttempts > 0; reconnectAttempts-- {
		ws, err = deviceconnect.Connect(d.serverUrl, d.deviceConnectUrl, token)
		if err != nil {
			if reconnectAttempts == 1 {
				log.Errorf("main-loop ws failed to re-connect to %s%s, error: %s; giving up after %d tries", d.serverUrl, d.deviceConnectUrl, err.Error(), configuration.MaxReconnectAttempts)
				return nil, err
			}
			log.Errorf("main-loop ws failed to connect to %s%s, error: %s", d.serverUrl, d.deviceConnectUrl, err.Error())
			time.Sleep(time.Second)
		} else {
			log.Info("reconnected")
			session.UpdateWSConnection(ws)
			return ws, nil
		}
	}
	return nil, errors.New("failed to reconnect after " + strconv.Itoa(configuration.MaxReconnectAttempts) + " tries")
}

func (d *MenderShellDaemon) outputStatus() {
	log.Infof("mender-shell daemon v%s", configuration.VersionString())
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

func (d *MenderShellDaemon) messageMainLoop(ws *websocket.Conn, token string) (err error) {
	for {
		if d.shouldStop() {
			log.Info("messageMainLoop: returning")
			break
		}

		if d.shouldPrintStatus() {
			d.outputStatus()
		}

		var message *shell.MenderShellMessage
		log.Debugf("messageMainLoop: calling readMessage")
		message, err = d.readMessage(ws)
		log.Debugf("messageMainLoop: calling readMessage: %v,%v", message, err)
		if err != nil {
			log.Errorf("main-loop: error reading message: %s; attempting reconnect.", err.Error())
			if ws != nil {
				err = ws.Close()
				if err != nil {
					log.Errorf("main-loop: error on closing the connection: %s", err.Error())
				}
			}
			ws, err = d.wsReconnect(token)
			if err != nil {
				log.Errorf("main-loop: error on reconnect: %s", err.Error())
				break
			}
			continue
		}

		log.Debugf("got message: type:%s data length:%d", message.Type, len(message.Data))
		err = d.routeMessage(ws, message)
		if err != nil {
			log.Debugf("error routing message")
		}
	}

	return err
}

//returns true if GetJWTToken returns "" meaning that we lost auth status
//see below for some notes
func deviceUnauth(client mender.AuthClient) bool {
	jwtToken, err := client.GetJWTToken()
	if err == nil {
		//in case there is an error, it can mean that client was stopped, and/or there is
		//a problem with DBus communication. the decision here is: we only assume that
		//the device is unauthorized when there is a successful response from DBus
		//in hope to save someone when mender-shell is the last access point
		return jwtToken == ""
	}
	return false
}

func waitForJWTToken(client mender.AuthClient) (jwtToken string, err error) {
	for {
		jwtToken, err = client.GetJWTToken()
		if jwtToken != "" {
			log.Infof("JWT token is available.")
			break
		}
		time.Sleep(time.Second)
	}
	return jwtToken, nil
}

//starts all needed elements of the mender-shell daemon
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

	log.Infof("daemon Run starting")
	u, err := user.Lookup(d.username)
	if err == nil && u == nil {
		return errors.New("unknown error while getting a user id")
	}
	if err != nil {
		return err
	}

	d.uid, err = strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return err
	}

	d.gid, err = strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return err
	}

	log.Info("mender-shell connecting dbus and getting the token")
	//dbus main loop, required.
	dbusAPI, err := dbus.GetDBusAPI()
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

	log.Infof("waiting for JWT token (GetJWTToken)")
	jwtToken, err := waitForJWTToken(client)
	log.Debugf("mender-shell got len(JWT)=%d", len(jwtToken))

	// skip verification of HTTPS certificate if skipVerify is set in the config file
	if d.skipVerify {
		websocket.DefaultDialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	//make websocket connection to the backend, this will be used to exchange messages
	log.Infof("mender-shell connecting websocket; url: %s%s", d.serverUrl, d.deviceConnectUrl)
	ws, err := deviceconnect.Connect(d.serverUrl, d.deviceConnectUrl, jwtToken)
	if err != nil {
		log.Errorf("mender-shall ws failed to connect to %s%s, error: %s", d.serverUrl, d.deviceConnectUrl, err.Error())
		return err
	}

	go d.messageMainLoop(ws, jwtToken)

	log.Infof("mender-shell entering main loop.")
	for {
		if d.shouldStop() {
			break
		}

		if d.shouldPrintStatus() {
			d.outputStatus()
		}

		if deviceUnauth(client) {
			log.Warnf("device was denied authorization.")
			jwtToken, err = waitForJWTToken(client)
			d.wsReconnect(jwtToken)
		}

		if d.timeToSweepSessions() {
			shellStoppedCount, sessionStoppedCount, totalExpiredLeft, err := session.MenderSessionTerminateExpired()
			if err != nil {
				log.Errorf("main-loop: failed to terminate some expired sessions, left: %d", totalExpiredLeft)
			} else if sessionStoppedCount != 0 {
				log.Infof("main-loop: stopped %d sessions, %d shells, expired sessions left: %d", shellStoppedCount, sessionStoppedCount, totalExpiredLeft)
			}
		}

		time.Sleep(time.Second)
	}
	return nil
}

func (d *MenderShellDaemon) responseMessage(ws *websocket.Conn, m *shell.MenderShellMessage) (err error) {
	data, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	d.writeMutex.Lock()
	err = ws.SetWriteDeadline(time.Now().Add(configuration.MessageWriteTimeout))
	err = ws.WriteMessage(websocket.BinaryMessage, data)
	d.writeMutex.Unlock()

	return err
}

func (d *MenderShellDaemon) routeMessage(ws *websocket.Conn, message *shell.MenderShellMessage) (err error) {
	switch message.Type {
	case shell.MessageTypeSpawnShell:
		if d.shellsSpawned >= configuration.MaxShellsSpawned {
			return session.ErrSessionTooManyShellsAlreadyRunning
		}
		s := session.MenderShellSessionGetById(message.SessionId)
		if s == nil {
			userId := string(message.Data)
			s, err = session.NewMenderShellSession(d.writeMutex, ws, userId, d.expireSessionsAfter, d.expireSessionsAfterIdle)
			if err != nil {
				return err
			}
			log.Debugf("created a new session: %s", s.GetId())
		}

		log.Debugf("starting shell session_id=%s", s.GetId())
		err = s.StartShell(s.GetId(), session.MenderShellTerminalSettings{
			Uid:            uint32(d.uid),
			Gid:            uint32(d.gid),
			Shell:          d.shell,
			TerminalString: d.terminalString,
			Height:         d.terminalHeight,
			Width:          d.terminalWidth,
		})

		message := "Shell started"
		status := shell.NormalMessage
		if err != nil {
			log.Errorf("failed to start shell: %s", err.Error())
			message = "failed to start shell: " + err.Error()
			status = shell.ErrorMessage
		} else {
			log.Debugf("started shell")
			d.shellsSpawned++
		}

		err = d.responseMessage(ws, &shell.MenderShellMessage{
			Type:      shell.MessageTypeSpawnShell,
			Status:    status,
			SessionId: s.GetId(),
			Data:      []byte(message),
		})
		return err
	case shell.MessageTypeStopShell:
		if len(message.SessionId) < 1 {
			userId := string(message.Data)
			if len(userId) < 1 {
				log.Error("routeMessage: StopShellMessage: sessionId not given and userId empty")
				return errors.New("StopShellMessage: sessionId not given and userId empty")
			}
			shellsStoppedCount, err := session.MenderShellStopByUserId(userId)
			if err == nil {
				if shellsStoppedCount > d.shellsSpawned {
					log.Errorf("StopByUserId: the shells stopped count (%d)"+
						"greater than total shells spawned (%d). resetting shells"+
						"spawned to 0.", shellsStoppedCount, d.shellsSpawned)
					d.shellsSpawned = 0
				} else {
					log.Debugf("StopByUserId: stopped %d shells.", shellsStoppedCount)
					d.shellsSpawned -= shellsStoppedCount
				}
			}
			return err
		}
		s := session.MenderShellSessionGetById(message.SessionId)
		if s == nil {
			log.Infof("routeMessage: StopShellMessage: session not found for id %s", message.SessionId)
			return err
		}

		err = s.StopShell()
		if err != nil {
			rErr := d.responseMessage(ws, &shell.MenderShellMessage{
				Type:      shell.MessageTypeSpawnShell,
				Status:    shell.ErrorMessage,
				SessionId: s.GetId(),
				Data:      []byte("failed to stop shell: " + err.Error()),
			})
			if rErr != nil {
				log.Errorf("failed to send response (%s) to failed stop-shell command (%s)", rErr.Error(), err.Error())
			} else {
				log.Errorf("failed to stop shell: %s", err.Error())
			}
			return err
		} else {
			if d.shellsSpawned == 0 {
				log.Error("can't decrement shellsSpawned count: it is 0.")
			} else {
				d.shellsSpawned--
			}
		}
		return session.MenderShellDeleteById(s.GetId())
	case shell.MessageTypeShellCommand:
		s := session.MenderShellSessionGetById(message.SessionId)
		if s == nil {
			log.Debugf("routeMessage: session not found for id %s", message.SessionId)
			return session.ErrSessionNotFound
		}

		err = s.ShellCommand(message)
		if err != nil {
			log.Debugf("routeMessage: shell command execution error, session_id=%s", message.SessionId)
			return err
		}
	}
	return nil
}

func (d *MenderShellDaemon) readMessage(ws *websocket.Conn) (*shell.MenderShellMessage, error) {
	if ws == nil {
		return nil, ErrNilParameterUnexpected
	}

	_, data, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}

	m := &shell.MenderShellMessage{}
	err = msgpack.Unmarshal(data, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
