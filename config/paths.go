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

package config

import "path"

// default configuration paths
var (
	DefaultConfDir     = "/etc/mender"
	DefaultPathDataDir = "/usr/share/mender"
	DefaultDataStore   = "/var/lib/mender"

	DefaultShellCommand      = "/bin/sh"
	DefaultDeviceConnectPath = "/api/devices/v1/deviceconnect/connect"

	DefaultTerminalHeight = uint16(40)
	DefaultTerminalWidth  = uint16(80)

	DefaultConfFile         = path.Join(GetConfDirPath(), "mender-shell.conf")
	DefaultFallbackConfFile = path.Join(GetStateDirPath(), "mender-shell.conf")
)

// GetStateDirPath returns the default data store directory
func GetStateDirPath() string {
	return DefaultDataStore
}

// GetConfDirPath returns the default config directory
func GetConfDirPath() string {
	return DefaultConfDir
}
