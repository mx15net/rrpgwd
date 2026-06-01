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
	"fmt"
	"log"
	"sync"
	"time"
)

// Pool is a channel-backed pool of upstream Sessions sized between minSessions
// and maxSessions. Idle sessions wait in the sessions channel; new ones are
// created on demand via factory up to maxSessions. A StartJanitor goroutine
// keepalives idle sessions and retires broken or surplus ones.
//
// factory may fail (e.g. the upstream is briefly unreachable); callers receive
// the error rather than a panic, so a transient outage degrades gracefully
// instead of crashing the daemon.
type Pool struct {
	mu          sync.Mutex
	sessions    chan *Session
	factory     func() (*Session, error)
	minSessions int
	maxSessions int
	activeCount int
}

// NewPool returns an empty pool that holds at most max sessions, creating them
// lazily with factory. Use NewPoolInit to pre-open a minimum set.
func NewPool(max int, factory func() (*Session, error)) *Pool {
	return &Pool{
		sessions:    make(chan *Session, max),
		factory:     factory,
		minSessions: 1,
		maxSessions: max,
	}
}

// NewPoolInit returns a pool with up to min sessions already connected and a
// ceiling of max sessions. Sessions that fail to open at startup are logged and
// skipped; the janitor refills the pool to minSessions once the upstream is
// reachable, so the daemon still starts when the upstream is briefly down.
func NewPoolInit(min int, max int, factory func() (*Session, error)) *Pool {
	pool := NewPool(max, factory)
	pool.minSessions = min
	for i := 0; i < min; i++ {
		sess, err := factory()
		if err != nil {
			log.Println("Initial session failed:", err)
			continue
		}
		pool.activeCount++
		pool.Put(sess)
	}
	return pool
}

// Get checks out a session: an idle one if available, otherwise a freshly
// created one while below maxSessions, otherwise it blocks until one is
// returned. It returns an error only when a new session must be created and the
// factory (connect + login) fails.
func (p *Pool) Get() (*Session, error) {
	select {
	case s := <-p.sessions:
		return s, nil
	default:
		p.mu.Lock()
		if p.activeCount < p.maxSessions {
			// Reserve the slot, then create the session without holding the
			// lock (factory does network I/O). Roll back on failure.
			p.activeCount++
			p.mu.Unlock()

			s, err := p.factory()
			if err != nil {
				p.mu.Lock()
				p.activeCount--
				p.mu.Unlock()
				return nil, err
			}
			return s, nil
		}
		p.mu.Unlock()

		// At capacity: wait for a session to be returned.
		return <-p.sessions, nil
	}
}

// Put returns a healthy session to the pool.
func (p *Pool) Put(s *Session) {
	p.sessions <- s
}

// RemoveBroken closes a session's connection and decrements the active count so
// the pool can create a replacement.
func (p *Pool) RemoveBroken(s *Session) {
	s.Conn.Close()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.activeCount--
}

// Close drains and closes every session in the pool.
func (p *Pool) Close() {
	p.maxSessions = 0
	for i := 0; i < p.activeCount; i++ {
		s, err := p.Get()
		if err != nil {
			continue
		}
		s.Close()
	}
	fmt.Println("All sessions closed")
}

// StartJanitor runs forever (intended as a goroutine), ticking every
// keepaliveInterval. On each tick it sweeps the idle sessions (keepalive,
// dropping dead ones, retiring those idle beyond idleTimeout while above
// minSessions) and then refills the pool back up to minSessions.
func (p *Pool) StartJanitor(keepaliveInterval, idleTimeout time.Duration) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.sweep(idleTimeout)
		p.refill()
	}
}

// sweep walks the currently idle sessions, sending a keepalive to each. Dead
// sessions are dropped; sessions idle beyond idleTimeout are retired while the
// pool is above minSessions; the rest are returned to the pool.
func (p *Pool) sweep(idleTimeout time.Duration) {
	numToCheck := len(p.sessions)
	log.Printf("Janitor sweep (%d idle)", numToCheck)
	for i := 0; i < numToCheck; i++ {
		select {
		case sess := <-p.sessions:
			// 1. Keepalive check.
			if err := sess.KeepAlive(); err != nil {
				log.Println("Keepalive failed, dropping session:", err)
				p.RemoveBroken(sess)
				continue
			}

			// 2. Idle check (only when above minSessions).
			p.mu.Lock()
			if p.activeCount > p.minSessions && time.Since(sess.lastActive) > idleTimeout {
				log.Println("Retiring idle session")
				p.activeCount--
				p.mu.Unlock()
				sess.Conn.Close()
				continue
			}
			p.mu.Unlock()

			// Session is still good -> back into the pool.
			p.sessions <- sess
		default:
			// Channel is empty; done for this sweep.
			return
		}
	}
}

// refill opens new sessions until the pool is back up to minSessions, so warm
// capacity recovers after dead sessions are dropped. It stops on the first
// factory failure and tries again on the next tick.
func (p *Pool) refill() {
	for {
		p.mu.Lock()
		if p.activeCount >= p.minSessions {
			p.mu.Unlock()
			return
		}
		p.activeCount++
		p.mu.Unlock()

		sess, err := p.factory()
		if err != nil {
			p.mu.Lock()
			p.activeCount--
			p.mu.Unlock()
			log.Println("Janitor refill failed:", err)
			return
		}
		p.Put(sess)
	}
}
