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
package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMainExitCodes(t *testing.T) {
	// Cache args.
	args := os.Args
	// Successful main call (0)
	os.Args = []string{"mender-connect", "--version"}
	exitCode := doMain()
	assert.Equal(t, 0, exitCode)
	os.Args = args
}

func TestMainRequiresConfig(t *testing.T) {
	args := os.Args
	os.Args = []string{"mender-connect", "daemon"}
	exitCode := doMain()
	//without any configuration we expect to fail the startup sequence
	assert.Equal(t, 1, exitCode)
	os.Args = args
}
