// Copyright 2024 Northern.tech AS
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.

package connectionmanager

import (
	"math/rand"
	"time"

	"github.com/mendersoftware/go-lib-micro/ws"
	log "github.com/sirupsen/logrus"

	"context"
)

type expBackoff struct {
	attempts int
	// Retry time does not increase beyond maxBackoff
	maxBackoff time.Duration
	// Smallest backoff sleep time
	smallestUnit time.Duration
	interval     time.Duration

	randomInterval time.Duration
}

// Simple algorithm: Start with `smallestUnit`, and try three times, then double
// `interval` until `maxBackoff` is reached and keep retrying with the maxBackoff
// interval
func (a *expBackoff) GetExponentialBackoffTime() time.Duration {
	var nextInterval time.Duration
	const perIntervalAttempts = 3

	// Random interval between 0 and 5 a.smallestUnit (e.g. seconds)
	a.randomInterval = time.Duration(rand.Float64()*float64(a.smallestUnit)) * 5

	interval := 1 * a.smallestUnit
	nextInterval = interval << (a.attempts / perIntervalAttempts)

	if nextInterval > a.maxBackoff {
		return a.maxBackoff + a.randomInterval
	}
	return nextInterval + a.randomInterval
}

func (a *expBackoff) WaitForBackoff(ctx context.Context, proto ws.ProtoType) error {
	a.attempts++

	a.interval = a.GetExponentialBackoffTime()

	if a.attempts <= 1 {
		return nil
	}

	log.Infof("connectionmanager backoff: retrying in %s", a.interval.Round(time.Second))

	select {
	case <-time.After(a.interval):
	case <-cancelReconnectChan[proto]:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (a *expBackoff) resetBackoff() {
	a.attempts = 0
}
