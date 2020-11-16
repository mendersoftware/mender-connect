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

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMenderShellVersionUnknown(t *testing.T) {
	Version = ""
	v := VersionString()
	assert.Equal(t, v, VersionUnknown)
}

func TestMenderShellVersion(t *testing.T) {
	Version = "1.0"
	v := VersionString()
	assert.Equal(t, v, Version)
}

func TestMenderShowVersionCLI(t *testing.T) {
	Version = "1.0"
	err := ShowVersionCLI(nil)
	assert.NoError(t, err)
}

func TestMenderShowVersion(t *testing.T) {
	Version = "v1.0"
	v := ShowVersion()
	assert.True(t, strings.Contains(v, "v1.0"))
}
