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
package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	log "github.com/sirupsen/logrus"
)

const defaultCmdDir = "/"

func ExecuteShell(uid uint32,
	gid uint32,
	homeDir string,
	shell string,
	termString string,
	height uint16,
	width uint16,
	shellArguments []string) (pid int, pseudoTTY *os.File, cmd *exec.Cmd, err error) {

	cmd = exec.Command(shell, shellArguments...)

	currentUser, err := user.Current()
	if err != nil {
		log.Debugf("cant get current user: %s", err.Error())
		return -1, nil, nil, errors.New("unknown error with exec.Command(" + shell + ")")
	}

	//in order to set uid and gid we have to be root, at the moment lets check
	//if our uid is 0
	if currentUser.Uid == "0" {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
	}

	if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
		cmd.Dir = homeDir
	} else {
		cmd.Dir = defaultCmdDir
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", homeDir))
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", termString))

	pseudoTTY, err = pty.Start(cmd)
	if err != nil {
		return -1, nil, nil, err
	}

	ResizeShell(pseudoTTY, height, width)

	pid = cmd.Process.Pid
	log.Debugf("started shell: %s pid:%d", shell, pid)

	return pid, pseudoTTY, cmd, nil
}

func ResizeShell(pseudoTTY *os.File, height uint16, width uint16) {
	log.Debugf("resizing terminal %v to %dx%d", *pseudoTTY, height, width)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, pseudoTTY.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct {
			h, w, x, y uint16
		}{
			height, width, 0, 0,
		})))
	if errno != 0 {
		log.Debugf("failed to resize terminal: %d", errno)
	}
}
