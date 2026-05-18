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
	"errors"
	"strings"
)

// Errors approximately as errors.Join, but uses `;` as separator and
// is compatible with go < 1.20.
type Errors []error

func (errs Errors) Error() string {
	var s []string
	for _, err := range errs {
		if err != nil {
			s = append(s, err.Error())
		}
	}
	return strings.Join(s, "; ")
}

func (errs Errors) Is(target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
