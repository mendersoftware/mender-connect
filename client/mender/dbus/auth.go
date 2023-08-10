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

package dbus

import (
	"time"

	"github.com/mendersoftware/mender-connect/client/dbus"
	"github.com/mendersoftware/mender-connect/client/mender"
)

// DbBus constants for the Mender Authentication Manager
const (
	DBusObjectName                    = "io.mender.AuthenticationManager"
	DBusObjectPath                    = "/io/mender/AuthenticationManager"
	DBusInterfaceName                 = "io.mender.Authentication1"
	DBusMethodNameGetJwtToken         = "GetJwtToken"
	DBusMethodNameFetchJwtToken       = "FetchJwtToken"
	DBusSignalNameJwtTokenStateChange = "JwtTokenStateChange"
	DBusMethodTimeoutInMilliSeconds   = 5000
)

var timeout = 10 * time.Second

// AuthClientDBUS is the implementation of the client for the Mender
// Authentication Manager which communicates using DBUS
type AuthClientDBUS struct {
	dbusAPI          dbus.DBusAPI
	dbusConnection   dbus.Handle
	authManagerProxy dbus.Handle
}

// NewAuthClient returns a new AuthClient
func NewAuthClient(dbusAPI dbus.DBusAPI) (mender.AuthClient, error) {
	if dbusAPI == nil {
		var err error
		dbusAPI, err = dbus.GetDBusAPI()
		if err != nil {
			return nil, err
		}
	}
	return &AuthClientDBUS{
		dbusAPI: dbusAPI,
	}, nil
}

// Connect to the Mender client interface
func (a *AuthClientDBUS) Connect(objectName, objectPath, interfaceName string) error {
	dbusConnection, err := a.dbusAPI.BusGet(dbus.GBusTypeSystem)
	if err != nil {
		return err
	}
	authManagerProxy, err := a.dbusAPI.BusProxyNew(
		dbusConnection,
		objectName,
		objectPath,
		interfaceName,
	)
	if err != nil {
		return err
	}
	a.dbusConnection = dbusConnection
	a.authManagerProxy = authManagerProxy
	return nil
}

// GetJWTToken returns a device JWT token and server URL
func (a *AuthClientDBUS) GetJWTToken() (string, string, error) {
	response, err := a.dbusAPI.BusProxyCall(
		a.authManagerProxy,
		DBusMethodNameGetJwtToken,
		nil,
		DBusMethodTimeoutInMilliSeconds,
	)
	if err != nil {
		return "", "", err
	}
	token, serverURL := response.GetTwoStrings()
	return token, serverURL, nil
}

// FetchJWTToken schedules the fetching of a new device JWT token
func (a *AuthClientDBUS) FetchJWTToken() (bool, error) {
	response, err := a.dbusAPI.BusProxyCall(
		a.authManagerProxy,
		DBusMethodNameFetchJwtToken,
		nil,
		DBusMethodTimeoutInMilliSeconds,
	)
	if err != nil {
		return false, err
	}
	return response.GetBoolean(), nil
}

// WaitForJwtTokenStateChange synchronously waits for the JwtTokenStateChange signal
func (a *AuthClientDBUS) WaitForJwtTokenStateChange() ([]dbus.SignalParams, error) {
	return a.dbusAPI.WaitForSignal(DBusSignalNameJwtTokenStateChange, timeout)
}
