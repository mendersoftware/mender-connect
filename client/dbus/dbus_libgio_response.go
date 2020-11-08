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
)

type dbusCallResponseLibgio struct {
	ptr unsafe.Pointer
}

// NewDBusCallResponse returns a new DBusCallResponse struct
func NewDBusCallResponse(ptr unsafe.Pointer) DBusCallResponse {
	return &dbusCallResponseLibgio{ptr: ptr}
}

// GetString returns a string stored in the response object
func (r *dbusCallResponseLibgio) GetString() string {
	str := C.string_from_g_variant(C.to_gvariant(r.ptr))
	return goString(str)
}

// GetBoolean returns a boolean stored in the response object
func (r *dbusCallResponseLibgio) GetBoolean() bool {
	val := C.boolean_from_g_variant(C.to_gvariant(r.ptr))
	return goBool(val)
}
