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
package session

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mendersoftware/mender-shell/shell"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

type MenderSessionType int

const (
	ShellInteractiveSession MenderSessionType = iota
	MonitoringSession
	RemoteDebugSession
	ConfigurationSession
)

type MenderSessionStatus int

const (
	ActiveSession MenderSessionStatus = iota
	ExpiredSession
	IdleSession
	HangedSession
	EmptySession
	NewSession
)

var (
	ErrSessionShellAlreadyRunning         = errors.New("shell is already running")
	ErrSessionShellNotRunning             = errors.New("shell is not running")
	ErrSessionShellStillRunning           = errors.New("shell is still running")
	ErrSessionShellTooManySessionsPerUser = errors.New("user has too many open sessions")
	ErrSessionNotFound                    = errors.New("session not found")
	ErrSessionTooManyShellsAlreadyRunning = errors.New("too many shells spawned")
)

var (
	defaultSessionExpiredTimeout = 1024 * time.Second
	shellProcessWaitTimeout      = 8 * time.Second
	MaxUserSessions              = 1
)

type MenderShellTerminalSettings struct {
	Uid            uint32
	Gid            uint32
	Shell          string
	TerminalString string
	Height         uint16
	Width          uint16
}

type MenderShellSession struct {
	//websocket to pass the messages via
	ws *websocket.Conn
	//mender shell represents a process of passing data between a running shell
	//subprocess running
	shell *shell.MenderShell
	//session id, generated
	id string
	//user id given with the MessageTypeSpawnShell message
	userId string
	//time at which session was created
	createdAt time.Time
	//time after which session is considered to be expired
	expiresAt time.Time
	//type of the session
	sessionType MenderSessionType
	//status of the session
	status MenderSessionStatus
	//terminal settings, for reference, usually it does not change
	//in theory size of the terminal can change
	terminal MenderShellTerminalSettings
	//the pid of the shell process mainly used for stopping the shell
	shellPid int
	//reader and writer are connected to the terminal stdio where the shell is running
	reader io.Reader
	//reader and writer are connected to the terminal stdio where the shell is running
	writer    io.Writer
	pseudoTTY *os.File
	command   *exec.Cmd
}

var sessionsMap = map[string]*MenderShellSession{}
var sessionsByUserIdMap = map[string][]*MenderShellSession{}

func NewMenderShellSession(ws *websocket.Conn, userId string) (s *MenderShellSession, err error) {
	if userSessions, ok := sessionsByUserIdMap[userId]; ok {
		log.Debugf("user %s has %d sessions.", userId, len(userSessions))
		if len(userSessions) >= MaxUserSessions {
			return nil, ErrSessionShellTooManySessionsPerUser
		}
	} else {
		sessionsByUserIdMap[userId] = []*MenderShellSession{}
	}

	uid := uuid.NewV4()
	id := uid.String()

	s = &MenderShellSession{
		ws:          ws,
		id:          id,
		userId:      userId,
		createdAt:   time.Now(),
		expiresAt:   time.Now().Add(defaultSessionExpiredTimeout),
		sessionType: ShellInteractiveSession,
		status:      NewSession,
	}
	sessionsMap[id] = s
	sessionsByUserIdMap[userId] = append(sessionsByUserIdMap[userId], s)
	return s, nil
}

func MenderShellSessionGetById(id string) *MenderShellSession {
	if v, ok := sessionsMap[id]; ok {
		return v
	} else {
		return nil
	}
}

func MenderShellDeleteById(id string) error {
	if v, ok := sessionsMap[id]; ok {
		userSessions := sessionsByUserIdMap[v.userId]
		for i, s := range userSessions {
			if s.id == id {
				sessionsByUserIdMap[v.userId] = append(userSessions[:i], userSessions[i+1:]...)
				break
			}
		}
		delete(sessionsMap, id)
		return nil
	} else {
		return ErrSessionNotFound
	}
}

func MenderShellSessionsGetByUserId(userId string) []*MenderShellSession {
	if v, ok := sessionsByUserIdMap[userId]; ok {
		return v
	} else {
		return nil
	}
}

func UpdateWSConnection(ws *websocket.Conn) error {
	for id, s := range sessionsMap {
		log.Debugf("updating ws in session %s and shell", id)
		s.ws = ws
		if s.shell != nil {
			s.shell.UpdateWSConnection(ws)
		}
	}
	return nil
}

func MenderShellStopByUserId(userId string) (count uint, err error) {
	a := sessionsByUserIdMap[userId]
	log.Debugf("stopping all shells of user %s.", userId)
	if len(a) == 0 {
		return 0, ErrSessionNotFound
	}
	count = 0
	err = nil
	for _, s := range a {
		if s.shell == nil {
			continue
		}
		e := s.StopShell()
		if e != nil {
			err = e
			continue
		}
		delete(sessionsMap, s.id)
		count++
	}
	delete(sessionsByUserIdMap, userId)
	return count, err
}

func (s *MenderShellSession) StartShell(sessionId string, terminal MenderShellTerminalSettings) error {
	if s.status == ActiveSession || s.status == HangedSession {
		return ErrSessionShellAlreadyRunning
	}

	pid, pseudoTTY, cmd, err := shell.ExecuteShell(terminal.Uid,
		terminal.Gid,
		terminal.Shell,
		terminal.TerminalString,
		terminal.Height,
		terminal.Width)
	if err != nil {
		return err
	}

	//MenderShell represents a process of passing messages between backend
	//and the shell subprocess (started above via shell.ExecuteShell) over
	//the websocket connection
	log.Info("mender-shell starting shell command passing process")
	s.shell = shell.NewMenderShell(sessionId, s.ws, pseudoTTY, pseudoTTY)
	s.shell.Start()

	s.shellPid = pid
	s.reader = pseudoTTY
	s.writer = pseudoTTY
	s.status = ActiveSession
	s.terminal = terminal
	s.pseudoTTY = pseudoTTY
	s.command = cmd
	return nil
}

func (s *MenderShellSession) GetId() string {
	return s.id
}

func (s *MenderShellSession) IsExpired() bool {
	e := time.Now().After(s.expiresAt)
	if e {
		s.status = ExpiredSession
	}
	return e
}

func (s *MenderShellSession) ShellCommand(m *shell.MenderShellMessage) error {
	data := m.Data
	commandLine := string(data)
	n, err := s.writer.Write(data)
	if err != nil && n != len(data) {
		err = shell.ErrExecWriteBytesShort
	}
	if err != nil {
		log.Debugf("error: '%s' while running '%s'.", err.Error(), commandLine)
	} else {
		log.Debugf("executed: '%s'", commandLine)
	}
	return err
}

func (s *MenderShellSession) StopShell() error {
	log.Infof("session %s stopping shell", s.id)
	if s.status != ActiveSession && s.status != HangedSession {
		return ErrSessionShellNotRunning
	}

	s.pseudoTTY.Close()
	p, _ := os.FindProcess(s.shellPid)
	if p != nil {
		p.Signal(syscall.SIGINT)
		time.Sleep(time.Second)
		p.Signal(syscall.SIGTERM)
		time.Sleep(2*time.Second)
		p.Signal(syscall.SIGKILL)
		time.Sleep(time.Second)
		done := make(chan error, 1)
		go func() {
			done <- s.command.Wait()
		}()
		select {
		case err := <-done:
			if err != nil {
				log.Errorf("failed to wait for the shell process to exit: %s", err.Error())
			}
		case <-time.After(shellProcessWaitTimeout):
			log.Infof("waiting for pid %d timeout. the process will remain as zombie.", s.shellPid)
		}
		err := p.Signal(syscall.Signal(0))
		if err == nil {
			s.status = HangedSession
			return ErrSessionShellStillRunning
		}
	}

	s.shell.Stop()
	s.terminal = MenderShellTerminalSettings{}
	s.status = EmptySession
	return nil
}
