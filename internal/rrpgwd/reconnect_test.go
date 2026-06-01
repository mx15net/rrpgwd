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
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
)

// fakeTimeout is a net.Error whose Timeout reports true.
type fakeTimeout struct{}

func (fakeTimeout) Error() string   { return "i/o timeout" }
func (fakeTimeout) Timeout() bool   { return true }
func (fakeTimeout) Temporary() bool { return false }

func TestIsDisconnect(t *testing.T) {
	disconnects := []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		net.ErrClosed,
		syscall.ECONNRESET,
		syscall.EPIPE,
		fakeTimeout{},
		// wrapped errors must still be recognised
		errors.New("read: " + io.EOF.Error()), // NOT a disconnect (plain string)
	}
	// The last entry above is a control: a plain string mentioning EOF is not
	// an unwrapped io.EOF, so it must be classified as non-disconnect.
	for i, err := range disconnects {
		got := isDisconnect(err)
		want := i < 6
		if got != want {
			t.Errorf("isDisconnect(%v) = %v, want %v", err, got, want)
		}
	}

	if isDisconnect(nil) {
		t.Error("isDisconnect(nil) = true, want false")
	}
	if isDisconnect(errors.New("boom")) {
		t.Error("isDisconnect(random) = true, want false")
	}
	// Wrapped ECONNRESET should be recognised.
	if !isDisconnect(&net.OpError{Op: "read", Err: syscall.ECONNRESET}) {
		t.Error("wrapped ECONNRESET not recognised as disconnect")
	}
}

func TestPoolGetFactoryErrorRollsBack(t *testing.T) {
	fail := true
	p := NewPool(2, func() (*Session, error) {
		if fail {
			return nil, errors.New("connect refused")
		}
		return &Session{}, nil
	})

	if _, err := p.Get(); err == nil {
		t.Fatal("Get returned nil error for a failing factory")
	}
	if p.activeCount != 0 {
		t.Fatalf("activeCount = %d after failed Get, want 0 (reservation must roll back)", p.activeCount)
	}

	// After rollback a subsequent successful create must work.
	fail = false
	s, err := p.Get()
	if err != nil || s == nil {
		t.Fatalf("Get after recovery: session=%v err=%v", s, err)
	}
	if p.activeCount != 1 {
		t.Fatalf("activeCount = %d after successful Get, want 1", p.activeCount)
	}
}

func TestNewPoolInitSkipsFailedSessions(t *testing.T) {
	calls := 0
	p := NewPoolInit(3, 5, func() (*Session, error) {
		calls++
		if calls == 2 { // second of three initial opens fails
			return nil, errors.New("transient")
		}
		return &Session{}, nil
	})

	if p.activeCount != 2 {
		t.Fatalf("activeCount = %d, want 2 (one initial open failed and was skipped)", p.activeCount)
	}
	if len(p.sessions) != 2 {
		t.Fatalf("idle sessions = %d, want 2", len(p.sessions))
	}
}

func TestPtfError(t *testing.T) {
	out := ptfError(421, "Upstream unavailable")
	for _, want := range []string{"[RESPONSE]", "code = 421", "description = Upstream unavailable", "EOF"} {
		if !strings.Contains(out, want) {
			t.Errorf("ptfError output missing %q:\n%s", want, out)
		}
	}
}
