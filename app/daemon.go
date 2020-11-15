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
	"errors"
	"os/user"
	"strconv"
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

type MenderShellDaemon struct {
	stop             bool
	username         string
	shell            string
	serverUrl        string
	deviceConnectUrl string
	terminalString   string
	terminalWidth    uint16
	terminalHeight   uint16
	uid              uint64
	gid              uint64
	shellsSpawned    uint
}

func NewDaemon(config *configuration.MenderShellConfig) *MenderShellDaemon {
	daemon := MenderShellDaemon{
		stop:             false,
		username:         config.User,
		shell:            config.ShellCommand,
		serverUrl:        config.ServerURL,
		deviceConnectUrl: configuration.DefaultDeviceConnectPath,
		terminalString:   configuration.DefaultTerminalString,
		terminalWidth:    config.Terminal.Width,
		terminalHeight:   config.Terminal.Height,
		shellsSpawned:    0,
	}
	return &daemon
}

func (d *MenderShellDaemon) StopDaemon() {
	d.stop = true
}

func (d *MenderShellDaemon) shouldStop() bool {
	return d.stop
}

//starts all needed elements of the mender-shell daemon
// * executes given shell (shell.ExecuteShell)
// * get dbus API and starts the dbus main loop (dbus.GetDBusAPI(), go dbusAPI.MainLoopRun(loop))
// * creates a new dbus client and connects to dbus (mender.NewAuthClient(dbusAPI), client.Connect(...))
// * gets the JWT token from the mender-client via dbus (client.GetJWTToken())
// * connects to the backend and returns a new websocket (deviceconnect.Connect(...))
// * starts the message flow between the shell and websocket (shell.NewMenderShell(...))
func (d *MenderShellDaemon) Run() error {
	log.Infof("mender-shell starting shell: %s", d.shell)
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

	//get the JWT token from the client over dbus API
	value, err := client.GetJWTToken()
	if err != nil {
		log.Errorf("mender-shall dbus failed to get JWT token, error: %s", err.Error())
		return err
	}
	log.Debugf("mender-shell got len(JWT)=%d", len(value))

	//make websocket connection to the backend, this will be used to exchange messages
	log.Infof("mender-shell connecting websocket; url: %s%s", d.serverUrl, d.deviceConnectUrl)
	ws, err := deviceconnect.Connect(d.serverUrl, d.deviceConnectUrl, value)
	if err != nil {
		log.Errorf("mender-shall ws failed to connect to %s%s, error: %s", d.serverUrl, d.deviceConnectUrl, err.Error())
		return err
	}

	log.Infof("mender-shell entering main loop.")
	for {
		if d.shouldStop() {
			break
		}
		message, err := d.readMessage(ws)
		if err != nil {
			log.Errorf("main-loop: error reading message, attempting reconnect.")
			err = ws.Close()
			if err != nil {
				log.Errorf("main-loop: error on closing the connection")
			}

			for reconnectAttempts := configuration.MaxReconnectAttempts; reconnectAttempts > 0; reconnectAttempts-- {
				ws, err = deviceconnect.Connect(d.serverUrl, d.deviceConnectUrl, value)
				if err != nil {
					if reconnectAttempts == 1 {
						log.Errorf("main-loop ws failed to re-connect to %s%s, error: %s; giving up after %d tries", d.serverUrl, d.deviceConnectUrl, err.Error(), configuration.MaxReconnectAttempts)
						return err
					}
					log.Errorf("main-loop ws failed to connect to %s%s, error: %s", d.serverUrl, d.deviceConnectUrl, err.Error())
					time.Sleep(time.Second)
				} else {
					log.Info("reconnected")
					session.UpdateWSConnection(ws)
					break
				}
			}
			continue
		}

		log.Debugf("got message: type:%s data length:%d", message.Type, len(message.Data))
		err = d.routeMessage(ws, message)
		if err != nil {
			log.Debugf("error routing message")
		}
	}
	return nil
}

func responseMessage(ws *websocket.Conn, m *shell.MenderShellMessage) (err error) {
	data, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}
	err = ws.SetWriteDeadline(time.Now().Add(configuration.MessageWriteTimeout))
	err = ws.WriteMessage(websocket.BinaryMessage, data)

	return err
}

func (d *MenderShellDaemon) routeMessage(ws *websocket.Conn, message *shell.MenderShellMessage) (err error) {
	switch message.Type {
	case shell.MessageTypeSpawnShell:
		if d.shellsSpawned >= configuration.MaxShellsSpawned {
			return session.ErrSessionNotFound
		}
		s := session.MenderShellSessionGetById(message.SessionId)
		if s == nil {
			userId := string(message.Data)
			s, err = session.NewMenderShellSession(ws, userId)
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
			Width:          d.terminalHeight,
		})
		message := "Shell started"
		status := shell.NormalMessage
		if err != nil {
			message = "failed to start shell: " + err.Error()
			status = shell.ErrorMessage
		} else {
			log.Debugf("started shell")
			d.shellsSpawned++
		}

		err = responseMessage(ws, &shell.MenderShellMessage{
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
			rErr := responseMessage(ws, &shell.MenderShellMessage{
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
