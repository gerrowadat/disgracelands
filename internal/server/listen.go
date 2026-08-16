// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// Listener accepts connections on one transport.
type Listener struct {
	// Name identifies the transport in logs and the who-list: "telnet",
	// "telnets", "websocket".
	Name string
	net.Listener
}

// ListenTelnet opens a plaintext listener.
func ListenTelnet(addr string) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	return &Listener{Name: "telnet", Listener: ln}, nil
}

// ListenTLS opens a TLS listener.
func ListenTLS(addr string, cfg *tls.Config) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	return &Listener{Name: "telnets", Listener: tls.NewListener(ln, cfg)}, nil
}

// Accept runs a listener's accept loop until ctx is cancelled.
//
// Every transport goes through here and therefore through session.Serve,
// which is what sends the greeting — a licence requirement that no transport
// may skip (docs/proposals/go-port-plan.md §12). Adding a listener that does
// not use this function would be the way to break that, which is why there
// is a test asserting each one does.
func (s *Server) Accept(ctx context.Context, ln *Listener, limits Limits) error {
	var (
		nextID  atomic.Uint64
		perHost sync.Map // host -> *atomic.Int64
		wg      sync.WaitGroup
	)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	defer wg.Wait()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		host, _, splitErr := net.SplitHostPort(conn.RemoteAddr().String())
		if splitErr != nil {
			host = conn.RemoteAddr().String()
		}

		// One address may not use up every slot the server has. This is the
		// cheapest defence there is against a trivial denial of service, and
		// the C server has none at all.
		countAny, _ := perHost.LoadOrStore(host, new(atomic.Int64))
		count := countAny.(*atomic.Int64)
		if limits.MaxPerHost > 0 && count.Load() >= int64(limits.MaxPerHost) {
			s.logger.Warn("refusing a connection: too many from this address",
				"host", host, "limit", limits.MaxPerHost)
			_, _ = conn.Write([]byte("Too many connections from your address.\r\n"))
			_ = conn.Close()
			continue
		}
		count.Add(1)

		sess := session.New(nextID.Add(1), conn, ln.Name, s.logger)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer count.Add(-1)
			s.serve(ctx, sess, limits)
		}()
	}
}

// Limits bound what one connection may do.
type Limits struct {
	// MaxPerHost caps simultaneous connections from one address. Zero means
	// no limit.
	MaxPerHost int
	// LoginGrace is how long a connection may stay unauthenticated.
	LoginGrace time.Duration
}

// serve runs one session.
func (s *Server) serve(ctx context.Context, sess *session.Session, limits Limits) {
	// A connection that sits at the name prompt forever costs a goroutine, a
	// socket and a slot in the per-host count. The C server has an idle
	// timeout for this; a deadline is the same idea, applied earlier.
	if limits.LoginGrace > 0 {
		go func() {
			timer := time.NewTimer(limits.LoginGrace)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
				if sess.State() != session.StatePlaying && !sess.Closed() {
					sess.Send("\r\nYou took too long to log in.\r\n")
					sess.Close()
				}
			}
		}()
	}

	sess.Serve(ctx, session.Deps{
		Logger: s.logger,
		Text:   s.text,
		Login:  s,
		Commands: &session.Dispatcher{
			Run: func(ctx context.Context, f func(*game.Live)) error {
				return s.engine.DoSync(ctx, f)
			},
			Text: s.text,
		},
	})
}
