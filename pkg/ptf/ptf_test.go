// Copyright 2026 Mathias Schwenk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ptf

import (
	"fmt"
	"testing"
)

var ptfrequest = `
[HEADER]
USER = test
[COMMAND]
command = Test
param = 12
param2 = Test jkflsjklj = kflsjkl
EOF
`

func TestParse(t *testing.T) {
	// Parse a PTF Request
	request, err := NewRequestParse(&ptfrequest)
	if err != nil {
		err = fmt.Errorf("Parser error: %w", err)
		panic(err)
	}

	want := "Test"
	if got := request.Command; got != want {
		t.Errorf("request.Command = %q, want %q", got, want)
	}
}
