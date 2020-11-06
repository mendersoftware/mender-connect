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
package shell

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/kr/pty"
	log "github.com/sirupsen/logrus"
)

func ExecuteShell(uid uint32, gid uint32, shell string, height uint16, width uint16) (r io.Reader, w io.Writer, err error) {
	shellTerm := "xterm-256color"
	cmd := exec.Command(shell)
	if cmd == nil {
		log.Debugf("failed exec.Command shell: %s", shell)
		return nil, nil, errors.New("unknown error with exec.Command(" + shell + ")")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}

	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", shellTerm))
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, err
	}

	pid := cmd.Process.Pid
	log.Debugf("started shell: %s pid:%d", shell, pid)

	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct {
			h, w, x, y uint16
		}{
			uint16(height), uint16(width), 0, 0,
		})))

	return f, f, nil
}
