// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// --trust-proxy-headers, which was accepted, validated, plumbed to
// Config.TrustProxyHeaders and then read by nothing.
//
// What it costs to leave inert is not cosmetic. Every /ws connection
// arrives from the proxy's own address, and that one address is what the
// server then uses for **site bans** (session/login.go asks
// LoginHandler.BanFor with Session.Host()), for `last_host` on the player
// record, for what `users` and the wizlog show, and for
// --max-connections-per-ip. So behind a proxy: `ban site` cannot reach a
// web player at all, banning the proxy locks out every web player at once,
// and the default `--max-connections-per-ip 8` caps the *whole* web
// interface at eight players sharing one bucket.
//
// The fix is one address, resolved once, in one place: handleWS wraps the
// upgraded net.Conn so RemoteAddr reports the forwarded address, and
// everything downstream — admitConn's per-host counter, session.New's own
// host, and therefore the bans, the roster and the who-list — follows with
// no further plumbing. That is why this is a wrapper rather than a new
// argument threaded through four call sites.

// forwardedAddr is a net.Addr for an address that came out of a header
// rather than off a socket, so it has no port to report.
//
// Both places that read RemoteAddr in this tree do
// `net.SplitHostPort(...)` and fall back to the whole string when that
// fails (session.go's New, web.go's admitConn), so a bare address is
// already handled by construction and comes through as itself. That is
// deliberately preferred over synthesising a `:0`: a fake port is a thing
// that eventually gets logged, printed or compared, and there is no port
// here to be honest about.
type forwardedAddr struct{ addr netip.Addr }

func (a forwardedAddr) Network() string { return "tcp" }
func (a forwardedAddr) String() string  { return a.addr.String() }

// proxiedConn reports a RemoteAddr that is not the socket's.
//
// The embedded net.Conn carries everything else unchanged, including
// LocalAddr — only the peer is a claim rather than a fact.
type proxiedConn struct {
	net.Conn
	remote net.Addr
}

func (c proxiedConn) RemoteAddr() net.Addr { return c.remote }

// proxied returns conn as it should be seen from behind a trusted proxy,
// or conn itself when the headers say nothing usable.
func (h *webHandler) proxied(conn net.Conn, r *http.Request) net.Conn {
	addr, ok := forwardedFor(r.Header)
	if ok {
		return proxiedConn{Conn: conn, remote: forwardedAddr{addr: addr}}
	}

	// Two different failures, and they want different volumes.
	if raw := strings.Join(r.Header.Values(headerForwardedFor), ", "); raw != "" {
		// A header arrived and none of it parsed. That is either a proxy
		// writing something unexpected or somebody probing, and either way
		// it is anomalous per-connection rather than a standing
		// misconfiguration — so it is logged per connection.
		h.s.logger.Warn("ignoring an unparseable X-Forwarded-For",
			"header", truncateForLog(raw), "peer", conn.RemoteAddr().String())
		return conn
	}

	// No header at all, with --trust-proxy-headers on: a standing
	// misconfiguration, and the operator's bans and per-address limit are
	// quietly not working. Once, not once per connection — a scripted
	// flood must not be able to turn this into the log.
	h.missingHeader.Do(func() {
		h.s.logger.Warn("--trust-proxy-headers is on but no X-Forwarded-For is arriving; "+
			"site bans and --max-connections-per-ip are seeing the proxy's own address",
			"peer", conn.RemoteAddr().String())
	})
	return conn
}

const (
	headerForwardedFor   = "X-Forwarded-For"
	headerForwardedProto = "X-Forwarded-Proto"
)

// forwardedFor is the client address X-Forwarded-For claims.
//
// **The rightmost entry wins, not the leftmost**, and that is the whole
// security content of this function. X-Forwarded-For is a list each proxy
// appends its own view of the peer to, so the last entry is the one added
// by the hop nearest this process — the proxy --trust-proxy-headers is
// documented as being about ("only enable behind a proxy you control").
// Everything to its left arrived from further out and, with one proxy in
// front, is simply whatever the client typed: a client sending
// `X-Forwarded-For: 1.2.3.4` to an nginx configured the usual way
// ($proxy_add_x_forwarded_for) produces `1.2.3.4, <the real client>`, and
// reading the leftmost would hand every player a free way to forge their
// apparent address, evade a site ban and reset their per-address count.
// Reading the rightmost is also correct for the replacing configuration
// ($remote_addr), where there is only ever one entry.
//
// It is worth being clear about what this costs: with *two* proxies in
// front, the rightmost entry is the outer proxy rather than the client. A
// boolean flag cannot express a chain depth, and the deployment the flag
// describes is one hop; a tree that grows a second one wants a
// --trusted-proxies list of addresses, not a different reading of this
// header.
func forwardedFor(h http.Header) (netip.Addr, bool) {
	var candidates []string
	for _, value := range h.Values(headerForwardedFor) {
		candidates = append(candidates, strings.Split(value, ",")...)
	}

	for i := len(candidates) - 1; i >= 0; i-- {
		if addr, ok := parseForwardedAddr(candidates[i]); ok {
			return addr, true
		}
	}
	return netip.Addr{}, false
}

// parseForwardedAddr reads one X-Forwarded-For element.
//
// A bare address is the specified form; `host:port` is accepted too
// because proxies do write it, and an address in square brackets because
// that is how an IPv6 one has to be written when a port follows.
func parseForwardedAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		// Not a bare address: try the host:port and [host]:port spellings
		// before giving up on it.
		host, _, splitErr := net.SplitHostPort(s)
		if splitErr != nil {
			return netip.Addr{}, false
		}
		if addr, err = netip.ParseAddr(host); err != nil {
			return netip.Addr{}, false
		}
	}

	// Unmap, so that ::ffff:1.2.3.4 is the IPv4 address it is. This is not
	// tidiness: bans match on the host *string*, so a ban on 1.2.3.4 would
	// not catch a player whose address arrived in the mapped spelling. (The
	// --max-connections-per-ip bucket is already right either way —
	// perHostKey splits on net.IP.To4, which is non-nil for a mapped
	// address — which is exactly how a difference like this stays hidden
	// until it matters in the other place.)
	//
	// Zone dropped for the same reason: a scope identifier is meaningful on
	// the machine that owns the interface and nowhere else, so carrying one
	// into a ban list or a roster entry can only make two spellings of one
	// address.
	return addr.Unmap().WithZone(""), true
}

// forwardedHTTPS reports whether X-Forwarded-Proto says the browser's own
// hop to the proxy was HTTPS.
//
// Rightmost again, and for the same reason — it is the hop this process
// can actually trust something about. Getting it wrong in this direction
// matters more than it might look: too cautious and the captcha cookie
// merely lacks Secure, which is today's behaviour; too eager and a browser
// on a genuinely plaintext hop is issued a cookie it will never send back,
// and the captcha becomes a loop with no error message anywhere.
func forwardedHTTPS(h http.Header) bool {
	var last string
	for _, value := range h.Values(headerForwardedProto) {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				last = part
			}
		}
	}
	return strings.EqualFold(last, "https")
}

// truncateForLog caps a header value on its way into a log line, since it
// is attacker-controlled and unbounded up to the server's header limit.
func truncateForLog(s string) string {
	const limit = 120
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
