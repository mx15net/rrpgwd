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
	"fmt"
	"strconv"
	"strings"
)

// Response is a parsed PTF response: the result code and description plus
// optional reason, timing, transaction IDs, scalar Params, and multi-valued
// Properties (the [RESPONSE] block).
type Response struct {
	Code        int
	Description string
	Reason      string
	Runtime     float32
	Queuetime   float32
	Cltrid      string
	Svtrid      string
	Params      Params
	Properties  Properties
	isParam     bool
}

// Properties maps a property name to its ordered list of values, mirroring the
// PTF "property[name][index]" lines.
type Properties map[string][]string

// Set stores value at index for the named property, growing the slice as
// needed.
func (p Properties) Set(key string, index int, value string) {
	p[key] = sliceSet(p[key], index, value)
}

// Get returns the first value stored for the named property, or "" if the
// property is absent. The lookup is case-insensitive, matching the lower-cased
// names produced when a response is parsed.
func (p Properties) Get(name string) string {
	vals := p[strings.ToLower(name)]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// NewResponse returns a Response with the given code (its description filled in
// from the code table) and reason, with empty Params and Properties.
func NewResponse(code int, reason string) *Response {
	return &Response{
		Code:        code,
		Description: ResponseCodeDescription[code],
		Reason:      reason,
		Params:      make(Params),
		Properties:  make(Properties),
	}
}

// String renders the response in PTF wire form: a [RESPONSE] block with the
// code, description, timing, optional transaction IDs, scalar params, and
// indexed properties, ending with an "EOF" line.
func (r Response) String() string {
	var b bytes.Buffer
	b.WriteString("[RESPONSE]\n")
	if r.Code != 0 {
		b.WriteString("code = " + strconv.Itoa(r.Code) + "\n")
		if len(r.Description) == 0 {
			r.Description = ResponseCodeDescription[r.Code]
		}
	}
	description := r.FullDescription()
	if len(description) != 0 {
		b.WriteString("description = " + description + "\n")
	}
	b.WriteString("runtime = " + strconv.FormatFloat(float64(r.Runtime), 'f', 3, 32) + "\n")
	b.WriteString("queuetime = " + strconv.FormatFloat(float64(r.Queuetime), 'f', 3, 32) + "\n")
	if len(r.Cltrid) != 0 {
		b.WriteString("cltrid = " + r.Cltrid + "\n")
	}
	if len(r.Svtrid) != 0 {
		b.WriteString("svtrid = " + r.Svtrid + "\n")
	}

	for param := range r.Params {
		if strings.ContainsAny(r.Params[param], "\r\n") {
			strings.Replace(r.Params[param], "\n", " ", -1)
			strings.Replace(r.Params[param], "\r", "", -1)
		}
		b.WriteString(param + " = " + r.Params[param] + "\n")
	}
	if len(r.Properties) > 0 {
		for property, value := range r.Properties {
			for i, v := range value {
				if strings.ContainsAny(v, "\r\n") {
					v = strings.Replace(v, "\n", " ", -1)
					v = strings.Replace(v, "\r", "", -1)
				}
				b.WriteString("property[" + property + "][" + strconv.Itoa(i) + "] = " + v + "\n")
			}
		}
	}
	b.WriteString("EOF\n")
	return b.String()
}

// Parse fills the response from the PTF text in s, resetting Params and
// Properties first.
func (r *Response) Parse(s *string) error {
	// io scanner
	scanner := bufio.NewScanner(strings.NewReader(*s))

	// new options and params
	r.Params = make(Params)
	r.Properties = make(Properties)

	// Read from io scanner line by line
	for scanner.Scan() {
		line := scanner.Text()

		r.ParseLine(line)
	}

	// Handle scanner errors
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading standard input: %w", err)
	}

	return nil
}

// ParseLine processes a single PTF response line: it tracks the [RESPONSE]
// block, routes "property[name][i]" lines into Properties, and maps known keys
// (code, description, reason, runtime, queuetime, cltrid, svtrid) onto the
// corresponding fields, with everything else going into Params.
func (r *Response) ParseLine(line string) error {
	// Detect blocks
	if line == "[RESPONSE]" {
		r.isParam = true
		return nil
	} else if line == "" { // Ignore empty line
		return nil
	} else if line == "EOF" { // Finish parser on EOF line
		return nil
	} else if !r.isParam {
		return fmt.Errorf("Invalid line outside response block: %s", line)
	}

	// Split key/value
	kv := strings.SplitN(line, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("Invalid line with out key = value format: %s", line)
	}

	// Remove spaces and lowercase the key
	kv[0] = strings.ToLower(strings.TrimSpace(kv[0]))
	kv[1] = strings.TrimSpace(kv[1])

	if strings.HasPrefix(kv[0], "property[") {
		// Split property[key][i]
		splitProperty := strings.FieldsFunc(kv[0], func(r rune) bool {
			if r == '[' || r == ']' {
				return true
			}
			return false
		})
		// Parse property index
		i, err := strconv.Atoi(splitProperty[2])
		if err != nil {
			return fmt.Errorf("Property index or %s is invalid integer: %w", splitProperty[1], err)
		}
		r.Properties.Set(splitProperty[1], i, strings.TrimSpace(kv[1]))
	} else {
		switch kv[0] {
		case "code":
			code, err := strconv.Atoi(kv[1])
			if err != nil {
				return fmt.Errorf("Invalid response code: %w", err)
			}
			r.Code = code
		case "description":
			if strings.Contains(kv[1], ";") {
				s := strings.Split(kv[1], "; ")
				r.Description = s[0]
				r.Reason = s[1]
			} else {
				r.Description = kv[1]
			}
		case "reason":
			r.Reason = kv[1]
		case "runtime":
			f, err := strconv.ParseFloat(kv[1], 32)
			if err != nil {
				return fmt.Errorf("Parse runtime error: %w", err)
			}
			r.Runtime = float32(f)
		case "queuetime":
			f, err := strconv.ParseFloat(kv[1], 32)
			if err != nil {
				return fmt.Errorf("Parse queuetime error: %w", err)
			}
			r.Queuetime = float32(f)
		case "cltrid":
			r.Cltrid = kv[1]
		case "svtrid":
			r.Svtrid = kv[1]
		default:
			r.Params[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	return nil
}

// FullDescription returns the description (falling back to the code table) with
// the reason appended as "description; reason" when a reason is present.
func (r *Response) FullDescription() string {
	description := r.Description
	if len(description) == 0 && r.Code != 0 {
		description = ResponseCodeDescription[r.Code]
	}
	if len(r.Reason) != 0 {
		description = description + "; " + r.Reason
	}
	return description
}

// CodeDescription returns "<code> <full description>".
func (r *Response) CodeDescription() string {
	return strconv.Itoa(r.Code) + " " + r.FullDescription()
}

// IsSuccess reports whether the response code is 200.
func (r *Response) IsSuccess() bool {
	return r.Code == 200
}

// IsError reports whether the response code is anything other than 200.
func (r *Response) IsError() bool {
	return r.Code != 200
}

// sliceSet stores value at index in a, growing a with empty strings if the
// index is beyond its current length.
func sliceSet(a []string, index int, value string) []string {
	if len(a) < index { // The index is out of range
		b := make([]string, (index-len(a))+1) // add missing elements
		a = append(a, b...)
		a[index] = value
	} else if len(a) == index { // nil or empty slice or after last element
		a = append(a, value)
	} else {
		a[index] = value
	}
	return a
}
