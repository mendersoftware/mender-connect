// Copyright 2026 Northern.tech AS
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
	"net/http"
	"net/http/httptest"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/mendersoftware/go-lib-micro/ws"

	"github.com/mendersoftware/mender-connect/connectionmanager"
)

// onMessage fires fn in a new goroutine the first time a log line containing
// substr is emitted.
type onMessage struct {
	substr string
	once   sync.Once
	fn     func()
}

func (h *onMessage) Levels() []log.Level { return log.AllLevels }

func (h *onMessage) Fire(e *log.Entry) error {
	if strings.Contains(e.Message, h.substr) {
		h.once.Do(func() { go h.fn() })
	}
	return nil
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

func mustNotNil(t *testing.T, s *MenderShellSession) {
	t.Helper()
	if s == nil {
		t.Fatal("session is nil")
	}
}

func allGoroutines() string {
	buf := make([]byte, 1<<20)
	return string(buf[:runtime.Stack(buf, true)])
}

func startHealthcheckTestSession(t *testing.T, sessionID, userID string) *MenderShellSession {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(newShellTransaction))
	t.Cleanup(server.Close)
	u := "ws" + strings.TrimPrefix(server.URL, "http")
	connectionmanager.Connect(ws.ProtoTypeShell, u, "/", "token", nil)

	currentUser, err := user.Current()
	mustNoErr(t, err)
	uid, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	mustNoErr(t, err)
	gid, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	mustNoErr(t, err)

	s, err := NewMenderShellSession(sessionID, userID, defaultSessionExpiredTimeout, NoExpirationTimeout)
	mustNoErr(t, err)
	mustNotNil(t, s)

	err = s.StartShell(sessionID, MenderShellTerminalSettings{
		Uid:            uint32(uid),
		Gid:            uint32(gid),
		Shell:          "/bin/sh",
		TerminalString: "xterm-256color",
		Height:         40,
		Width:          80,
	})
	mustNoErr(t, err)
	return s
}

// shortenHealthcheck reduces the 60s+5s healthcheck timers so the test does not
// have to wait 65 seconds.  It does not touch the 4 second window inside
// procps.TerminateAndWait, which is what the failure actually needs.
func shortenHealthcheck(t *testing.T) {
	t.Helper()
	origInterval, origTimeout := healthcheckInterval, healthcheckTimeout
	healthcheckInterval = 1 * time.Second
	healthcheckTimeout = 1 * time.Second
	t.Cleanup(func() {
		healthcheckInterval, healthcheckTimeout = origInterval, origTimeout
	})
}

// isolateLogHooks removes the test's log hooks when it finishes; logrus has no
// RemoveHook.
func isolateLogHooks(t *testing.T) {
	t.Helper()
	orig := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	t.Cleanup(func() {
		log.StandardLogger().ReplaceHooks(orig)
	})
}

// pongRoute performs exactly what app.(*MenderShellDaemon).routeMessagePongShell
// does on the messageLoop goroutine.
func pongRoute(sessionID string) error {
	s := MenderShellSessionGetById(sessionID)
	if s == nil {
		return ErrSessionNotFound
	}
	s.HealthcheckPong()
	return nil
}

// TestLatePongDuringHealthcheckStop delivers a pong while the healthcheck
// goroutine is stopping its own session.  The healthcheck goroutine stops
// reading s.pong the moment it logs "health check failed", but the session
// stays in sessionsMap until MenderShellStopById finishes, at least 4 seconds
// later, because procps.TerminateAndWait sleeps 2s + 2s unconditionally.  A
// pong delivered in that window must not block: HealthcheckPong is called
// synchronously from the daemon's messageLoop goroutine, so a blocking send
// leaves the daemon waiting forever on a channel that no longer has a reader.
func TestLatePongDuringHealthcheckStop(t *testing.T) {
	MaxUserSessions = 2
	shortenHealthcheck(t)
	isolateLogHooks(t)

	const sessionID = "11111111-1111-1111-1111-111111111111"
	returned := make(chan error, 1)
	stopped := make(chan struct{})

	// The healthcheck goroutine logs "health check failed" from inside the
	// timeout branch of its select.  From that point it never reads s.pong
	// again, but the session stays in sessionsMap until MenderShellStopById
	// finishes, at least 4 seconds later.
	log.AddHook(&onMessage{substr: sessionID + ", health check failed", fn: func() {
		returned <- pongRoute(sessionID)
	}})
	log.AddHook(&onMessage{substr: sessionID + " successfully stopped", fn: func() {
		close(stopped)
	}})

	startHealthcheckTestSession(t, sessionID, "user-late-pong")

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("routeMessagePongShell did not return: the messageLoop goroutine is "+
			"blocked on the pong channel.\n\n%s", allGoroutines())
	}

	// Wait for MenderShellStopById to finish, so the healthcheck goroutine and
	// its sessionsMap removal do not outlive the test.
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatalf("session stop did not finish\n\n%s", allGoroutines())
	}
}

// TestPongAfterSessionDeleted is the negative control.  A pong that arrives
// after MenderShellStopById removed the session is rejected with
// ErrSessionNotFound and does not block, which shows the failure above is
// caused by the timing of the send and not by the harness itself.
func TestPongAfterSessionDeleted(t *testing.T) {
	MaxUserSessions = 2
	shortenHealthcheck(t)
	isolateLogHooks(t)

	const sessionID = "22222222-2222-2222-2222-222222222222"
	returned := make(chan error, 1)

	// "successfully stopped" is logged after MenderShellDeleteById has run.
	log.AddHook(&onMessage{substr: sessionID + " successfully stopped", fn: func() {
		returned <- pongRoute(sessionID)
	}})

	startHealthcheckTestSession(t, sessionID, "user-pong-control")

	select {
	case err := <-returned:
		if err != ErrSessionNotFound {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("negative control blocked, the harness is wrong\n\n%s", allGoroutines())
	}
}
