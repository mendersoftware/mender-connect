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
	defaultSessionExpiredTimeout = 512 * time.Second
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
	writer io.Writer
}

var sessionsMap = map[string]*MenderShellSession{}
var sessionsByUserIdMap = map[string][]*MenderShellSession{}

//reconnect and lost sessid for use -- one shell per user/reuse
func NewMenderShellSession(ws *websocket.Conn, userId string) (s *MenderShellSession, err error) {
	if _, ok := sessionsByUserIdMap[userId]; ok {
		return nil, ErrSessionShellTooManySessionsPerUser
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
	sessionsByUserIdMap[userId] = []*MenderShellSession{s}
	return s, nil
}

func MenderShellSessionGetById(id string) *MenderShellSession {
	if v, ok := sessionsMap[id]; ok {
		return v
	} else {
		return nil
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

func (s *MenderShellSession) StartShell(sessionId string, terminal MenderShellTerminalSettings) error {
	if s.status == ActiveSession || s.status == HangedSession {
		return ErrSessionShellAlreadyRunning
	}

	pid, r, w, err := shell.ExecuteShell(terminal.Uid,
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
	s.shell = shell.NewMenderShell(sessionId, s.ws, r, w)
	s.shell.Start()

	s.shellPid = pid
	s.reader = r
	s.writer = w
	s.status = ActiveSession
	s.terminal = terminal
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
	if s.status != ActiveSession && s.status != HangedSession {
		return ErrSessionShellNotRunning
	}

	p, _ := os.FindProcess(s.shellPid)
	if p != nil {
		p.Signal(syscall.SIGINT)
		time.Sleep(time.Second)
		p.Signal(syscall.SIGTERM)
		time.Sleep(time.Second)
		time.Sleep(time.Second)
		p.Signal(syscall.SIGKILL)
		time.Sleep(time.Second)
		err := p.Signal(syscall.Signal(0))
		if err != nil {
			s.status = HangedSession
			return ErrSessionShellStillRunning
		}
	}

	s.shell.Stop()
	s.terminal = MenderShellTerminalSettings{}
	s.status = EmptySession
	return nil
}
