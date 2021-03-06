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

// Code generated by mockery v2.2.2. DO NOT EDIT.

package mocks

import (
	ws "github.com/mendersoftware/go-lib-micro/ws"
	session "github.com/mendersoftware/mender-connect/session"
	mock "github.com/stretchr/testify/mock"
)

// Router is an autogenerated mock type for the Router type
type Router struct {
	mock.Mock
}

// RouteMessage provides a mock function with given fields: msg, w
func (_m *Router) RouteMessage(msg *ws.ProtoMsg, w session.ResponseWriter) error {
	ret := _m.Called(msg, w)

	var r0 error
	if rf, ok := ret.Get(0).(func(*ws.ProtoMsg, session.ResponseWriter) error); ok {
		r0 = rf(msg, w)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
