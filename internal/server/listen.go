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
// may skip (docs/design/go-port-plan.md §12). Adding a listener that does
// not use this function would be the way to break that, which is why there
// is a test asserting each one does.
func (s *Server) Accept(ctx context.Context, ln *Listener, limits Limits) error {
	var (
		nextID  atomic.Uint64
		perHost sync.Map // perHostKey(host) -> *atomic.Int64
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

		// The C's own sockets_connected >= max_players (comm.c:1337): the
		// first thing accept() does with a new descriptor, before even
		// resolving its hostname, is check whether there is room for it at
		// all. s.connections is shared across every listener this server
		// runs, matching sockets_connected's own count of every descriptor
		// regardless of which port it came in on.
		if limits.MaxPlayers > 0 && s.connections.count() >= limits.MaxPlayers {
			s.logger.Warn("refusing a connection: server full",
				"limit", limits.MaxPlayers)
			_, _ = conn.Write([]byte("Sorry, CircleMUD is full right now... please try again later!\r\n"))
			_ = conn.Close()
			continue
		}

		host, _, splitErr := net.SplitHostPort(conn.RemoteAddr().String())
		if splitErr != nil {
			host = conn.RemoteAddr().String()
		}

		// One address may not use up every slot the server has. This is the
		// cheapest defence there is against a trivial denial of service, and
		// the C server has none at all. Counted by perHostKey, not by host
		// itself — see its own doc comment for why an IPv6 address needs a
		// wider bucket than an IPv4 one for "one address" to mean anything.
		countAny, _ := perHost.LoadOrStore(perHostKey(host), new(atomic.Int64))
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
	// MaxPlayers caps how many connections may be open at once, across
	// every listener — the C's own max_players (comm.c:1337). Zero means
	// no limit.
	MaxPlayers int
	// MaxPerHost caps simultaneous connections from one address — one
	// perHostKey bucket, in practice, not literally one net.IP. Zero means
	// no limit.
	MaxPerHost int
	// LoginGrace is how long a connection may stay unauthenticated.
	LoginGrace time.Duration
}

// perHostKey is what --max-connections-per-ip actually counts against, and
// it is not always the address on the wire.
//
// An IPv4 address counts by itself — the C has no equivalent limit at all,
// so "one address" is this port's own invention, and for IPv4 one address
// is a reasonable stand-in for one machine. It stops being one for IPv6:
// a residential ISP hands a single subscriber a /64 or wider (RFC 6177),
// an ordinary OS's privacy extensions (RFC 4941) rotate an outgoing
// address from within it every so often on their own, and nothing stops a
// deliberate abuser picking a fresh one from the same /64 for every
// connection, for free — an address-exact counter is then not a limit,
// it is a formality. Bucketing by the /64 the address belongs to is the
// same "one subscriber" boundary the address space itself already draws.
//
// net.IP.To4 is what tells an ordinary IPv4 address apart from an IPv6
// one — it also returns non-nil for an IPv4-mapped IPv6 address
// (::ffff:a.b.c.d), which is exactly the "this is really IPv4" case and
// belongs on the IPv4 side of this split too.
func perHostKey(host string) string {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		return host
	}
	return ip.Mask(net.CIDRMask(64, 128)).String()
}

// serve runs one session.
func (s *Server) serve(ctx context.Context, sess *session.Session, limits Limits) {
	// Registered for the whole life of the connection, so `users` and `dc`
	// can see it and number it. Unregistered on the way out, and the snoop
	// links broken with it — a snooper watching a connection that has gone
	// would otherwise write into a closed session forever.
	s.connections.add(sess)
	defer func() {
		sess.StopSnooping()
		if watcher := sess.SnoopedBy(); watcher != nil {
			watcher.StopSnooping()
		}
		s.connections.remove(sess)
	}()

	// Shutting down must actually shut down. A session sits blocked in
	// conn.Read, which no amount of context cancellation interrupts on its
	// own, so the connection is closed for it — otherwise Accept's wg.Wait
	// below waits for players to hang up of their own accord.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			sess.Send("\r\nThe server is shutting down. Come back soon!\r\n")
			sess.Close()
		case <-done:
		}
	}()

	// A connection that sits at the name prompt forever costs a goroutine, a
	// socket and a slot in the per-host count. The C server has an idle
	// timeout for this; a deadline is the same idea, applied earlier.
	if limits.LoginGrace > 0 {
		go func() {
			timer := time.NewTimer(limits.LoginGrace)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-done:
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
			Text:         s.text,
			RNG:          s.rng,
			Violence:     s,
			NoSpecials:   s.noSpecials,
			RoundLength:  s.roundLength,
			Rent:         s.RentCharacter,
			Extract:      s.ExtractCharacter,
			SaveBoard:    s.SaveBoard,
			Mail:         mailOrNil(s),
			Houses:       housesOrNilIface(s),
			Operator:     s,
			Bans:         bansOrNil(s),
			Reports:      reportsOrNil(s),
			TextEdit:     s,
			MobReload:    s,
			ZoneReload:   s,
			ObjectReload: s,
			ShopReload:   s,
			SetPassword: func(c *game.Character, password string) error {
				return s.SetPassword(context.Background(), c, password)
			},
			Save: func(c *game.Character) {
				// Off the world goroutine, which is where the command that
				// asked for it is running.
				s.background(func() {
					if err := s.Save(context.Background(), c); err != nil {
						s.logger.Error("saving on request", "character", c.Name, "error", err)
					}
				})
			},
			SaveAliases: func(c *game.Character) {
				s.background(func() {
					if err := s.SaveAliases(context.Background(), c); err != nil {
						s.logger.Error("saving aliases on request", "character", c.Name, "error", err)
					}
				})
			},
		},
	})
}
