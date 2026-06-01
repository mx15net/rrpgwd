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

// Package client is a small PTF client for the rrpgwd daemon. It builds a
// connection target from a rrpgwd:// URL (see ParseSocket), sends a ptf.Request,
// and returns the response either as raw text (SendRaw) or parsed (Send).
package client

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"mx15net/rrpgwd/pkg/ptf"
)

// Socket describes how to reach an rrpgwd daemon and the credentials to present
// at its login prompt.
type Socket struct {
	Host     string
	Port     string
	Proto    string
	User     string
	Password string
}

// NewSocket returns a TCP Socket for host:port with the default test/test
// credentials.
func NewSocket(host string, port string) *Socket {
	return &Socket{
		Host:     host,
		Port:     port,
		User:     "test",
		Password: "test",
		Proto:    "tcp",
	}
}

// ParseSocket parses a socket URL:
//
//	rrpgwd://user:password@host:port   (TCP)
//	rrpgwd://user:password@/path.sock  (Unix domain socket)
//
// The user/password are optional and default to test/test.
func ParseSocket(rawurl string) (*Socket, error) {
	rest := strings.TrimPrefix(rawurl, "rrpgwd://")
	if rest == rawurl {
		return nil, fmt.Errorf("invalid socket %q: expected rrpgwd:// scheme", rawurl)
	}

	s := &Socket{Proto: "tcp", User: "test", Password: "test"}

	// Optional user:password@ prefix.
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		cred := rest[:at]
		rest = rest[at+1:]
		if c := strings.IndexByte(cred, ':'); c >= 0 {
			s.User = cred[:c]
			s.Password = cred[c+1:]
		} else {
			s.User = cred
		}
	}

	// Unix socket path.
	if strings.HasPrefix(rest, "/") {
		s.Proto = "unix"
		s.Host = rest
		return s, nil
	}

	host, port, err := net.SplitHostPort(rest)
	if err != nil {
		return nil, fmt.Errorf("invalid socket address %q: %w", rest, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	s.Host = host
	s.Port = port
	return s, nil
}

// address returns the dial target for the socket's protocol.
func (s *Socket) address() string {
	if s.Proto == "unix" {
		return s.Host
	}
	return s.Host + ":" + s.Port
}

// SendRaw sends a PTF request to the daemon and returns the raw PTF response
// text (from "[RESPONSE]" through the trailing "EOF"). The daemon's
// "login: "/"password: " prompts are stripped from the leading line.
func SendRaw(r *ptf.Request, s *Socket) (string, error) {
	conn, err := net.Dial(s.Proto, s.address())
	if err != nil {
		return "", fmt.Errorf("connect error: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(s.User + "\n" + s.Password + "\n" + r.String())); err != nil {
		return "", fmt.Errorf("write error: %v", err)
	}

	var b strings.Builder
	reader := bufio.NewReader(conn)
	first := true
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Connection closed; keep whatever was read (covers the
			// "Authorization failed" path, which has no EOF terminator).
			if len(line) > 0 {
				b.WriteString(strings.TrimRight(line, "\r\n") + "\n")
			}
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if first {
			// Prompts are written without trailing newlines, so they share
			// the first line with the start of the response.
			line = strings.TrimPrefix(line, "login: ")
			line = strings.TrimPrefix(line, "password: ")
			first = false
		}
		b.WriteString(line + "\n")
		if line == "EOF" {
			break
		}
	}
	return b.String(), nil
}

// Client talks to an rrpgwd daemon over a fixed Socket.
type Client struct {
	socket *Socket
}

// New returns a Client bound to the daemon at the given socket URL
// (rrpgwd://user:password@host:port, or rrpgwd:///path.sock).
func New(socketURL string) (*Client, error) {
	sock, err := ParseSocket(socketURL)
	if err != nil {
		return nil, err
	}
	return &Client{socket: sock}, nil
}

// Socket returns the daemon socket the client is bound to.
func (c *Client) Socket() *Socket {
	return c.socket
}

// Send sends a PTF request and returns the parsed response. Transport failures
// and non-response output (such as a rejected login) are surfaced as a
// synthetic error response rather than a separate error value.
func (c *Client) Send(r *ptf.Request) *ptf.Response {
	raw, err := SendRaw(r, c.socket)
	if err != nil {
		return ptf.NewResponse(421, err.Error())
	}
	if !strings.Contains(raw, "[RESPONSE]") {
		return ptf.NewResponse(421, strings.TrimSpace(raw))
	}
	resp := ptf.NewResponse(0, "")
	resp.Parse(&raw)
	return resp
}
