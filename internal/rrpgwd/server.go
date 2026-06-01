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
	"log"
	"net"
	"os"
)

// Server listens for local PTF clients on a plain TCP socket and serves each
// accepted connection with a Client goroutine, forwarding requests through the
// shared session Pool. AuthUser/AuthPassword gate the local login prompt.
type Server struct {
	Proto          string
	Host           string
	Port           string
	RequestTimeout int32
	AuthUser       string
	AuthPassword   string
	Pool           *Pool
}

// NewServer builds a Server from the listener/auth settings in cfg, backed by
// the given session pool.
func NewServer(cfg *Config, pool *Pool) *Server {
	return &Server{
		Host:           cfg.ListenHost,
		Port:           cfg.ListenPort,
		Proto:          "tcp",
		RequestTimeout: 5,
		AuthUser:       cfg.AuthUser,
		AuthPassword:   cfg.AuthPassword,
		Pool:           pool,
	}
}

// Socket returns the "host:port" the server listens on.
func (s *Server) Socket() string {
	return s.Host + ":" + s.Port
}

// Run opens the listener and serves connections forever, spawning a Client
// goroutine per accepted connection. It does not return under normal operation.
func (s *Server) Run() {
	listen, err := net.Listen(s.Proto, s.Socket())
	if err != nil {
		log.Fatalf("Can't open port %s:%s proto: %s error: %s\n",
			s.Host, s.Port, s.Proto, err.Error())
		os.Exit(1)
	}
	defer listen.Close()

	log.Println("Listen on", s.Socket())

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Println("Error accept conn")
			os.Exit(1)
		}

		client := &Client{
			conn:   conn,
			server: s,
		}

		go client.handleRequest()
	}
}
