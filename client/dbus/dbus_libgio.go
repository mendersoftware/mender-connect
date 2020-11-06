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
// +build !nodbus,cgo

package dbus

// #cgo pkg-config: gio-2.0
// #include <gio/gio.h>
// #include "dbus_libgio.go.h"
import "C"
import (
	"unsafe"

	"github.com/pkg/errors"
)

type dbusAPILibGio struct {
}

// constants for GDBusProxyFlags
const (
	GDbusProxyFlagsNone                         = 0
	GDbusProxyFlagsDoNotLoadProperties          = (1 << 0)
	GDbusProxyFlagsDoNotConnectSignals          = (1 << 1)
	GDbusProxyFlagsDoNotAutoStart               = (1 << 2)
	GDbusProxyFlagsGetInvalidatedProperties     = (1 << 3)
	GDbusProxyFlagsDoNotAutoStartAtConstruction = (1 << 4)
)

// constants for GDBusCallFlags
const (
	GDBusCallFlagsNone                          = 0
	GDBusCallFlagsNoAutoStart                   = (1 << 0)
	GDBusCallFlagsAllowInteractiveAuthorization = (1 << 1)
)

// BusGet synchronously connects to the message bus specified by bus_type
// https://developer.gnome.org/gio/stable/GDBusConnection.html#g-bus-get-sync
func (d *dbusAPILibGio) BusGet(busType uint) (Handle, error) {
	var gerror *C.GError
	conn := C.g_bus_get_sync(C.GBusType(busType), nil, &gerror)
	if Handle(gerror) != nil {
		return Handle(nil), ErrorFromNative(Handle(gerror))
	}
	return Handle(conn), nil
}

// BusProxyNew creates a proxy for accessing an interface over DBus
// https://developer.gnome.org/gio/stable/GDBusProxy.html#g-dbus-proxy-new-sync
func (d *dbusAPILibGio) BusProxyNew(conn Handle, name string, objectPath string, interfaceName string) (Handle, error) {
	var gerror *C.GError
	gconn := C.to_gdbusconnection(unsafe.Pointer(conn))
	flags := C.GDBusProxyFlags(GDbusProxyFlagsDoNotLoadProperties)
	cname := C.CString(name)
	cobjectPath := C.CString(objectPath)
	cinterfaceName := C.CString(interfaceName)
	proxy := C.g_dbus_proxy_new_sync(gconn, flags, nil, cname, cobjectPath, cinterfaceName, nil, &gerror)
	if Handle(gerror) != nil {
		return Handle(nil), ErrorFromNative(Handle(gerror))
	} else if proxy == nil {
		return Handle(nil), errors.New("unable to create a new dbus proxy")
	}
	return Handle(proxy), nil
}

// BusProxyCall synchronously invokes a method method on a proxy
// https://developer.gnome.org/gio/stable/GDBusProxy.html#g-dbus-proxy-call-sync
func (d *dbusAPILibGio) BusProxyCall(proxy Handle, methodName string, params interface{}, timeout int) (DBusCallResponse, error) {
	var gerror *C.GError
	gproxy := C.to_gdbusproxy(unsafe.Pointer(proxy))
	cmethodName := C.CString(methodName)
	flags := C.GDBusCallFlags(GDBusCallFlagsNone)
	result := C.g_dbus_proxy_call_sync(gproxy, cmethodName, nil, flags, C.gint(timeout), nil, &gerror)
	if Handle(gerror) != nil {
		return nil, ErrorFromNative(Handle(gerror))
	}
	return NewDBusCallResponse(unsafe.Pointer(result)), nil
}

func init() {
	dbusAPI = &dbusAPILibGio{}
}
