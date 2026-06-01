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
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"syscall"
	"time"
)

// Client is one accepted local connection. It handles the login prompt, reads
// PTF requests until EOF, and relays them through the server's session pool.
type Client struct {
	conn   net.Conn
	server *Server
}

func (c *Client) newReader() *bufio.Reader {
	return bufio.NewReader(c.conn)
}

// Close closes the client connection.
func (c *Client) Close() {
	c.conn.Close()
}

// RemoteIP returns the client's remote address.
func (c *Client) RemoteIP() string {
	return c.conn.RemoteAddr().String()
}

func (c *Client) setTimeout(seconds int32) {
	c.conn.SetReadDeadline(time.Now().Add(time.Duration(seconds) * time.Second))
}

func (c *Client) setRequestTimeout() {
	timeout := c.server.RequestTimeout
	c.setTimeout(timeout)
}

// handleRequest runs the per-connection protocol loop: prompt for and verify
// login/password, then read PTF requests (each terminated by an "EOF" line),
// forward them via CallApi, and write back the responses until the client
// disconnects or sends "quit".
func (c *Client) handleRequest() {
	reader := c.newReader()
	defer c.Close()

	c.setRequestTimeout()

	// Read user name
	c.conn.Write([]byte("login: "))
	user, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	user = strings.TrimSpace(user)
	// Read password
	c.conn.Write([]byte("password: "))
	password, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	password = strings.TrimSpace(password)

	// Check authentication
	if user != c.server.AuthUser || password != c.server.AuthPassword {
		log.Println("Connect from", c.RemoteIP(), "user:", user, "authentication failed")
		c.conn.Write([]byte("Authorization failed\n"))
		return
	}

	var request string
	for {
		c.setRequestTimeout()

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)

		if line == "quit" { // close connection
			return
		} else {
			request += line + "\n"
			if line == "EOF" { // End of file
				resp := c.CallApi(request)
				c.conn.Write([]byte(resp))

				// Cleanup request
				request = ""
			}
			continue
		}
	}
}

// maxAttempts bounds how many times CallApi will (re)connect and resend a
// request that fails before it reaches the upstream.
const maxAttempts = 4

// CallApi forwards a PTF request to the upstream via a pooled session and
// returns the PTF response.
//
// Resend policy (so non-idempotent commands are not executed twice): a request
// is only resent when the failure happened on the send, which means it never
// reached the upstream. If the send succeeds but the read fails, the upstream
// may already have processed the command, so the broken session is discarded
// and an error is returned to the caller instead of resending.
//
// It never panics: if no session can be obtained (upstream unreachable) it
// returns a synthetic error response.
func (c *Client) CallApi(request string) string {
	log.Println("Send request:", request)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		sess, err := c.server.Pool.Get()
		if err != nil {
			log.Println("Cannot obtain upstream session:", err)
			return ptfError(421, "Upstream unavailable")
		}

		start := time.Now()

		// Send. A send failure means the request was not delivered, so on a
		// dead connection we can safely reconnect and resend.
		if err := sess.SendPTFRequest(request); err != nil {
			c.server.Pool.RemoveBroken(sess)
			if isDisconnect(err) {
				log.Println("Send failed on dead connection, retrying:", err)
				continue
			}
			log.Println("Send error:", err)
			return ptfError(423, "Send to upstream failed")
		}

		// Read. The request has already been sent, so we must not resend it.
		resp, err := sess.ReadPTFResponse()
		if err != nil {
			log.Println("Read failed after send; discarding session, not resending:", err)
			c.server.Pool.RemoveBroken(sess)
			return ptfError(423, "Upstream connection lost")
		}

		log.Printf("Response:\n%s", resp)
		log.Printf("Runtime: %v\n", time.Since(start))

		// Put healthy session back to pool
		c.server.Pool.Put(sess)
		return resp
	}

	log.Printf("Giving up after %d attempts", maxAttempts)
	return ptfError(423, "Upstream connection lost")
}

// isDisconnect reports whether err indicates a dead connection that warrants
// reconnecting, covering the common ways a broken socket surfaces on read or
// write.
func isDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	return false
}

// ptfError renders a minimal PTF error response so local clients always receive
// a parseable [RESPONSE] block.
func ptfError(code int, reason string) string {
	return fmt.Sprintf("[RESPONSE]\ncode = %d\ndescription = %s\nEOF\n", code, reason)
}
