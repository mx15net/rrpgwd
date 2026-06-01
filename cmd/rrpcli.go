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

package main

// rrpcli is a command-line PTF client for the rrpgwd daemon:
//
//	rrpcli [options] Command key=value key2=value2 ...
//	rrpcli [options] nameserver,=ns1.example,ns2.example   # indexed list param
//	echo "[COMMAND]\ncommand=...\nEOF" | rrpcli [options]   # raw PTF from stdin
//
// Options:
//	-v             verbose: print "<code> <description>" to stderr
//	-p[NAME]       property mode: print values of property NAME, or every
//	               property as "KEY:value" when NAME is omitted
//	-OK<code>      treat <code> as success (exit 0); defaults to 200
//	--socket=URL   override the daemon socket (rrpgwd://user:pass@host:port)
//
// The socket also defaults from $RRPCLI_SOCKET, then rrpgwd://test:test@127.0.0.1:2000.

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"mx15net/rrpgwd/pkg/client"
	"mx15net/rrpgwd/pkg/ptf"
)

const defaultSocket = "rrpgwd://test:test@127.0.0.1:2000"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	socketURL := os.Getenv("RRPCLI_SOCKET")
	if socketURL == "" {
		socketURL = defaultSocket
	}

	verbose := false
	propertyMode := false
	property := ""
	okcode := 200
	var positional []string

	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--") && strings.Contains(a, "="):
			kv := strings.SplitN(a[2:], "=", 2)
			if strings.EqualFold(kv[0], "socket") {
				socketURL = kv[1]
			}
			// Other --key=value overrides are accepted but unused.
		case a == "-v":
			verbose = true
		case strings.HasPrefix(a, "-OK"):
			n, err := strconv.Atoi(a[3:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid option: %s\n", a)
				return 2
			}
			okcode = n
		case strings.HasPrefix(a, "-p"):
			propertyMode = true
			property = strings.ToUpper(a[2:])
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "invalid option: %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}

	sock, err := client.ParseSocket(socketURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	request, err := buildRequest(positional)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	raw, err := client.SendRaw(request, sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !strings.Contains(raw, "[RESPONSE]") {
		// Authentication failure or other protocol error.
		fmt.Fprint(os.Stderr, raw)
		return 1
	}

	resp := ptf.NewResponse(0, "")
	resp.Parse(&raw)

	if verbose {
		fmt.Fprintf(os.Stderr, "%d %s\n", resp.Code, resp.FullDescription())
	}

	if propertyMode {
		printProperties(resp, property)
	} else {
		fmt.Print(raw)
	}

	code := resp.Code
	if okcode != 0 && code == okcode {
		code = 0
	}
	return code
}

// buildRequest assembles a PTF request from the positional arguments. If no
// command word is present it falls back to reading raw PTF from stdin.
func buildRequest(args []string) (*ptf.Request, error) {
	if !hasCommand(args) {
		return readRequestFromStdin()
	}

	req := ptf.NewRequest()
	for _, a := range args {
		if len(a) == 0 || !isAlpha(a[0]) {
			continue
		}

		eq := strings.IndexByte(a, '=')

		// Indexed list parameter: key,=v1,v2,v3  ->  KEY0=v1, KEY1=v2, ...
		if eq > 0 && a[eq-1] == ',' {
			key := strings.ToUpper(a[:eq-1])
			for i, v := range strings.Split(a[eq+1:], ",") {
				req.Params[key+strconv.Itoa(i)] = v
			}
			continue
		}

		// Scalar parameter: key=value
		if eq >= 0 {
			key := strings.ToUpper(a[:eq])
			val := a[eq+1:]
			switch key {
			case "COMMAND":
				req.Command = val
			case "USER":
				req.User = val
			default:
				req.Params[key] = val
			}
			continue
		}

		// Bare word: the command name.
		req.Command = a
	}
	return req, nil
}

// hasCommand reports whether the args contain a command (a bare alpha word or
// an explicit command=... pair).
func hasCommand(args []string) bool {
	for _, a := range args {
		if len(a) == 0 || !isAlpha(a[0]) {
			continue
		}
		if eq := strings.IndexByte(a, '='); eq < 0 {
			return true
		} else if strings.EqualFold(a[:eq], "command") {
			return true
		}
	}
	return false
}

func readRequestFromStdin() (*ptf.Request, error) {
	var b strings.Builder
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		b.WriteString(line + "\n")
		if strings.TrimSpace(line) == "EOF" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	s := b.String()
	return ptf.NewRequestParse(&s)
}

func printProperties(resp *ptf.Response, name string) {
	if name != "" {
		for k, vals := range resp.Properties {
			if strings.EqualFold(k, name) {
				for _, v := range vals {
					fmt.Println(v)
				}
			}
		}
		return
	}

	keys := make([]string, 0, len(resp.Properties))
	for k := range resp.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range resp.Properties[k] {
			fmt.Printf("%s:%s\n", strings.ToUpper(k), v)
		}
	}
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
