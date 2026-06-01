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

// Command ptfdemo sends a "Describe" command to a running rrpgwd daemon and
// prints the response in human-readable form, highlighting the time, opmode,
// and target properties.
//
//	go build -o bin/ptfdemo ./cmd/ptfdemo.go
//	./bin/ptfdemo [rrpgwd://user:pass@host:port]
//
// The daemon socket defaults to $RRPCLI_SOCKET, then to
// rrpgwd://test:test@127.0.0.1:2000, and can be overridden by the first argument.
package main

import (
	"fmt"
	"os"

	"mx15net/rrpgwd/pkg/client"
	"mx15net/rrpgwd/pkg/ptf"
)

const defaultSocket = "rrpgwd://test:test@127.0.0.1:2000"

func main() {
	socketURL := os.Getenv("RRPCLI_SOCKET")
	if socketURL == "" {
		socketURL = defaultSocket
	}
	if len(os.Args) > 1 {
		socketURL = os.Args[1]
	}

	c, err := client.New(socketURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	resp := c.Send(&ptf.Request{Command: "Describe"})

	fmt.Printf("Describe response from %s:%s\n", c.Socket().Host, c.Socket().Port)
	fmt.Printf("  status : %s\n", resp.CodeDescription())
	fmt.Printf("  time   : %s\n", resp.Properties.Get("time"))
	fmt.Printf("  opmode : %s\n", resp.Properties.Get("opmode"))
	fmt.Printf("  target : %s\n", resp.Properties.Get("target"))

	if resp.IsError() {
		os.Exit(1)
	}
}
