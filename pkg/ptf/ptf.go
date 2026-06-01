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

// Package ptf implements the PTF wire format used to talk to an XRRP server.
// It marshals and parses Request and Response messages; the format is
// line-oriented with [HEADER]/[COMMAND]/[RESPONSE] sections, key=value lines,
// and an "EOF" terminator. The package performs no I/O — it only converts
// between these structs and their string form.
package ptf

import ()

// Params is a set of key/value pairs, used for request parameters and options
// and for scalar response attributes.
type Params map[string]string
