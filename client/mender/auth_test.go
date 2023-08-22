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

package mender

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/northerntechhq/nt-connect/client/dbus"
	dbus_mocks "github.com/northerntechhq/nt-connect/client/dbus/mocks"
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
				response.On("GetTwoStrings").Return(JWTTokenValue, "")
			}

			dbusAPI := &dbus_mocks.DBusAPI{}
			defer dbusAPI.AssertExpectations(t)

			dbusAPI.On("BusProxyCall",
				dbus.Handle(nil),
				DBusMethodNameGetJwtToken,
				nil,
				DBusMethodTimeoutInMilliSeconds,
			).Return(response, tc.busProxyCallError)

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			value, _, err := client.GetJWTToken()
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
				DBusMethodTimeoutInMilliSeconds,
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

func TestAuthClientWaitForJwtTokenStateChange(t *testing.T) {
	testCases := map[string]struct {
		params []dbus.SignalParams
		err    error
	}{
		"ok-no-params": {
			err: nil,
		},
		"ok-with-params": {
			params: []dbus.SignalParams{
				{
					ParamType: "s",
					ParamData: "the token",
				},
				{
					ParamType: "i",
					ParamData: 15,
				},
			},
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
				DBusSignalNameJwtTokenStateChange,
				timeout,
			).Return(tc.params, tc.err)

			client, err := NewAuthClient(dbusAPI)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			params, err := client.WaitForJwtTokenStateChange()
			if tc.err != nil {
				assert.Error(t, err, tc.err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.params, params)
			}
		})
	}
}
