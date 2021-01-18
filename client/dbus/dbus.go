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
	"errors"
	"time"
	"unsafe"
)

// These are unsafe pointers, only prettier :)
type Handle unsafe.Pointer
type MainLoop unsafe.Pointer

var dbusAPI DBusAPI = nil

// DBusCallResponse stores the response of a method invocation
type DBusCallResponse interface {
	// GetString returns a string stored in the response object
	GetString() string
	// GetString returns two strings stored in the response object
	GetTwoStrings() (string, string)
	// GetBoolean returns a boolean stored in the response object
	GetBoolean() bool
}

type SignalParams struct {
	ParamType string
	ParamData interface{}
}

// DBusAPI is the interface which describes a DBus API
type DBusAPI interface {
	// BusGet synchronously connects to the message bus specified by bus_type
	BusGet(uint) (Handle, error)
	// BusProxyNew creates a proxy for accessing an interface over DBus
	BusProxyNew(Handle, string, string, string) (Handle, error)
	// BusProxyCall synchronously invokes a method method on a proxy
	BusProxyCall(Handle, string, interface{}, int) (DBusCallResponse, error)
	// MainLoopNew creates a new GMainLoop structure
	MainLoopNew() MainLoop
	// MainLoopRun runs a main loop until MainLoopQuit() is called
	MainLoopRun(MainLoop)
	// MainLoopQuit stops a main loop from running
	MainLoopQuit(MainLoop)
	// HandleSignal handles a DBus signal
	HandleSignal(signalName string, params []SignalParams)
	// GetChannelForSignal returns a channel that can be used to wait for signals
	GetChannelForSignal(signalName string) chan []SignalParams
	// WaitForSignal waits for a DBus signal
	WaitForSignal(signalName string, timeout time.Duration) ([]SignalParams, error)
}

// GetDBusAPI returns the global DBusAPI object
func GetDBusAPI() (DBusAPI, error) {
	if dbusAPI != nil {
		return dbusAPI, nil
	}
	return nil, errors.New("no D-Bus interface available")
}

func setDBusAPI(api DBusAPI) {
	dbusAPI = api
}
