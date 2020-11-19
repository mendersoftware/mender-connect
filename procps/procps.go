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
package procps

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func ProcessExists(pid int) bool {
	p, err := os.FindProcess(pid)
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

func TerminateAndWait(pid int, command *exec.Cmd, waitTimeout time.Duration) (err error) {
	p, _ := os.FindProcess(pid)
	p.Signal(syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	p.Signal(syscall.SIGKILL)
	time.Sleep(2 * time.Second)
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		if err != nil && err.Error() != "signal: killed" && err.Error() != "signal: hangup" {
			return errors.New("error waiting for the process: " + err.Error())
		}
	case <-time.After(waitTimeout):
		return errors.New("waiting for pid " + strconv.Itoa(pid) + " timeout. the process will remain as zombie.")
	}

	return nil
}
