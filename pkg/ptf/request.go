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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Request is a PTF request: a command with its parameters (the [COMMAND]
// block) plus optional header fields such as User, SessionID, and arbitrary
// Options (the [HEADER] block).
type Request struct {
	Command        string
	User           string
	SessionID      string
	Params         Params
	Options        Params
	isCommandBlock bool
}

// ErrEof is returned by ParseLine when the "EOF" terminator line is reached.
var ErrEof = errors.New("The end of the file has been reached")

// NewRequest returns an empty Request with its Params and Options maps
// initialised.
func NewRequest() *Request {
	r := Request{
		Options: make(map[string]string),
		Params:  make(map[string]string),
	}
	return &r
}

// NewRequestParse returns a Request parsed from the PTF text in s.
func NewRequestParse(s *string) (*Request, error) {
	request := NewRequest()
	err := request.Parse(s)
	return request, err
}

// String renders the request in PTF wire form: an optional [HEADER] block
// followed by the [COMMAND] block and a trailing "EOF" line.
func (r Request) String() string {
	var b bytes.Buffer
	if len(r.Options) > 0 || len(r.User) > 0 {
		b.WriteString("[HEADER]\n")
		if len(r.User) != 0 {
			b.WriteString("user=" + r.User + "\n")
		}
		if len(r.SessionID) != 0 {
			b.WriteString("sessionid=" + r.SessionID + "\n")
		}
		for option := range r.Options {
			if strings.ContainsAny(r.Options[option], "\r\n") {
				strings.Replace(r.Options[option], "\n", " ", -1)
				strings.Replace(r.Options[option], "\r", "", -1)
			}
			b.WriteString(option + "=" + r.Options[option] + "\n")
		}
	}
	b.WriteString("[COMMAND]\n")
	b.WriteString("command=" + r.Command + "\n")
	for param := range r.Params {
		if strings.ContainsAny(r.Params[param], "\r\n") {
			strings.Replace(r.Params[param], "\n", " ", -1)
			strings.Replace(r.Params[param], "\r", "", -1)
		}
		b.WriteString(param + "=" + r.Params[param] + "\n")
	}
	b.WriteString("EOF\n")
	return b.String()
}

// Parse fills the request from the PTF text in s.
func (r *Request) Parse(s *string) error {
	return r.ParseBufioReader(bufio.NewReader(strings.NewReader(*s)))
}

// ParseBufioReader fills the request by reading PTF lines from reader until the
// "EOF" terminator or end of input. Malformed lines are logged to stderr and
// skipped rather than aborting the parse.
func (r *Request) ParseBufioReader(reader *bufio.Reader) error {
	// io scanner
	scanner := bufio.NewScanner(reader)

	// Read from io scanner line by line
	for scanner.Scan() {
		line := scanner.Text()
		if err := r.ParseLine(line); err != nil {
			if errors.Is(err, ErrEof) {
				return nil
			}
			fmt.Fprintln(os.Stderr, "ParseLine error:", err)
		}
	}

	// Handle scanner errors
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

	return nil
}

// ParseLine processes a single PTF line: it tracks the current block, routes
// key=value pairs to Command, User, SessionID, Params, or Options as
// appropriate, and returns ErrEof on the "EOF" terminator line.
func (r *Request) ParseLine(line string) error {

	// Detect blocks
	if line == "[COMMAND]" {
		r.isCommandBlock = true
		return nil
	} else if line == "[HEADER]" {
		return nil
	} else if line == "" { // Ignore empty line
		return nil
	} else if line == "EOF" { // Finish parser on EOF line
		//return nil
		return ErrEof
	}

	// Split key/value
	kv := strings.SplitN(line, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("Invalid parameter line: %s", line)
	}
	// Remove spaces and lowercase the key
	kv[0] = strings.ToLower(strings.TrimSpace(kv[0]))
	kv[1] = strings.TrimSpace(kv[1])

	// Set value on request struct or Params mapping
	if kv[0] == "command" {
		r.Command = kv[1]
	} else if r.isCommandBlock {
		r.Params[kv[0]] = kv[1]
	} else if kv[0] == "user" {
		r.User = kv[1]
	} else if kv[0] == "sessionid" {
		r.SessionID = kv[1]
	} else {
		r.Options[kv[0]] = kv[1]
	}

	return nil
}
