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

	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/mender-shell/procps"
	"github.com/mendersoftware/mender-shell/shell"
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

const (
	NoExpirationTimeout = time.Second * 0
)

var (
	ErrSessionShellAlreadyRunning         = errors.New("shell is already running")
	ErrSessionShellNotRunning             = errors.New("shell is not running")
	ErrSessionShellTooManySessionsPerUser = errors.New("user has too many open sessions")
	ErrSessionNotFound                    = errors.New("session not found")
	ErrSessionTooManyShellsAlreadyRunning = errors.New("too many shells spawned")
)

var (
	defaultSessionExpiredTimeout     = 1024 * time.Second
	defaultSessionIdleExpiredTimeout = NoExpirationTimeout
	defaultTimeFormat                = "Mon Jan 2 15:04:05 -0700 MST 2006"
	MaxUserSessions                  = 1
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
	//time of a last received message used to determine if the session is active
	activeAt time.Time
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

func timeNow() time.Time {
	return time.Now().UTC()
}

func NewMenderShellSession(userId string, expireAfter time.Duration, expireAfterIdle time.Duration) (s *MenderShellSession, err error) {
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

	if expireAfter == NoExpirationTimeout {
		expireAfter = defaultSessionExpiredTimeout
	}

	if expireAfterIdle != NoExpirationTimeout {
		defaultSessionIdleExpiredTimeout = expireAfterIdle
	}

	createdAt := timeNow()
	s = &MenderShellSession{
		id:          id,
		userId:      userId,
		createdAt:   createdAt,
		expiresAt:   createdAt.Add(expireAfter),
		sessionType: ShellInteractiveSession,
		status:      NewSession,
	}
	sessionsMap[id] = s
	sessionsByUserIdMap[userId] = append(sessionsByUserIdMap[userId], s)
	return s, nil
}

func MenderShellSessionGetCount() int {
	return len(sessionsMap)
}

func MenderShellSessionGetSessionIds() []string {
	keys := make([]string, 0, len(sessionsMap))
	for k := range sessionsMap {
		keys = append(keys, k)
	}

	return keys
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
		if e != nil && procps.ProcessExists(s.shellPid) {
			err = e
			continue
		}
		delete(sessionsMap, s.id)
		count++
	}
	delete(sessionsByUserIdMap, userId)
	return count, err
}

func MenderSessionTerminateAll() (shellCount int, sessionCount int, err error) {
	shellCount = 0
	sessionCount = 0
	for id, s := range sessionsMap {
		e := s.StopShell()
		if e == nil {
			shellCount++
		} else {
			log.Debugf("terminate sessions: failed to stop shell for session: %s: %s", id, e.Error())
			err = e
		}
		e = MenderShellDeleteById(id)
		if e == nil {
			sessionCount++
		} else {
			log.Debugf("terminate sessions: failed to remove session: %s: %s", id, e.Error())
			err = e
		}
	}

	return shellCount, sessionCount, err
}

func MenderSessionTerminateExpired() (shellCount int, sessionCount int, totalExpiredLeft int, err error) {
	shellCount = 0
	sessionCount = 0
	totalExpiredLeft = 0
	for id, s := range sessionsMap {
		if s.IsExpired(false) {
			e := s.StopShell()
			if e == nil {
				shellCount++
			} else {
				log.Debugf("expire sessions: failed to stop shell for session: %s: %s", id, e.Error())
				err = e
			}
			e = MenderShellDeleteById(id)
			if e == nil {
				sessionCount++
			} else {
				log.Debugf("expire sessions: failed to delete session: %s: %s", id, e.Error())
				totalExpiredLeft++
				err = e
			}
		}
	}

	return shellCount, sessionCount, totalExpiredLeft, err
}

func (s *MenderShellSession) GetStatus() MenderSessionStatus {
	return s.status
}

func (s *MenderShellSession) GetStartedAtFmt() string {
	return s.createdAt.Format(defaultTimeFormat)
}

func (s *MenderShellSession) GetExpiresAtFmt() string {
	return s.expiresAt.Format(defaultTimeFormat)
}

func (s *MenderShellSession) GetActiveAtFmt() string {
	return s.activeAt.Format(defaultTimeFormat)
}

func (s *MenderShellSession) GetShellCommandPath() string {
	return s.command.Path
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
	log.Infof("mender-shell starting shell command passing process, pid: %d", pid)
	s.shell = shell.NewMenderShell(sessionId, pseudoTTY, pseudoTTY)
	s.shell.Start()

	s.shellPid = pid
	s.reader = pseudoTTY
	s.writer = pseudoTTY
	s.status = ActiveSession
	s.terminal = terminal
	s.pseudoTTY = pseudoTTY
	s.command = cmd
	s.activeAt = timeNow()
	return nil
}

func (s *MenderShellSession) GetId() string {
	return s.id
}

func (s *MenderShellSession) GetShellPid() int {
	return s.shellPid
}

func (s *MenderShellSession) IsExpired(setStatus bool) bool {
	if defaultSessionIdleExpiredTimeout != NoExpirationTimeout {
		idleTimeoutReached := s.activeAt.Add(defaultSessionIdleExpiredTimeout)
		return timeNow().After(idleTimeoutReached)
	}
	e := timeNow().After(s.expiresAt)
	if e && setStatus {
		s.status = ExpiredSession
	}
	return e
}

func (s *MenderShellSession) ShellCommand(m *shell.MenderShellMessage) error {
	s.activeAt = timeNow()
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

func (s *MenderShellSession) StopShell() (err error) {
	log.Infof("session %s status:%d stopping shell", s.id, s.status)
	if s.status != ActiveSession && s.status != HangedSession {
		return ErrSessionShellNotRunning
	}

	p, _ := os.FindProcess(s.shellPid)
	p.Signal(syscall.SIGINT)
	time.Sleep(2 * time.Second)
	s.shell.Stop()
	time.Sleep(2 * s.shell.GetWriteTimeout())
	s.pseudoTTY.Close()

	err = procps.TerminateAndWait(s.shellPid, s.command, 2*time.Second)
	if err != nil {
		log.Errorf("session %s, shell pid %d, termination error: %s", s.id, s.shellPid, err.Error())
	}

	s.shell.Stop()
	s.terminal = MenderShellTerminalSettings{}
	s.status = EmptySession
	return err
}
