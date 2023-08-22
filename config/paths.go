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

package config

import (
	"path"
	"time"
)

// default configuration paths
var (
	DefaultConfDir     = "/etc/nt-connect"
	DefaultPathDataDir = "/usr/share/nt-connect"
	DefaultDataStore   = "/var/lib/nt-connect"

	DefaultShellCommand      = "/bin/sh"
	DefaultShellArguments    = []string{"--login"}
	DefaultDeviceConnectPath = "/api/devices/v1/deviceconnect/connect"

	DefaultTerminalString = "xterm-256color"
	DefaultTerminalHeight = uint16(40)
	DefaultTerminalWidth  = uint16(80)

	DefaultConfFile         = path.Join(GetConfDirPath(), "nt-connect.conf")
	DefaultFallbackConfFile = path.Join(GetStateDirPath(), "nt-connect.conf")

	DefaultDebug = false
	DefaultTrace = false

	MaxReconnectAttempts             = uint(10)
	DefaultReconnectIntervalsSeconds = 5
	MessageWriteTimeout              = 2 * time.Second
	MaxShellsSpawned                 = uint(16)
)

// GetStateDirPath returns the default data store directory
func GetStateDirPath() string {
	return DefaultDataStore
}

// GetConfDirPath returns the default config directory
func GetConfDirPath() string {
	return DefaultConfDir
}
