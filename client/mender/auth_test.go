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

package mender

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/mender-shell/client/dbus"
	dbus_mocks "github.com/mendersoftware/mender-shell/client/dbus/mocks"
)

func TestNewAuthClientDefaultDBusAPI(t *testing.T) {
	client, err := NewAuthClient(nil)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewAuthClient(t *testing.T) {
	dbusAPI := &dbus_mocks.DBusAPI{}
	defer dbusAPI.AssertExpectations(t)

	client, err := NewAuthClient(dbusAPI)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestAuthClientConnect(t *testing.T) {
	testCases := map[string]struct {
		busGet           dbus.Handle
		busGetError      error
		busProxyNew      dbus.Handle
		busProxyNewError error
	}{
		"ok": {
			busGet:      dbus.Handle(nil),
			busProxyNew: dbus.Handle(nil),
		},
		"error BusGet": {
			busGet:      dbus.Handle(nil),
			busGetError: errors.New("error"),
			busProxyNew: dbus.Handle(nil),
		},
		"error ProxyNew": {
			busGet:           dbus.Handle(nil),
			busProxyNew:      dbus.Handle(nil),
			busProxyNewError: errors.New("error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("BusGet",
				uint(dbus.GBusTypeSystem),
			).Return(tc.busGet, tc.busGetError)

			if tc.busGetError == nil {
				dbusAPI.On("BusProxyNew",
					tc.busGet,
					DBusObjectName,
					DBusObjectPath,
					DBusInterfaceName,
				).Return(tc.busProxyNew, tc.busProxyNewError)
			}

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			err = client.Connect(DBusObjectName, DBusObjectPath, DBusInterfaceName)
			if tc.busGetError != nil {
				assert.Error(t, err, tc.busGetError)
			} else if tc.busProxyNewError != nil {
				assert.Error(t, err, tc.busProxyNewError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthClientGetJWTToken(t *testing.T) {
	const JWTTokenValue = "value"

	testCases := map[string]struct {
		busProxyCallError error
		result            string
	}{
		"ok": {
			result: JWTTokenValue,
		},
		"error": {
			busProxyCallError: errors.New("error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			response := &dbus_mocks.DBusCallResponse{}
			defer response.AssertExpectations(t)

			if tc.busProxyCallError == nil {
				response.On("GetString").Return(JWTTokenValue)
			}

			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("BusProxyCall",
				dbus.Handle(nil),
				DBusMethodNameGetJwtToken,
				nil,
				DBusMethodTimeoutInSeconds,
			).Return(response, tc.busProxyCallError)

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			value, err := client.GetJWTToken()
			if tc.busProxyCallError != nil {
				assert.Error(t, err, tc.busProxyCallError)
			} else {
				assert.Equal(t, value, JWTTokenValue)
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthClientFetchJWTToken(t *testing.T) {
	const returnValue = true

	testCases := map[string]struct {
		busProxyCallError error
		result            bool
	}{
		"ok": {
			result: returnValue,
		},
		"error": {
			busProxyCallError: errors.New("error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			response := &dbus_mocks.DBusCallResponse{}
			defer response.AssertExpectations(t)

			if tc.busProxyCallError == nil {
				response.On("GetBoolean").Return(returnValue)
			}

			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("BusProxyCall",
				dbus.Handle(nil),
				DBusMethodNameFetchJwtToken,
				nil,
				DBusMethodTimeoutInSeconds,
			).Return(response, tc.busProxyCallError)

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			value, err := client.FetchJWTToken()
			if tc.busProxyCallError != nil {
				assert.Error(t, err, tc.busProxyCallError)
			} else {
				assert.Equal(t, value, returnValue)
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthClientWaitForValidJWTTokenAvailable(t *testing.T) {
	testCases := map[string]struct {
		err error
	}{
		"ok": {
			err: nil,
		},
		"error": {
			err: errors.New("error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("WaitForSignal",
				DBusSignalNameValidJwtTokenAvailable,
				timeout,
			).Return(tc.err)

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			err = client.WaitForValidJWTTokenAvailable()
			if tc.err != nil {
				assert.Error(t, err, tc.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFetchAndGetJWTToken(t *testing.T) {
	testCases := map[string]struct {
		fetch    bool
		fetchErr error
		waitErr  error
		get      string
		getErr   error
		res      string
		resErr   error
	}{
		"ok": {
			fetch: true,
			get:   "token",
			res:   "token",
		},
		"ko, fetch error": {
			fetchErr: errors.New("fetch error"),
			resErr:   errors.New("fetch error"),
		},
		"ko, fetch false": {
			fetch:  false,
			resErr: errFetchTokenFailed,
		},
		"ko, wait error": {
			fetch:   true,
			waitErr: errors.New("timeout"),
			resErr:  errors.New("timeout"),
		},
		"ko, get error": {
			fetch:  true,
			getErr: errors.New("get error"),
			resErr: errors.New("get error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			response := &dbus_mocks.DBusCallResponse{}
			defer response.AssertExpectations(t)

			if tc.fetchErr == nil {
				response.On("GetBoolean").Return(tc.fetch)
			}

			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("BusProxyCall",
				dbus.Handle(nil),
				DBusMethodNameFetchJwtToken,
				nil,
				DBusMethodTimeoutInSeconds,
			).Return(response, tc.fetchErr)

			if tc.fetchErr == nil && tc.fetch == true {
				dbusAPI.On("WaitForSignal",
					DBusSignalNameValidJwtTokenAvailable,
					timeout,
				).Return(tc.waitErr)
			}

			if tc.fetchErr == nil && tc.fetch == true && tc.waitErr == nil {
				response := &dbus_mocks.DBusCallResponse{}
				defer response.AssertExpectations(t)

				if tc.getErr == nil {
					response.On("GetString").Return(tc.res)
				}

				dbusAPI.On("BusProxyCall",
					dbus.Handle(nil),
					DBusMethodNameGetJwtToken,
					nil,
					DBusMethodTimeoutInSeconds,
				).Return(response, tc.getErr)
			}

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			res, err := client.FetchAndGetJWTToken()
			assert.Equal(t, res, tc.res)
			if tc.resErr != nil {
				assert.Error(t, err, tc.resErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
