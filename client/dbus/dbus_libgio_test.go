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

package dbus

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var libgio *dbusAPILibGio

func TestMain(m *testing.M) {
	libgio = newDBusAPILibGio()
	setDBusAPI(libgio)
	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestBusGet(t *testing.T) {
	conn, err := libgio.BusGet(GBusTypeSystem)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
}

func TestBusProxyNew(t *testing.T) {
	testCases := map[string]struct {
		name          string
		objectPath    string
		interfaceName string
		err           bool
	}{
		"ok": {
			name:          "org.freedesktop.DBus",
			objectPath:    "/org/freedesktop/DBus",
			interfaceName: "org.freedesktop.DBus",
			err:           false,
		},
		"ko, wrong path": {
			name:          "org.freedesktop.DBus",
			objectPath:    "dummy",
			interfaceName: "org.freedesktop.DBus",
			err:           true,
		},
	}

	conn, err := libgio.BusGet(GBusTypeSystem)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			proxy, err := libgio.BusProxyNew(conn, tc.name, tc.objectPath, tc.interfaceName)
			if tc.err {
				assert.Error(t, err)
				assert.Equal(t, Handle(nil), proxy)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, Handle(nil), proxy)
			}
		})
	}
}

func TestBusProxyCall(t *testing.T) {
	testCases := map[string]struct {
		name          string
		objectPath    string
		interfaceName string
		methodName    string
		err           bool
	}{
		"ok": {
			name:          "org.freedesktop.DBus",
			objectPath:    "/org/freedesktop/DBus",
			interfaceName: "org.freedesktop.DBus",
			methodName:    "ListNames",
			err:           false,
		},
		"ko, already handled Hello message": {
			name:          "org.freedesktop.DBus",
			objectPath:    "/org/freedesktop/DBus",
			interfaceName: "org.freedesktop.DBus",
			methodName:    "Hello",
			err:           true,
		},
	}

	conn, err := libgio.BusGet(GBusTypeSystem)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			proxy, err := libgio.BusProxyNew(conn, tc.name, tc.objectPath, tc.interfaceName)
			assert.NoError(t, err)
			assert.NotEqual(t, Handle(nil), proxy)

			response, err := libgio.BusProxyCall(proxy, tc.methodName, nil, 10)
			if tc.err {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}
		})
	}
}

func TestMainLoop(t *testing.T) {
	loop := libgio.MainLoopNew()
	go libgio.MainLoopRun(loop)
	defer libgio.MainLoopQuit(loop)

	time.Sleep(100 * time.Millisecond)
}

func TestWaitForSignal(t *testing.T) {
	const signalName = "test"

	// received signal
	libgio.DrainSignal(signalName)
	go libgio.HandleSignal(signalName)
	err := libgio.WaitForSignal(signalName, 100*time.Millisecond)
	assert.NoError(t, err)

	// timeout
	libgio.DrainSignal(signalName)
	err = libgio.WaitForSignal(signalName, 100*time.Millisecond)
	assert.Error(t, err)

	// multiple drains are idempotent
	libgio.DrainSignal(signalName)
	libgio.DrainSignal(signalName)
}

func TestMenderclient(t *testing.T) {
	// we do not run this test, as we cannot depend on having the Mender client
	// running at unit test execution time. Still, we leave it here for debugging
	// purposes; comment out the t.Skip below if you want to manually run this test
	t.Skip("skip TestMenderclient because it requires a running Mender client")

	testCases := map[string]struct {
		name          string
		objectPath    string
		interfaceName string
		methodName    string
		signalName    string
		err           bool
	}{
		"GetJwtToken, with mender client running": {
			name:          "io.mender.AuthenticationManager",
			objectPath:    "/io/mender/AuthenticationManager",
			interfaceName: "io.mender.Authentication1",
			methodName:    "GetJwtToken",
			err:           false,
		},
		"FetchJwtToken, with mender client running": {
			name:          "io.mender.AuthenticationManager",
			objectPath:    "/io/mender/AuthenticationManager",
			interfaceName: "io.mender.Authentication1",
			methodName:    "FetchJwtToken",
			signalName:    "ValidJwtTokenAvailable",
			err:           false,
		},
	}

	conn, err := libgio.BusGet(GBusTypeSystem)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			proxy, err := libgio.BusProxyNew(conn, tc.name, tc.objectPath, tc.interfaceName)
			assert.NoError(t, err)
			assert.NotEqual(t, Handle(nil), proxy)

			if tc.signalName != "" {
				libgio.DrainSignal(tc.signalName)
			}

			loop := libgio.MainLoopNew()
			go libgio.MainLoopRun(loop)
			defer libgio.MainLoopQuit(loop)

			response, err := libgio.BusProxyCall(proxy, tc.methodName, nil, 10)
			if tc.err {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}

			if tc.signalName != "" {
				err = libgio.WaitForSignal(tc.signalName, 5*time.Second)
				assert.NoError(t, err)
			}
		})
	}
}
