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

package deviceconnect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWebSocketScheme(t *testing.T) {
	testCases := map[string]struct {
		scheme string
		result string
	}{
		"http": {
			scheme: httpProtocol,
			result: wsProtocol,
		},
		"https": {
			scheme: httpsProtocol,
			result: wssProtocol,
		},
		"ws": {
			scheme: wsProtocol,
			result: wsProtocol,
		},
		"wss": {
			scheme: wssProtocol,
			result: wssProtocol,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := getWebSocketScheme(tc.scheme)
			assert.Equal(t, tc.result, result)
		})
	}
}
