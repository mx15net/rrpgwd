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

// Command rrpgwd runs the rrpgwd daemon: it loads configuration (defaults, an
// optional YAML file, then RRPGWD_* environment variables), opens a pool of
// authenticated TLS sessions to the upstream XRRP server, starts the pool
// janitor, and serves local PTF clients until terminated.
package main

import (
	"log"
	"os"
	"time"

	"mx15net/rrpgwd/internal/rrpgwd"
)

func main() {

	cfg, err := rrpgwd.LoadConfig(os.Getenv("RRPGWD_CONFIG"))
	if err != nil {
		log.Fatalf("Config error: %s", err)
	}

	pool := rrpgwd.NewPoolInit(cfg.PoolMin, cfg.PoolMax, func() (*rrpgwd.Session, error) {
		// Create new session and connect.
		sess, err := rrpgwd.NewSessionInit(cfg)
		if err != nil {
			return nil, err
		}
		return &sess, nil
	})
	defer pool.Close()

	go pool.StartJanitor(
		time.Duration(cfg.KeepaliveSeconds)*time.Second,
		time.Duration(cfg.IdleTimeoutSeconds)*time.Second,
	)

	server := rrpgwd.NewServer(cfg, pool)
	server.Run()
}
