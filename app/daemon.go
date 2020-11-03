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
package app

import (
	log "github.com/sirupsen/logrus"
	"time"
)

type MenderShellDaemon struct {
	stop bool
}

func NewDaemon() *MenderShellDaemon {
	daemon := MenderShellDaemon{
		stop: false,
	}
	return &daemon
}

func (d *MenderShellDaemon) StopDaemon() {
	d.stop = true
}

func (d *MenderShellDaemon) shouldStop() bool {
	return d.stop
}

func (d *MenderShellDaemon) Run() error {
	log.Infof("mender-shell entering main loop.")
	for {
		if d.shouldStop() {
			return nil
		}
		time.Sleep(15 * time.Second)
	}
	return nil
}
