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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNum64(t *testing.T) {
	testCases := map[string]struct {
		input  interface{}
		output int64
	}{
		"int": {
			input:  int(10),
			output: int64(10),
		},
		"int8": {
			input:  int8(10),
			output: int64(10),
		},
		"int16": {
			input:  int16(10),
			output: int64(10),
		},
		"int32": {
			input:  int32(10),
			output: int64(10),
		},
		"int64": {
			input:  int64(10),
			output: int64(10),
		},
		"uint": {
			input:  uint(10),
			output: int64(10),
		},
		"uint8": {
			input:  uint8(10),
			output: int64(10),
		},
		"uint16": {
			input:  uint16(10),
			output: int64(10),
		},
		"uint32": {
			input:  uint32(10),
			output: int64(10),
		},
		"uint64": {
			input:  uint64(10),
			output: int64(10),
		},
		"uintptr": {
			input:  uintptr(10),
			output: int64(10),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			out, ok := Num64(tc.input)
			assert.Equal(t, tc.output, out)
			assert.True(t, ok)
		})
	}

	_, ok := Num64("dummy")
	assert.False(t, ok)
}
