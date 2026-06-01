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

package rrpgwd

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Session is a single authenticated TLS connection to the upstream XRRP server.
// It owns the protocol lifecycle: Connect, ReadGreeting, Login, then any number
// of SendPTFRequest/ReadPTFResponse exchanges, and finally Quit/Close.
//
// All reads go through the single bufio.Reader created in Connect so that bytes
// buffered past a message terminator are not dropped between calls. On the wire,
// messages are terminated by a lone "." line and PTF newlines are rewritten to
// CRLF.
type Session struct {
	Config     *Config
	Conn       *tls.Conn
	reader     *bufio.Reader
	lastActive time.Time
}

// NewSession returns a Session bound to config. The connection is not opened
// until Connect (or Init) is called.
func NewSession(config *Config) Session {
	return Session{Config: config}
}

// Connect opens the TLS connection to the upstream server and initialises the
// session's buffered reader.
func (s *Session) Connect() error {

	conn, err := tls.Dial("tcp", s.Config.Host+":"+s.Config.Port, &s.Config.TLSConfig)
	if err != nil {
		return fmt.Errorf("Connection error: %s", err)
	}
	s.Conn = conn
	s.reader = bufio.NewReader(conn)
	s.lastActive = time.Now()

	return nil
}

// Close logs out (Quit) and closes the underlying connection.
func (s *Session) Close() {
	s.Quit()
	s.Conn.Close()
}

// Greeting is the banner the server sends immediately after connecting.
type Greeting struct {
	serverID   string
	serverTime string
}

// ReadGreeting consumes the server banner sent on connect and returns it.
func (s *Session) ReadGreeting() *Greeting {
	greet := Greeting{}
	for i := 1; i < 5; i++ {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			log.Printf("Read line error: %v\n", err)
			break
		}
		line = strings.TrimSpace(line)
		if i == 1 {
			greet.serverID = line
		} else if i == 2 {
			greet.serverTime = line
		} else if line == "." {
			break
		} else {
			log.Printf("Unknown line: %s\n", line)
		}
	}

	return &greet
}

// Login authenticates the session with the configured upstream credentials and
// returns an error if the server rejects them.
func (s *Session) Login() error {
	s.SendLogin()
	resp := s.ReadResponse()
	if resp.IsSuccess() != true {
		return fmt.Errorf("Login error: %s", resp.String())
	}
	s.lastActive = time.Now()
	return nil
}

// SendLogin writes the XRRP "session" login command (id + password).
func (s *Session) SendLogin() {
	s.Conn.Write([]byte("session\r\n-Id:" + s.Config.Username + "\r\n-Password:" + s.Config.Password + "\r\n.\r\n"))
}

// SendQuit writes the XRRP "quit" command.
func (s *Session) SendQuit() {
	s.Conn.Write([]byte("quit\r\n.\r\n"))
}

// Quit logs the session out and returns an error if the server does not
// acknowledge the logout.
func (s *Session) Quit() error {
	s.SendQuit()
	resp := s.ReadResponse()
	if resp.IsSuccess() != true {
		return fmt.Errorf("Quit error: %s", resp.String())
	}
	fmt.Printf("Session closed: %s\n", resp)
	return nil
}

// KeepAlive sends a "hello" command to keep the upstream session from timing
// out. It returns an error if the response is missing or implausibly short,
// which the pool janitor treats as a broken connection.
func (s *Session) KeepAlive() error {
	s.Conn.Write([]byte("hello\r\n.\r\n"))
	resp := ""
	for i := 1; i < 5; i++ {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			log.Printf("Read line error X: %v\n", err)
			break
		}
		line = strings.TrimSpace(line)
		if line == "." {
			break
		} else if i == 1 {
			resp = line
		}
	}

	log.Println("Hello response:", resp)
	if len(resp) < 20 {
		return fmt.Errorf("Hello response not valid: %s", resp)
	}

	return nil
}

// Init runs the full bring-up sequence: Connect, ReadGreeting, Login.
func (s *Session) Init() error {
	// Connect to server
	if err := s.Connect(); err != nil {
		return err
	}

	// Read greeting
	greet := s.ReadGreeting()
	log.Printf("Greet: %#v", greet)

	// Login
	return s.Login()
}

// NewSessionInit returns a Session that is already connected and logged in.
func NewSessionInit(config *Config) (Session, error) {
	s := NewSession(config)
	return s, s.Init()
}

// Response is a parsed XRRP control response (the numeric code line returned by
// session/quit/etc.), as opposed to a full PTF API response.
type Response struct {
	Code        int
	Description string
}

// IsSuccess reports whether the response code is 200.
func (r *Response) IsSuccess() bool {
	if r.Code == 200 {
		return true
	}
	return false
}

// String renders the response as "<code> <description>".
func (r *Response) String() string {
	return fmt.Sprintf("%d %s", r.Code, r.Description)
}

// ReadResponse reads a single XRRP control response (terminated by a "." line)
// and parses its code and description.
func (s *Session) ReadResponse() *Response {
	resp := Response{}
	for i := 1; i < 5; i++ {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			log.Printf("Read line error X: %v\n", err)
			break
		}
		line = strings.TrimSpace(line)
		if i == 1 {
			code, err := strconv.Atoi(line[0:3])
			if err != nil {
				log.Printf("Invalid response code: %v", err)
			}
			resp.Code = code
			resp.Description = line[4:]
		} else if line == "." {
			break
		} else {
			log.Printf("Attribute: %s\n", line)
		}
	}
	s.lastActive = time.Now()

	return &resp
}

// SendPTFRequest forwards a PTF request to the upstream server, rewriting LF to
// CRLF and appending the lone-"." wire terminator.
func (s *Session) SendPTFRequest(request string) error {
	i, err := s.Conn.Write([]byte(strings.ReplaceAll(request, "\n", "\r\n") + "\r\n.\r\n"))
	if err != nil {
		log.Printf("Write error: %v\n", err)
		return err
	}
	log.Printf("Write %d bytes\n", i)
	return nil
}

// ReadPTFResponse reads a full PTF response (up to the lone "." wire terminator)
// and returns it with a synthetic "EOF\n" appended so consumers always see the
// PTF-style terminator. If the connection fails before the terminator arrives it
// returns the partial response together with the read error, so the caller can
// treat the session as broken.
func (s *Session) ReadPTFResponse() (string, error) {
	resp := ""
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			log.Printf("Read line error Y: %v\n", err)
			return resp + "EOF\n", err
		}
		line = strings.TrimSpace(line)
		if line == "." {
			break
		}
		resp += line + "\n"
	}
	s.lastActive = time.Now()

	return resp + "EOF\n", nil
}
