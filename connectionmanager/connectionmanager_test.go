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

package connectionmanager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	SetDefaultPingWait(10 * time.Second)
}

func TestSetReconnectIntervalSeconds(t *testing.T) {
	SetReconnectIntervalSeconds(15)
	assert.Equal(t, 15, reconnectIntervalSeconds)
}

func TestGetWriteTimeout(t *testing.T) {
	timeOut := GetWriteTimeout()
	assert.Equal(t, writeWait, timeOut)
}

func TestGetWsScheme(t *testing.T) {
	assert.Equal(t, "wss", getWebSocketScheme("https"))
	assert.Equal(t, "ws", getWebSocketScheme("http"))
	assert.Equal(t, "wss", getWebSocketScheme("wss"))
	assert.Equal(t, "ws", getWebSocketScheme("ws"))
}
