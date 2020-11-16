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
	"runtime"
	"time"
	"unsafe"

	"github.com/pkg/errors"
)

type dbusAPILibGio struct {
	signals map[string]chan interface{}
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
	defer C.free(unsafe.Pointer(cname))
	cobjectPath := C.CString(objectPath)
	defer C.free(unsafe.Pointer(cobjectPath))
	cinterfaceName := C.CString(interfaceName)
	defer C.free(unsafe.Pointer(cinterfaceName))
	proxy := C.g_dbus_proxy_new_sync(gconn, flags, nil, cname, cobjectPath, cinterfaceName, nil, &gerror)
	if Handle(gerror) != nil {
		return Handle(nil), ErrorFromNative(Handle(gerror))
	} else if proxy == nil {
		return Handle(nil), errors.New("unable to create a new dbus proxy")
	}
	C.g_signal_connect_on_proxy(proxy)
	return Handle(proxy), nil
}

// BusProxyCall synchronously invokes a method method on a proxy
// https://developer.gnome.org/gio/stable/GDBusProxy.html#g-dbus-proxy-call-sync
func (d *dbusAPILibGio) BusProxyCall(proxy Handle, methodName string, params interface{}, timeout int) (DBusCallResponse, error) {
	var gerror *C.GError
	gproxy := C.to_gdbusproxy(unsafe.Pointer(proxy))
	cmethodName := C.CString(methodName)
	defer C.free(unsafe.Pointer(cmethodName))
	flags := C.GDBusCallFlags(GDBusCallFlagsNone)
	result := C.g_dbus_proxy_call_sync(gproxy, cmethodName, nil, flags, C.gint(timeout), nil, &gerror)
	if Handle(gerror) != nil {
		return nil, ErrorFromNative(Handle(gerror))
	}
	return NewDBusCallResponse(unsafe.Pointer(result)), nil
}

// MainLoopNew creates a new GMainLoop structure
// https://developer.gnome.org/glib/stable/glib-The-Main-Event-Loop.html#g-main-loop-new
func (d *dbusAPILibGio) MainLoopNew() MainLoop {
	loop := MainLoop(C.g_main_loop_new(nil, 0))
	runtime.SetFinalizer(&loop, func(loop *MainLoop) {
		gloop := C.to_gmainloop(unsafe.Pointer(*loop))
		C.g_main_loop_unref(gloop)
	})
	return loop
}

// MainLoopRun runs a main loop until MainLoopQuit() is called
// https://developer.gnome.org/glib/stable/glib-The-Main-Event-Loop.html#g-main-loop-run
func (d *dbusAPILibGio) MainLoopRun(loop MainLoop) {
	gloop := C.to_gmainloop(unsafe.Pointer(loop))
	go C.g_main_loop_run(gloop)
}

// MainLoopQuit stops a main loop from running
// https://developer.gnome.org/glib/stable/glib-The-Main-Event-Loop.html#g-main-loop-quit
func (d *dbusAPILibGio) MainLoopQuit(loop MainLoop) {
	gloop := C.to_gmainloop(unsafe.Pointer(loop))
	C.g_main_loop_quit(gloop)
}

func (d *dbusAPILibGio) getChannelForSignal(signalName string) chan interface{} {
	channel, ok := d.signals[signalName]
	if !ok {
		channel := make(chan interface{}, 1)
		d.signals[signalName] = channel
	}
	return channel
}

// DrainSignal drains the channel used to wait for signals
func (d *dbusAPILibGio) DrainSignal(signalName string) {
	channel := d.getChannelForSignal(signalName)
	select {
	case _ = <-channel:
	default:
	}
}

// HandleSignal handles a DBus signal
func (d *dbusAPILibGio) HandleSignal(signalName string) {
	channel := d.getChannelForSignal(signalName)
	select {
	case channel <- true:
	default:
	}
}

// WaitForSignal waits for a DBus signal
func (d *dbusAPILibGio) WaitForSignal(signalName string, timeout time.Duration) error {
	channel := d.getChannelForSignal(signalName)
	select {
	case <-channel:
		break
	case <-time.After(timeout):
		return errors.New("timeout waiting for signal " + signalName)
	}
	return nil
}

//export handle_on_signal_callback
func handle_on_signal_callback(proxy *C.GDBusProxy, senderName *C.gchar, signalName *C.gchar, params *C.GVariant, userData C.gpointer) {
	goSignalName := C.GoString(signalName)
	api, _ := GetDBusAPI()
	api.HandleSignal(goSignalName)
}

func newDBusAPILibGio() *dbusAPILibGio {
	return &dbusAPILibGio{
		signals: make(map[string]chan interface{}),
	}
}

func init() {
	dbusAPI = newDBusAPILibGio()
}
