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

	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/mender-shell/client/dbus"
	"github.com/mendersoftware/mender-shell/client/mender"
	configuration "github.com/mendersoftware/mender-shell/config"
	"github.com/mendersoftware/mender-shell/deviceconnect"
	"github.com/mendersoftware/mender-shell/shell"
)

type MenderShellDaemon struct {
	stop             bool
	username         string
	shell            string
	serverUrl        string
	deviceConnectUrl string
	terminalWidth    uint16
	terminalHeight   uint16
}

func NewDaemon(config *configuration.MenderShellConfig) *MenderShellDaemon {
	daemon := MenderShellDaemon{
		stop:             false,
		username:         config.User,
		shell:            config.ShellCommand,
		serverUrl:        config.ServerURL,
		deviceConnectUrl: configuration.DefaultDeviceConnectPath,
		terminalWidth:    config.Terminal.Width,
		terminalHeight:   config.Terminal.Height,
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

	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return err
	}

	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return err
	}

	r, w, err := shell.ExecuteShell(uint32(uid), uint32(gid), d.shell, d.terminalHeight, d.terminalWidth)
	if err != nil {
		log.Errorf("mender-shell failed to execute: %s error: %s", d.shell, err.Error())
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

	//MenderShell represents a process of passing messages between backend
	//and the shell subprocess (started above via shell.ExecuteShell) over
	//the websocket connection
	log.Info("mender-shell starting shell command passing process")
	shell := shell.NewMenderShell(ws, r, w)
	shell.Start()
	log.Infof("mender-shell entering main loop.")
	for {
		if d.shouldStop() {
			break
		}
		time.Sleep(15 * time.Second)
	}
	shell.Stop()
	return nil
}
